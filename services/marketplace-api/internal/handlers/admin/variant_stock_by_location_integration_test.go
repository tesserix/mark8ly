//go:build integration

// Package admin_test — #177 PR 5e at the HTTP boundary.
//
// The rule the slice is built around: a store with ONE warehouse sees
// exactly what it saw before. Everything here that exercises the
// per-warehouse path is therefore a two-warehouse store.
package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
)

// seedWarehouseFor inserts a live warehouse for a store.
func seedStockWarehouse(t *testing.T, db *gorm.DB, tenantID, storeID, name string) string {
	t.Helper()
	id := uuid.NewString()
	if err := db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region,
		                         postal_code, country_code, phone)
		 VALUES (?, ?, ?, ?, '1 Dock Rd', 'Mumbai', 'MH', '400001', 'IN', '+912200000000')`,
		id, tenantID, storeID, name).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	return id
}

// storeWarehouseID returns the warehouse the store's stock writes resolve
// to. Product writes create one when the store has none (#177 PR 6), so
// after seeding a product this always finds a row.
func storeWarehouseID(t *testing.T, db *gorm.DB, storeID string) string {
	t.Helper()
	var id string
	if err := db.Raw(
		`SELECT id::text FROM warehouses
		  WHERE store_id = ? AND archived_at IS NULL
		  ORDER BY is_default DESC, priority ASC, created_at ASC LIMIT 1`,
		storeID).Row().Scan(&id); err != nil {
		t.Fatalf("no warehouse for store %s: %v", storeID, err)
	}
	return id
}

func stockAtLoc(t *testing.T, db *gorm.DB, variantID, locationID string) int {
	t.Helper()
	var q int
	if err := db.Raw(
		`SELECT COALESCE((SELECT quantity FROM variant_stock
		                   WHERE variant_id = ? AND location_id = ?), -1)`,
		variantID, locationID).Scan(&q).Error; err != nil {
		t.Fatalf("read stock: %v", err)
	}
	return q
}

// seedProductWithStock creates a product whose single variant holds qty on
// the SENTINEL — the state every product in production is in today.
func seedProductWithStock(t *testing.T, env *testEnv, storeID, tenantID, userID string, qty int) (string, string) {
	t.Helper()
	body := map[string]any{
		"title": "Linen Shirt", "handle": "linen-" + uuid.NewString()[:8],
		"status": "draft",
		"variants": []map[string]any{
			{"sku": "SKU-" + uuid.NewString()[:8], "price": "10.00", "inventory_quantity": qty},
		},
	}
	w := request(t, env.router, http.MethodPost,
		"/api/v1/admin/stores/"+storeID+"/products", body, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("seed product: status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID       string `json:"id"`
		Variants []struct {
			ID string `json:"id"`
		} `json:"variants"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, w.Body.String())
	}
	if len(resp.Variants) == 0 {
		t.Fatalf("no variants: %s", w.Body.String())
	}
	return resp.ID, resp.Variants[0].ID
}

// THE test. A merchant opening a product that reads 10, typing 10 against
// their main warehouse and 5 against a new one, must end up with 15 — not
// 25. Leaving the sentinel row behind sums it with the real row for the
// same warehouse in checkout_availability, and their stock doubles.
func TestAPI_VariantStock_PerLocationSaveConservesTheTotal(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	productID, variantID := seedProductWithStock(t, env, storeID, tenantID, userID, 10)
	// Seeding the product created the store's warehouse and put the units
	// there — the sentinel is gone (#177 PR 6).
	whA := storeWarehouseID(t, env.db, storeID)
	if got := stockAtLoc(t, env.db, variantID, whA); got != 10 {
		t.Fatalf("precondition: warehouse stock = %d, want 10", got)
	}
	whB := seedStockWarehouse(t, env.db, tenantID, storeID, "Bravo")

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, productID, variantID),
		map[string]any{"inventory_by_location": map[string]int{whA: 10, whB: 5}},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	if got := stockAtLoc(t, env.db, variantID, retiredSentinelLoc); got != -1 {
		t.Fatalf("nothing may live on the retired sentinel, found %d", got)
	}
	if got := stockAtLoc(t, env.db, variantID, whA); got != 10 {
		t.Fatalf("warehouse A = %d, want 10", got)
	}
	if got := stockAtLoc(t, env.db, variantID, whB); got != 5 {
		t.Fatalf("warehouse B = %d, want 5", got)
	}

	var total int
	_ = env.db.Raw(`SELECT inventory_quantity FROM product_variants WHERE id = ?`, variantID).
		Scan(&total).Error
	if total != 15 {
		t.Fatalf("inventory_quantity = %d, want 15 (10 + 5), not 25", total)
	}
}

