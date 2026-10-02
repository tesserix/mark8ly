package personalisationupload

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/media"
)

// The property the whole package is built around: with no private bucket
// there is no fallback to the public product-media bucket, because the
// fallback would publish a photograph a buyer uploaded.
func TestDisabledWithoutAPrivateBucket(t *testing.T) {
	for name, s := range map[string]*Service{
		"no uploader at all": NewService(Config{}),
		"uploader but no bucket name": NewService(Config{
			Uploader: media.NewFakeUploader(),
		}),
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, s.Enabled())

			_, err := s.CreateUploadURL(context.Background(), CreateRequest{})
			require.ErrorIs(t, err, ErrUploadsDisabled)

			_, err = s.Confirm(context.Background(), "u1", "c1", nil, nil)
			require.ErrorIs(t, err, ErrUploadsDisabled)

			_, err = s.PrepareCrop(context.Background(), "u1", "c1", CropRect{W: 1, H: 1})
			require.ErrorIs(t, err, ErrUploadsDisabled)

			_, _, err = s.PreviewURL(context.Background(), "u1", "c1")
			require.ErrorIs(t, err, ErrUploadsDisabled)

			require.ErrorIs(t, s.Delete(context.Background(), "u1", "c1"), ErrUploadsDisabled)
		})
	}
}

func TestNilServiceIsNotEnabled(t *testing.T) {
	var s *Service
	require.False(t, s.Enabled())
}

func TestBuildKey_SegregatesBuyerArtworkFromProductMedia(t *testing.T) {
	key := BuildKey("t1", "cart-9", "u7", "Holiday Photo.JPG")
	// A different prefix from product media's `tenants/...` so the two can
	// never be confused by a reaper, a lifecycle rule, or a human reading
	// a bucket listing.
	require.True(t, strings.HasPrefix(key, "buyer-uploads/"), key)
	require.NotContains(t, key, "products/media")
	// The cart is in the path so an orphan can be traced without a database.
	require.Contains(t, key, "/cart-9/")
	require.True(t, strings.HasSuffix(key, ".jpg"), key)
}

func TestBuildKey_SanitisesTheExtension(t *testing.T) {
	// The filename is buyer-supplied; only a lowercase alphanumeric
	// extension survives into a key.
	require.True(t, strings.HasSuffix(BuildKey("t", "c", "u", "x.png"), ".png"))
	require.True(t, strings.HasSuffix(BuildKey("t", "c", "u", "x.../../etc"), "."))
	require.True(t, strings.HasSuffix(BuildKey("t", "c", "u", "noext"), "."))
	for _, f := range []string{"a.jpg", "a.../..", "", "a.<script>"} {
		require.NotContains(t, BuildKey("t", "c", "u", f), "..")
		require.NotContains(t, BuildKey("t", "c", "u", f), "<")
	}
}

func TestAllowedContentTypes_RejectHEIC(t *testing.T) {
	// Every photo an iPhone picks is HEIC unless something converts it,
	// and most print pipelines cannot read it — so accepting it would
	// hand merchants files they cannot use.
	require.False(t, IsAllowedContentType("image/heic"))
	require.False(t, IsAllowedContentType("image/heif"))
	require.False(t, IsAllowedContentType("application/pdf"))
	require.False(t, IsAllowedContentType("image/svg+xml"))
	require.False(t, IsAllowedContentType(""))

	require.True(t, IsAllowedContentType("image/jpeg"))
	require.True(t, IsAllowedContentType("image/png"))
	require.True(t, IsAllowedContentType("image/webp"))
}

func TestTTLIsLongEnoughToSleepOn(t *testing.T) {
	// Holds protect inventory from other shoppers and last minutes. This
	// protects a shopper's own work from us, and has to survive them
	// sleeping on the decision.
	require.Equal(t, 72.0, TTL.Hours())
}
