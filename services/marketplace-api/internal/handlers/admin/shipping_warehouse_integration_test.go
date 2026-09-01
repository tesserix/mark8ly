//go:build integration

// Package admin_test — write-path coverage for #177 (the cheap half).
//
// Before this change, a "warehouse" was 8 warehouse_* columns hanging off
// shipping_carrier_configs — a row per (store, carrier). Configuring a
// second carrier for the same store meant retyping the same physical
// address into a second row, with no mechanism keeping the two copies in
// sync. These tests pin the fix at the HTTP boundary: PUT
// /settings/shipping/:provider must now ALSO upsert the store-level
// warehouses row (migration 000095) and point warehouse_id at it. #484 (the
// contract half of #177) stopped writing the legacy warehouse_* columns
// entirely — see TestShippingUpsert_DoesNotWriteLegacyWarehouseColumns
// below. Dropping those columns is separate future work.
package admin_test

import (
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// shippingWarehouseTables is the dependency-ordered truncate list for this
// file's fixtures. supported_countries is deliberately NOT here — it is
// seeded once by migration and shared across the whole suite; this file
// adds its own throw-away row (country code "ZZ", cleaned up per-test)
// rather than truncating a table other packages depend on.
var shippingWarehouseTables = []string{
	"shipping_carrier_configs",
	"warehouses",
	"stores",
}

// shippingWarehouseTestEnv bundles the router, db, and fga fake.
type shippingWarehouseTestEnv struct {
	router *gin.Engine
	db     *gorm.DB
	fga    *authz.FakeClient
}

func setupShippingWarehouseRouter(t *testing.T) *shippingWarehouseTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, shippingWarehouseTables...)

	countryRepo := country.NewRepository(db)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	shippingHandler := admin.NewShippingSettingsHandler(db, countryRepo, crypto.NewNoopEncryptor(), logger)

	fga := authz.NewFakeClient()
	storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{
		Repo:   stores.NewRepository(db),
		Client: stubClient{},
		Flight: &singleflight.Group{},
	})
	authzMW := authz.NewMiddleware(fga, nil)

	r := gin.New()
	admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
		ShippingSettingsHandler: shippingHandler,
		StoresMiddleware:        storeMW,
		AuthzMiddleware:         authzMW,
		InternalSecret:          "",
	})

	return &shippingWarehouseTestEnv{router: r, db: db, fga: fga}
}

// seedShippingCountry inserts a throw-away supported_countries row ("ZZ")
// that supports TWO shipping carriers, so a single test store can
// configure both and exercise the shared-warehouse-row scenario. The real
// seed rows (migration 000090) each list exactly one carrier per country,
// which cannot express "second carrier for the same store" — hence the
// dedicated fixture country instead of mutating real data.
func seedShippingCountry(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO supported_countries
		    (country_code, name, currency_code, region, payment_providers, shipping_carriers, tax_strategy)
		 VALUES ('ZZ', 'Warehouse Test Land', 'USD', 'test', '{}', '{delhivery,couriersplease}', 'flat')
		 ON CONFLICT (country_code) DO NOTHING`).Error)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM supported_countries WHERE country_code = 'ZZ'`)
	})
}

func seedShippingWarehouseStore(t *testing.T, db *gorm.DB) (storeID, tenantID string) {
	t.Helper()
	tenantID = uuid.NewString()
	storeID = uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "wh-" + storeID[:8],
		Name:         "Warehouse Test Store",
		CountryCode:  "ZZ",
		CurrencyCode: "USD",
		Timezone:     "UTC",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	require.NoError(t, db.Create(s).Error)
	return storeID, tenantID
}

func shippingSettingsURL(storeID, provider string) string {
	return "/api/v1/admin/stores/" + storeID + "/settings/shipping/" + provider
}

func warehouseUpsertBody(name string) map[string]any {
	return map[string]any{
		"api_key":           "test-token-1234",
		"mode":              "test",
		"is_active":         true,
		"warehouse_name":    name,
		"warehouse_line1":   "12 Industrial Estate",
		"warehouse_city":    "Mumbai",
		"warehouse_region":  "MH",
		"warehouse_postal":  "400001",
		"warehouse_country": "IN",
		"warehouse_phone":   "+912200000000",
	}
}

