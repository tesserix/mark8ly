//go:build integration

// Package admin_test — write-path coverage for #177 (the cheap half).
//
// Before this change, a "warehouse" was 8 warehouse_* columns hanging off
// shipping_carrier_configs — a row per (store, carrier). Configuring a
// second carrier for the same store meant retyping the same physical
// address into a second row, with no mechanism keeping the two copies in
// sync. These tests pin the fix at the HTTP boundary: PUT
// /settings/shipping/:provider must now ALSO upsert the store-level
// warehouses row (migration 000095) and point warehouse_id at it, while
// continuing to write the legacy columns untouched (the contract half —
// dropping those columns — is separate future work).
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

// TestShippingUpsert_StillWritesLegacyWarehouseColumns guards the
// expand/contract contract: the old warehouse_* columns on
// shipping_carrier_configs must keep being written exactly as before.
// Dropping them is a separate, future contract-half change (#177 is only
// the expand half's write path) and other readers still depend on them.
func TestShippingUpsert_StillWritesLegacyWarehouseColumns(t *testing.T) {
	env := setupShippingWarehouseRouter(t)
	seedShippingCountry(t, env.db)
	storeID, tenantID := seedShippingWarehouseStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	w := request(t, env.router, http.MethodPut, shippingSettingsURL(storeID, "delhivery"),
		warehouseUpsertBody("Main Warehouse"), authHeaders(userID, tenantID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var name, line1, city, region, postal, phone string
	require.NoError(t, env.db.Raw(
		`SELECT warehouse_name, warehouse_line1, warehouse_city, warehouse_region, warehouse_postal, warehouse_phone
		 FROM shipping_carrier_configs WHERE store_id = ? AND provider = 'delhivery'`, storeID).
		Row().Scan(&name, &line1, &city, &region, &postal, &phone))
	require.Equal(t, "Main Warehouse", name)
	require.Equal(t, "12 Industrial Estate", line1)
	require.Equal(t, "Mumbai", city)
	require.Equal(t, "MH", region)
	require.Equal(t, "400001", postal)
	require.Equal(t, "+912200000000", phone)
}

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
