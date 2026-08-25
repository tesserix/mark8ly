//go:build integration

// This file proves the tenant purge handler (#288) against a REAL Postgres
// (see pkg/testdb — TEST_DATABASE_URL gates every test here; unset, they
// SKIP). The unit tests in tenant_purge_test.go proved the handler's call
// ORDER against fakes; a fake can record "I was called third" but cannot
// tell you whether a row a fake never wrote actually survives a real
// DELETE. Three properties specifically need a live database:
//
//  1. tenant isolation — a purge scoped to tenant A must leave tenant B's
//     rows, in the SAME tables, completely untouched. Only a real DELETE
//     with a real WHERE clause can prove this; a fake purger just records
//     "I was asked to purge A" and never touches a row.
//  2. the audit row surviving the very DELETE FROM audit_logs WHERE
//     tenant_id = ? that the purge plan issues for that tenant. A fake
//     emitter has nothing to survive.
//  3. that Count()'s preview numbers are the same numbers Purge() then
//     destroys — both walk the real table contents through the real
//     purgePlan, and only equal real row counts prove that.
//
// # platform-api IS STUBBED HERE
//
// TenantTeardown normally calls platform-api's internal teardown endpoint,
// which owns the tenant row and the confirmation check. platform-api is
// not part of this test binary, so an httptest.Server stands in for it,
// replaying the documented contract (200 with {tenant_id, tenant_name,
// store_ids, store_slugs} on a matching confirmation; 409 with {expected}
// on a mismatch). That means this suite proves marketplace-api's half of
// the flow end-to-end for real — routing, handler ordering, the real local
// DELETE, and real audit persistence — but it proves the CROSS-SERVICE
// contract only as far as this stub is faithful to platform-api's actual
// behavior. A change to platform-api's teardown response shape would not
// be caught here; that boundary is internal/tenantlifecycle's job (see its
// own tests), not this file's.
package platformadmin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// purgeIntegrationTablesToCleanup lists every table this file writes to
// directly or through the handler, so testdb.NewDB's TRUNCATE ... CASCADE
// leaves each test starting clean. "stores" cascades most of the rest via
// FK, but products/orders/audit_logs are listed explicitly since this file
// also inserts into them directly (seedProduct/seedOrder/seedAuditLog).
var purgeIntegrationTablesToCleanup = []string{
	"order_items", "orders", "products", "audit_logs", "stores",
}

// seedProduct inserts one product row for tenantID/storeID and returns its
// id. vendor_id is NOT NULL (migration 000028) but carries no enforced FK
// to vendors(id), so a fresh UUID satisfies the constraint without a
// vendors row — same pattern as internal/tenantpurge's own integration
// fixtures (purge_integration_test.go).
func seedProduct(t *testing.T, db *gorm.DB, tenantID, storeID string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, handle, title, status, vendor_id, tags)
		 VALUES (?, ?, ?, ?, 'Seed Product', 'draft', ?, '{}')`,
		id, tenantID, storeID, "seed-product-"+id[:8], uuid.NewString(),
	).Error)
	return id
}

// seedOrder inserts one order row for tenantID/storeID and returns its id.
func seedOrder(t *testing.T, db *gorm.DB, tenantID, storeID string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key, customer_email, subtotal, grand_total, currency_code)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 100, 100, 'USD')`,
		id, tenantID, storeID, "SEED-"+id[:8], "idem-"+id,
	).Error)
	return id
}

