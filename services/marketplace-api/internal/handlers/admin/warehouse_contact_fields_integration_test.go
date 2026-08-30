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
// row has contact_person/email as NULL (the backfill never set them), and
// the UPDATE path in Upsert only looks up a prior warehouse row to fall
// back to (blankPreservesExistingWarehouseField) when !isCreate &&
// existing.WarehouseID != nil. A single PUT never reaches that branch — the
// first save for a (store, provider) is always a create — so this test does
// TWO PUTs for the same store/provider, neither touching contact/email:
// the first creates the config and links a fresh (NULL-contact) warehouse
// row, the second is the real UPDATE that exercises the "look up the prior
// row" branch. It must succeed (200, not an error) and must NOT populate
// the fields with garbage — there's nothing to preserve on a row that never
// had a value, so they stay blank, matching
// blankPreservesExistingWarehouseField's documented behavior for "nothing
// to fall back to".
func TestShippingUpsert_ExistingWarehouseWithNullContactColumnsStillSavesFine(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	// First PUT: create path. No contact/email in the body, so the linked
	// warehouse row lands with NULL contact_person/email — the 000095
	// backfill state this test targets.
	body := warehouseUpsertBody("Main Warehouse")
	w1 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		body, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w1.Code, w1.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Len(t, rows, 1)
	contactPerson, email := loadWarehouseContact(t, env.db, rows[0].ID)
	require.Empty(t, contactPerson, "the freshly-created warehouse must have no contact person")
	require.Empty(t, email, "the freshly-created warehouse must have no email")

	// Second PUT: same store, same provider, still no contact/email. This
	// is the UPDATE — existing.WarehouseID is now set from the first PUT —
	// so it's the one that actually exercises the "look up the prior row"
	// branch in Upsert.
	w2 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		body, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	contactPerson, email = loadWarehouseContact(t, env.db, rows[0].ID)
	require.Empty(t, contactPerson, "a NULL-backfilled row with no submitted value must stay blank, not error or fabricate a value")
	require.Empty(t, email, "a NULL-backfilled row with no submitted value must stay blank, not error or fabricate a value")
}

// TestShippingUpsert_ClearingThenRestoringTheWarehouseNamePreservesContactFields
// pins the real-world trigger for the bug the by-ID lookup had: clearing a
// config's warehouse_name (existing, documented pre-#483 behavior) sets
// cfg.WarehouseID back to nil, even though the underlying warehouses row is
// untouched and still holds its contact_person/email. A merchant who then
// re-enters the SAME warehouse name is doing an UPDATE (isCreate is false),
// but a lookup keyed on existing.WarehouseID would find nothing — that
// pointer is still nil from the clear — so it would resolve "nothing to
// preserve" and blankPreservesExistingWarehouseField would return "", which
// warehouseRepo.Upsert's ON CONFLICT DO UPDATE on (store_id, name) would
// then write straight over the still-existing row, wiping its contact
// fields. Resolving the prior row by (store_id, name) instead — the same
// key Upsert conflicts on — finds it correctly across this clear/restore
// cycle. This is also reachable within a single store once two carriers
// share one warehouse row (see internal/warehouse's own package doc), not
// just via this clear-then-restore sequence.
func TestShippingUpsert_ClearingThenRestoringTheWarehouseNamePreservesContactFields(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	// 1. Save the warehouse with a contact person.
	withContact := warehouseUpsertBody("Main Warehouse")
	withContact["warehouse_contact_person"] = "Priya Sharma"
	withContact["warehouse_email"] = "priya@example.com"
	w1 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		withContact, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w1.Code, w1.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Len(t, rows, 1)
	warehouseID := rows[0].ID

	// 2. Clear the warehouse name. warehouse_id must go NULL on the config,
	// but the warehouses row itself (and its contact fields) survives.
	w2 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseUpsertBody(""), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	require.Nil(t, warehouseIDForConfig(t, env.db, storeID, "delhivery"),
		"clearing the name must clear the config's warehouse_id link")

	// 3. Re-enter the SAME warehouse name, still omitting contact/email.
	// isCreate is false here, but the config's warehouse_id has been nil
	// since step 2 — a by-ID lookup would miss entirely. The fix must
	// still find the row by (store_id, name) and preserve its fields.
	w3 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseUpsertBody("Main Warehouse"), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w3.Code, w3.Body.String())

	require.Len(t, loadWarehousesForStore(t, env.db, storeID), 1,
		"re-entering the same name must reuse the existing warehouse row, not create a second one")
	contactPerson, email := loadWarehouseContact(t, env.db, warehouseID)
	require.Equal(t, "Priya Sharma", contactPerson,
		"restoring the same warehouse name must not wipe the contact person saved before it was cleared")
	require.Equal(t, "priya@example.com", email,
		"restoring the same warehouse name must not wipe the email saved before it was cleared")
}
