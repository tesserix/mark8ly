//go:build integration

package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// seedProductViaService inserts a product with two variants and returns
// (productID, variant1ID, variant2ID).
//
// Note: this must supply an explicit VendorID (products.vendor_id has
// been NOT NULL since migration 000028, and this helper's Config carries
// no VendorLookup to default it) and a matching Options/OptionValues
// matrix (ValidateMatrix rejects >1 variant when no option axis is
// given). Without both, every Create through this helper fails before
// the test under it gets to run.
func seedProductViaService(t *testing.T, env *testEnv, storeID, tenantID string) (string, string, string) {
	t.Helper()
	svc := product.NewService(product.Config{
		DB:         env.db,
		Repo:       product.NewRepository(env.db),
		StoresRepo: stores.NewRepository(env.db),
		OutboxRepo: outbox.NewRepository(env.db),
		Uploader:   env.uploader,
	})
	vendorID := uuid.NewString()
	agg, err := svc.Create(context.Background(), product.CreateRequest{
		StoreID:  storeID,
		TenantID: tenantID,
		Title:    "V Product " + uuid.NewString()[:6],
		VendorID: &vendorID,
		Options: []product.OptionSpec{
			{Name: "Size", Values: []product.OptionValueSpec{{Value: "S"}, {Value: "L"}}},
		},
		Variants: []product.VariantInput{
			{SKU: "V1-" + uuid.NewString()[:6], Price: decimal.NewFromInt(10), CurrencyCode: "USD",
				OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}}},
			{SKU: "V2-" + uuid.NewString()[:6], Price: decimal.NewFromInt(20), CurrencyCode: "USD",
				OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "L"}}},
		},
	})
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return agg.Product.ID, agg.Variants[0].ID, agg.Variants[1].ID
}

func variantURL(storeID, productID, variantID string) string {
	return "/api/v1/admin/stores/" + storeID + "/products/" + productID + "/variants/" + variantID
}

func TestAPI_AdminVariants_Patch_Price_200(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, v1, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, pid, v1),
		map[string]any{"price": "55.50"}, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["price"] != "55.5" && resp["price"] != "55.50" {
		t.Fatalf("price = %v", resp["price"])
	}
}

func TestAPI_AdminVariants_Patch_Inventory_TriggerSyncs_200(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, v1, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, pid, v1),
		map[string]any{"inventory_quantity": 77}, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}

	// Re-fetch full product and verify inventory synced.
	w2 := request(t, env.router, http.MethodGet, productsURL(storeID)+"/"+pid, nil, authHeaders(userID, tenantID))
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	variants, _ := resp["variants"].([]any)
	for _, v := range variants {
		m := v.(map[string]any)
		if m["id"] == v1 {
			if int(m["inventory_quantity"].(float64)) != 77 {
				t.Fatalf("inventory = %v", m["inventory_quantity"])
			}
			return
		}
	}
	t.Fatalf("variant %s not found in response", v1)
}

func TestAPI_AdminVariants_Patch_SKUCollision_409(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, v1, v2 := seedProductViaService(t, env, storeID, tenantID)

	// Get v1's sku.
	w := request(t, env.router, http.MethodGet, productsURL(storeID)+"/"+pid, nil, authHeaders(userID, tenantID))
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	variants, _ := resp["variants"].([]any)
	var v1sku string
	for _, v := range variants {
		m := v.(map[string]any)
		if m["id"] == v1 {
			v1sku, _ = m["sku"].(string)
		}
	}
	if v1sku == "" {
		t.Fatal("could not find v1 sku")
	}

	// Try to set v2 to v1's sku.
	w2 := request(t, env.router, http.MethodPatch, variantURL(storeID, pid, v2),
		map[string]any{"sku": v1sku}, authHeaders(userID, tenantID))
	if w2.Code != http.StatusConflict {
		t.Fatalf("status = %d, %s", w2.Code, w2.Body.String())
	}
	var er map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &er)
	if er["error"] != "sku_taken" {
		t.Fatalf("error = %v", er["error"])
	}
}

func TestAPI_AdminVariants_Patch_CurrencyChange_409(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, v1, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, pid, v1),
		map[string]any{"currency_code": "EUR"}, authHeaders(userID, tenantID))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "currency_change_forbidden" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestAPI_AdminVariants_Patch_NotFound_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	pid, _, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, pid, uuid.NewString()),
		map[string]any{"price": "1.00"}, authHeaders(userID, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
}

func TestAPI_AdminVariants_Patch_StaffDenied_404(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	adminUser := uuid.NewString()
	staffUser := uuid.NewString()
	env.fga.Grant(adminUser, authz.RoleAdmin, tenantID)
	env.fga.Grant(staffUser, authz.RoleStaff, tenantID)

	pid, v1, _ := seedProductViaService(t, env, storeID, tenantID)

	w := request(t, env.router, http.MethodPatch, variantURL(storeID, pid, v1),
		map[string]any{"price": "1.00"}, authHeaders(staffUser, tenantID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}
}