// seedAuditLog inserts one PRE-EXISTING audit_logs row for tenantID/storeID,
// simulating ordinary tenant activity recorded before the purge. This is
// what TestPurge_Integration_AuditRowSurvivesTheDeleteItRecords needs to
// prove got deleted by purgePlan's `DELETE FROM audit_logs WHERE tenant_id
// = ?` — the purge's OWN audit row must be the sole survivor, not simply "a
// row exists".
func seedAuditLog(t *testing.T, db *gorm.DB, tenantID, storeID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, metadata)
		 VALUES (?, ?, ?, 'system', 'product.updated', 'product', 'success', 'info', '{}'::jsonb)`,
		uuid.NewString(), tenantID, storeID,
	).Error)
}

// countByTenant is a small helper so assertions read as VALUES, not
// merely non-zero checks.
func countByTenant(t *testing.T, db *gorm.DB, table, tenantID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM "+table+" WHERE tenant_id = ?", tenantID).Scan(&n).Error)
	return n
}

// teardownFixture is one tenant's canned answer from the platform-api
// teardown stub below.
type teardownFixture struct {
	tenantID   string
	tenantName string
	storeIDs   []string
	storeSlugs []string
}

// newTeardownStub stands in for platform-api's internal teardown endpoint
// (POST /internal/tenants/:id/teardown). It replays the documented
// contract: 200 with the fixture's tenant/store data when the caller's
// store_slugs match the fixture's own slugs (order-insensitive, mirroring
// platform-api's actual confirmation check), 409 with {"expected": [...]}
// otherwise. See this file's top doc comment for what that does and does
// not prove.
func newTeardownStub(t *testing.T, fixtures ...teardownFixture) *httptest.Server {
	t.Helper()
	byID := make(map[string]teardownFixture, len(fixtures))
	for _, f := range fixtures {
		byID[f.tenantID] = f
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path shape: /internal/tenants/{id}/teardown
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 4 || parts[0] != "internal" || parts[1] != "tenants" || parts[3] != "teardown" {
			http.NotFound(w, r)
			return
		}
		id := parts[2]
		f, ok := byID[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var body struct {
			StoreSlugs []string `json:"store_slugs"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		got := append([]string(nil), body.StoreSlugs...)
		want := append([]string(nil), f.storeSlugs...)
		sort.Strings(got)
		sort.Strings(want)

		if len(got) != len(want) || func() bool {
			for i := range got {
				if got[i] != want[i] {
					return true
				}
			}
			return false
		}() {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":    "confirmation_mismatch",
				"expected": f.storeSlugs,
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"tenant_id":   f.tenantID,
				"tenant_name": f.tenantName,
				"store_ids":   f.storeIDs,
				"store_slugs": f.storeSlugs,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newPurgeIntegrationRouter wires the REAL purger (tenantpurge.GormPurger)
// and the REAL synchronous audit emitter (audit.Emitter.EmitSync via
// platformadmin.NewOperatorActionSyncFunc) against db, with td and dir
// supplied by the caller (td is the stub above; dir is a directory fake —
// TenantDirectory is a separate upstream from teardown and is not this
// file's concern to prove against a stub server).
func newPurgeIntegrationRouter(t *testing.T, db *gorm.DB, td platformadmin.TenantTeardown, dir platformadmin.TenantDirectory) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformadmin.RequirePlatformAuth(platformadmin.AuthConfig{
		Secret:     testSecret,
		NonceStore: newMemNonces(),
		Now:        func() time.Time { return fixedNow },
	}))

	emitter := audit.NewEmitter(audit.EmitterConfig{DB: db, Repo: audit.NewRepository(), Logger: slog.Default()})
	t.Cleanup(func() { emitter.Stop(context.Background()) })

	purger := tenantpurge.NewGormPurger(db)
	h := platformadmin.NewTenantPurgeHandler(td, purger, dir, platformadmin.NewOperatorActionSyncFunc(emitter), nil, nil)
	h.Register(r.Group(""))
	return r
}

