package storefront

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/order"
)

// The buyer's own artwork on their own order (#966).
//
// The property that matters is a privacy boundary, not a rendering
// detail. GET /storefront/.../orders/:id allows ANONYMOUS, store-scoped
// reads — that is how the post-checkout confirmation page works before
// anyone has an account. So "can read the order" and "may be handed a
// link to the photograph" are different questions, and conflating them
// would let anybody who learns an order id collect a stranger's images.

type stubSigner struct {
	calls []string
	err   error
}

func (s *stubSigner) SignedReadURL(_ context.Context, key string, _ time.Duration) (string, time.Time, error) {
	s.calls = append(s.calls, key)
	if s.err != nil {
		return "", time.Time{}, s.err
	}
	return "https://signed.example/" + key, time.Now().Add(time.Minute), nil
}

func strPtr(s string) *string { return &s }

// one order item, and one image + one text answer against it.
func artworkFixture() ([]order.OrderItem, []order.ItemPersonalisation) {
	itemID := uuid.New()
	items := []order.OrderItem{{ID: itemID, TitleSnapshot: "Figurine"}}
	rows := []order.ItemPersonalisation{
		{
			ID: uuid.New(), OrderItemID: itemID,
			FieldKey: "photo", FieldLabel: "Your photo", Kind: "image",
			StorageKeyOriginal: strPtr("buyer-uploads/t1/c1/original.jpg"),
			StorageKey:         strPtr("buyer-uploads/t1/c1/original.jpg.preview-ab.jpg"),
			Position:           0,
		},
		{
			ID: uuid.New(), OrderItemID: itemID,
			FieldKey: "name", FieldLabel: "Name to engrave", Kind: "text",
			TextValue: strPtr("Asha"), Position: 1,
		},
	}
	return items, rows
}

func respFor(items []order.OrderItem) storefrontOrderResponse {
	return storefrontOrderResponse{
		Items: make([]storefrontOrderItemResponse, len(items)),
	}
}

func TestStorefrontArtwork_AnonymousCallerGetsNoLink(t *testing.T) {
	// The whole point. This caller may legitimately read the order — the
	// confirmation page does — and must still not receive a URL.
	items, rows := artworkFixture()
	resp := respFor(items)
	signer := &stubSigner{}

	attachStorefrontPersonalisation(
		context.Background(), &resp, items, rows, signer, false, nil)

	require.Len(t, resp.Items[0].Personalisation, 2)
	img := resp.Items[0].Personalisation[0]
	require.True(t, img.HasArtwork, "they should be told a photo exists")
	require.Equal(t, "Your photo", img.FieldLabel)
	require.Empty(t, img.PreviewURL, "an anonymous caller must not get a link")
	require.Empty(t, signer.calls, "nothing should even be signed for them")

	// The text answer is still readable — it always was, as part of the
	// order, and withholding it would break the confirmation page.
	require.Equal(t, "Asha", resp.Items[0].Personalisation[1].TextValue)
}

func TestStorefrontArtwork_OwnerGetsTheCroppedPreview(t *testing.T) {
	items, rows := artworkFixture()
	resp := respFor(items)
	signer := &stubSigner{}

	attachStorefrontPersonalisation(
		context.Background(), &resp, items, rows, signer, true, nil)

	img := resp.Items[0].Personalisation[0]
	require.NotEmpty(t, img.PreviewURL)
	// The PREVIEW, not the original: the original is the print source
	// and can be tens of megabytes. The buyer wants to see what they
	// chose, which is the crop.
	require.Equal(t, []string{"buyer-uploads/t1/c1/original.jpg.preview-ab.jpg"}, signer.calls)
}

func TestStorefrontArtwork_FallsBackToOriginalWhenNeverCropped(t *testing.T) {
	items, rows := artworkFixture()
	rows[0].StorageKey = nil // uploaded, never cropped
	resp := respFor(items)
	signer := &stubSigner{}

	attachStorefrontPersonalisation(
		context.Background(), &resp, items, rows, signer, true, nil)

	require.Equal(t, []string{"buyer-uploads/t1/c1/original.jpg"}, signer.calls)
	require.NotEmpty(t, resp.Items[0].Personalisation[0].PreviewURL)
}

func TestStorefrontArtwork_NoSignerStillShowsTheAnswers(t *testing.T) {
	// A deployment with no private bucket. The order page must still
	// render what the buyer typed and say a photo was supplied.
	items, rows := artworkFixture()
	resp := respFor(items)

	attachStorefrontPersonalisation(
		context.Background(), &resp, items, rows, nil, true, nil)

	require.Len(t, resp.Items[0].Personalisation, 2)
	require.True(t, resp.Items[0].Personalisation[0].HasArtwork)
	require.Empty(t, resp.Items[0].Personalisation[0].PreviewURL)
}

func TestStorefrontArtwork_ASigningFailureDoesNotLoseTheLine(t *testing.T) {
	// They came to read their order. A thumbnail that cannot be signed
	// must not cost them the delivery status.
	items, rows := artworkFixture()
	resp := respFor(items)
	signer := &stubSigner{err: errors.New("iam unavailable")}

	attachStorefrontPersonalisation(
		context.Background(), &resp, items, rows, signer, true, nil)

	require.Len(t, resp.Items[0].Personalisation, 2)
	require.True(t, resp.Items[0].Personalisation[0].HasArtwork)
	require.Empty(t, resp.Items[0].Personalisation[0].PreviewURL)
	require.Equal(t, "Asha", resp.Items[0].Personalisation[1].TextValue)
}

func TestStorefrontArtwork_NeverPutsAStorageKeyOnTheWire(t *testing.T) {
	// The buyer has no use for a bucket path, and this response is
	// reachable without being signed in.
	items, rows := artworkFixture()
	resp := respFor(items)

	attachStorefrontPersonalisation(
		context.Background(), &resp, items, rows, &stubSigner{}, true, nil)

	body, err := json.Marshal(resp)
	require.NoError(t, err)
	require.NotContains(t, string(body), "storage_key")
	require.NotContains(t, string(body), "upload_id")
	// The signed URL necessarily contains the key; what must not leak is
	// a bare key field the caller could reuse after the URL expires.
	require.NotContains(t, string(body), `"buyer-uploads/`)
}

func TestStorefrontArtwork_NothingToDoLeavesTheResponseAlone(t *testing.T) {
	items := []order.OrderItem{{ID: uuid.New(), TitleSnapshot: "Mug"}}
	resp := respFor(items)

	attachStorefrontPersonalisation(
		context.Background(), &resp, items, nil, &stubSigner{}, true, nil)

	require.Nil(t, resp.Items[0].Personalisation,
		"an ordinary order must not grow an empty array")
}
