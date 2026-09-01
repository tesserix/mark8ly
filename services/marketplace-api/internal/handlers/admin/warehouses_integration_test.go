//go:build integration

package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
)

// #177 PR 5b — the admin warehouse API.
//
// The rule the whole slice exists for is TestAPI_Warehouses_Rename_*: a
// rename must EDIT the row. The carrier form's name-keyed upsert forked a
// second, stockless warehouse instead, and allocation then reported nothing
// available while the stock sat on the orphan.

func warehousesURL(storeID string) string {
	return "/api/v1/admin/stores/" + storeID + "/warehouses"
}

func warehouseBody(name string) map[string]any {
	return map[string]any{
		"name": name, "line1": "12 Industrial Estate", "city": "Mumbai",
		"region": "MH", "postal_code": "400001", "country_code": "IN",
		"phone": "+912200000000", "contact_person": "Warehouse Manager",
	}
}

// createWarehouse posts one and returns its id.
func createWarehouse(t *testing.T, env *testEnv, storeID, tenantID, userID, name string) string {
	t.Helper()
	w := request(t, env.router, http.MethodPost, warehousesURL(storeID),
		warehouseBody(name), authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %q: status = %d, body = %s", name, w.Code, w.Body.String())
	}
	return dataObject(t, w.Body.Bytes())["id"].(string)
}

func dataObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, string(raw))
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data object: %s", string(raw))
	}
	return data
}

func dataArray(t *testing.T, raw []byte) []any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, string(raw))
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("missing data array: %s", string(raw))
	}
	return data
}

// ownerEnv sets up a store whose caller holds RoleOwner.
func ownerEnv(t *testing.T) (*testEnv, string, string, string) {
	t.Helper()
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	return env, storeID, tenantID, userID
}

func TestAPI_Warehouses_Create_HappyPath(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)

	w := request(t, env.router, http.MethodPost, warehousesURL(storeID),
		warehouseBody("Main Warehouse"), authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	data := dataObject(t, w.Body.Bytes())
	if data["name"] != "Main Warehouse" || data["phone"] != "+912200000000" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
	if data["id"] == nil || data["id"] == "" {
		t.Fatalf("missing id: %s", w.Body.String())
	}
}

