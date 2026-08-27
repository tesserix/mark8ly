//go:build integration

package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func seedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()
	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]
	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	require.NoError(t, err)
	t.Cleanup(func() { db.Exec("DELETE FROM stores WHERE id = ?", storeID) })
}

// seedSub inserts an expired subscription and forces updated_at to a fixed
// point in the past, past GORM's autoupdate.
func seedSub(t *testing.T, db *gorm.DB, addr *string, updatedAt time.Time) subscription.StoreSubscription {
	t.Helper()
	storeID, tenantID := uuid.New(), uuid.New()
	seedStore(t, db, tenantID, storeID)

	sub := subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_" + uuid.NewString()[:12],
		Status:           subscription.StatusExpired,
		Email:            addr,
	}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Exec(
		`UPDATE store_subscriptions SET updated_at = ? WHERE id = ?`, updatedAt, sub.ID).Error)
	return sub
}

func readUpdatedAt(t *testing.T, db *gorm.DB, id uuid.UUID) time.Time {
	t.Helper()
	var got time.Time
	require.NoError(t, db.Raw(`SELECT updated_at FROM store_subscriptions WHERE id = ?`, id).Scan(&got).Error)
	return got
}

// The win-back cron selects expired rows by updated_at and derives its
// idempotency key from it. If the backfill stamped updated_at, every
// merchant's win-back clock would be reset — see the package comment.
func TestBackfill_DoesNotTouchUpdatedAt(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	ctx := context.Background()

	was := time.Now().UTC().Add(-30*24*time.Hour - time.Hour).Truncate(time.Microsecond)
	sub := seedSub(t, db, nil, was)
	before := readUpdatedAt(t, db, sub.ID)

	lookup := func(_ context.Context, customerID string) (string, error) {
		if customerID == sub.StripeCustomerID {
			return "merchant@example.com", nil
		}
		return "", nil
	}

	stats, err := run(ctx, db.Where("id = ?", sub.ID), lookup, 200, 0, false, quietLogger())
	require.NoError(t, err)
	require.Equal(t, 1, stats.Updated)

	var email *string
	require.NoError(t, db.Raw(`SELECT email FROM store_subscriptions WHERE id = ?`, sub.ID).Scan(&email).Error)
	require.NotNil(t, email)
	require.Equal(t, "merchant@example.com", *email)

	after := readUpdatedAt(t, db, sub.ID)
	require.True(t, before.Equal(after),
		"backfill moved updated_at (%s -> %s): win-back timing destroyed", before, after)
}

// citext: a case-only difference is the same address to Postgres, so the
// row must count as Unchanged rather than being rewritten on every re-run.
func TestBackfill_CaseOnlyDifferenceIsUnchanged(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	ctx := context.Background()

	stored := "Merchant@Example.com"
	sub := seedSub(t, db, &stored, time.Now().UTC().Add(-90*24*time.Hour))

	lookup := func(context.Context, string) (string, error) { return "merchant@example.com", nil }

	stats, err := run(ctx, db.Where("id = ?", sub.ID), lookup, 200, 0, false, quietLogger())
	require.NoError(t, err)
	require.Equal(t, 1, stats.Unchanged, "case-only difference counted as a change")
	require.Equal(t, 0, stats.Updated)
}
