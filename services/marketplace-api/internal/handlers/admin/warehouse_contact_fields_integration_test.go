//go:build integration

// Package admin_test — write/read-path coverage for #483.
//
// #177 added warehouses.contact_person/email (migration 000095) and taught
// warehouse.Repository.Upsert to write them, and the read paths
// (resolveWarehouseForSync in settings.go, resolvePickupAddress in
// shipments.go) already forwarded them to Delhivery — but nothing in the
// admin shipping settings HTTP handler ever collected these two fields from
// the merchant: shippingUpsertRequest had no contact_person/email field, so
// every save sent them blank, silently wiping any value a merchant might
// have set another way, and the settings GET/PUT response never returned
// them either. This file pins the fix at the HTTP boundary: PUT
// /settings/shipping/:provider now collects, persists, and round-trips
// warehouse_contact_person/warehouse_email, and a save that omits them does
// NOT clear a previously-saved value.
package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/warehouse"
)

// loadWarehouseContact reads contact_person/email directly off the
// warehouses row, bypassing the HTTP layer, so these tests assert against
// what actually landed in the database rather than trusting the handler's
// own response echo.
func loadWarehouseContact(t *testing.T, db *gorm.DB, warehouseID string) (contactPerson, email string) {
	t.Helper()
	require.NoError(t, db.Raw(
		`SELECT COALESCE(contact_person, ''), COALESCE(email, '') FROM warehouses WHERE id = ?`,
		warehouseID).Row().Scan(&contactPerson, &email))
	return contactPerson, email
}

// shippingUpsertResponseData is the subset of the PUT response body this
// file cares about — just enough to decode data.warehouse_contact_person
// and data.warehouse_email for the round-trip assertions.
type shippingUpsertResponseData struct {
	Data struct {
		WarehouseContactPerson string `json:"warehouse_contact_person"`
		WarehouseEmail         string `json:"warehouse_email"`
	} `json:"data"`
}

// TestShippingUpsert_SavingContactPersonAndEmailPersistsThemOnTheWarehouseRow
// is the baseline for #483: a save that supplies warehouse_contact_person
// and warehouse_email must actually reach the warehouses row, not just be
// accepted and dropped (the bug this issue reports).
func TestShippingUpsert_SavingContactPersonAndEmailPersistsThemOnTheWarehouseRow(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	body := warehouseUpsertBody("Main Warehouse")
	body["warehouse_contact_person"] = "Priya Sharma"
	body["warehouse_email"] = "priya@example.com"

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		body, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Len(t, rows, 1)

	contactPerson, email := loadWarehouseContact(t, env.db, rows[0].ID)
	require.Equal(t, "Priya Sharma", contactPerson)
	require.Equal(t, "priya@example.com", email)
}

// TestShippingUpsert_ContactPersonAndEmailRoundTripOnTheResponse asserts the
// PUT response itself (which calls toShippingResponse, same as GET /settings
// would) reflects what was just saved. Asserting off the PUT's own response
// body is simpler than issuing a second GET request and exercises the exact
// same toShippingResponse code path this task changed.
func TestShippingUpsert_ContactPersonAndEmailRoundTripOnTheResponse(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	body := warehouseUpsertBody("Main Warehouse")
	body["warehouse_contact_person"] = "Priya Sharma"
	body["warehouse_email"] = "priya@example.com"

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		body, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp shippingUpsertResponseData
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "Priya Sharma", resp.Data.WarehouseContactPerson)
	require.Equal(t, "priya@example.com", resp.Data.WarehouseEmail)
}

// TestShippingUpsert_OmittingContactPersonAndEmailDoesNotWipeExistingValues
// is the regression #483 calls out by name: warehouse.Repository.Upsert's
// ON CONFLICT clause unconditionally overwrites contact_person/email, so a
// merchant resaving the form WITHOUT touching these two fields (e.g. only
// changing is_active, or re-entering an API key) must not blank out values
// saved on an earlier PUT. blankPreservesExistingWarehouseField in
// settings.go is the fix: a blank/omitted submission falls back to the
// warehouse's currently-stored value rather than overwriting it with "".
func TestShippingUpsert_OmittingContactPersonAndEmailDoesNotWipeExistingValues(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	seeded := warehouseUpsertBody("Main Warehouse")
	seeded["warehouse_contact_person"] = "Priya Sharma"
	seeded["warehouse_email"] = "priya@example.com"
	w1 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		seeded, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w1.Code, w1.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Len(t, rows, 1)

	// Second save omits both keys entirely — not an explicit "", just
	// absent from the JSON body, though Go's json.Unmarshal treats a
	// missing key and an explicit "" identically for a plain string field,
	// so this also covers the empty-string case.
	resave := warehouseUpsertBody("Main Warehouse")
	resave["is_active"] = false
	w2 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		resave, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	contactPerson, email := loadWarehouseContact(t, env.db, rows[0].ID)
	require.Equal(t, "Priya Sharma", contactPerson, "a blank resave must not wipe the previously-saved contact person")
	require.Equal(t, "priya@example.com", email, "a blank resave must not wipe the previously-saved email")
}

// TestShippingUpsert_ExistingWarehouseWithNullContactColumnsStillSavesFine
// covers migration 000095's own backfill state: every pre-#483 warehouses
// row has contact_person/email as NULL (the backfill never set them). A
// save that resolves to this warehouse and still doesn't touch these two
// fields must succeed (200, not an error) and must NOT populate them with
// garbage — there's nothing to preserve on a row that never had a value,
// so the fields stay blank, matching blankPreservesExistingWarehouseField's
// documented behavior for "nothing to fall back to".
func TestShippingUpsert_ExistingWarehouseWithNullContactColumnsStillSavesFine(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	// Seed a warehouse row directly, simulating the 000095 backfill state:
	// ContactPerson/Email left at their Go zero value (never set).
	whRepo := warehouse.NewRepository()
	wh, err := whRepo.Upsert(t.Context(), env.db, warehouse.Warehouse{
		TenantID:    tenantID,
		StoreID:     storeID,
		Name:        "Main Warehouse",
		Line1:       "12 Industrial Estate",
		City:        "Mumbai",
		Region:      "MH",
		PostalCode:  "400001",
		CountryCode: "IN",
		Phone:       "+912200000000",
	})
	require.NoError(t, err)

	// Link a carrier config to it the same way the handler's own Upsert
	// would, so the PUT below resolves existing.WarehouseID and takes the
	// "look up the prior row" branch this test is targeting.
	body := warehouseUpsertBody("Main Warehouse")
	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		body, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	contactPerson, email := loadWarehouseContact(t, env.db, wh.ID)
	require.Empty(t, contactPerson, "a NULL-backfilled row with no submitted value must stay blank, not error or fabricate a value")
	require.Empty(t, email, "a NULL-backfilled row with no submitted value must stay blank, not error or fabricate a value")
}
