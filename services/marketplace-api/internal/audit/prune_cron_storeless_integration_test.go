//go:build integration

package audit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestPruneCronSkipsStoreLessRows proves #311's decision holds at the SQL
// level, not just in a comment: a tenant-scoped platform operator audit row
// (store_id IS NULL) survives a prune run, while an ordinary store-scoped
// row on the same plan and well past its retention cutoff is deleted.
//
// The control row is the point of this test. Without it, an assertion that
// only checks "the store-less row is still there" would pass even if the
// prune silently no-oped — which is exactly the kind of regression the
// `a.store_id IS NOT NULL` guard in pruneBucket exists to prevent someone
// from introducing unnoticed (e.g. by changing the JOIN to a LEFT JOIN).
func TestPruneCronSkipsStoreLessRows(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions", "stores")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()

	// A real store + a starter-plan subscription, so the control row is
	// eligible under the trial+starter 90-day bucket.
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, synced_at, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, 'Test Store', 'US', 'USD', 'UTC', 'active', now(), ?)`,
		storeID, tenantID, "prune-test-"+uuid.NewString()[:8], strings.Repeat("a", 64),
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO store_subscriptions (id, tenant_id, store_id, stripe_customer_id, plan, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'active', now(), now())`,
		uuid.New(), tenantID, storeID, "cus_prune_test", string(subscription.PlanStarter),
	).Error)

	longAgo := time.Now().UTC().AddDate(0, 0, -200) // well past the 90-day starter cutoff

	// Control row: ordinary store-scoped audit row, eligible for pruning.
	// Its deletion is what proves the prune actually ran.
	controlID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, created_at)
		 VALUES (?, ?, ?, 'user', 'product.updated', 'product', 'success', 'info', ?)`,
		controlID, tenantID, storeID, longAgo,
	).Error)

	// Store-less row: a platform operator action against the tenant, with
	// no store. Created even further back so it would definitely have been
	// eligible were it not for the store_id IS NOT NULL guard.
	storelessID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, created_at)
		 VALUES (?, ?, NULL, 'operator', 'tenant.suspended', 'tenant', 'success', 'warning', ?)`,
		storelessID, tenantID, longAgo.AddDate(0, 0, -800),
	).Error)

	cron := audit.NewPruneCron(db, nil, nil, 0)
	stats, err := cron.Run(ctx)
	require.NoError(t, err)
	require.Empty(t, stats.ErrorsByPlan, "prune bucket errors: %+v", stats.ErrorsByPlan)
	require.GreaterOrEqual(t, stats.RowsDeleted, int64(1), "prune must have deleted at least the control row")

	var controlCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM audit_logs WHERE id = ?`, controlID).Scan(&controlCount).Error)
	require.Equal(t, int64(0), controlCount, "control row (store-scoped, past cutoff) must be deleted by the prune")

	var storelessCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM audit_logs WHERE id = ?`, storelessID).Scan(&storelessCount).Error)
	require.Equal(t, int64(1), storelessCount, "store-less operator audit row must survive the prune (#311)")
}
