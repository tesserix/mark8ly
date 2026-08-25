//go:build integration

package account

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/internal/tenant"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// testDB returns a real (non-transactional) *gorm.DB with tenants, stores
// and outbox_events truncated before and after the test.
//
// PurgeTenant runs its own gorm.DB.Transaction internally, and the
// concurrency test needs two goroutines to observe each other's commits —
// neither is possible on testdb.NewTx's shared, uncommitted transaction
// (see its doc comment), so this package needs testdb.NewDB instead of
// the tx-rollback helper the rest of the tenant package's integration
// tests use.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.NewDB(t, "outbox_events", "stores", "tenants")
}

// realService builds an account.Service wired to the real tenant
// repository and outbox.Enqueue, with nil FGA and GIP clients — exactly
// what Task 2 made tolerable and what production runs in an env without
// them configured.
func realService(t *testing.T, db *gorm.DB) *Service {
	t.Helper()
	return NewService(db, tenant.NewRepository(db), nil, nil, outbox.EnqueueAfter, slog.Default())
}

// seedTenantWithStores inserts a tenant with a single store under the
// given slug, returning the tenant ID. Uses GB/GBP/Europe/London — the
// only country/currency/timezone reference rows present in platform-api's
// seed; stores FKs to all three.
func seedTenantWithStores(t *testing.T, db *gorm.DB, name, slug string) string {
	t.Helper()

	var tenantID string
	require.NoError(t, db.Raw(
		`INSERT INTO tenants (name, owner_user_id, owner_email, status)
		 VALUES (?, ?, ?, 'active')
		 RETURNING id`,
		name, "owner-uid-"+slug, slug+"@example.com",
	).Scan(&tenantID).Error)

	require.NoError(t, db.Exec(
		`INSERT INTO stores (tenant_id, slug, name, country_code, currency_code, timezone, status)
		 VALUES (?, ?, ?, 'GB', 'GBP', 'Europe/London', 'active')`,
		tenantID, slug, name,
	).Error)

	return tenantID
}

func TestPurgeTenant_Integration_MismatchLeavesTenantIntact(t *testing.T) {
	db := testDB(t)
	svc := realService(t, db)

	tenantID := seedTenantWithStores(t, db, "The Bondi Store", "the-bondi-store")

	_, err := svc.PurgeTenant(t.Context(), tenantID, []string{"the-facade-factory"})
	require.Error(t, err)

	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM tenants WHERE id = ?`, tenantID).Scan(&n).Error)
	require.EqualValues(t, 1, n, "a mismatched purge must roll back the transaction")

	var outboxRows int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM outbox_events WHERE payload->>'tenant_id' = ?`, tenantID).Scan(&outboxRows).Error)
	require.EqualValues(t, 0, outboxRows, "a mismatched purge must enqueue nothing")
}

func TestPurgeTenant_Integration_SuccessDeletesAndEnqueuesTogether(t *testing.T) {
	db := testDB(t)
	svc := realService(t, db)

	tenantID := seedTenantWithStores(t, db, "The Bondi Store", "the-bondi-store")

	res, err := svc.PurgeTenant(t.Context(), tenantID, []string{"the-bondi-store"})
	require.NoError(t, err)
	require.Len(t, res.StoreIDs, 1)

	var tenants, stores, outboxRows int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM tenants WHERE id = ?`, tenantID).Scan(&tenants).Error)
	require.NoError(t, db.Raw(`SELECT count(*) FROM stores WHERE tenant_id = ?`, tenantID).Scan(&stores).Error)
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM outbox_events WHERE payload->>'tenant_id' = ?`, tenantID).Scan(&outboxRows).Error)

	require.EqualValues(t, 0, tenants)
	require.EqualValues(t, 0, stores, "stores CASCADE from tenants")
	require.EqualValues(t, 1, outboxRows, "exactly one tenant.deleted event")
}

// Two concurrent purges of one tenant. The property discriminates between
// "one winner" and "two winners", so the fixture contains two callers —
// a single call could never tell those apart.
func TestPurgeTenant_Integration_ConcurrentPurgesHaveExactlyOneWinner(t *testing.T) {
	db := testDB(t)
	svc := realService(t, db)

	tenantID := seedTenantWithStores(t, db, "The Bondi Store", "the-bondi-store")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.PurgeTenant(t.Context(), tenantID, []string{"the-bondi-store"})
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		}
	}
	require.Equal(t, 1, winners, "exactly one purge may succeed, got %d (errors: %v)", winners, errs)

	var outboxRows int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM outbox_events WHERE payload->>'tenant_id' = ?`, tenantID).Scan(&outboxRows).Error)
	require.EqualValues(t, 1, outboxRows, "the loser must not enqueue a second purge event")
}