// The single-quantity path: one number, written to the store's warehouse.
// It used to write the sentinel; #177 PR 6 retired that, and a store with
// no warehouse gets one created rather than falling back to a location
// that names nothing.
func TestAPI_VariantStock_PlainQuantityWritesTheStoreWarehouse(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	productID, variantID := seedProductWithStock(t, env, storeID, tenantID, userID, 10)

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, productID, variantID),
		map[string]any{"inventory_quantity": 42}, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	wh := storeWarehouseID(t, env.db, storeID)
	if got := stockAtLoc(t, env.db, variantID, wh); got != 42 {
		t.Fatalf("warehouse stock = %d, want 42", got)
	}
	if got := stockAtLoc(t, env.db, variantID, retiredSentinelLoc); got != -1 {
		t.Fatalf("nothing may be written to the retired sentinel, found %d", got)
	}
}

func TestAPI_VariantStock_BothFieldsIsRefused(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	productID, variantID := seedProductWithStock(t, env, storeID, tenantID, userID, 10)
	whA := seedStockWarehouse(t, env.db, tenantID, storeID, "Alpha")

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, productID, variantID),
		map[string]any{"inventory_quantity": 3, "inventory_by_location": map[string]int{whA: 7}},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// Stock written against another store's warehouse is stock the allocator
// will never offer — to the merchant it reads as inventory that vanished.
func TestAPI_VariantStock_AnotherStoresWarehouseIsRefused(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	otherStoreID, otherTenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	productID, variantID := seedProductWithStock(t, env, storeID, tenantID, userID, 10)
	foreign := seedStockWarehouse(t, env.db, otherTenantID, otherStoreID, "Theirs")

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, productID, variantID),
		map[string]any{"inventory_by_location": map[string]int{foreign: 7}},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := stockAtLoc(t, env.db, variantID, storeWarehouseID(t, env.db, storeID)); got != 10 {
		t.Fatalf("a refused save moved stock: warehouse = %d, want 10", got)
	}
}

func TestAPI_VariantStock_ArchivedWarehouseIsRefused(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	productID, variantID := seedProductWithStock(t, env, storeID, tenantID, userID, 10)
	whA := seedStockWarehouse(t, env.db, tenantID, storeID, "Gone")
	if err := env.db.Exec(`UPDATE warehouses SET archived_at = now() WHERE id = ?`, whA).Error; err != nil {
		t.Fatalf("archive: %v", err)
	}

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, productID, variantID),
		map[string]any{"inventory_by_location": map[string]int{whA: 7}},
		authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// The detail view must expose the breakdown, including a variant still on
// the sentinel — the admin needs to tell "this warehouse holds 10" from
// "10 units exist but are not assigned anywhere yet".
func TestAPI_ProductGet_CarriesThePerLocationBreakdown(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	productID, variantID := seedProductWithStock(t, env, storeID, tenantID, userID, 10)

	get := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/products/"+productID, nil,
		authHeaders(userID, tenantID))
	if get.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", get.Code, get.Body.String())
	}
	var resp struct {
		Variants []struct {
			ID                  string         `json:"id"`
			InventoryByLocation map[string]int `json:"inventory_by_location"`
		} `json:"variants"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Variants) == 0 {
		t.Fatalf("no variants: %s", get.Body.String())
	}
	wh := storeWarehouseID(t, env.db, storeID)
	if got := resp.Variants[0].InventoryByLocation[wh]; got != 10 {
		t.Fatalf("warehouse breakdown = %d, want 10 (variant %s): %s",
			got, variantID, get.Body.String())
	}
	if _, present := resp.Variants[0].InventoryByLocation[retiredSentinelLoc]; present {
		t.Fatalf("the retired sentinel must not appear in a breakdown: %s", get.Body.String())
	}
}

// retiredSentinelLoc is the location every stock row carried before #177 PR 6
// moved them onto real warehouses. The production constant is gone;
// these tests still need the value precisely BECAUSE nothing writes it
// any more — a straggler row from an old pod or a restored backup must
// still be swept, and that is what they pin.
const retiredSentinelLoc = "00000000-0000-0000-0000-000000000001"
