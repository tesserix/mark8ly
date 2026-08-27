//go:build integration

// Package storefront_test — API-level integration tests for the M6 storefront
// read routes. These tests stand up the real gin router + real repositories
// against a live Postgres (TEST_DATABASE_URL) and exercise every endpoint
// through HTTP, with the primary goal of asserting no forbidden admin fields
// leak into the anonymous JSON payloads and that the cache-header / 304 flow
// behaves correctly.
package storefront_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// ------------------------------------------------------------------
// Test infrastructure
// ------------------------------------------------------------------

var storefrontTruncateTables = []string{
	"outbox_events",
	"product_media",
	"variant_option_values",
	"variant_stock",
	"product_variants",
	"product_option_values",
	"product_options",
	"product_categories",
	"products",
	"categories",
	"store_watermarks",
	"stores",
}

// fakeStoresClient satisfies stores.Client. Since tests pre-seed the stores
// projection row directly, the slug cache's fast path hits the DB and never
// falls through to the remote client. When it does (e.g. unknown-slug path),
// we return ErrPlatformUnavailable which the cache treats as a miss.
type fakeStoresClient struct{}

func (fakeStoresClient) GetStore(ctx context.Context, tenantID, storeID string) (*stores.Store, error) {
	return nil, stores.ErrPlatformUnavailable
}
func (fakeStoresClient) GetStoreBySlug(ctx context.Context, slug string) (*stores.Store, error) {
	return nil, stores.ErrPlatformUnavailable
}

type setupOpts struct {
	storefrontKey string
}

func setupStorefrontRouter(t *testing.T, opts setupOpts) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := testdb.NewDB(t, storefrontTruncateTables...)

	productRepo := product.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	storesRepo := stores.NewRepository(db)
	slugCache := stores.NewSlugCache(storesRepo, fakeStoresClient{}, &singleflight.Group{}, 5*time.Minute)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	handler := storefront.NewStorefrontHandler(productRepo, categoryRepo, storesRepo, logger)

	r := gin.New()
	grp := r.Group("/api/v1")
	storefront.RegisterStorefront(grp, storefront.Deps{
		Handler:       handler,
		SlugCache:     slugCache,
		StorefrontKey: opts.storefrontKey,
	})
	return r, db
}

// ------------------------------------------------------------------
// Seeding helpers
// ------------------------------------------------------------------

func seedStorefrontStore(t *testing.T, db *gorm.DB, slug, currency string) (storeID, tenantID string) {
	t.Helper()
	storeID = uuid.NewString()
	tenantID = uuid.NewString()
	if err := db.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, synced_at, storefront_customer_portal_secret)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, now(), encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Storefront Test Store", "US", currency, "UTC", stores.StatusActive).Error; err != nil {
		t.Fatalf("seed store: %v", err)
	}
	testdb.SeedVendor(t, db, uuid.MustParse(tenantID))
	return
}

func seedSuspendedStore(t *testing.T, db *gorm.DB, slug string) string {
	t.Helper()
	storeID := uuid.NewString()
	tenantID := uuid.NewString()
	if err := db.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, synced_at, storefront_customer_portal_secret)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, now(), encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Suspended Store", "US", "USD", "UTC", stores.StatusSuspended).Error; err != nil {
		t.Fatalf("seed suspended store: %v", err)
	}
	return storeID
}

