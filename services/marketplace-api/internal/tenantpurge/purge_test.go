package tenantpurge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testTenantID = "11111111-1111-1111-1111-111111111111"

var testStoreIDs = []string{
	"22222222-2222-2222-2222-222222222222",
	"33333333-3333-3333-3333-333333333333",
}

// globalDenyTables must NEVER appear in a purge plan. These are shared
// reference tables with no tenant/store ownership at all.
var globalDenyTables = []string{
	"supported_countries",
	"fx_rates",
	"shipping_zones",
}

// protectedTables must NEVER appear in a purge plan: deleting them would
// destroy records that must outlive the tenant, or (for webhook_events and
// the global reference rows) touch data no tenant owns.
//
// This test is not a formality. Corrected 2026-08-25: the claim that the
// database also protects the first four — "DB role has DELETE revoked" —
// is false. marketplace_api OWNS them with full DELETE. This list is the
// only enforcement there is.
var protectedTables = []string{
	"business_entity_attestations",
	"app_contract_attestations",
	"subscription_plan_change_audit",
	"billing_archive",
	"webhook_events", // unscopable — no tenant/store/order column exists
	"email_templates",
	"user_profiles",
	"promo_codes",
	"signup_anomaly_log",
	// break_glass_lockouts is NO LONGER protected: #457 removed the
	// ownership constraint and #469 added it to the plan, because the row
	// links a tenant to an HMAC'd client IP. Its NULL-tenant rows are
	// guarded by TestPurgePlan_BreakGlassLockoutsNeverTouchesNullTenantRows
	// rather than by exclusion.
	//
	// break_glass_ACCOUNTS (the sensitive emergency-admin row) is owned by
	// marketplace_api and has always been purged.
}

func stepIndex(steps []deleteStep, table string) int {
	for i, s := range steps {
		if s.table == table {
			return i
		}
	}
	return -1
}

func mustIndex(t *testing.T, steps []deleteStep, table string) int {
	t.Helper()
	i := stepIndex(steps, table)
	if i == -1 {
		t.Fatalf("purgePlan: expected a step for table %q, found none", table)
	}
	return i
}

func TestPurgePlan_StoresIsLast(t *testing.T) {
	steps := purgePlan(testTenantID, testStoreIDs)
	if len(steps) == 0 {
		t.Fatal("purgePlan returned no steps")
	}
	last := steps[len(steps)-1]
	if last.table != "stores" {
		t.Fatalf("expected last step to be \"stores\", got %q", last.table)
	}
	// stores must appear exactly once, and only at the end.
	for i, s := range steps[:len(steps)-1] {
		if s.table == "stores" {
			t.Fatalf("\"stores\" appears at index %d, before the end (index %d)", i, len(steps)-1)
		}
	}
}

func TestPurgePlan_FinancialSubtreeOrder(t *testing.T) {
	steps := purgePlan(testTenantID, testStoreIDs)

	refundIdx := mustIndex(t, steps, "refund_transactions")
	paymentIdx := mustIndex(t, steps, "payment_transactions")
	ordersIdx := mustIndex(t, steps, "orders")

	if !(refundIdx < paymentIdx) {
		t.Fatalf("refund_transactions (idx %d) must come before payment_transactions (idx %d)", refundIdx, paymentIdx)
	}
	if !(paymentIdx < ordersIdx) {
		t.Fatalf("payment_transactions (idx %d) must come before orders (idx %d)", paymentIdx, ordersIdx)
	}

	// Also verify the other RESTRICT-blocked-by-orders tables land before orders:
	// platform_fee_ledger.order_id, coupon_usage.order_id, shipments.order_id,
	// and returns.order_id all RESTRICT-reference orders(id).
	for _, table := range []string{"platform_fee_ledger", "coupon_usage", "shipments", "returns"} {
		idx := mustIndex(t, steps, table)
		if !(idx < ordersIdx) {
			t.Fatalf("%s (idx %d) must come before orders (idx %d)", table, idx, ordersIdx)
		}
	}
}

