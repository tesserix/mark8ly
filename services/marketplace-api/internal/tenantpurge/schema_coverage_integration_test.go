//go:build integration

// This file is in the INTERNAL package (unlike marketplace-api's other
// integration tests, which are external `_test` packages) because it
// asserts against purgePlan, which is unexported by design — the plan is
// an implementation detail everywhere except here.
package tenantpurge

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestPurgePlan_CoversEveryTenantScopedTable fails when a tenant-scoped
// table exists that the plan neither deletes explicitly, nor reaches by ON
// DELETE CASCADE from a table it does delete, nor names as a deliberate
// exclusion.
//
// It reads the live schema and the live FK graph rather than the
// migrations, because the migrations are what the plan was originally
// derived from and re-reading them would only re-derive the same answer.
func TestPurgePlan_CoversEveryTenantScopedTable(t *testing.T) {
	db := testdb.NewDB(t)
	require.Empty(t, uncoveredTenantScopedTables(t, db),
		"tenant-scoped tables neither purged, nor cascaded, nor declared an exclusion.\n"+
			"A tenant purge would leave these rows behind. Either add a step to purgePlan or add the "+
			"table to declaredExclusions WITH a justification in purge.go's package doc.")
}

// uncoveredTenantScopedTables is the guard's computation, extracted so the
// test above and the probe test below run the SAME code — one asserting it
// finds nothing, the other asserting it finds a table planted for it.
func uncoveredTenantScopedTables(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	type fk struct {
		Child       string
		Parent      string
		Confdeltype string
	}
	var fks []fk
	require.NoError(t, db.Raw(`
		SELECT conrelid::regclass::text  AS child,
		       confrelid::regclass::text AS parent,
		       confdeltype
		FROM pg_constraint
		WHERE contype = 'f' AND connamespace = 'public'::regnamespace`).Scan(&fks).Error)

	type tbl struct {
		TableName string
		HasTenant bool
		HasStore  bool
	}
	var tables []tbl
	require.NoError(t, db.Raw(`
		SELECT t.table_name,
		       COALESCE(bool_or(c.column_name = 'tenant_id'), false) AS has_tenant,
		       COALESCE(bool_or(c.column_name = 'store_id'),  false) AS has_store
		FROM information_schema.tables t
		LEFT JOIN information_schema.columns c
		  ON c.table_schema = t.table_schema AND c.table_name = t.table_name
		WHERE t.table_schema = 'public' AND t.table_type = 'BASE TABLE'
		GROUP BY t.table_name`).Scan(&tables).Error)

	deleted := map[string]bool{}
	for _, s := range purgePlan("11111111-1111-1111-1111-111111111111", []string{"22222222-2222-2222-2222-222222222222"}) {
		deleted[s.table] = true
	}
	require.NotEmpty(t, deleted, "purgePlan returned no steps — the guard would vacuously pass")

	// Cascade closure: a child of a deleted parent, via ON DELETE CASCADE,
	// is itself deleted. Iterate to a fixed point — the graph has chains
	// (products -> product_variants -> variant_stock).
	for changed := true; changed; {
		changed = false
		for _, e := range fks {
			if e.Confdeltype == "c" && deleted[e.Parent] && !deleted[e.Child] && e.Child != e.Parent {
				deleted[e.Child] = true
				changed = true
			}
		}
	}

	// Tables that carry a tenant_id or store_id and are deliberately NOT
	// purged. Each is justified in purge.go's package doc. Adding to this
	// list is a DECISION about a tenant's data surviving its own deletion,
	// and this test exists to make that decision explicit rather than
	// accidental.
	declaredExclusions := map[string]bool{
		"business_entity_attestations":   true, // KYB attestation log, must outlive the tenant
		"app_contract_attestations":      true, // Apple 4.2.6 attestation log
		"subscription_plan_change_audit": true, // append-only billing-change trail
		"break_glass_lockouts":           true, // owned by postgres; DELETE aborts the whole tx
	}

	uncovered := make([]string, 0, 4)
	for _, tb := range tables {
		if !tb.HasTenant && !tb.HasStore {
			continue // global reference data owns no tenant's rows
		}
		if deleted[tb.TableName] || declaredExclusions[tb.TableName] {
			continue
		}
		uncovered = append(uncovered, tb.TableName)
	}
	sort.Strings(uncovered)
	return uncovered
}

// The guard is only worth having if it can fail. This runs the SAME
// computation against a deliberately-uncovered table and asserts it is
// reported.
//
// Asserting instead that information_schema can see the probe table would
// be a test of information_schema, not of the guard: a coverage function
// that returned "everything is covered" unconditionally would pass it.
func TestPurgePlan_CoverageGuardDetectsAnUncoveredTable(t *testing.T) {
	db := testdb.NewDB(t)

	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS tenantpurge_guard_probe (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL)`).Error)
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS tenantpurge_guard_probe`).Error })

	require.Contains(t, uncoveredTenantScopedTables(t, db), "tenantpurge_guard_probe",
		"a tenant-scoped table that the plan neither deletes nor cascades to nor excludes must be reported")
}
