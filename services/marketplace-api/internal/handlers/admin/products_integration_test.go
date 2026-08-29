//go:build integration

// Package admin_test — API integration tests for the M5a admin products
// HTTP surface. Tests drive the full middleware chain (header-trust auth
// + store middleware + FGA middleware) against a real Postgres via
// pkg/testdb, pre-seeding the stores projection so the stub platform
// client is never invoked.
package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/vendor"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// stubClient is an inert stores.Client — the tests pre-seed the stores
// projection so the middleware never refreshes.
type stubClient struct{}

func (stubClient) GetStore(_ context.Context, _, _ string) (*stores.Store, error) {
	return nil, stores.ErrPlatformUnavailable
}

func (stubClient) GetStoreBySlug(_ context.Context, _ string) (*stores.Store, error) {
	return nil, stores.ErrPlatformUnavailable
}

// productsTables is the dependency-ordered truncate list for tests that
// run through the admin products surface.
//
// promo_redemptions/promo_codes are here even though no products test touches
// them: promo_integration_test.go shares this fixture and seeds a promo code
// with a fixed literal `code`. Nothing else reclaims those rows — promo_codes
// has no FK to `stores`, so the TRUNCATE ... CASCADE below never reaches it —
// and the seed therefore committed permanently, so the second-ever run of that
// test collided on promo_codes_code_uidx. It passed exactly once (2026-08-23)
// and its own success is what broke it (#446). Truncating the pair fixes the
// whole class rather than one test's literal.
var productsTables = []string{
	"outbox_events",
	"promo_redemptions",
	"promo_codes",
	"product_media",
	"variant_option_values",
	"variant_stock",
	"product_variants",
	"product_option_values",
	"product_options",
	"product_categories",
	"products",
	"categories",
	"stores",
}

// testEnv bundles the per-test router + its fakes + the DB handle so
// individual tests can seed fixtures and flip FGA grants.
type testEnv struct {
	router   *gin.Engine
	uploader *media.FakeUploader
	fga      *authz.FakeClient
	db       *gorm.DB
}

func setupTestRouter(t *testing.T) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, productsTables...)

	productRepo := product.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	outboxRepo := outbox.NewRepository(db)
	storesRepo := stores.NewRepository(db)

	uploader := media.NewFakeUploader()
	fga := authz.NewFakeClient()

	svc := product.NewService(product.Config{
		DB:           db,
		Repo:         productRepo,
		StoresRepo:   storesRepo,
		OutboxRepo:   outboxRepo,
		Uploader:     uploader,
		VendorLookup: vendor.NewService(vendor.NewRepository(db)),
	})

	catSvc := category.NewService(category.Config{
		DB:         db,
		Repo:       categoryRepo,
		OutboxRepo: outboxRepo,
	})

	storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{
		Repo:   storesRepo,
		Client: stubClient{},
		Flight: &singleflight.Group{},
	})
	authzMW := authz.NewMiddleware(fga, nil)
	handler := admin.NewProductHandler(svc, categoryRepo, nil)
	catHandler := admin.NewCategoryHandler(catSvc, categoryRepo, nil)
	variantHandler := admin.NewVariantHandler(svc, nil)
	mediaHandler := admin.NewMediaHandler(svc, uploader, nil)

	subSvc := subscription.NewService(subscription.ServiceConfig{
		DB:   db,
		Repo: subscription.NewRepository(),
	})
	subHandler := admin.NewSubscriptionHandler(subSvc, nil)
	promoHandler := admin.NewPromoHandler(db, promo.NewService(db, promo.NewRepository(), nil, nil), subscription.NewRepository(), nil)

	r := gin.New()
	admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
		ProductHandler:      handler,
		CategoryHandler:     catHandler,
		VariantHandler:      variantHandler,
		MediaHandler:        mediaHandler,
		SubscriptionHandler: subHandler,
		PromoHandler:        promoHandler,
		StoresMiddleware:    storeMW,
		AuthzMiddleware:     authzMW,
		InternalSecret:      "",
	})

	return &testEnv{router: r, uploader: uploader, fga: fga, db: db}
}