func TestPurgePlan_ProductSubtreeOrder(t *testing.T) {
	steps := purgePlan(testTenantID, testStoreIDs)

	productsIdx := mustIndex(t, steps, "products")
	vendorsIdx := mustIndex(t, steps, "vendors")
	categoriesIdx := mustIndex(t, steps, "categories")

	if !(productsIdx < vendorsIdx) {
		t.Fatalf("products (idx %d) must come before vendors (idx %d)", productsIdx, vendorsIdx)
	}
	if !(productsIdx < categoriesIdx) {
		t.Fatalf("products (idx %d) must come before categories (idx %d)", productsIdx, categoriesIdx)
	}

	for _, table := range []string{"product_categories", "reviews", "review_media", "review_replies", "review_reactions", "wishlists"} {
		idx := mustIndex(t, steps, table)
		if !(idx < productsIdx) {
			t.Fatalf("%s (idx %d) must come before products (idx %d)", table, idx, productsIdx)
		}
	}
}

func TestPurgePlan_ReferralsBeforeCustomerLoyalties(t *testing.T) {
	// referrals.referrer_id/referee_id -> customer_loyalties(id) RESTRICT
	// (migration 000011, no ON DELETE clause) — referrals must be deleted
	// first or the customer_loyalties delete would violate the FK.
	steps := purgePlan(testTenantID, testStoreIDs)
	referralsIdx := mustIndex(t, steps, "referrals")
	loyaltiesIdx := mustIndex(t, steps, "customer_loyalties")
	if !(referralsIdx < loyaltiesIdx) {
		t.Fatalf("referrals (idx %d) must come before customer_loyalties (idx %d)", referralsIdx, loyaltiesIdx)
	}
}

func TestPurgePlan_NeverTouchesGlobalOrProtectedTables(t *testing.T) {
	steps := purgePlan(testTenantID, testStoreIDs)

	seen := make(map[string]bool, len(steps))
	for _, s := range steps {
		seen[s.table] = true
	}

	for _, table := range globalDenyTables {
		if seen[table] {
			t.Errorf("purgePlan must never touch global table %q", table)
		}
	}
	for _, table := range protectedTables {
		if seen[table] {
			t.Errorf("purgePlan must never touch protected/unscopable table %q", table)
		}
	}
}