type warehouseRow struct {
	ID   string
	Name string
}

func loadWarehousesForStore(t *testing.T, db *gorm.DB, storeID string) []warehouseRow {
	t.Helper()
	var rows []warehouseRow
	require.NoError(t, db.Raw(`SELECT id, name FROM warehouses WHERE store_id = ?`, storeID).Scan(&rows).Error)
	return rows
}

func warehouseIDForConfig(t *testing.T, db *gorm.DB, storeID, provider string) *string {
	t.Helper()
	var id *string
	require.NoError(t, db.Raw(
		`SELECT warehouse_id::text FROM shipping_carrier_configs WHERE store_id = ? AND provider = ?`,
		storeID, provider).Row().Scan(&id))
	return id
}

// TestShippingUpsert_SavingAWarehouseCreatesTheWarehouseRowAndLinksIt is the
// baseline: saving a single carrier config with a warehouse name must
// create exactly one warehouses row for the store and point
// shipping_carrier_configs.warehouse_id at it.
func TestShippingUpsert_SavingAWarehouseCreatesTheWarehouseRowAndLinksIt(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseUpsertBody("Main Warehouse"), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Len(t, rows, 1, "exactly one warehouse row must exist for the store")
	require.Equal(t, "Main Warehouse", rows[0].Name)

	linkedID := warehouseIDForConfig(t, env.db, storeID, "delhivery")
	require.NotNil(t, linkedID)
	require.Equal(t, rows[0].ID, *linkedID)
}

// TestShippingUpsert_SecondCarrierReusesTheSameWarehouseRow is the single
// most important test in this file: it pins the whole point of #177.
// Configuring a SECOND carrier for the same store, with the same warehouse
// name, must NOT create a second warehouses row — both carrier configs
// must point at the one row that already exists.
func TestShippingUpsert_SecondCarrierReusesTheSameWarehouseRow(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	w1 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseUpsertBody("Main Warehouse"), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w1.Code, w1.Body.String())

	w2 := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "couriersplease"),
		warehouseUpsertBody("Main Warehouse"), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Len(t, rows, 1, "the second carrier must reuse the store's warehouse, not create another")

	delhiveryID := warehouseIDForConfig(t, env.db, storeID, "delhivery")
	couriersPleaseID := warehouseIDForConfig(t, env.db, storeID, "couriersplease")
	require.NotNil(t, delhiveryID)
	require.NotNil(t, couriersPleaseID)
	require.Equal(t, *delhiveryID, *couriersPleaseID, "both carrier configs must point at the same warehouse row")
	require.Equal(t, rows[0].ID, *delhiveryID)
}

// TestShippingUpsert_EditingAddressUpdatesTheSharedRowForBothCarriers
// verifies the address is shared, not duplicated: editing it through one
// carrier's config must be visible to the other carrier's config too.
func TestShippingUpsert_EditingAddressUpdatesTheSharedRowForBothCarriers(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	for _, provider := range []string{"delhivery", "couriersplease"} {
		w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, provider),
			warehouseUpsertBody("Main Warehouse"), authHeaders(userID, tenantID))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}

	moved := warehouseUpsertBody("Main Warehouse")
	moved["warehouse_line1"] = "99 New Road"
	moved["warehouse_postal"] = "400002"
	wEdit := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		moved, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, wEdit.Code, wEdit.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Len(t, rows, 1, "editing the address must update the existing row, not create a new one")

	var line1, postal string
	require.NoError(t, env.db.Raw(`SELECT line1, postal_code FROM warehouses WHERE id = ?`, rows[0].ID).
		Row().Scan(&line1, &postal))
	require.Equal(t, "99 New Road", line1)
	require.Equal(t, "400002", postal)

	// The unedited carrier's warehouse_id must still resolve to the same,
	// now-updated row — it never had its own copy to fall out of sync.
	couriersPleaseID := warehouseIDForConfig(t, env.db, storeID, "couriersplease")
	require.NotNil(t, couriersPleaseID)
	require.Equal(t, rows[0].ID, *couriersPleaseID)
}

