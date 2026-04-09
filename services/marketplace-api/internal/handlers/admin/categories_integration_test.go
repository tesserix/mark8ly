//go:build integration

package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/category"
)

func categoriesURL(storeID string) string {
	return "/api/v1/admin/stores/" + storeID + "/categories"
}

func seedCategoryViaRepo(t *testing.T, env *testEnv, storeID, tenantID, name, slug string) string {
	t.Helper()
	repo := category.NewRepository(env.db)
	c := &category.Category{
		TenantID: tenantID,
		StoreID:  storeID,
		Name:     name,
		Slug:     slug,
		IsActive: true,
	}
	if err := repo.Create(context.Background(), c); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	return c.ID
}

func TestAPI_AdminCategories_Create_HappyPath(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	body := map[string]any{"name": "Shoes", "position": 0}
	w := request(t, env.router, http.MethodPost, categoriesURL(storeID), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] == nil || resp["id"] == "" {
		t.Fatalf("missing id: %s", w.Body.String())
	}
	if resp["name"] != "Shoes" {
		t.Fatalf("name = %v", resp["name"])
	}
	if resp["slug"] != "shoes" {
		t.Fatalf("slug = %v", resp["slug"])
	}
}

func TestAPI_AdminCategories_Create_MissingName_400(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	w := request(t, env.router, http.MethodPost, categoriesURL(storeID),
		map[string]any{}, authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "validation_failed" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestAPI_AdminCategories_Create_DuplicateSlug_409(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	body := map[string]any{"name": "Shoes", "slug": "shoes"}
	w := request(t, env.router, http.MethodPost, categoriesURL(storeID), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("first = %d, %s", w.Code, w.Body.String())
	}
	w2 := request(t, env.router, http.MethodPost, categoriesURL(storeID), body, authHeaders(userID, tenantID))
	if w2.Code != http.StatusConflict {
		t.Fatalf("second = %d, %s", w2.Code, w2.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["error"] != "slug_taken" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestAPI_AdminCategories_List_ReturnsAll(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	seedCategoryViaRepo(t, env, storeID, tenantID, "A", "a")
	seedCategoryViaRepo(t, env, storeID, tenantID, "B", "b")
	seedCategoryViaRepo(t, env, storeID, tenantID, "C", "c")

	w := request(t, env.router, http.MethodGet, categoriesURL(storeID), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].([]any)
	if len(data) != 3 {
		t.Fatalf("data len = %d", len(data))
	}
}

func TestAPI_AdminCategories_Patch_UpdatesName(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	id := seedCategoryViaRepo(t, env, storeID, tenantID, "Old", "old")

	w := request(t, env.router, http.MethodPatch, categoriesURL(storeID)+"/"+id,
		map[string]any{"name": "New"}, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["name"] != "New" {
		t.Fatalf("name = %v", resp["name"])
	}
}

func TestAPI_AdminCategories_Delete_Succeeds_204(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	id := seedCategoryViaRepo(t, env, storeID, tenantID, "D", "d")

	w := request(t, env.router, http.MethodDelete, categoriesURL(storeID)+"/"+id, nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, %s", w.Code, w.Body.String())
	}
	w2 := request(t, env.router, http.MethodGet, categoriesURL(storeID), nil, authHeaders(userID, tenantID))
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	data, _ := resp["data"].([]any)
	for _, row := range data {
		if row.(map[string]any)["id"] == id {
			t.Fatalf("deleted row still listed")
		}
	}
}

func TestAPI_AdminCategories_Delete_HasChildren_409(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	parentID := seedCategoryViaRepo(t, env, storeID, tenantID, "Parent", "parent")
	child := &category.Category{
		TenantID: tenantID, StoreID: storeID, ParentID: &parentID,
		Name: "Child", Slug: "child", IsActive: true,
	}
	if err := env.db.Create(child).Error; err != nil {
		t.Fatalf("seed child: %v", err)
	}

	w := request(t, env.router, http.MethodDelete, categoriesURL(storeID)+"/"+parentID, nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "category_has_children" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestAPI_AdminCategories_Delete_HasProducts_409(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	catID := seedCategoryViaRepo(t, env, storeID, tenantID, "Pop", "pop")

	// Create a product via HTTP then link it to the category.
	pid := createOneHTTP(t, env, storeID, tenantID, userID, "Linked", "LK-1")
	if err := env.db.Exec(
		`INSERT INTO product_categories (product_id, category_id) VALUES (?, ?)`,
		pid, catID).Error; err != nil {
		t.Fatalf("link product: %v", err)
	}

	w := request(t, env.router, http.MethodDelete, categoriesURL(storeID)+"/"+catID, nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "category_not_empty" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestAPI_AdminCategories_Authz_StaffCannotCreate_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	staffUser := uuid.NewString()
	env.fga.Grant(staffUser, authz.RoleStaff, tenantID)

	w := request(t, env.router, http.MethodPost, categoriesURL(storeID),
		map[string]any{"name": "Nope"}, authHeaders(staffUser, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminCategories_Authz_UnauthenticatedRequest_401(t *testing.T) {
	env := setupTestRouter(t)
	storeID, _ := seedStoreRow(t, env.db, "")

	w := request(t, env.router, http.MethodGet, categoriesURL(storeID), nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}