// TestPurgePlan_EveryStepIsScoped asserts every DELETE either:
//   - filters on tenant_id / store_id directly with a bound arg, or
//   - filters via a subquery ("IN (SELECT id FROM <parent> WHERE tenant_id = ?)")
//
// and never emits a bare, unscoped `DELETE FROM <table>`.
func TestPurgePlan_EveryStepIsScoped(t *testing.T) {
	steps := purgePlan(testTenantID, testStoreIDs)
	if len(steps) == 0 {
		t.Fatal("purgePlan returned no steps")
	}

	for _, s := range steps {
		upper := strings.ToUpper(s.sql)
		if !strings.Contains(upper, "WHERE") {
			t.Errorf("step %q has no WHERE clause: %s", s.table, s.sql)
			continue
		}
		scoped := strings.Contains(s.sql, "tenant_id = ?") ||
			strings.Contains(s.sql, "store_id IN") ||
			strings.Contains(s.sql, "WHERE tenant_id = ?)") // subquery form
		if !scoped {
			t.Errorf("step %q does not appear to be scoped by tenant_id/store_id: %s", s.table, s.sql)
		}
		// Every step must carry at least one bound arg that matches the
		// tenant or one of its stores (non-empty storeIDs in this test).
		if len(s.args) == 0 {
			t.Errorf("step %q has no bound args — looks unscoped: %s", s.table, s.sql)
			continue
		}
		for _, a := range s.args {
			str, ok := a.(string)
			if !ok {
				t.Errorf("step %q has a non-string arg %#v", s.table, a)
				continue
			}
			if str != testTenantID && !containsString(testStoreIDs, str) {
				t.Errorf("step %q has arg %q that is neither tenantID nor a storeID", s.table, str)
			}
		}
	}
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// TestPurgePlan_EmptyStoreIDs verifies store-scoped steps degrade to a safe
// no-op (`IN (NULL)`) rather than emitting invalid SQL (`IN ()`) when the
// tenant has no stores.
func TestPurgePlan_EmptyStoreIDs(t *testing.T) {
	steps := purgePlan(testTenantID, nil)
	idx := mustIndex(t, steps, "csv_import_jobs")
	if !strings.Contains(steps[idx].sql, "IN (NULL)") {
		t.Fatalf("expected csv_import_jobs step to degrade to IN (NULL) for empty storeIDs, got: %s", steps[idx].sql)
	}
	if len(steps[idx].args) != 0 {
		t.Fatalf("expected zero args for the empty-storeIDs case, got %d", len(steps[idx].args))
	}
}

// TestPurgePlan_Deterministic verifies purgePlan is a pure function: same
// inputs, same plan, every time — a property the transactional Purge
// wrapper depends on implicitly (no hidden state, no randomness in order).
func TestPurgePlan_Deterministic(t *testing.T) {
	a := purgePlan(testTenantID, testStoreIDs)
	b := purgePlan(testTenantID, testStoreIDs)
	if len(a) != len(b) {
		t.Fatalf("plan length differs across calls: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].table != b[i].table || a[i].sql != b[i].sql {
			t.Fatalf("plan differs at step %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestPurge_RejectsEmptyTenantID(t *testing.T) {
	_, err := Purge(nil, nil, "", nil) //nolint:staticcheck // deliberately nil ctx/db to hit the validation guard before either is used
	if err == nil {
		t.Fatal("expected an error for empty tenantID, got nil")
	}
}

func TestPurge_RejectsNilDB(t *testing.T) {
	_, err := Purge(nil, nil, testTenantID, nil) //nolint:staticcheck // deliberately nil ctx to hit the validation guard before it's used
	if err == nil {
		t.Fatal("expected an error for nil db, got nil")
	}
}

// The preview and the purge must enumerate exactly the same tables in
// exactly the same order. Two lists of "every tenant-scoped table" that
// must agree, with nothing forcing them to, is the defect this package's
// sibling (subscription/harddelete) already demonstrates.
func TestCountPlan_MatchesPurgePlanTableForTable(t *testing.T) {
	purge := purgePlan(testTenantID, testStoreIDs)
	count := countPlan(testTenantID, testStoreIDs)

	require.Equal(t, len(purge), len(count), "the two plans must have the same length")
	for i := range purge {
		require.Equal(t, purge[i].table, count[i].table, "step %d", i)
		require.Equal(t, purge[i].args, count[i].args, "step %d args", i)
	}
}

func TestCountPlan_SelectsRatherThanDeletes(t *testing.T) {
	for _, s := range countPlan(testTenantID, testStoreIDs) {
		require.True(t, strings.HasPrefix(s.sql, "SELECT count(*)"),
			"step %q must count, got %q", s.table, s.sql)
		require.NotContains(t, s.sql, "DELETE",
			"a preview step must contain no DELETE at all: %q", s.sql)
	}
}

// storeSequenceNames is the whole safety argument for the one part of the
// purge that cannot be parameterized, so it is tested directly: both kinds
// per store, in the trigger's format, and nothing but [a-z0-9_] reaching
// the SQL.

func TestStoreSequenceNames_TwoPerStoreInTriggerFormat(t *testing.T) {
	names, err := storeSequenceNames(testStoreIDs)
	require.NoError(t, err)
	require.Equal(t, []string{
		"mk_seq_order_22222222_2222_2222_2222_222222222222",
		"mk_seq_return_22222222_2222_2222_2222_222222222222",
		"mk_seq_order_33333333_3333_3333_3333_333333333333",
		"mk_seq_return_33333333_3333_3333_3333_333333333333",
	}, names, "must match mk_create_store_sequences() in migration 000004 exactly")
}

func TestStoreSequenceNames_EmptyStoreListIsANoOpNotAnError(t *testing.T) {
	names, err := storeSequenceNames(nil)
	require.NoError(t, err)
	require.Empty(t, names, "a tenant with no stores has no sequences to drop")
}

func TestStoreSequenceNames_RejectsAnythingThatIsNotAUUID(t *testing.T) {
	// The names go into an identifier position in DDL. A non-uuid id is a
	// caller bug; failing the purge is correct, silently dropping nothing
	// would re-create the leak this step exists to close.
	for _, bad := range []string{
		"",
		"not-a-uuid",
		"22222222-2222-2222-2222-222222222222; DROP TABLE stores; --",
		"22222222_2222_2222_2222_222222222222",
	} {
		_, err := storeSequenceNames([]string{bad})
		require.Error(t, err, "store id %q must be rejected", bad)
	}
}

func TestStoreSequenceNames_ProduceOnlyIdentifierSafeCharacters(t *testing.T) {
	names, err := storeSequenceNames(testStoreIDs)
	require.NoError(t, err)
	require.NotEmpty(t, names)
	for _, name := range names {
		for _, r := range name {
			require.Truef(t,
				(r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_',
				"sequence name %q contains %q, outside the [a-z0-9_] set that makes interpolation safe", name, r)
		}
	}
}
