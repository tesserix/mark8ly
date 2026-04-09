//go:build integration

package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// fakeSigningUploader wraps FakeUploader so it also satisfies both
// media.SignedURLGenerator and media.SignedReadURLGenerator, which the
// recrop handler type-asserts against. Real GCS is obviously not
// involved — the returned URLs are deterministic fakes for assertions.
type fakeSigningUploader struct {
	inner *media.FakeUploader
}

func (f *fakeSigningUploader) Verify(ctx context.Context, key string) (*media.Attrs, error) {
	return f.inner.Verify(ctx, key)
}

func (f *fakeSigningUploader) Register(a media.Attrs) { f.inner.Register(a) }

func (f *fakeSigningUploader) SignedUploadURL(_ context.Context, key, _ string, expires time.Duration) (string, time.Time, error) {
	return "https://fake.gcs/put/" + key, time.Now().Add(expires), nil
}

func (f *fakeSigningUploader) SignedReadURL(_ context.Context, key string, expires time.Duration) (string, time.Time, error) {
	return "https://fake.gcs/get/" + key, time.Now().Add(expires), nil
}

// setupTestRouterWithSigningUploader is like setupTestRouter but wires
// a uploader that implements SignedURLGenerator + SignedReadURLGenerator
// so the recrop handler's type assertions succeed.
func setupTestRouterWithSigningUploader(t *testing.T) (*testEnv, *fakeSigningUploader) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, productsTables...)

	productRepo := product.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	outboxRepo := outbox.NewRepository(db)
	storesRepo := stores.NewRepository(db)

	signer := &fakeSigningUploader{inner: media.NewFakeUploader()}
	fga := authz.NewFakeClient()

	svc := product.NewService(product.Config{
		DB:         db,
		Repo:       productRepo,
		StoresRepo: storesRepo,
		OutboxRepo: outboxRepo,
		Uploader:   signer,
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
	mediaHandler := admin.NewMediaHandler(svc, signer, nil)

	r := gin.New()
	admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
		ProductHandler:   handler,
		CategoryHandler:  catHandler,
		VariantHandler:   variantHandler,
		MediaHandler:     mediaHandler,
		StoresMiddleware: storeMW,
		AuthzMiddleware:  authzMW,
		InternalSecret:   "",
	})
	return &testEnv{router: r, uploader: signer.inner, fga: fga, db: db}, signer
}

func mediaURL(storeID, productID string) string {
	return "/api/v1/admin/stores/" + storeID + "/products/" + productID + "/media"
}

