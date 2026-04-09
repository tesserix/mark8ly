//go:build integration

package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/media"
)

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
