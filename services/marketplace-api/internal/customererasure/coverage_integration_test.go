//go:build integration

// This file is in the INTERNAL package (unlike marketplace-api's other
// integration tests, which are external `_test` packages) because it
// asserts against erasurePlan, which is unexported by design — the plan is
// an implementation detail everywhere except here.
package customererasure

import (
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// customerLinkColumns are the column names by which a row names a customer.
// A table carrying any of them holds data attributable to one person and
// must therefore have a declared disposition.
//
// It reads the LIVE schema rather than the migrations, because the
// migrations are what the plan was derived from and re-reading them would
// only re-derive the same answer.
var customerLinkColumns = []string{
	"customer_id",
	"customer_email",
	"customer_profile_id",
	"recipient_user_id",
	"author_email",
	"submitted_by_email",
	"actor_email",
	"recipient",
	"sender_email",
	"recipient_email",
	"purchased_by_email",
	"email",
}

// declaredExclusions are tables that carry a customer-link column but are
// deliberately NOT erased. Each value is the justification, and it is a
// DECISION about a person's data surviving their own erasure request — the
// test below exists to make that decision explicit rather than accidental.
//
// This map is the only way to silence the guard, so it is deliberately
// expensive to abuse: an empty justification fails the test, and so does an
// entry the guard would never have flagged in the first place. It cannot
// quietly accumulate dead weight.
var declaredExclusions = map[string]string{
	"user_profiles":             "GIP merchant/staff uid — a store's operators, not its customers",
	"store_subscriptions":       "merchant billing contact, not a customer",
	"warehouses":                "merchant facility contact, not a customer",
	"tenant_sso_user_mappings":  "merchant SSO identity, not a customer",
	"promo_redemptions":         "subscription promo-code redemption (§7). Its `email` is the MERCHANT's billing address — promo.Service writes normaliseEmail(in.MerchantEmail) (internal/promo/service.go:156) and the row references a subscription_id, not an order. Anonymising it during a customer erasure would rewrite a merchant's own billing record",
	"customer_erasure_requests": "the request itself — its own status and receipt are the evidence the erasure happened, and destroying it would destroy the proof",
	"journal_subscribers":       "mark8ly.com marketing-list signup (#153), not a merchant's customer. The table carries no store_id and no tenant_id by design — a subscriber belongs to the platform, not to any store — so a store-scoped erasure plan has no key to reach it by, and erasing on a customer's request would delete a platform subscription that customer may never have made. NOT a statement that these addresses are unerasable: they are personal data and still carry an art.17 right, exercised against the platform rather than through a merchant's store-scoped flow",
}

// TestErasurePlan_CoversEveryCustomerLinkedTable fails when a table exists
// that names a customer and that the plan neither erases nor declares as a
// deliberate exclusion. A table added next year cannot silently escape
// erasure.
func TestErasurePlan_CoversEveryCustomerLinkedTable(t *testing.T) {
	db := testdb.NewDB(t)
	require.Empty(t, uncoveredCustomerLinkedTables(t, db),
		"tables naming a customer that the erasure plan neither deletes nor anonymises nor declares an exclusion.\n"+
			"A GDPR art.17 erasure would leave this person's data behind. Either add a step to erasurePlan or add the "+
			"table to declaredExclusions WITH a justification.")
}

// TestDeclaredExclusions_AreJustifiedAndLive keeps the escape hatch honest.
// An exclusion with no reason is a silenced guard; an exclusion for a table
// the guard would never flag is dead weight that makes the map look more
// considered than it is.
func TestDeclaredExclusions_AreJustifiedAndLive(t *testing.T) {
	db := testdb.NewDB(t)
	linked := customerLinkedTables(t, db)
	planned := plannedTables()

	for table, justification := range declaredExclusions {
		require.NotEmpty(t, justification,
			"exclusion %q has no justification — excluding a table from GDPR erasure requires a written reason", table)
		require.Contains(t, linked, table,
			"exclusion %q names no customer-link column, so the guard would never flag it; remove the entry", table)
		require.NotContains(t, planned, table,
			"exclusion %q is also covered by the plan; an entry that excludes nothing is misleading", table)
	}
}

// uncoveredCustomerLinkedTables is the guard's computation, extracted so the
// test above and the probe test below run the SAME code — one asserting it
// finds nothing, the other asserting it finds a table planted for it.
func uncoveredCustomerLinkedTables(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	planned := plannedTables()
	uncovered := make([]string, 0, 4)
	for _, table := range customerLinkedTables(t, db) {
		if planned[table] {
			continue
		}
		if _, excluded := declaredExclusions[table]; excluded {
			continue
		}
		uncovered = append(uncovered, table)
	}
	sort.Strings(uncovered)
	return uncovered
}

// plannedTables is the set of tables the plan writes to. It is derived from
// erasurePlan itself, not from a hand-maintained list, so the guard and the
// erasure can never enumerate different sets of tables.
func plannedTables() map[string]bool {
	planned := map[string]bool{}
	for _, s := range erasurePlan(uuid.New(), "guard@example.test", Token(uuid.New())) {
		planned[s.Table] = true
	}
	return planned
}

// customerLinkedTables reads the live schema for every base table carrying a
// column by which a row names a customer.
func customerLinkedTables(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	var names []string
	require.NoError(t, db.Raw(`
		SELECT DISTINCT c.table_name
		FROM information_schema.columns c
		JOIN information_schema.tables tb
		  ON tb.table_schema = c.table_schema AND tb.table_name = c.table_name
		WHERE c.table_schema = 'public'
		  AND tb.table_type = 'BASE TABLE'
		  AND c.column_name IN (?)
		ORDER BY c.table_name`, customerLinkColumns).Scan(&names).Error)
	require.NotEmpty(t, names, "found no customer-linked tables at all — the guard would pass vacuously")
	return names
}

// The guard is only worth having if it can fail. This runs the SAME
// computation against a deliberately-uncovered table and asserts it is
// reported.
//
// Asserting instead that information_schema can see the probe table would be
// a test of information_schema, not of the guard: a coverage function that
// returned "everything is covered" unconditionally would pass it.
func TestErasurePlan_CoverageGuardDetectsAnUncoveredTable(t *testing.T) {
	db := testdb.NewDB(t)

	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS customererasure_guard_probe (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(), customer_email varchar(300) NOT NULL)`).Error)
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS customererasure_guard_probe`).Error })

	require.Contains(t, uncoveredCustomerLinkedTables(t, db), "customererasure_guard_probe",
		"a table naming a customer that the plan neither erases nor excludes must be reported")
}

// TestErasurePlan_EveryStepIsValidSQLAgainstTheLiveSchema executes the whole
// plan against a real database inside a transaction that is rolled back. It
// asserts nothing about row counts — that is the executor's test. What it
// catches is the failure the pure tests cannot see: a column that does not
// exist, a table renamed under the plan, a cast Postgres refuses. The plan
// matches a store and an email that exist nowhere, so every statement
// affects zero rows and the rollback makes even that moot.
func TestErasurePlan_EveryStepIsValidSQLAgainstTheLiveSchema(t *testing.T) {
	db := testdb.NewTx(t)

	steps := erasurePlan(uuid.New(), "no-such-subject@erasure.test", Token(uuid.New()))
	require.NotEmpty(t, steps)

	for i, s := range steps {
		require.NoError(t, db.Exec(s.SQL, s.Args...).Error,
			"step %d (%s, %s) is not executable against the live schema", i, s.Table, s.Disposition)
	}
}