// seedStoreRow inserts a fresh stores projection row with the given
// tenant and returns (storeID, tenantID). SyncedAt is "now" so the
// StoreMiddleware treats it as fresh and never calls the stub client.
func seedStoreRow(t *testing.T, db *gorm.DB, tenantID string) (string, string) {
	t.Helper()
	if tenantID == "" {
		tenantID = uuid.NewString()
	}
	storeID := uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "api-" + storeID[:8],
		Name:         "API Test Store",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "UTC",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("seed store: %v", err)
	}
	testdb.SeedVendor(t, db, uuid.MustParse(tenantID))
	return storeID, tenantID
}

func authHeaders(userID, tenantID string) map[string]string {
	return map[string]string{
		"X-User-Id":   userID,
		"X-Tenant-Id": tenantID,
	}
}

func request(t *testing.T, r http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// simpleCreateBody returns a minimal valid create request body with a
// unique SKU so tests can create more than one product in the same
// store without colliding on the unique index.
func simpleCreateBody(title, sku string) map[string]any {
	return map[string]any{
		"title": title,
		"variants": []map[string]any{{
			"sku":           sku,
			"price":         "19.99",
			"currency_code": "USD",
		}},
	}
}

func productsURL(storeID string) string {
	return "/api/v1/admin/stores/" + storeID + "/products"
}

// ---------------- tests ----------------

func TestAPI_AdminProducts_Create_HappyPath(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	env.uploader.Register(media.Attrs{StorageKey: "k-1", Size: 1, ContentType: "image/jpeg"})

	body := map[string]any{
		"title": "Linen Shirt",
		"variants": []map[string]any{{
			"sku":           "LINEN-1",
			"price":         "19.99",
			"currency_code": "USD",
		}},
		"media": []map[string]any{{
			"storage_key": "k-1",
			"url":         "https://cdn/x.jpg",
		}},
	}

	w := request(t, env.router, http.MethodPost, productsURL(storeID), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Fatalf("expected id, got %v", resp["id"])
	}
	if resp["handle"] != "linen-shirt" {
		t.Fatalf("handle = %v", resp["handle"])
	}
	variants, _ := resp["variants"].([]any)
	if len(variants) != 1 {
		t.Fatalf("variants = %d", len(variants))
	}
	v0 := variants[0].(map[string]any)
	if v0["sku"] != "LINEN-1" {
		t.Fatalf("sku = %v", v0["sku"])
	}
}

func TestAPI_AdminProducts_Create_MissingTitle_400_ValidationFailed(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	body := map[string]any{
		"variants": []map[string]any{{
			"sku": "X", "price": "1", "currency_code": "USD",
		}},
	}
	w := request(t, env.router, http.MethodPost, productsURL(storeID), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "validation_failed" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestAPI_AdminProducts_Create_NoAuthHeader_401(t *testing.T) {
	env := setupTestRouter(t)
	storeID, _ := seedStoreRow(t, env.db, "")

	w := request(t, env.router, http.MethodPost, productsURL(storeID), simpleCreateBody("X", "X-1"), nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminProducts_Create_NoFGAGrant_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	// No grant.

	w := request(t, env.router, http.MethodPost, productsURL(storeID),
		simpleCreateBody("X", "X-1"), authHeaders(userID, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminProducts_Create_HandleCollision_409(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	body := simpleCreateBody("Linen Shirt", "L-1")
	w := request(t, env.router, http.MethodPost, productsURL(storeID), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("first status = %d, body = %s", w.Code, w.Body.String())
	}

	// Force the same handle for the collision.
	body2 := map[string]any{
		"title":  "Linen Shirt",
		"handle": "linen-shirt",
		"variants": []map[string]any{{
			"sku": "L-2", "price": "19.99", "currency_code": "USD",
		}},
	}
	w2 := request(t, env.router, http.MethodPost, productsURL(storeID), body2, authHeaders(userID, tenantID))
	if w2.Code != http.StatusConflict {
		t.Fatalf("second status = %d, body = %s", w2.Code, w2.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["error"] != "handle_taken" {
		t.Fatalf("error = %v", resp["error"])
	}
	details, ok := resp["details"].(map[string]any)
	if !ok || details["suggested"] == nil || details["suggested"] == "" {
		t.Fatalf("expected details.suggested, got %v", resp["details"])
	}
}

func TestAPI_AdminProducts_List_EmptyPage(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	w := request(t, env.router, http.MethodGet, productsURL(storeID), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].([]any)
	if len(data) != 0 {
		t.Fatalf("data len = %d", len(data))
	}
	meta, _ := resp["meta"].(map[string]any)
	if int(meta["total"].(float64)) != 0 {
		t.Fatalf("total = %v", meta["total"])
	}
}

// createProductsViaService seeds n products fast by calling the service
// directly (the HTTP path is exercised by dedicated tests).
func createProductsViaService(t *testing.T, env *testEnv, storeID, tenantID string, n int, status string) {
	t.Helper()
	svc := product.NewService(product.Config{
		DB:           env.db,
		Repo:         product.NewRepository(env.db),
		StoresRepo:   stores.NewRepository(env.db),
		OutboxRepo:   outbox.NewRepository(env.db),
		Uploader:     env.uploader,
		VendorLookup: vendor.NewService(vendor.NewRepository(env.db)),
	})
	for i := 0; i < n; i++ {
		title := "Seed " + uuid.NewString()[:8]
		_, err := svc.Create(context.Background(), product.CreateRequest{
			StoreID:  storeID,
			TenantID: tenantID,
			Title:    title,
			Status:   status,
			Variants: []product.VariantInput{{
				SKU:          uuid.NewString()[:8],
				Price:        decimal.NewFromInt(10),
				CurrencyCode: "USD",
			}},
		})
		if err != nil {
			t.Fatalf("seed product %d: %v", i, err)
		}
	}
}

func TestAPI_AdminProducts_List_Paginates(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	createProductsViaService(t, env, storeID, tenantID, 25, product.StatusDraft)

	w := request(t, env.router, http.MethodGet,
		productsURL(storeID)+"?page=2&page_size=10", nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].([]any)
	if len(data) != 10 {
		t.Fatalf("page 2 len = %d, want 10", len(data))
	}
	meta, _ := resp["meta"].(map[string]any)
	if int(meta["total"].(float64)) != 25 {
		t.Fatalf("total = %v", meta["total"])
	}
	if int(meta["total_pages"].(float64)) != 3 {
		t.Fatalf("total_pages = %v", meta["total_pages"])
	}
}

func TestAPI_AdminProducts_List_FiltersByStatus(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	createProductsViaService(t, env, storeID, tenantID, 3, product.StatusDraft)
	createProductsViaService(t, env, storeID, tenantID, 2, product.StatusActive)

	w := request(t, env.router, http.MethodGet,
		productsURL(storeID)+"?status=active", nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("active count = %d, want 2", len(data))
	}
}

// createOneHTTP posts a simple product via the HTTP surface and returns
// its id. Caller must have granted the userID admin already.
func createOneHTTP(t *testing.T, env *testEnv, storeID, tenantID, userID, title, sku string) string {
	t.Helper()
	w := request(t, env.router, http.MethodPost, productsURL(storeID),
		simpleCreateBody(title, sku), authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("missing id in response: %s", w.Body.String())
	}
	return id
}

func TestAPI_AdminProducts_Get_Existing_200(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	id := createOneHTTP(t, env, storeID, tenantID, userID, "Get Me", "GET-1")

	w := request(t, env.router, http.MethodGet,
		productsURL(storeID)+"/"+id, nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != id {
		t.Fatalf("id = %v, want %s", resp["id"], id)
	}
}

func TestAPI_AdminProducts_Get_NonExistent_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	w := request(t, env.router, http.MethodGet,
		productsURL(storeID)+"/"+uuid.NewString(), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminProducts_Get_CrossTenant_404(t *testing.T) {
	env := setupTestRouter(t)
	storeA, tenantA := seedStoreRow(t, env.db, "")
	storeB, tenantB := seedStoreRow(t, env.db, "")
	userA := uuid.NewString()
	userB := uuid.NewString()
	env.fga.Grant(userA, authz.RoleAdmin, tenantA)
	env.fga.Grant(userB, authz.RoleAdmin, tenantB)

	id := createOneHTTP(t, env, storeA, tenantA, userA, "Tenant A Product", "TA-1")

	// User B tries to read A's product via B's store. Should 404.
	w := request(t, env.router, http.MethodGet,
		productsURL(storeB)+"/"+id, nil, authHeaders(userB, tenantB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminProducts_Patch_UpdatesTitle_200(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	id := createOneHTTP(t, env, storeID, tenantID, userID, "Old Title", "P-1")

	w := request(t, env.router, http.MethodPatch,
		productsURL(storeID)+"/"+id,
		map[string]any{"title": "New Title"},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["title"] != "New Title" {
		t.Fatalf("title = %v", resp["title"])
	}
}

func TestAPI_AdminProducts_Patch_NotFound_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	w := request(t, env.router, http.MethodPatch,
		productsURL(storeID)+"/"+uuid.NewString(),
		map[string]any{"title": "Nope"},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminProducts_Delete_204(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	id := createOneHTTP(t, env, storeID, tenantID, userID, "To Delete", "D-1")

	w := request(t, env.router, http.MethodDelete,
		productsURL(storeID)+"/"+id, nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}

	// Subsequent GET is 404.
	w2 := request(t, env.router, http.MethodGet,
		productsURL(storeID)+"/"+id, nil, authHeaders(userID, tenantID))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("post-delete get = %d", w2.Code)
	}
}

func TestAPI_AdminProducts_Copy_HappyPath_201(t *testing.T) {
	env := setupTestRouter(t)
	storeA, tenantID := seedStoreRow(t, env.db, "")
	storeB, _ := seedStoreRow(t, env.db, tenantID)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	id := createOneHTTP(t, env, storeA, tenantID, userID, "To Copy", "C-1")

	w := request(t, env.router, http.MethodPost,
		productsURL(storeA)+"/"+id+"/copy",
		map[string]any{"target_store_id": storeB},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["copy_source_product_id"] != id {
		t.Fatalf("copy_source_product_id = %v, want %s", resp["copy_source_product_id"], id)
	}
	if resp["store_id"] != storeB {
		t.Fatalf("store_id = %v, want %s", resp["store_id"], storeB)
	}
}

func TestAPI_AdminProducts_Copy_TargetSameAsSource_400_TargetStoreInvalid(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	id := createOneHTTP(t, env, storeID, tenantID, userID, "Self Copy", "SC-1")

	w := request(t, env.router, http.MethodPost,
		productsURL(storeID)+"/"+id+"/copy",
		map[string]any{"target_store_id": storeID},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "target_store_invalid" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestAPI_AdminProducts_Authz_RequireOwnerOnDelete_StaffDenied_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	adminUser := uuid.NewString()
	staffUser := uuid.NewString()
	env.fga.Grant(adminUser, authz.RoleAdmin, tenantID)
	env.fga.Grant(staffUser, authz.RoleStaff, tenantID)

	id := createOneHTTP(t, env, storeID, tenantID, adminUser, "Keep Me", "K-1")

	w := request(t, env.router, http.MethodDelete,
		productsURL(storeID)+"/"+id, nil, authHeaders(staffUser, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminProducts_Authz_RequireAdminOnCreate_StaffDenied_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	staffUser := uuid.NewString()
	env.fga.Grant(staffUser, authz.RoleStaff, tenantID)

	w := request(t, env.router, http.MethodPost, productsURL(storeID),
		simpleCreateBody("Nope", "N-1"), authHeaders(staffUser, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// A categories-only PATCH must persist. This pins already-correct behaviour
// that had no direct coverage: such a request matches neither the aggregate
// path (no options/variants) nor the basics path (UpdateBasicsRequest has no
// CategoryIDs field), and is instead handled by the third, independent
// section of the Patch handler — products.go:215, which calls
// UpdateCategoryLinks. That section's guard exists to avoid double-firing
// when the aggregate path already replaced the links in its own tx; this
// test locks in the categories-only case it is responsible for.
func TestAPI_AdminProducts_Patch_CategoryIDsOnly_Persists(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)
	catID := seedCategoryViaRepo(t, env, storeID, tenantID, "Swimwear", "swimwear-"+uuid.NewString()[:6])

	w := request(t, env.router, http.MethodPatch,
		"/api/v1/admin/stores/"+storeID+"/products/"+pid,
		map[string]any{"category_ids": []string{catID}},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("patch category_ids: want 200, got %d: %s", w.Code, w.Body.String())
	}

	// Read it back — a 200 alone proves nothing here; that was the bug.
	w2 := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/products/"+pid, nil,
		authHeaders(userID, tenantID))
	if w2.Code != http.StatusOK {
		t.Fatalf("get product: want 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var got struct {
		Categories []struct {
			ID string `json:"id"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Categories) != 1 || got.Categories[0].ID != catID {
		t.Fatalf("category link not persisted: got %+v, want [%s]", got.Categories, catID)
	}
}
