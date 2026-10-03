//go:build integration

package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// The merchant's end of #968, over HTTP.
//
// The assertions that matter are about what must NOT happen:
//
//   - a storage key must never appear in a response body. The whole
//     reason downloads go through an audited endpoint is that holding
//     the key routes around the audit; if a key leaks onto the wire the
//     endpoint is decoration.
//   - the signed URL must be for the ORIGINAL, never the preview. The
//     preview is a re-encoded thumbnail and printing from it ships a
//     blurry product — a defect nobody notices until a customer opens
//     the box.
//   - another merchant holding a valid order id must get a 404.

// recordingSigner stands in for GCS. Records what it was asked to sign,
// which is how the original-vs-preview assertion is made at all.
type recordingSigner struct {
	mu   sync.Mutex
	keys []string
}

func (s *recordingSigner) SignedReadURL(
	_ context.Context, key string, expires time.Duration,
) (string, time.Time, error) {
	s.mu.Lock()
	s.keys = append(s.keys, key)
	s.mu.Unlock()
	return "https://signed.example/" + key, time.Now().Add(expires), nil
}

func (s *recordingSigner) signed() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.keys...)
}

type artworkTestEnv struct {
	router *gin.Engine
	db     *gorm.DB
	fga    *authz.FakeClient
	signer *recordingSigner
}

// setupArtworkRouter mounts the orders surface plus the artwork handler.
// `signer` nil exercises the no-private-bucket deployment, which must
// answer 501 rather than 500.
func setupArtworkRouter(t *testing.T, withSigner bool) *artworkTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, append([]string{"order_item_personalisations"}, ordersTables...)...)

	storesRepo := stores.NewRepository(db)
	outboxRepo := outbox.NewRepository(db)
	orderRepo := order.NewRepository()
	orderSvc := order.NewService(db, orderRepo, outboxRepo)

	fga := authz.NewFakeClient()
	storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{
		Repo:   storesRepo,
		Client: stubClient{},
		Flight: &singleflight.Group{},
	})

	signer := &recordingSigner{}
	var artwork *admin.OrderPersonalisationHandler
	if withSigner {
		artwork = admin.NewOrderPersonalisationHandler(db, signer, nil)
	} else {
		// A nil interface, not a nil *recordingSigner: the handler's
		// guard is `h.artwork == nil`, and a typed nil would sail past
		// it and panic on the first call.
		artwork = admin.NewOrderPersonalisationHandler(db, nil, nil)
	}

	r := gin.New()
	admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
		OrdersHandler: admin.NewOrdersHandler(db, orderSvc, orderRepo, nil, nil),
		// Registered unconditionally so the no-bucket case reaches the
		// handler and returns 501. Leaving it nil would 404 instead,
		// which tells an operator the wrong thing.
		OrderPersonalisationHandler: artwork,
		StoresMiddleware:            storeMW,
		AuthzMiddleware:             authz.NewMiddleware(fga, nil),
	})

	return &artworkTestEnv{router: r, db: db, fga: fga, signer: signer}
}