// A warehouse with no phone made every ShipEngine quote fail with a bare
// "valid from zip code" and no cause (#508). The rule lives here now.
func TestAPI_Warehouses_Create_WithoutPhone_400(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)

	body := warehouseBody("No Phone")
	delete(body, "phone")
	w := request(t, env.router, http.MethodPost, warehousesURL(storeID), body,
		authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAPI_Warehouses_Create_DuplicateLiveName_409(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	createWarehouse(t, env, storeID, tenantID, userID, "Main Warehouse")

	w := request(t, env.router, http.MethodPost, warehousesURL(storeID),
		warehouseBody("Main Warehouse"), authHeaders(userID, tenantID))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "warehouse_name_taken" {
		t.Fatalf("expected warehouse_name_taken, got %s", w.Body.String())
	}
}

// THE test. A rename must move the row, not fork it.
func TestAPI_Warehouses_Rename_EditsTheRowInsteadOfForkingIt(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	id := createWarehouse(t, env, storeID, tenantID, userID, "Main Warehouse")

	body := warehouseBody("Bondi Depot")
	w := request(t, env.router, http.MethodPatch, warehousesURL(storeID)+"/"+id, body,
		authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	data := dataObject(t, w.Body.Bytes())
	if data["id"] != id {
		t.Fatalf("rename created a new row: %v != %v", data["id"], id)
	}
	if data["name"] != "Bondi Depot" {
		t.Fatalf("name = %v", data["name"])
	}

	list := request(t, env.router, http.MethodGet, warehousesURL(storeID), nil,
		authHeaders(userID, tenantID))
	if got := len(dataArray(t, list.Body.Bytes())); got != 1 {
		t.Fatalf("after rename the store has %d warehouses, want 1 — the old row kept the stock", got)
	}
}

func TestAPI_Warehouses_Update_AnotherStoresWarehouse_404(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	id := createWarehouse(t, env, storeID, tenantID, userID, "Mine")

	otherStore, otherTenant := seedStoreRow(t, env.db, "")
	otherUser := uuid.NewString()
	env.fga.Grant(otherUser, authz.RoleOwner, otherTenant)

	w := request(t, env.router, http.MethodPatch, warehousesURL(otherStore)+"/"+id,
		warehouseBody("Stolen"), authHeaders(otherUser, otherTenant))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var name string
	if err := env.db.Raw(`SELECT name FROM warehouses WHERE id = ?`, id).Scan(&name).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if name != "Mine" {
		t.Fatalf("another store's warehouse was renamed to %q", name)
	}
}

func TestAPI_Warehouses_Delete_NoHistory_IsADelete(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	id := createWarehouse(t, env, storeID, tenantID, userID, "Temp")

	w := request(t, env.router, http.MethodDelete, warehousesURL(storeID)+"/"+id, nil,
		authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if outcome := dataObject(t, w.Body.Bytes())["outcome"]; outcome != "deleted" {
		t.Fatalf("outcome = %v, want deleted", outcome)
	}

	var rows int64
	_ = env.db.Raw(`SELECT count(*) FROM warehouses WHERE id = ?`, id).Scan(&rows).Error
	if rows != 0 {
		t.Fatalf("row still present")
	}
}

// The merchant gets ONE verb. A warehouse holding stock cannot be deleted,
// so "remove" archives it — and says how many units it stranded, because
// archiving does not move stock and those units stop being sellable.
func TestAPI_Warehouses_Delete_WithStock_ArchivesAndReportsUnits(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	id := createWarehouse(t, env, storeID, tenantID, userID, "Stocked")
	seedStockAt(t, env.db, storeID, id, 7)

	w := request(t, env.router, http.MethodDelete, warehousesURL(storeID)+"/"+id, nil,
		authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	data := dataObject(t, w.Body.Bytes())
	if data["outcome"] != "archived" || data["reason"] != "holds_stock" {
		t.Fatalf("unexpected outcome: %s", w.Body.String())
	}
	if units, _ := data["units_remaining"].(float64); units != 7 {
		t.Fatalf("units_remaining = %v, want 7", data["units_remaining"])
	}

	// Archived: gone from the default list, still there with the flag.
	list := request(t, env.router, http.MethodGet, warehousesURL(storeID), nil,
		authHeaders(userID, tenantID))
	if got := len(dataArray(t, list.Body.Bytes())); got != 0 {
		t.Fatalf("archived warehouse still listed (%d rows)", got)
	}
	withArchived := request(t, env.router, http.MethodGet,
		warehousesURL(storeID)+"?include_archived=true", nil, authHeaders(userID, tenantID))
	if got := len(dataArray(t, withArchived.Body.Bytes())); got != 1 {
		t.Fatalf("include_archived returned %d rows, want 1", got)
	}
}

func TestAPI_Warehouses_Delete_AnotherStoresWarehouse_404(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	id := createWarehouse(t, env, storeID, tenantID, userID, "Mine")

	otherStore, otherTenant := seedStoreRow(t, env.db, "")
	otherUser := uuid.NewString()
	env.fga.Grant(otherUser, authz.RoleOwner, otherTenant)

	w := request(t, env.router, http.MethodDelete, warehousesURL(otherStore)+"/"+id, nil,
		authHeaders(otherUser, otherTenant))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var rows int64
	_ = env.db.Raw(`SELECT count(*) FROM warehouses WHERE id = ?`, id).Scan(&rows).Error
	if rows != 1 {
		t.Fatalf("another store's warehouse was removed")
	}
}

func TestAPI_Warehouses_Reorder_AppliesTheGivenOrder(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	first := createWarehouse(t, env, storeID, tenantID, userID, "Alpha")
	second := createWarehouse(t, env, storeID, tenantID, userID, "Bravo")

	w := request(t, env.router, http.MethodPut, warehousesURL(storeID)+"/reorder",
		map[string]any{"order": []string{second, first}}, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	rows := dataArray(t, w.Body.Bytes())
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].(map[string]any)["id"] != second {
		t.Fatalf("list is not in the new fill order: %s", w.Body.String())
	}
}

// A delta over a list that changed underneath reorders the wrong rows and
// the caller never finds out. Requiring the complete set makes it a 400.
func TestAPI_Warehouses_Reorder_PartialSet_400(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	first := createWarehouse(t, env, storeID, tenantID, userID, "Alpha")
	createWarehouse(t, env, storeID, tenantID, userID, "Bravo")

	w := request(t, env.router, http.MethodPut, warehousesURL(storeID)+"/reorder",
		map[string]any{"order": []string{first}}, authHeaders(userID, tenantID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestAPI_Warehouses_SetDefault_MovesTheFlag(t *testing.T) {
	env, storeID, tenantID, userID := ownerEnv(t)
	first := createWarehouse(t, env, storeID, tenantID, userID, "Alpha")
	second := createWarehouse(t, env, storeID, tenantID, userID, "Bravo")

	for _, id := range []string{first, second} {
		w := request(t, env.router, http.MethodPut, warehousesURL(storeID)+"/"+id+"/default",
			nil, authHeaders(userID, tenantID))
		if w.Code != http.StatusOK {
			t.Fatalf("set default %s: status = %d, body = %s", id, w.Code, w.Body.String())
		}
	}

	var defaults []string
	if err := env.db.Raw(
		`SELECT id FROM warehouses WHERE store_id = ? AND is_default`, storeID).
		Scan(&defaults).Error; err != nil {
		t.Fatalf("read defaults: %v", err)
	}
	if len(defaults) != 1 || defaults[0] != second {
		t.Fatalf("defaults = %v, want exactly [%s]", defaults, second)
	}
}

// seedStockAt puts qty units of a fresh variant into a warehouse.
func seedStockAt(t *testing.T, db *gorm.DB, storeID, warehouseID string, qty int) {
	t.Helper()
	var tenantID string
	if err := db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID); err != nil {
		t.Fatalf("tenant lookup: %v", err)
	}
	productID, variantID := uuid.NewString(), uuid.NewString()
	if err := db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'WH Product', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "wh-"+uuid.NewString()[:8], uuid.NewString()).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'INR')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error; err != nil {
		t.Fatalf("seed variant: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, ?, now())`, variantID, warehouseID, qty).Error; err != nil {
		t.Fatalf("seed stock: %v", err)
	}
}
