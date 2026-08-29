//go:build integration

package dispatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/postcommit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// arbitrageSpyCounter satisfies arbitrage.Counter.
type arbitrageSpyCounter struct {
	flagged  int
	cleared  int
	mismatch int
}

func (s *arbitrageSpyCounter) IncArbitrageFlagged()              { s.flagged++ }
func (s *arbitrageSpyCounter) IncArbitrageFalsePositiveCleared() { s.cleared++ }
func (s *arbitrageSpyCounter) IncArbitrageTenantMismatch()       { s.mismatch++ }

// arbitrageKeySource satisfies arbitrage.VersionsSource with one static key.
type arbitrageKeySource struct{}

func (arbitrageKeySource) ListEnabled(_ context.Context) ([]arbitrage.KeyVersion, error) {
	return []arbitrage.KeyVersion{
		{Name: "v1", Payload: []byte("test-key-32-bytes-padded--------"), CreatedAt: time.Now()},
	}, nil
}

// TestArbitrage_RecordedAfterTransactionCommits pins #438.
//
// The failure mode is a HANG, not a wrong value, so run this package with an
// explicit -timeout: without the fix the test never returns, it is killed by
// the test binary's timeout and prints a goroutine dump.
//
// The stall: handleCheckoutSessionCompleted runs inside
// subscription.WithAdvisoryLock and UPDATEs the store_subscriptions row on
// tx, taking a FOR NO KEY UPDATE row lock held until commit. The arbitrage
// recorder writes on the POOL handle — a different connection — and its flag
// toggle is `UPDATE store_subscriptions ... SET arbitrage_flag = true` on that
// same row. Called inline, the recorder's connection waits on the uncommitted
// row lock while the transaction holding it waits, in Go, for the recorder to
// return. Postgres sees one waiter and one idle-in-transaction session — no
// cycle — so deadlock_timeout never fires, and the repo sets no lock_timeout
// or statement_timeout. The webhook hangs forever.
//
// This test reproduces that shape exactly: locking UPDATE on tx first, then
// the recorder call, with a RecordInput that actually flags (production
// hard-codes IPCountry: "" and so never reaches the recorder's writes).
func TestArbitrage_RecordedAfterTransactionCommits(t *testing.T) {
	db := testdb.NewDB(t, "subscription_arbitrage_audit", "store_subscriptions", "stores")

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	customerID := "cus_" + uuid.NewString()[:12]
	sub := subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: customerID,
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		PriceTier:        subscription.PriceTierPPP,
	}
	require.NoError(t, db.Create(&sub).Error)

	spy := &arbitrageSpyCounter{}
	hasher := arbitrage.NewHasher(arbitrage.NewKeyLoader(arbitrageKeySource{}, 5*time.Minute))
	// The recorder gets the POOL handle, exactly as main.go wires it — that
	// second connection is the whole point of the bug.
	d := dispatch.New(nil).WithRecorder(arbitrage.NewRecorder(db, hasher, spy))

	// A tier/country combination that Evaluate actually flags: a PPP-tier
	// subscription paid with a developed-market card.
	in := arbitrage.RecordInput{
		SubscriptionID: sub.ID,
		TenantID:       tenantID,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "IN",
		RawIP:          "203.0.113.9",
	}

	// Production shape: collector installed before the lock, drained after.
	ctx, deferred := postcommit.WithDeferredSends(context.Background())
	require.NoError(t, subscription.WithAdvisoryLock(ctx, db, storeID, func(tx *gorm.DB) error {
		// Stage 1 of handleCheckoutSessionCompleted: the row lock the
		// recorder would collide with.
		res := tx.WithContext(ctx).Exec(
			`UPDATE store_subscriptions
             SET stripe_subscription_id = ?, updated_at = ?
             WHERE stripe_customer_id = ?`,
			"sub_"+uuid.NewString()[:12], time.Now(), customerID)
		require.NoError(t, res.Error)
		require.EqualValues(t, 1, res.RowsAffected)

		// If this runs inline it blocks forever on the lock taken above.
		dispatch.RecordArbitrageForTest(ctx, d, sub, in)
		return nil
	}), "the advisory-lock transaction must complete; a hang here is #438")

	// Nothing may have been written yet: the recorder call belongs after the
	// commit, not inside it.
	var pending int64
	require.NoError(t, db.Model(&arbitrage.SubscriptionArbitrageAudit{}).
		Where("subscription_id = ?", sub.ID).Count(&pending).Error)
	require.Zero(t, pending, "the arbitrage write ran inside the webhook transaction")
	require.Zero(t, spy.flagged)

	// Draining does the work, on a connection that now contends with nobody.
	require.Empty(t, deferred.Run(ctx), "the deferred arbitrage record must succeed")

	var row arbitrage.SubscriptionArbitrageAudit
	require.NoError(t, db.Where("subscription_id = ?", sub.ID).First(&row).Error)
	require.Equal(t, arbitrage.ResolutionOngoing, row.Resolution)

	var reloaded subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", sub.ID).First(&reloaded).Error)
	require.True(t, reloaded.ArbitrageFlag, "the flag toggle must have run post-commit")
	require.Equal(t, 1, spy.flagged)
}

// TestArbitrage_NoCollectorFallsBackInline pins the fallback branch: a caller
// that forgot postcommit.WithDeferredSends must still get the fraud record,
// not silently lose it. Deliberately NOT inside a transaction — inline is only
// safe when there is no open transaction holding the row, which is exactly
// what the loud warning on that branch is there to say.
func TestArbitrage_NoCollectorFallsBackInline(t *testing.T) {
	db := testdb.NewDB(t, "subscription_arbitrage_audit", "store_subscriptions", "stores")

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	sub := subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_" + uuid.NewString()[:12],
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		PriceTier:        subscription.PriceTierPPP,
	}
	require.NoError(t, db.Create(&sub).Error)

	spy := &arbitrageSpyCounter{}
	hasher := arbitrage.NewHasher(arbitrage.NewKeyLoader(arbitrageKeySource{}, 5*time.Minute))
	d := dispatch.New(nil).WithRecorder(arbitrage.NewRecorder(db, hasher, spy))

	// No postcommit.WithDeferredSends on this context.
	dispatch.RecordArbitrageForTest(context.Background(), d, sub, arbitrage.RecordInput{
		SubscriptionID: sub.ID,
		TenantID:       tenantID,
		StoreID:        storeID,
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "IN",
		RawIP:          "203.0.113.9",
	})

	var n int64
	require.NoError(t, db.Model(&arbitrage.SubscriptionArbitrageAudit{}).
		Where("subscription_id = ?", sub.ID).Count(&n).Error)
	require.EqualValues(t, 1, n, "no collector must fall back to inline, never drop the fraud record")
	require.Equal(t, 1, spy.flagged)
}