// seedArtworkOrder creates one order with one image answer and one text
// answer, and returns the order id and the image answer's id.
func seedArtworkOrder(
	t *testing.T, db *gorm.DB, storeID, tenantID string, seq int64,
) (orderID, imageID, textID string) {
	t.Helper()

	sid, err := uuid.Parse(storeID)
	if err != nil {
		t.Fatalf("seedArtworkOrder: store id: %v", err)
	}
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		t.Fatalf("seedArtworkOrder: tenant id: %v", err)
	}

	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))
	res, err := svc.Create(context.Background(), order.CreateInput{
		TenantID:       tid,
		StoreID:        sid,
		StorePrefix:    "TST",
		OrderNumberSeq: seq,
		IdempotencyKey: "artwork-" + uuid.NewString(),
		CustomerEmail:  "buyer@example.com",
		Items: []order.OrderItem{{
			TitleSnapshot: "Custom figurine",
			SKUSnapshot:   "FIG-1",
			UnitPrice:     decimal.RequireFromString("80.00"),
			Quantity:      1,
			LineTotal:     decimal.RequireFromString("80.00"),
			CurrencyCode:  "USD",
		}},
		Shipping:     order.OrderAddress{Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE"},
		Billing:      order.OrderAddress{Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE"},
		Subtotal:     decimal.RequireFromString("80.00"),
		GrandTotal:   decimal.RequireFromString("80.00"),
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("seedArtworkOrder: create: %v", err)
	}

	itemID := res.Items[0].ID
	imgID, txtID := uuid.New(), uuid.New()

	if err := db.Exec(`
		INSERT INTO order_item_personalisations
			(id, order_item_id, field_key, field_label, kind, price_delta,
			 storage_key_original, storage_key, content_type, size_bytes,
			 original_filename, crop, position)
		VALUES (?, ?, 'photo', 'Your photo', 'image', 10.00,
			 'buyer-uploads/pristine-original.jpg',
			 'buyer-uploads/lossy-preview.jpg',
			 'image/jpeg', 204800, 'nana.jpg',
			 '{"x":10,"y":20,"width":400,"height":400}'::jsonb, 0)`,
		imgID, itemID).Error; err != nil {
		t.Fatalf("seedArtworkOrder: insert image answer: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO order_item_personalisations
			(id, order_item_id, field_key, field_label, kind, text_value,
			 price_delta, position)
		VALUES (?, ?, 'name', 'Name to engrave', 'text', 'Asha', 0.00, 1)`,
		txtID, itemID).Error; err != nil {
		t.Fatalf("seedArtworkOrder: insert text answer: %v", err)
	}

	return res.Order.ID.String(), imgID.String(), txtID.String()
}

func TestAPI_OrderDetail_CarriesPersonalisationWithoutStorageKeys(t *testing.T) {
	env := setupArtworkRouter(t, true)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)

	orderID, imageID, textID := seedArtworkOrder(t, env.db, storeID, tenantID, 1)

	w := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/orders/"+orderID, nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("get order: status %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	// The guarantee the whole audited-download design rests on.
	for _, leak := range []string{
		"storage_key", "pristine-original.jpg", "lossy-preview.jpg",
	} {
		if strings.Contains(body, leak) {
			t.Fatalf("order detail leaked %q onto the wire:\n%s", leak, body)
		}
	}

	var resp admin.AdminOrderResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	answers := resp.Items[0].Personalisation
	if len(answers) != 2 {
		t.Fatalf("personalisation = %d, want 2: %s", len(answers), body)
	}

	img := answers[0]
	if img.ID != imageID {
		t.Fatalf("first answer id = %q, want the image answer %q", img.ID, imageID)
	}
	if !img.HasArtwork {
		t.Fatal("image answer: has_artwork = false, so the UI renders no download button")
	}
	if img.OriginalFilename == nil || *img.OriginalFilename != "nana.jpg" {
		t.Fatalf("image answer: original_filename = %v, want nana.jpg", img.OriginalFilename)
	}
	// The reference is what ties a printed document to a downloaded file.
	if img.Reference != imageID[:8] {
		t.Fatalf("image answer: reference = %q, want %q", img.Reference, imageID[:8])
	}

	txt := answers[1]
	if txt.ID != textID {
		t.Fatalf("second answer id = %q, want the text answer %q", txt.ID, textID)
	}
	if txt.HasArtwork {
		t.Fatal("text answer: has_artwork = true, so the UI offers a download of nothing")
	}
	if txt.TextValue == nil || *txt.TextValue != "Asha" {
		t.Fatalf("text answer: text_value = %v, want Asha", txt.TextValue)
	}
}

func TestAPI_ArtworkDownload_SignsTheOriginalNotThePreview(t *testing.T) {
	env := setupArtworkRouter(t, true)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)

	orderID, imageID, _ := seedArtworkOrder(t, env.db, storeID, tenantID, 1)

	w := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/orders/"+orderID+
			"/personalisations/"+imageID+"/download", nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("download: status %d body=%s", w.Code, w.Body.String())
	}

	signed := env.signer.signed()
	if len(signed) != 1 {
		t.Fatalf("signed %d keys, want 1: %v", len(signed), signed)
	}
	if signed[0] != "buyer-uploads/pristine-original.jpg" {
		t.Fatalf("signed %q — the merchant would print from the lossy preview", signed[0])
	}

	var link struct {
		PersonalisationID string          `json:"personalisation_id"`
		Reference         string          `json:"reference"`
		Filename          string          `json:"filename"`
		URL               string          `json:"url"`
		ExpiresAt         time.Time       `json:"expires_at"`
		Crop              json.RawMessage `json:"crop"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &link); err != nil {
		t.Fatalf("unmarshal link: %v", err)
	}
	if link.PersonalisationID != imageID {
		t.Fatalf("personalisation_id = %q, want %q", link.PersonalisationID, imageID)
	}
	if link.Filename != "nana.jpg" {
		t.Fatalf("filename = %q, want the buyer's own filename", link.Filename)
	}
	if link.ExpiresAt.IsZero() || time.Until(link.ExpiresAt) > admin.DownloadURLTTL+time.Minute {
		t.Fatalf("expires_at = %v — a long-lived link to a buyer's photograph", link.ExpiresAt)
	}
	// Crop travels alongside the uncropped original so the merchant's own
	// tooling applies it at full resolution.
	if len(link.Crop) == 0 {
		t.Fatal("crop missing — the merchant cannot reproduce the buyer's framing")
	}
}

func TestAPI_ArtworkDownloadAll_ReturnsEveryOriginalAndSkipsText(t *testing.T) {
	env := setupArtworkRouter(t, true)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)

	orderID, imageID, _ := seedArtworkOrder(t, env.db, storeID, tenantID, 1)

	w := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/orders/"+orderID+
			"/personalisations/download", nil, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("download all: status %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Artwork []struct {
			PersonalisationID string `json:"personalisation_id"`
			URL               string `json:"url"`
		} `json:"artwork"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// One link, not two: the text answer has no file and must not appear
	// as a broken download.
	if len(resp.Artwork) != 1 {
		t.Fatalf("artwork = %d, want 1 (text answers are not downloads): %s",
			len(resp.Artwork), w.Body.String())
	}
	if resp.Artwork[0].PersonalisationID != imageID {
		t.Fatalf("artwork id = %q, want %q", resp.Artwork[0].PersonalisationID, imageID)
	}
	if signed := env.signer.signed(); len(signed) != 1 ||
		signed[0] != "buyer-uploads/pristine-original.jpg" {
		t.Fatalf("signed %v, want the single pristine original", signed)
	}
}

// Another merchant with a valid session and a real order id from someone
// else's store. This is the case that decides whether the feature is
// shippable at all.
func TestAPI_ArtworkDownload_RefusesAnotherStoresOrder(t *testing.T) {
	env := setupArtworkRouter(t, true)
	storeA, tenantA := seedStoreRow(t, env.db, "")
	storeB, tenantB := seedStoreRow(t, env.db, "")

	orderA, imageA, _ := seedArtworkOrder(t, env.db, storeA, tenantA, 1)

	// A fully legitimate admin of store B.
	userB := uuid.NewString()
	env.fga.Grant(userB, authz.RoleAdmin, tenantB)
	headersB := authHeaders(userB, tenantB)

	// Store B's URL, store A's order and answer ids.
	w := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeB+"/orders/"+orderA+
			"/personalisations/"+imageA+"/download", nil, headersB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-store single download: status %d, want 404; body=%s",
			w.Code, w.Body.String())
	}

	w = request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeB+"/orders/"+orderA+
			"/personalisations/download", nil, headersB)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-store download-all: status %d, want 404; body=%s",
			w.Code, w.Body.String())
	}

	if signed := env.signer.signed(); len(signed) != 0 {
		t.Fatalf("signed %v for a cross-store request — store B got a URL to store A's buyer's photograph", signed)
	}
}

// No private bucket: 501, not 500 and not a silent empty list. An
// operator reading the status code should learn that the deployment is
// missing configuration, not that the order has no artwork.
func TestAPI_ArtworkDownload_NotImplementedWithoutPrivateBucket(t *testing.T) {
	env := setupArtworkRouter(t, false)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	headers := authHeaders(userID, tenantID)

	orderID, imageID, _ := seedArtworkOrder(t, env.db, storeID, tenantID, 1)

	for _, path := range []string{
		"/personalisations/" + imageID + "/download",
		"/personalisations/download",
	} {
		w := request(t, env.router, http.MethodGet,
			"/api/v1/admin/stores/"+storeID+"/orders/"+orderID+path, nil, headers)
		if w.Code != http.StatusNotImplemented {
			t.Fatalf("%s: status %d, want 501; body=%s", path, w.Code, w.Body.String())
		}
	}
}

// A viewer, not an editor. Reading the production brief is part of
// reading the order; requiring an edit role would lock warehouse staff
// out of the one thing they need.
func TestAPI_ArtworkDownload_AllowedForOrdersViewer(t *testing.T) {
	env := setupArtworkRouter(t, true)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	orderID, imageID, _ := seedArtworkOrder(t, env.db, storeID, tenantID, 1)

	viewer := uuid.NewString()
	env.fga.Grant(viewer, authz.OrdersViewRole, tenantID)

	w := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/orders/"+orderID+
			"/personalisations/"+imageID+"/download", nil,
		authHeaders(viewer, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("viewer download: status %d body=%s", w.Code, w.Body.String())
	}
}

func TestAPI_ArtworkDownload_404ForTextAnswer(t *testing.T) {
	env := setupArtworkRouter(t, true)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	orderID, _, textID := seedArtworkOrder(t, env.db, storeID, tenantID, 1)

	// A text answer exists but has no file. Asking for its "original"
	// must 404 rather than signing an empty key, which GCS would answer
	// with a URL to nothing.
	w := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/orders/"+orderID+
			"/personalisations/"+textID+"/download", nil,
		authHeaders(userID, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("text answer download: status %d, want 404; body=%s", w.Code, w.Body.String())
	}
	if signed := env.signer.signed(); len(signed) != 0 {
		t.Fatalf("signed %v for an answer with no file", signed)
	}
}