func seedStorefrontWatermark(t *testing.T, db *gorm.DB, storeID string, ts time.Time) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO store_watermarks (store_id, products_updated_at) VALUES (?, ?)
		ON CONFLICT (store_id) DO UPDATE SET products_updated_at = EXCLUDED.products_updated_at`,
		storeID, ts).Error; err != nil {
		t.Fatalf("seed watermark: %v", err)
	}
}

// productOpts controls the visibility state of a seeded product.
type productOpts struct {
	Status      string     // draft / active / archived
	PublishedAt *time.Time // nil means not published
	DeletedAt   *time.Time
	Handle      string
	Title       string
	CostPrice   *string // set to raw decimal string to seed cost_price on the variant
}

func seedProduct(t *testing.T, db *gorm.DB, storeID, tenantID string, opts productOpts) string {
	t.Helper()
	productID := uuid.NewString()
	if opts.Handle == "" {
		opts.Handle = "p-" + productID[:8]
	}
	if opts.Title == "" {
		opts.Title = "Product " + opts.Handle
	}
	// Idempotent per tenant — seedStorefrontStore already seeded this
	// tenant's self-vendor; this just looks it up.
	vendorID := testdb.SeedVendor(t, db, uuid.MustParse(tenantID))
	if err := db.Exec(`
		INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, status, published_at, deleted_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now(), now())`,
		productID, tenantID, storeID, vendorID, opts.Handle, opts.Title, opts.Status, opts.PublishedAt, opts.DeletedAt).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	variantID := uuid.NewString()
	costExpr := "NULL"
	args := []any{variantID, productID, storeID, "SKU-" + variantID[:8]}
	if opts.CostPrice != nil {
		costExpr = "?"
		args = append(args, *opts.CostPrice)
	}
	stmt := `INSERT INTO product_variants
		(id, product_id, store_id, sku, price, currency_code, inventory_quantity, inventory_policy, cost_price, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, 19.99, 'USD', 7, 'deny', ` + costExpr + `, 0, now(), now())`
	if err := db.Exec(stmt, args...).Error; err != nil {
		t.Fatalf("seed variant: %v", err)
	}
	return productID
}

func seedCategory(t *testing.T, db *gorm.DB, storeID, tenantID, slug, name string, active bool) string {
	t.Helper()
	id := uuid.NewString()
	if err := db.Exec(`
		INSERT INTO categories (id, tenant_id, store_id, name, slug, position, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, now(), now())`,
		id, tenantID, storeID, name, slug, active).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	return id
}

func linkProductCategory(t *testing.T, db *gorm.DB, productID, categoryID string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO product_categories (product_id, category_id) VALUES (?, ?)`,
		productID, categoryID).Error; err != nil {
		t.Fatalf("link product category: %v", err)
	}
}

// ------------------------------------------------------------------
// HTTP helper
// ------------------------------------------------------------------

func request(t *testing.T, r http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func pastTime() *time.Time {
	t := time.Now().Add(-24 * time.Hour)
	return &t
}

func futureTime() *time.Time {
	t := time.Now().Add(24 * time.Hour)
	return &t
}

func nowPtr() *time.Time {
	t := time.Now()
	return &t
}

// ------------------------------------------------------------------
// Happy-path tests
// ------------------------------------------------------------------

func TestAPI_Storefront_ListProducts_HappyPath_200(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "happy", "USD")
	for i := 0; i < 3; i++ {
		seedProduct(t, db, storeID, tenantID, productOpts{
			Status: product.StatusActive, PublishedAt: pastTime(),
		})
	}
	w := request(t, r, "GET", "/api/v1/storefront/stores/happy/products", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 3 {
		t.Fatalf("want 3 products, got %d", len(body.Data))
	}
}

func TestAPI_Storefront_GetProductByHandle_200(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "shop", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(), Handle: "linen-shirt",
	})
	w := request(t, r, "GET", "/api/v1/storefront/stores/shop/products/linen-shirt", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_Storefront_ListProducts_BySlugs_PreservesOrder(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "slugs", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(), Handle: "alpha",
	})
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(), Handle: "bravo",
	})
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(), Handle: "charlie",
	})
	w := request(t, r, "GET", "/api/v1/storefront/stores/slugs/products?slugs=charlie,alpha", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("want 2 products, got %d", len(body.Data))
	}
	if body.Data[0]["handle"] != "charlie" || body.Data[1]["handle"] != "alpha" {
		t.Fatalf("want order [charlie, alpha], got [%v, %v]", body.Data[0]["handle"], body.Data[1]["handle"])
	}
}

func TestAPI_Storefront_ListProducts_BySlugs_DropsMissing(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "slugs2", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(), Handle: "only-one",
	})
	w := request(t, r, "GET", "/api/v1/storefront/stores/slugs2/products?slugs=missing,only-one,also-missing", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Data) != 1 || body.Data[0]["handle"] != "only-one" {
		t.Fatalf("want [only-one], got %+v", body.Data)
	}
}

func TestAPI_Storefront_ListProducts_BySlugs_ExcludesDraft(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "slugs3", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusDraft, Handle: "hidden",
	})
	w := request(t, r, "GET", "/api/v1/storefront/stores/slugs3/products?slugs=hidden", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Data) != 0 {
		t.Fatalf("want 0 products (draft filtered), got %d", len(body.Data))
	}
}

func TestAPI_Storefront_ListCategories_200(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "cats", "USD")
	seedCategory(t, db, storeID, tenantID, "apparel", "Apparel", true)
	seedCategory(t, db, storeID, tenantID, "home", "Home", true)
	w := request(t, r, "GET", "/api/v1/storefront/stores/cats/categories", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("want 2 categories, got %d", len(body.Data))
	}
}