func TestPurge_Integration_DestroysOneTenantAndLeavesTheOther(t *testing.T) {
	db := testdb.NewDB(t, purgeIntegrationTablesToCleanup...)
	repo := stores.NewRepository(db)

	tenantA := uuid.NewString()
	storeA := seedIntegrationStore(t, repo, tenantA)
	seedProduct(t, db, tenantA, storeA.ID)
	seedProduct(t, db, tenantA, storeA.ID)
	seedOrder(t, db, tenantA, storeA.ID)

	// A SECOND, wholly independent tenant with its OWN store and rows.
	// One tenant cannot prove tenant isolation — the fixture needs two,
	// or there would be no other tenant's row to leak into. This is the
	// exact lesson #286 paid a Critical for (a cross-tenant read from a
	// test that used one store for every call).
	tenantB := uuid.NewString()
	storeB := seedIntegrationStore(t, repo, tenantB)
	seedProduct(t, db, tenantB, storeB.ID)
	seedProduct(t, db, tenantB, storeB.ID)
	seedProduct(t, db, tenantB, storeB.ID)

	stub := newTeardownStub(t, teardownFixture{
		tenantID: tenantA, tenantName: "Tenant A",
		storeIDs: []string{storeA.ID}, storeSlugs: []string{storeA.Slug},
	})
	client := tenantlifecycle.NewClient(stub.URL, "", nil)
	r := newPurgeIntegrationRouter(t, db, client, &stubDirectory{})

	target := "/admin/tenants/" + tenantA + "/purge"
	body := fmt.Sprintf(`{"store_slugs":["%s"],"reason_code":"merchant_request"}`, storeA.Slug)
	req := signedRequest(t, http.MethodPost, target, []byte(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Data struct {
			Tables []struct {
				Table string `json:"table"`
				Rows  int64  `json:"rows"`
			} `json:"tables"`
			TotalRows int64 `json:"total_rows"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	reported := map[string]int64{}
	for _, tr := range resp.Data.Tables {
		reported[tr.Table] = tr.Rows
	}
	// The report's counts equal what was SEEDED, as VALUES, not merely
	// non-zero: 2 products + 1 order + 1 store for tenant A.
	require.EqualValues(t, 2, reported["products"], "purge report must show tenant A's exact product count")
	require.EqualValues(t, 1, reported["orders"], "purge report must show tenant A's exact order count")
	require.EqualValues(t, 1, reported["stores"], "purge report must show tenant A's exact store count")
	require.EqualValues(t, 4, resp.Data.TotalRows, "2 products + 1 order + 1 store")

	// Tenant A is gone.
	require.EqualValues(t, 0, countByTenant(t, db, "products", tenantA))
	require.EqualValues(t, 0, countByTenant(t, db, "orders", tenantA))
	require.EqualValues(t, 0, countByTenant(t, db, "stores", tenantA))

	// Tenant B is COMPLETELY intact — VALUES, not presence.
	require.EqualValues(t, 3, countByTenant(t, db, "products", tenantB),
		"purging tenant A must not touch tenant B's products")
	require.EqualValues(t, 1, countByTenant(t, db, "stores", tenantB),
		"purging tenant A must not touch tenant B's store")
}

func TestPurge_Integration_AuditRowSurvivesTheDeleteItRecords(t *testing.T) {
	db := testdb.NewDB(t, purgeIntegrationTablesToCleanup...)
	repo := stores.NewRepository(db)

	tenantA := uuid.NewString()
	storeA := seedIntegrationStore(t, repo, tenantA)

	// 3 PRE-EXISTING audit rows, recording ordinary activity BEFORE the
	// purge. purgePlan issues `DELETE FROM audit_logs WHERE tenant_id = ?`
	// for this same tenant — these three must be gone, and the handler's
	// own tenant.purged row (written AFTER that delete commits) must be
	// the sole survivor. Asserting only "one row exists" would pass
	// against an emitter that wrote nothing and a purge that never ran;
	// both halves are required together.
	seedAuditLog(t, db, tenantA, storeA.ID)
	seedAuditLog(t, db, tenantA, storeA.ID)
	seedAuditLog(t, db, tenantA, storeA.ID)
	require.EqualValues(t, 3, countByTenant(t, db, "audit_logs", tenantA), "sanity: 3 pre-existing rows seeded")

	stub := newTeardownStub(t, teardownFixture{
		tenantID: tenantA, tenantName: "Tenant A",
		storeIDs: []string{storeA.ID}, storeSlugs: []string{storeA.Slug},
	})
	client := tenantlifecycle.NewClient(stub.URL, "", nil)
	r := newPurgeIntegrationRouter(t, db, client, &stubDirectory{})

	target := "/admin/tenants/" + tenantA + "/purge"
	body := fmt.Sprintf(`{"store_slugs":["%s"],"reason_code":"fraud","reason":"confirmed fraud"}`, storeA.Slug)
	req := signedRequest(t, http.MethodPost, target, []byte(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Half 1: the pre-existing rows are gone, the purge's own row survives,
	// and it is the ONLY row left for this tenant.
	require.EqualValues(t, 1, countByTenant(t, db, "audit_logs", tenantA),
		"exactly one audit_logs row must remain: the 3 pre-existing rows are destroyed by the purge, "+
			"and the purge's own audit row must survive the very DELETE that destroyed them")

	type row struct {
		Action          string          `gorm:"column:action"`
		ActorType       string          `gorm:"column:actor_type"`
		ActorOperatorID *string         `gorm:"column:actor_operator_id"`
		Capability      *string         `gorm:"column:capability"`
		StoreID         *string         `gorm:"column:store_id"`
		Metadata        json.RawMessage `gorm:"column:metadata"`
	}
	var got row
	require.NoError(t, db.Raw(
		`SELECT action, actor_type, actor_operator_id, capability, store_id, metadata FROM audit_logs WHERE tenant_id = ?`,
		tenantA,
	).Scan(&got).Error)

	// Half 2: that surviving row IS the purge record, attributed to the
	// operator, carrying the reason and per-table counts.
	require.Equal(t, "tenant.purged", got.Action)
	require.Equal(t, "operator", got.ActorType)
	require.NotNil(t, got.ActorOperatorID)
	require.Equal(t, "op_7f3a", *got.ActorOperatorID, "operator id from the signed request's X-Platform-Operator header")
	require.NotNil(t, got.Capability)
	require.Equal(t, "audit.read", *got.Capability)
	require.Nil(t, got.StoreID, "a purge is tenant-scoped, not store-scoped — store_id must be NULL")

	var meta map[string]any
	require.NoError(t, json.Unmarshal(got.Metadata, &meta))
	require.Equal(t, "fraud", meta["reason_code"])
	tables, ok := meta["tables"].([]any)
	require.True(t, ok, "metadata.tables must be a populated array of per-table counts")
	require.NotEmpty(t, tables)
}

func TestPurge_Integration_PreviewCountsMatchWhatThePurgeThenDestroys(t *testing.T) {
	db := testdb.NewDB(t, purgeIntegrationTablesToCleanup...)
	repo := stores.NewRepository(db)

	tenantA := uuid.NewString()
	storeA := seedIntegrationStore(t, repo, tenantA)
	seedProduct(t, db, tenantA, storeA.ID)
	seedProduct(t, db, tenantA, storeA.ID)
	seedProduct(t, db, tenantA, storeA.ID)
	seedOrder(t, db, tenantA, storeA.ID)
	seedOrder(t, db, tenantA, storeA.ID)

	dir := &stubDirectory{detail: &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{ID: tenantA, Name: "Tenant A", Status: "active"},
		Stores: []tenantdirectory.StoreSummary{
			{ID: storeA.ID, Slug: storeA.Slug, Name: storeA.Name, Status: storeA.Status},
		},
	}}
	stub := newTeardownStub(t, teardownFixture{
		tenantID: tenantA, tenantName: "Tenant A",
		storeIDs: []string{storeA.ID}, storeSlugs: []string{storeA.Slug},
	})
	client := tenantlifecycle.NewClient(stub.URL, "", nil)
	r := newPurgeIntegrationRouter(t, db, client, dir)

	type tableCount struct {
		Table string `json:"table"`
		Rows  int64  `json:"rows"`
	}

	// GET preview -> report P.
	previewReq := signedRequest(t, http.MethodGet, "/admin/tenants/"+tenantA+"/purge/preview", nil)
	previewRec := httptest.NewRecorder()
	r.ServeHTTP(previewRec, previewReq)
	require.Equal(t, http.StatusOK, previewRec.Code, previewRec.Body.String())

	var previewResp struct {
		Data struct {
			Tables    []tableCount `json:"tables"`
			TotalRows int64        `json:"total_rows"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(previewRec.Body.Bytes(), &previewResp))
	require.EqualValues(t, 6, previewResp.Data.TotalRows, "3 products + 2 orders + 1 store; preview must destroy NOTHING to get this")
	require.EqualValues(t, 3, countByTenant(t, db, "products", tenantA), "preview (Count) must not have deleted anything")

	// POST purge -> report Q.
	purgeBody := fmt.Sprintf(`{"store_slugs":["%s"],"reason_code":"merchant_request"}`, storeA.Slug)
	purgeReq := signedRequest(t, http.MethodPost, "/admin/tenants/"+tenantA+"/purge", []byte(purgeBody))
	purgeRec := httptest.NewRecorder()
	r.ServeHTTP(purgeRec, purgeReq)
	require.Equal(t, http.StatusOK, purgeRec.Code, purgeRec.Body.String())

	var purgeResp struct {
		Data struct {
			Tables    []tableCount `json:"tables"`
			TotalRows int64        `json:"total_rows"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(purgeRec.Body.Bytes(), &purgeResp))

	// The number an operator read before the irreversible action is the
	// number that action then destroyed — both TotalRows and every
	// per-table count.
	require.Equal(t, previewResp.Data.TotalRows, purgeResp.Data.TotalRows,
		"P.TotalRows must equal Q.TotalRows")
	require.Equal(t, previewResp.Data.Tables, purgeResp.Data.Tables,
		"every per-table count in the preview must equal what the purge then destroyed")
	require.EqualValues(t, 0, countByTenant(t, db, "products", tenantA), "the purge must actually have destroyed what it previewed")
}

func TestPurge_Integration_MismatchDestroysNothing(t *testing.T) {
	db := testdb.NewDB(t, purgeIntegrationTablesToCleanup...)
	repo := stores.NewRepository(db)

	tenantA := uuid.NewString()
	storeA := seedIntegrationStore(t, repo, tenantA)
	seedProduct(t, db, tenantA, storeA.ID)
	seedOrder(t, db, tenantA, storeA.ID)

	// Tenant B exists only to supply a REAL, distinct slug that is
	// nonetheless wrong for tenant A.
	tenantB := uuid.NewString()
	storeB := seedIntegrationStore(t, repo, tenantB)

	stub := newTeardownStub(t, teardownFixture{
		tenantID: tenantA, tenantName: "Tenant A",
		storeIDs: []string{storeA.ID}, storeSlugs: []string{storeA.Slug},
	})
	client := tenantlifecycle.NewClient(stub.URL, "", nil)
	r := newPurgeIntegrationRouter(t, db, client, &stubDirectory{})

	// Purge tenant A, but confirm with tenant B's slug.
	target := "/admin/tenants/" + tenantA + "/purge"
	body := fmt.Sprintf(`{"store_slugs":["%s"],"reason_code":"merchant_request"}`, storeB.Slug)
	req := signedRequest(t, http.MethodPost, target, []byte(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"error":"confirmation_mismatch"`)

	// A's rows are UNCHANGED — the mismatch must destroy nothing.
	require.EqualValues(t, 1, countByTenant(t, db, "products", tenantA))
	require.EqualValues(t, 1, countByTenant(t, db, "orders", tenantA))
	require.EqualValues(t, 1, countByTenant(t, db, "stores", tenantA))
	// B is untouched too — it was never the purge target.
	require.EqualValues(t, 1, countByTenant(t, db, "stores", tenantB))
}

// TestPurge_Integration_OutboxBackstopReRunDoesNotDeleteTheAuditRow composes
// the TWO purges that actually happen in production, which no other test
// constructs.
//
// platform-api's teardown enqueues a `tenant.deleted` outbox event INSIDE
// the teardown transaction — the designed durability backstop. Its drainer
// polls every second and calls marketplace-api's internal purge endpoint,
// which runs tenantpurge.Purge a SECOND time with the same (tenantID,
// storeIDs). Before the actor_type predicate, that second run executed
// `DELETE FROM audit_logs WHERE tenant_id = ?` and destroyed the
// `tenant.purged` row the governed handler had written after its own inline
// purge: an irreversible destruction, reported as audited, recorded nowhere.
//
// So: run the handler end to end, assert the row exists, then invoke Purge
// DIRECTLY with the drainer's arguments and assert the row is STILL there —
// while the tenant's ordinary (non-operator) audit rows stay destroyed, so
// the predicate cannot pass by excluding everything.
func TestPurge_Integration_OutboxBackstopReRunDoesNotDeleteTheAuditRow(t *testing.T) {
	db := testdb.NewDB(t, purgeIntegrationTablesToCleanup...)
	repo := stores.NewRepository(db)

	tenantA := uuid.NewString()
	storeA := seedIntegrationStore(t, repo, tenantA)

	// Two ORDINARY (actor_type = 'system') rows. These are tenant data and
	// MUST be destroyed — without them a predicate that excluded every row
	// would look identical to the correct one.
	seedAuditLog(t, db, tenantA, storeA.ID)
	seedAuditLog(t, db, tenantA, storeA.ID)
	require.EqualValues(t, 2, countByTenant(t, db, "audit_logs", tenantA), "sanity: 2 pre-existing non-operator rows")

	stub := newTeardownStub(t, teardownFixture{
		tenantID: tenantA, tenantName: "Tenant A",
		storeIDs: []string{storeA.ID}, storeSlugs: []string{storeA.Slug},
	})
	client := tenantlifecycle.NewClient(stub.URL, "", nil)
	r := newPurgeIntegrationRouter(t, db, client, &stubDirectory{})

	target := "/admin/tenants/" + tenantA + "/purge"
	body := fmt.Sprintf(`{"store_slugs":["%s"],"reason_code":"erasure_request"}`, storeA.Slug)
	req := signedRequest(t, http.MethodPost, target, []byte(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// After the governed handler: the two ordinary rows are gone, the
	// tenant.purged row is the sole survivor.
	require.EqualValues(t, 1, countByTenant(t, db, "audit_logs", tenantA),
		"after the inline purge exactly the tenant.purged row must remain")
	require.EqualValues(t, 0, countOperatorAudit(t, db, tenantA, "!="),
		"every non-operator row for this tenant must already be destroyed")
	require.EqualValues(t, 1, countOperatorAudit(t, db, tenantA, "="),
		"the surviving row must be the operator governance row")

	// NOW the backstop. These are exactly the arguments platform-api's
	// outbox drainer passes: the tenant id and the store ids captured in
	// the tenant.deleted payload before the tenant row was deleted.
	rep, err := tenantpurge.Purge(context.Background(), db, tenantA, []string{storeA.ID})
	require.NoError(t, err, "the backstop purge must succeed; it is idempotent by design")

	var auditRows int64 = -1
	for _, tr := range rep.Tables {
		if tr.Table == "audit_logs" {
			auditRows = tr.RowsDeleted
		}
	}
	require.EqualValues(t, 0, auditRows,
		"the backstop must delete ZERO audit_logs rows: the tenant's own rows are already gone "+
			"and the operator governance row is excluded from the plan")

	// The whole point: the record of the destruction survives the backstop
	// that was supposed to be backing that destruction up.
	require.EqualValues(t, 1, countByTenant(t, db, "audit_logs", tenantA),
		"the tenant.purged row must survive the outbox drainer's second purge")

	var got struct {
		Action    string `gorm:"column:action"`
		ActorType string `gorm:"column:actor_type"`
	}
	require.NoError(t, db.Raw(
		`SELECT action, actor_type FROM audit_logs WHERE tenant_id = ?`, tenantA,
	).Scan(&got).Error)
	require.Equal(t, "tenant.purged", got.Action)
	require.Equal(t, "operator", got.ActorType)
}

// countOperatorAudit counts this tenant's audit_logs rows whose actor_type
// compares to 'operator' with op ("=" or "!="). Split out so the two halves
// of the property — operator rows kept, non-operator rows destroyed — read
// as two distinct assertions rather than one aggregate that either could
// satisfy alone.
func countOperatorAudit(t *testing.T, db *gorm.DB, tenantID, op string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM audit_logs WHERE tenant_id = ? AND actor_type "+op+" 'operator'",
		tenantID,
	).Scan(&n).Error)
	return n
}
