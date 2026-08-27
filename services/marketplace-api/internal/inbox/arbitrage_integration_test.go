//go:build integration

package inbox_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedSubscription returns the id of a store_subscriptions row for storeID,
// inserting one if none exists yet. subscription_arbitrage_audit.subscription_id
// is a NOT NULL FK to store_subscriptions(id) ON DELETE CASCADE, so a
// synthetic uuid.New() would violate the constraint — a real parent row is
// required, the same reason testdb.SeedStore exists for stores.
//
// store_subscriptions has UNIQUE (store_id), and this test seeds several
// arbitrage rows against one store, so the insert is idempotent per store
// rather than unconditional — mirroring testdb.SeedVendor's pattern.
func seedSubscription(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) uuid.UUID {
	t.Helper()

	var existing uuid.UUID
	err := db.Raw(`SELECT id FROM store_subscriptions WHERE store_id = ?`, storeID).Row().Scan(&existing)
	if err == nil {
		return existing
	}
	require.ErrorIs(t, err, sql.ErrNoRows)

	id := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO store_subscriptions (id, tenant_id, store_id, stripe_customer_id)
		VALUES (?, ?, ?, ?)`,
		id, tenantID, storeID, "cus_test_"+id.String()[:8],
	).Error)
	return id
}

func seedArbitrage(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, resolution string, flaggedAt time.Time) uuid.UUID {
	t.Helper()
	subscriptionID := seedSubscription(t, db, tenantID, storeID)
	id := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO subscription_arbitrage_audit
			(id, subscription_id, tenant_id, store_id, resolved_price_tier, resolution, flagged_at)
		VALUES (?, ?, ?, ?, 'sea', ?, ?)`,
		id, subscriptionID, tenantID, storeID, resolution, flaggedAt,
	).Error)
	return id
}

func TestArbitrageProvider_OnlyOngoing(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	// "upheld" is not a valid resolution — the CHECK constraint on
	// subscription_arbitrage_audit.resolution only allows 'ongoing',
	// 'false_positive_cleared', 'reprice_developed', 'terminated'
	// (migrations/000044_subscription_arbitrage_audit.up.sql). Use a
	// declared, resolved Resolution constant instead.
	seedArbitrage(t, db, tenantID, storeID, string(arbitrage.ResolutionTerminated), now.Add(-time.Hour))
	wanted := seedArbitrage(t, db, tenantID, storeID, string(arbitrage.ResolutionOngoing), now.Add(-6*time.Hour))

	p := inbox.NewArbitrageProvider(db)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, wanted.String(), items[0].ID)
	require.Equal(t, inbox.KindArbitrageAppeal, items[0].Kind)
	require.Nil(t, items[0].DueAt)
}

func TestArbitrageProvider_FilterStatus(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	seedArbitrage(t, db, tenantID, storeID, string(arbitrage.ResolutionOngoing), now.Add(-6*time.Hour))

	p := inbox.NewArbitrageProvider(db)

	items, err := p.List(context.Background(), inbox.Filter{Limit: 10, Status: "bogus"})
	require.NoError(t, err)
	require.Empty(t, items, "a non-matching status must short-circuit to empty, not query")

	n, err := p.Count(context.Background(), inbox.Filter{Status: "bogus"})
	require.NoError(t, err)
	require.Zero(t, n)

	items, err = p.List(context.Background(), inbox.Filter{Limit: 10, Status: string(arbitrage.ResolutionOngoing)})
	require.NoError(t, err)
	require.Len(t, items, 1, "the matching status must behave exactly as an empty status")

	n, err = p.Count(context.Background(), inbox.Filter{Status: string(arbitrage.ResolutionOngoing)})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}
