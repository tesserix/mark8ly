package tenantpurge

import (
	"strings"
	"testing"
)

// #469: break_glass_lockouts is now purged with its tenant.
//
// It was excluded because it COULD NOT be deleted — the table was owned by
// `postgres` in production, marketplace_api had no DELETE, and including it
// aborted the whole single-tx purge (SQLSTATE 42501). #457 transferred
// ownership, so that constraint is gone and the exclusion became a choice.
//
// The choice is to purge it: the row links a tenant to an HMAC'd client IP,
// which is pseudonymous personal data, and "we could not delete it" is no
// longer an answer under art.17.

func TestPurgePlan_IncludesBreakGlassLockouts(t *testing.T) {
	steps := purgePlan(testTenantID, testStoreIDs)
	if stepIndex(steps, "break_glass_lockouts") < 0 {
		t.Fatal("break_glass_lockouts must be purged with its tenant (#469) — " +
			"the ownership constraint that forced its exclusion was removed in #457")
	}
}

// THE TRAP THIS TEST EXISTS FOR.
//
// tenant_id is NULLABLE. A row with no tenant is an IP-level lockout
// belonging to nobody — protection earned against the platform, not against
// one merchant. A purge that deleted those would let anyone clear an active
// lockout by having any tenant purged, and would silently discard other
// tenants' protection too.
//
// `WHERE tenant_id = ?` never matches NULL in SQL, so the correct behaviour
// falls out of tenantScoped for free. This asserts it deliberately rather
// than relying on that remaining true through a future refactor — a broader
// predicate, an OR IS NULL, or a switch to a different scoping helper would
// all compile and all be wrong.
func TestPurgePlan_BreakGlassLockoutsNeverTouchesNullTenantRows(t *testing.T) {
	steps := purgePlan(testTenantID, testStoreIDs)
	i := stepIndex(steps, "break_glass_lockouts")
	if i < 0 {
		t.Fatal("step missing; see TestPurgePlan_IncludesBreakGlassLockouts")
	}
	sql := strings.ToUpper(steps[i].sql)

	if !strings.Contains(sql, "WHERE TENANT_ID = ?") {
		t.Errorf("must scope on an equality against tenant_id, which cannot match NULL; got: %s", steps[i].sql)
	}
	if strings.Contains(sql, "IS NULL") || strings.Contains(sql, " OR ") {
		t.Errorf("must not widen past a single tenant — NULL-tenant rows are "+
			"platform-wide lockouts belonging to nobody; got: %s", steps[i].sql)
	}
	if len(steps[i].args) != 1 || steps[i].args[0] != testTenantID {
		t.Errorf("must bind exactly the purged tenant; got args %v", steps[i].args)
	}
}
