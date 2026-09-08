//go:build integration

package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// This test exists because the first version of this command could not run.
//
// Its scan selected s.created_at from the stores projection, which had no such
// column — the projection carried only synced_at. Every unit test passed, the
// build was green, and the command would have failed on its first real
// invocation with `column s.created_at does not exist`. Nothing in the suite
// executed the query against a real schema, so nothing could have known.
//
// The dry-run default made it worse rather than safer: the failure was waiting
// behind an operator flag, so even a cautious first run would have hit it.
//
// What this pins is narrow and deliberate: that the SQL this command actually
// issues matches the migrated schema. It is the assertion whose absence let a
// broken command merge.
func TestRun_QueryMatchesTheMigratedSchema(t *testing.T) {
	db := testdb.NewTx(t)
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	// Dry run: proves the scan executes and reads its columns without writing
	// anything. A schema mismatch fails here, which is the whole point.
	stats, err := run(context.Background(), db, 100, false, time.Now().UTC(), log)
	require.NoError(t, err, "the backfill query does not match the schema")
	require.Zero(t, stats.Created, "a dry run wrote rows")
}

// A store with a known creation date is picked up and dated from it — not from
// the moment of the backfill, which would hand a year-old store a fresh trial.
func TestRun_DatesTheTrialFromTheStore(t *testing.T) {
	db := testdb.NewTx(t)
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	storeID := uuid.New()
	tenantID := uuid.New()
	created := time.Now().UTC().AddDate(0, 0, -200)
	seedStore(t, db, storeID, tenantID, &created)

	now := time.Now().UTC()
	stats, err := run(context.Background(), db, 100, true, now, log)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Created, 1)

	var trialEndsAt time.Time
	require.NoError(t, db.Raw(
		`SELECT trial_ends_at FROM store_subscriptions WHERE store_id = ?`, storeID,
	).Scan(&trialEndsAt).Error)

	want, expired := backfilledTrialEnd(created, now)
	require.True(t, expired, "a 200-day-old store should backfill as already expired")
	require.WithinDuration(t, want, trialEndsAt, time.Second,
		"the trial was not dated from the store's own creation")
}

// A row mirrored before migration 136 has no created_at. It must be skipped
// and COUNTED, not guessed at: synced_at would date the trial from the last
// settings edit, and now() would grant a fresh 90 days to an old store.
func TestRun_SkipsAndCountsAnUndatedStore(t *testing.T) {
	db := testdb.NewTx(t)
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	storeID := uuid.New()
	seedStore(t, db, storeID, uuid.New(), nil)

	stats, err := run(context.Background(), db, 100, true, time.Now().UTC(), log)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Undated, 1, "an undated store was not reported")

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM store_subscriptions WHERE store_id = ?`, storeID,
	).Scan(&n).Error)
	require.Zero(t, n, "a store with no creation date was given a trial anyway")
}

// Re-running must not double-insert. The command is an operator tool and will
// be run twice by someone who is unsure whether the first run took.
func TestRun_IsIdempotent(t *testing.T) {
	db := testdb.NewTx(t)
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	storeID := uuid.New()
	created := time.Now().UTC().AddDate(0, 0, -10)
	seedStore(t, db, storeID, uuid.New(), &created)

	now := time.Now().UTC()
	_, err := run(context.Background(), db, 100, true, now, log)
	require.NoError(t, err)
	second, err := run(context.Background(), db, 100, true, now, log)
	require.NoError(t, err)
	require.Zero(t, second.Created, "a second run inserted again")

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM store_subscriptions WHERE store_id = ?`, storeID,
	).Scan(&n).Error)
	require.EqualValues(t, 1, n)
}

func seedStore(t *testing.T, db interface {
	Exec(string, ...any) *gorm.DB
}, storeID, tenantID uuid.UUID, createdAt *time.Time) {
	t.Helper()
	// storefront_customer_portal_secret is NOT NULL and generated server-side
	// by the upsert handler, so a hand-rolled INSERT has to supply one. Any
	// 64 hex characters satisfy the column; nothing here reads it back.
	require.NoError(t, db.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code,
		                    timezone, status, storefront_customer_portal_secret,
		                    created_at, synced_at)
		VALUES (?, ?, ?, ?, 'AU', 'AUD', 'Australia/Sydney', 'active', ?, ?, now())`,
		storeID, tenantID, "s-"+storeID.String()[:8], "Test Store",
		strings.Repeat("0", 64), createdAt,
	).Error)
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Log(string(p)); return len(p), nil }