func TestAPI_Storefront_ListByCategorySlug_200(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "cs", "USD")
	catID := seedCategory(t, db, storeID, tenantID, "apparel", "Apparel", true)
	pid := seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(),
	})
	linkProductCategory(t, db, pid, catID)
	// unrelated product, not linked
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(),
	})
	w := request(t, r, "GET", "/api/v1/storefront/stores/cs/categories/apparel/products", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Data) != 1 {
		t.Fatalf("want 1 product via category, got %d", len(body.Data))
	}
}

// ------------------------------------------------------------------
// Leak regression tests — core M6 guarantee
// ------------------------------------------------------------------

func TestAPI_Storefront_ListProducts_ExcludesDraft(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "ld", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime()})
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusDraft})
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusDraft})
	w := request(t, r, "GET", "/api/v1/storefront/stores/ld/products", nil)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body struct{ Data []map[string]any }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Data) != 1 {
		t.Fatalf("want 1 (drafts excluded), got %d", len(body.Data))
	}
}

func TestAPI_Storefront_ListProducts_ExcludesArchived(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "la", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime()})
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusArchived, PublishedAt: pastTime()})
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusArchived, PublishedAt: pastTime()})
	w := request(t, r, "GET", "/api/v1/storefront/stores/la/products", nil)
	var body struct{ Data []map[string]any }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Data) != 1 {
		t.Fatalf("want 1 (archived excluded), got %d", len(body.Data))
	}
}

func TestAPI_Storefront_ListProducts_ExcludesSoftDeleted(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "lsd", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime()})
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime(), DeletedAt: nowPtr()})
	w := request(t, r, "GET", "/api/v1/storefront/stores/lsd/products", nil)
	var body struct{ Data []map[string]any }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Data) != 1 {
		t.Fatalf("want 1 (soft-deleted excluded), got %d", len(body.Data))
	}
}

func TestAPI_Storefront_ListProducts_ExcludesUnpublished(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "lu", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime()})
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: futureTime()})
	w := request(t, r, "GET", "/api/v1/storefront/stores/lu/products", nil)
	var body struct{ Data []map[string]any }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Data) != 1 {
		t.Fatalf("want 1 (future-published excluded), got %d", len(body.Data))
	}
}

func assertFieldAbsent(t *testing.T, body, field string) {
	t.Helper()
	if strings.Contains(body, field) {
		t.Fatalf("forbidden field %q leaked into storefront JSON: %s", field, body)
	}
}

func TestAPI_Storefront_ListProducts_JSONHasNoCostPrice(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "nc", "USD")
	cost := "5.00"
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(), CostPrice: &cost,
	})
	w := request(t, r, "GET", "/api/v1/storefront/stores/nc/products", nil)
	assertFieldAbsent(t, w.Body.String(), "cost_price")
}

func TestAPI_Storefront_ListProducts_JSONHasNoInventoryQuantity(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "niq", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime()})
	w := request(t, r, "GET", "/api/v1/storefront/stores/niq/products", nil)
	assertFieldAbsent(t, w.Body.String(), "inventory_quantity")
}

func TestAPI_Storefront_ListProducts_JSONHasNoTenantID(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "nt", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime()})
	w := request(t, r, "GET", "/api/v1/storefront/stores/nt/products", nil)
	assertFieldAbsent(t, w.Body.String(), "tenant_id")
}

func TestAPI_Storefront_ListProducts_JSONHasNoDeletedAt(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "nd", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime()})
	w := request(t, r, "GET", "/api/v1/storefront/stores/nd/products", nil)
	assertFieldAbsent(t, w.Body.String(), "deleted_at")
}