func TestAPI_AdminMedia_UploadURL_WithFake_Returns501(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	body := map[string]any{
		"content_hash": "abcdef0123456789",
		"filename":     "photo.jpg",
		"content_type": "image/jpeg",
	}
	w := request(t, env.router, http.MethodPost, mediaURL(storeID, pid)+"/upload-url", body, authHeaders(userID, tenantID))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "not_implemented" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestAPI_AdminMedia_Create_HappyPath_201(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)
	env.uploader.Register(media.Attrs{StorageKey: "k-media-1", Size: 10, ContentType: "image/jpeg"})

	body := map[string]any{
		"storage_key": "k-media-1",
		"url":         "https://cdn/x.jpg",
		"position":    0,
	}
	w := request(t, env.router, http.MethodPost, mediaURL(storeID, pid), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["storage_key"] != "k-media-1" {
		t.Fatalf("storage_key = %v", resp["storage_key"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Fatalf("missing id")
	}
}

func TestAPI_AdminMedia_Create_UploadNotFound_400(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	body := map[string]any{
		"storage_key": "unknown-key",
		"url":         "https://cdn/x.jpg",
	}
	w := request(t, env.router, http.MethodPost, mediaURL(storeID, pid), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "upload_not_found" {
		t.Fatalf("error = %v", resp["error"])
	}
}

// createMediaHTTP adds a media row via the HTTP surface and returns its id.
func createMediaHTTP(t *testing.T, env *testEnv, storeID, tenantID, userID, pid, key string) string {
	t.Helper()
	env.uploader.Register(media.Attrs{StorageKey: key, Size: 1, ContentType: "image/jpeg"})
	w := request(t, env.router, http.MethodPost, mediaURL(storeID, pid),
		map[string]any{"storage_key": key, "url": "https://cdn/" + key}, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("seed media: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["id"].(string)
}

func TestAPI_AdminMedia_Patch_UpdatesAlt_200(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)
	mid := createMediaHTTP(t, env, storeID, tenantID, userID, pid, "k-alt")

	alt := "a new alt"
	w := request(t, env.router, http.MethodPatch, mediaURL(storeID, pid)+"/"+mid,
		map[string]any{"alt": alt}, authHeaders(userID, tenantID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}

	// Verify via GET product.
	w2 := request(t, env.router, http.MethodGet, productsURL(storeID)+"/"+pid, nil, authHeaders(userID, tenantID))
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	mediaRows, _ := resp["media"].([]any)
	found := false
	for _, m := range mediaRows {
		row := m.(map[string]any)
		if row["id"] == mid && row["alt"] == alt {
			found = true
		}
	}
	if !found {
		t.Fatalf("alt update not visible: %s", w2.Body.String())
	}
}

func TestAPI_AdminMedia_Patch_NotFound_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	alt := "x"
	w := request(t, env.router, http.MethodPatch, mediaURL(storeID, pid)+"/"+uuid.NewString(),
		map[string]any{"alt": alt}, authHeaders(userID, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminMedia_Delete_Succeeds_204(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)
	mid := createMediaHTTP(t, env, storeID, tenantID, userID, pid, "k-del")

	w := request(t, env.router, http.MethodDelete, mediaURL(storeID, pid)+"/"+mid, nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}

	w2 := request(t, env.router, http.MethodGet, productsURL(storeID)+"/"+pid, nil, authHeaders(userID, tenantID))
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	mediaRows, _ := resp["media"].([]any)
	for _, m := range mediaRows {
		if m.(map[string]any)["id"] == mid {
			t.Fatalf("media row still present after delete")
		}
	}
}

func TestAPI_AdminMedia_Create_WithVariantID_Persists(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, v1, _ := seedProductViaService(t, env, storeID, tenantID)
	env.uploader.Register(media.Attrs{StorageKey: "k-variant-1", Size: 10, ContentType: "image/jpeg"})

	body := map[string]any{
		"storage_key": "k-variant-1",
		"url":         "https://cdn/v.jpg",
		"variant_id":  v1,
	}
	w := request(t, env.router, http.MethodPost, mediaURL(storeID, pid), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["variant_id"] != v1 {
		t.Fatalf("variant_id = %v, want %s", resp["variant_id"], v1)
	}
}

func TestAPI_AdminMedia_Patch_UpdatesVariantID(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, v1, _ := seedProductViaService(t, env, storeID, tenantID)
	mid := createMediaHTTP(t, env, storeID, tenantID, userID, pid, "k-vpatch")

	w := request(t, env.router, http.MethodPatch, mediaURL(storeID, pid)+"/"+mid,
		map[string]any{"variant_id": v1}, authHeaders(userID, tenantID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}

	w2 := request(t, env.router, http.MethodGet, productsURL(storeID)+"/"+pid, nil, authHeaders(userID, tenantID))
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	mediaRows, _ := resp["media"].([]any)
	found := false
	for _, m := range mediaRows {
		row := m.(map[string]any)
		if row["id"] == mid && row["variant_id"] == v1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("variant_id update not visible: %s", w2.Body.String())
	}
}

// ---------------- recrop tests (M7c gaps #3 + #8) ----------------

func recropURL(storeID, productID, mediaID string) string {
	return mediaURL(storeID, productID) + "/" + mediaID + "/recrop"
}

func TestAPI_AdminMedia_Recrop_ReturnsSignedUrlsPreservingOriginal(t *testing.T) {
	env, _ := setupTestRouterWithSigningUploader(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)
	mid := createMediaHTTP(t, env, storeID, tenantID, userID, pid, "orig-key-123")

	body := map[string]any{
		"crop_box": map[string]any{"x": 10, "y": 20, "width": 300, "height": 400},
		"rotation": 0,
	}
	w := request(t, env.router, http.MethodPost, recropURL(storeID, pid, mid), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["source_original_url"] == nil || resp["source_original_url"] == "" {
		t.Fatalf("missing source_original_url: %s", w.Body.String())
	}
	if resp["upload_url"] == nil || resp["upload_url"] == "" {
		t.Fatalf("missing upload_url: %s", w.Body.String())
	}
	newKey, _ := resp["new_storage_key"].(string)
	if newKey == "" || newKey == "orig-key-123" {
		t.Fatalf("new_storage_key must differ from original, got %q", newKey)
	}

	// DB row must be unchanged — recrop doesn't persist.
	var row product.Media
	if err := env.db.Where("id = ?", mid).First(&row).Error; err != nil {
		t.Fatalf("reload media: %v", err)
	}
	if row.StorageKey != "orig-key-123" {
		t.Fatalf("storage_key mutated prematurely: %q", row.StorageKey)
	}
	if row.GcsPathOriginal != "orig-key-123" {
		t.Fatalf("gcs_path_original mutated: %q", row.GcsPathOriginal)
	}
}

func TestAPI_AdminMedia_Recrop_AfterCommit_KeepsOriginalPinned(t *testing.T) {
	env, _ := setupTestRouterWithSigningUploader(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)
	mid := createMediaHTTP(t, env, storeID, tenantID, userID, pid, "orig-commit")

	// Recrop to obtain a new_storage_key.
	body := map[string]any{
		"crop_box": map[string]any{"x": 0, "y": 0, "width": 100, "height": 100},
		"rotation": 0,
	}
	w := request(t, env.router, http.MethodPost, recropURL(storeID, pid, mid), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("recrop status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	newKey := resp["new_storage_key"].(string)

	// Commit via PATCH with new storage_key.
	w2 := request(t, env.router, http.MethodPatch, mediaURL(storeID, pid)+"/"+mid,
		map[string]any{"storage_key": newKey}, authHeaders(userID, tenantID))
	if w2.Code != http.StatusNoContent {
		t.Fatalf("patch commit status = %d, %s", w2.Code, w2.Body.String())
	}

	var row product.Media
	if err := env.db.Where("id = ?", mid).First(&row).Error; err != nil {
		t.Fatalf("reload media: %v", err)
	}
	if row.StorageKey != newKey {
		t.Fatalf("storage_key = %q, want %q", row.StorageKey, newKey)
	}
	if row.GcsPathOriginal != "orig-commit" {
		t.Fatalf("gcs_path_original drifted: %q, want %q", row.GcsPathOriginal, "orig-commit")
	}
}

func TestAPI_AdminMedia_Recrop_RejectsOtherTenant_404(t *testing.T) {
	env, _ := setupTestRouterWithSigningUploader(t)
	storeA, tenantA := seedStoreRow(t, env.db, "")
	userA := uuid.NewString()
	env.fga.Grant(userA, authz.RoleAdmin, tenantA)
	pidA, _, _ := seedProductViaService(t, env, storeA, tenantA)
	midA := createMediaHTTP(t, env, storeA, tenantA, userA, pidA, "orig-tenantA")

	// Caller from tenant B with admin role in tenant B.
	_, tenantB := seedStoreRow(t, env.db, "")
	userB := uuid.NewString()
	env.fga.Grant(userB, authz.RoleAdmin, tenantB)

	body := map[string]any{
		"crop_box": map[string]any{"x": 0, "y": 0, "width": 10, "height": 10},
	}
	w := request(t, env.router, http.MethodPost, recropURL(storeA, pidA, midA), body, authHeaders(userB, tenantB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for cross-tenant, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminMedia_Recrop_WithFakeUploader_Returns501(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)
	mid := createMediaHTTP(t, env, storeID, tenantID, userID, pid, "k-recrop-501")

	body := map[string]any{
		"crop_box": map[string]any{"x": 0, "y": 0, "width": 1, "height": 1},
	}
	w := request(t, env.router, http.MethodPost, recropURL(storeID, pid, mid), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminMedia_StaffDenied_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	adminUser := uuid.NewString()
	staffUser := uuid.NewString()
	env.fga.Grant(adminUser, authz.RoleAdmin, tenantID)
	env.fga.Grant(staffUser, authz.RoleStaff, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	body := map[string]any{"storage_key": "k", "url": "https://cdn/x"}
	w := request(t, env.router, http.MethodPost, mediaURL(storeID, pid), body, authHeaders(staffUser, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
}