// TestShippingUpsert_BlankWarehouseNameCreatesNoWarehouseRow guards the
// "was never a usable warehouse" rule from migration 000095's own
// backfill: a config saved with no warehouse name must leave warehouse_id
// NULL and create nothing in the warehouses table.
func TestShippingUpsert_BlankWarehouseNameCreatesNoWarehouseRow(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	body := warehouseUpsertBody("")
	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		body, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Empty(t, rows, "a blank warehouse_name must not create a warehouse row")

	linkedID := warehouseIDForConfig(t, env.db, storeID, "delhivery")
	require.Nil(t, linkedID, "warehouse_id must stay NULL when no warehouse name was given")
}

// The write-path pin that used to live here —
// TestShippingUpsert_DoesNotWriteLegacyWarehouseColumns — asserted that a
// save left the 8 legacy warehouse_* columns at their pre-existing values
// instead of overwriting them with the submitted address. Migration
// 000117 dropped those columns, which turns that property into something
// the schema enforces rather than something a test can observe: its
// fixture seeded the columns it was checking. The replacement guard is
// TestMigration000117_LegacyWarehouseColumnsAreDropped in
// internal/warehouse.

// TestShippingUpsert_ClearingTheWarehouseNameClearsTheLink covers the UPDATE
// path that the create-path blank test above cannot reach: a merchant who
// saved a warehouse and then clears the name.
//
// The legacy warehouse_* columns are a full overwrite, so clearing the name
// has always cleared the pickup address. warehouse_id has to track that. If
// it kept pointing at the old row, the read path — which prefers
// warehouse_id over the columns — would go on shipping from an address the
// merchant deliberately removed.
//
// Only this config's pointer is cleared: the warehouses row survives, so a
// second carrier still referencing it is unaffected.
func TestShippingUpsert_ClearingTheWarehouseNameClearsTheLink(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	// Two carriers share the store's one warehouse.
	for _, provider := range []string{"delhivery", "couriersplease"} {
		w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, provider),
			warehouseUpsertBody("Main Warehouse"), authHeaders(userID, tenantID))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	shared := warehouseIDForConfig(t, env.db, storeID, "couriersplease")
	require.NotNil(t, shared)

	// Now clear it on one of them.
	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseUpsertBody(""), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.Nil(t, warehouseIDForConfig(t, env.db, storeID, "delhivery"),
		"clearing the name must clear the link, or a removed address keeps shipping")

	stillLinked := warehouseIDForConfig(t, env.db, storeID, "couriersplease")
	require.NotNil(t, stillLinked, "the other carrier's link must be untouched")
	require.Equal(t, *shared, *stillLinked)
	require.Len(t, loadWarehousesForStore(t, env.db, storeID), 1,
		"the warehouse row itself must survive — it is still in use")
}

// ─────────────────────────────────────────────────────────────────────────
// #177 PR 5d — the carrier binds to a warehouse BY ID.
//
// The free-text name was the trap: typing a name that did not exactly match
// created a second, stockless warehouse rather than editing the first, and
// the orders allocated to it never shipped. The admin now picks from a list
// and sends the id.
// ─────────────────────────────────────────────────────────────────────────