func TestAPI_Storefront_GetByHandle_Draft_Returns404(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "gd", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusDraft, Handle: "hidden"})
	w := request(t, r, "GET", "/api/v1/storefront/stores/gd/products/hidden", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestAPI_Storefront_GetByHandle_SoftDeleted_Returns404(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, tenantID := seedStorefrontStore(t, db, "gsd", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(), Handle: "gone", DeletedAt: nowPtr(),
	})
	w := request(t, r, "GET", "/api/v1/storefront/stores/gsd/products/gone", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestAPI_Storefront_GetByHandle_CrossStore_Returns404(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeA, tenantA := seedStorefrontStore(t, db, "sa", "USD")
	seedStorefrontStore(t, db, "sb", "USD")
	seedProduct(t, db, storeA, tenantA, productOpts{
		Status: product.StatusActive, PublishedAt: pastTime(), Handle: "cross",
	})
	w := request(t, r, "GET", "/api/v1/storefront/stores/sb/products/cross", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 (cross-store), got %d", w.Code)
	}
}

// ------------------------------------------------------------------
// Store slug + auth tests
// ------------------------------------------------------------------

func TestAPI_Storefront_UnknownStoreSlug_404(t *testing.T) {
	r, _ := setupStorefrontRouter(t, setupOpts{})
	w := request(t, r, "GET", "/api/v1/storefront/stores/nope/products", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestAPI_Storefront_SuspendedStore_404(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	seedSuspendedStore(t, db, "paused")
	w := request(t, r, "GET", "/api/v1/storefront/stores/paused/products", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 (suspended), got %d", w.Code)
	}
}

func TestAPI_Storefront_MissingKey_404(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{storefrontKey: "prod-secret"})
	seedStorefrontStore(t, db, "keyed", "USD")
	w := request(t, r, "GET", "/api/v1/storefront/stores/keyed/products", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 (missing key), got %d", w.Code)
	}
}

func TestAPI_Storefront_WrongKey_404(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{storefrontKey: "prod-secret"})
	seedStorefrontStore(t, db, "keyed2", "USD")
	w := request(t, r, "GET", "/api/v1/storefront/stores/keyed2/products",
		map[string]string{"X-Storefront-Key": "wrong"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 (wrong key), got %d", w.Code)
	}
}

func TestAPI_Storefront_CorrectKey_Passes(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{storefrontKey: "prod-secret"})
	storeID, tenantID := seedStorefrontStore(t, db, "keyed3", "USD")
	seedProduct(t, db, storeID, tenantID, productOpts{Status: product.StatusActive, PublishedAt: pastTime()})
	w := request(t, r, "GET", "/api/v1/storefront/stores/keyed3/products",
		map[string]string{"X-Storefront-Key": "prod-secret"})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_Storefront_EmptySecret_NoCheck(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{storefrontKey: ""})
	seedStorefrontStore(t, db, "nokey", "USD")
	w := request(t, r, "GET", "/api/v1/storefront/stores/nokey/products", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 with no secret configured, got %d", w.Code)
	}
}

// ------------------------------------------------------------------
// Cache header tests
// ------------------------------------------------------------------

func TestAPI_Storefront_ListProducts_SetsCacheControl(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	seedStorefrontStore(t, db, "cc", "USD")
	w := request(t, r, "GET", "/api/v1/storefront/stores/cc/products", nil)
	got := w.Header().Get("Cache-Control")
	want := "public, s-maxage=60, stale-while-revalidate=300"
	if got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
	}
}

func TestAPI_Storefront_ListProducts_SetsETagFromWatermark(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	storeID, _ := seedStorefrontStore(t, db, "et", "USD")
	ts := time.Unix(1700000000, 0)
	seedStorefrontWatermark(t, db, storeID, ts)
	w := request(t, r, "GET", "/api/v1/storefront/stores/et/products", nil)
	etag := w.Header().Get("ETag")
	// Postgres may round-trip the ts; check it contains store id and is a weak etag.
	if !strings.HasPrefix(etag, `W/"`+storeID+"-") {
		t.Fatalf("ETag = %q, want weak etag prefixed W/\"<storeID>-\"", etag)
	}
	if !strings.Contains(etag, "1700000000000") {
		t.Fatalf("ETag = %q, want unix ms 1700000000000", etag)
	}
}

func TestAPI_Storefront_ListProducts_IfNoneMatch_Returns304(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	seedStorefrontStore(t, db, "inm", "USD")
	first := request(t, r, "GET", "/api/v1/storefront/stores/inm/products", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on first response")
	}
	second := request(t, r, "GET", "/api/v1/storefront/stores/inm/products",
		map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Fatalf("want 304, got %d", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Fatalf("304 body should be empty, got %q", second.Body.String())
	}
}

func TestAPI_Storefront_ListProducts_IfNoneMatch_StaleETag_Returns200(t *testing.T) {
	r, db := setupStorefrontRouter(t, setupOpts{})
	seedStorefrontStore(t, db, "stale", "USD")
	w := request(t, r, "GET", "/api/v1/storefront/stores/stale/products",
		map[string]string{"If-None-Match": `W/"bogus-0"`})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 on stale etag, got %d", w.Code)
	}
}
