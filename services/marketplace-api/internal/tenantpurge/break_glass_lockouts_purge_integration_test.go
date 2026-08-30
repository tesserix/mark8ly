//go:build integration

package tenantpurge_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #469, against a real database rather than the plan alone.
//
// Two things need proving, and neither is visible in purgePlan's SQL:
//
//  1. The DELETE actually SUCCEEDS. Until #457 it could not — the table was
//     `postgres`-owned in production and the statement aborted the whole
//     single-tx purge with SQLSTATE 42501. A plan test cannot see that; only
//     running it can.
//  2. A NULL-tenant row SURVIVES. Those are IP-level lockouts belonging to
//     nobody, earned against the platform rather than one merchant. If a
//     purge cleared them, anyone could drop a live lockout by having any
//     tenant purged.
func TestIntegration_Purge_BreakGlassLockouts(t *testing.T) {
	db := testdb.NewDB(t, append(domainTablesToCleanup, "break_glass_lockouts")...)
	ctx := context.Background()

	purged := seedTenant(t, db, uuid.NewString())
	other := seedTenant(t, db, uuid.NewString())

	ins := `INSERT INTO break_glass_lockouts (ip_hash, tenant_id, locked_until, reason) VALUES (decode(?, 'hex'), ?, ?, ?)`
	until := time.Now().Add(24 * time.Hour)
	if err := db.Exec(ins, "010203", purged.tenantID, until, "test_purged").Error; err != nil {
		t.Fatalf("seed lockout for purged tenant: %v", err)
	}
	if err := db.Exec(ins, "040506", other.tenantID, until, "test_other").Error; err != nil {
		t.Fatalf("seed lockout for other tenant: %v", err)
	}
	// The one that must survive: no tenant at all.
	if err := db.Exec(`INSERT INTO break_glass_lockouts (ip_hash, tenant_id, locked_until, reason) VALUES (decode(?, 'hex'), NULL, ?, ?)`,
		"070809", until, "test_platform_wide").Error; err != nil {
		t.Fatalf("seed platform-wide lockout: %v", err)
	}

	if _, err := tenantpurge.Purge(ctx, db, purged.tenantID, []string{purged.storeID}); err != nil {
		t.Fatalf("Purge: %v — a permission error here means the table's ownership regressed (#457)", err)
	}

	if n := countRows(t, db, "break_glass_lockouts", "tenant_id", purged.tenantID); n != 0 {
		t.Errorf("purged tenant's lockouts should be gone, got %d", n)
	}
	if n := countRows(t, db, "break_glass_lockouts", "tenant_id", other.tenantID); n != 1 {
		t.Errorf("another tenant's lockout must be untouched, got %d rows", n)
	}

	var nullTenant int64
	if err := db.Raw(`SELECT count(*) FROM break_glass_lockouts WHERE tenant_id IS NULL AND reason = 'test_platform_wide'`).
		Scan(&nullTenant).Error; err != nil {
		t.Fatalf("count null-tenant lockouts: %v", err)
	}
	if nullTenant != 1 {
		t.Errorf("a NULL-tenant lockout is platform-wide protection belonging to nobody "+
			"and must survive any tenant purge; got %d rows", nullTenant)
	}
}