// seedWarehouseForStore inserts a warehouse directly, the way the
// warehouses page would have.
func seedWarehouseForStore(t *testing.T, db *gorm.DB, tenantID, storeID, name string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region,
		                         postal_code, country_code, phone)
		 VALUES (?, ?, ?, ?, '1 Campbell Parade', 'Bondi Beach', 'NSW', '2026', 'AU', '+61200000000')`,
		id, tenantID, storeID, name).Error)
	return id
}

func warehouseIDUpsertBody(warehouseID string) map[string]any {
	return map[string]any{
		"api_key":      "test-token-1234",
		"mode":         "test",
		"is_active":    true,
		"warehouse_id": warehouseID,
	}
}

func TestShippingUpsert_BindsToAnExistingWarehouseByID(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	whID := seedWarehouseForStore(t, env.db, tenantID, storeID, "Bondi Depot")

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseIDUpsertBody(whID), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	linked := warehouseIDForConfig(t, env.db, storeID, "delhivery")
	require.NotNil(t, linked)
	require.Equal(t, whID, *linked)

	require.Len(t, loadWarehousesForStore(t, env.db, storeID), 1,
		"binding by id must not create a warehouse")
}

// The address belongs to the warehouse now. A carrier save that could
// still rewrite it would put one address behind two forms, which is
// exactly how the two copies drifted apart before #177.
func TestShippingUpsert_BindingByIDDoesNotRewriteTheWarehouse(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	whID := seedWarehouseForStore(t, env.db, tenantID, storeID, "Bondi Depot")

	body := warehouseIDUpsertBody(whID)
	// A stale client sending BOTH must not win with the address half.
	body["warehouse_name"] = "Somewhere Else"
	body["warehouse_line1"] = "999 Wrong Street"
	body["warehouse_city"] = "Nowhere"

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		body, authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var name, line1 string
	require.NoError(t, env.db.Raw(
		`SELECT name, line1 FROM warehouses WHERE id = ?`, whID).Row().Scan(&name, &line1))
	require.Equal(t, "Bondi Depot", name)
	require.Equal(t, "1 Campbell Parade", line1)
	require.Len(t, loadWarehousesForStore(t, env.db, storeID), 1,
		"the name in the body must not have created a second warehouse")
}

func TestShippingUpsert_UnknownWarehouseIDIsRefusedNotA500(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseIDUpsertBody("00000000-0000-0000-0000-0000000000ff"),
		authHeaders(userID, tenantID))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "warehouse_id")
}

// An id is a guessable handle. The picker only ever offers this store's
// warehouses, so a foreign id means a crafted request.
func TestShippingUpsert_AnotherStoresWarehouseIDIsRefused(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	otherStoreID, otherTenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	foreign := seedWarehouseForStore(t, env.db, otherTenantID, otherStoreID, "Theirs")

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseIDUpsertBody(foreign), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// The whole save is refused, so there is no config row at all — assert
	// on the count rather than on warehouse_id, which would just error
	// with "no rows" and read like a different failure.
	var configs int64
	require.NoError(t, env.db.Raw(
		`SELECT count(*) FROM shipping_carrier_configs WHERE store_id = ?`, storeID).
		Scan(&configs).Error)
	require.Zero(t, configs, "a refused save must not have written a carrier config")
}

// Archiving is removal. A carrier bound to an archived warehouse would
// quote from a location the allocator refuses to use (#528).
func TestShippingUpsert_ArchivedWarehouseIDIsRefused(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	whID := seedWarehouseForStore(t, env.db, tenantID, storeID, "Gone")
	require.NoError(t, env.db.Exec(
		`UPDATE warehouses SET archived_at = now() WHERE id = ?`, whID).Error)

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseIDUpsertBody(whID), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// The picker needs to know which warehouse is currently bound.
func TestShippingResponse_CarriesTheBoundWarehouseID(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)
	whID := seedWarehouseForStore(t, env.db, tenantID, storeID, "Bondi Depot")

	save := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseIDUpsertBody(whID), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, save.Code, save.Body.String())
	require.Contains(t, save.Body.String(), whID,
		"the save response must name the bound warehouse")

	list := request(t, env.router, http.MethodGet,
		"/api/v1/admin/stores/"+storeID+"/settings/shipping", nil,
		authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	require.Contains(t, list.Body.String(), `"warehouse_id":"`+whID+`"`)
}

// The legacy name path must keep working for a client that has not been
// updated — this is the expand half of an expand/contract change.
func TestShippingUpsert_LegacyNamePathStillWorksWithoutAnID(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseUpsertBody("Legacy Warehouse"), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	rows := loadWarehousesForStore(t, env.db, storeID)
	require.Len(t, rows, 1)
	require.Equal(t, "Legacy Warehouse", rows[0].Name)
}
