//go:build integration

package webhookprune_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/webhookprune"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// now is the pinned clock every test in this file runs against. Fixed, with
// zero sub-second component, so every seeded created_at below is exact down
// to the value Postgres stores rather than merely close to it.
var now = time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

// insertEvent seeds one webhook_events row with an explicit status and
// created_at, and returns its id.
//
// ABSOLUTE AGES ONLY. Callers pass `now.AddDate(0, 0, -31)`, never
// `now.AddDate(0, 0, -webhookprune.ProcessedRetentionDays-1)`. A fixture
// derived from the constant moves with the constant: widen the window from 30
// days to 300 and a constant-derived "just past the edge" row is still just
// past the edge, so the test pins the comparison operator and never the
// window's VALUE. That exact mistake was caught during #369 and is not
// repeated here — 31 means thirty-one days, and if the policy changes to
// something other than 30/90 these tests must be edited deliberately.
func insertEvent(t *testing.T, db *gorm.DB, status string, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO webhook_events (id, provider, provider_event_id, event_type, payload, status, created_at)
		 VALUES (?, 'stripe', ?, 'payment_intent.succeeded', '{"data":{"object":{"billing_details":{"email":"a@b.test"}}}}'::jsonb, ?, ?)`,
		id, "evt_"+id.String(), status, createdAt,
	).Error)
	return id
}

func countRows(t *testing.T, db *gorm.DB, id uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM webhook_events WHERE id = ?`, id).Scan(&n).Error)
	return n
}

func runPrune(t *testing.T, db *gorm.DB, batchSize int) webhookprune.PruneStats {
	t.Helper()
	cron := webhookprune.NewPruneCron(db, nil, func() time.Time { return now }, batchSize)
	stats, err := cron.Run(context.Background())
	require.NoError(t, err)
	return stats
}

// THE BOUNDARY, ON THE BOUNDARY — processed rows, 30 days.
// 29 days old survives; 31 days old is deleted. Absolute ages, not
// constant-derived offsets: these two numbers assert that the window is
// THIRTY days, and they go red if it is moved.
func TestPrune_ProcessedThirtyDayBoundary(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	survives := insertEvent(t, db, "processed", now.AddDate(0, 0, -29))
	deleted := insertEvent(t, db, "processed", now.AddDate(0, 0, -31))

	stats := runPrune(t, db, 0)
	require.Equal(t, int64(1), stats.ProcessedDeleted)

	require.Equal(t, int64(1), countRows(t, db, survives),
		"a processed row 29 days old is inside the 30 day window and must survive")
	require.Equal(t, int64(0), countRows(t, db, deleted),
		"a processed row 31 days old is past the 30 day window and must be deleted")
}

// The exact edge. created_at == cutoff is the only row that can distinguish
// `<` from `<=`; the ±1 day rows above cannot, because neither is ever equal
// to the cutoff. The rule is strict `<`, so a row created exactly 30 days ago
// has not yet EXCEEDED 30 days and must survive.
func TestPrune_ProcessedExactlyAtCutoffSurvives(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	exact := insertEvent(t, db, "processed", now.AddDate(0, 0, -30))

	runPrune(t, db, 0)

	require.Equal(t, int64(1), countRows(t, db, exact),
		"created_at < cutoff is strict, and equal is not less-than")
}

// THE BOUNDARY, ON THE BOUNDARY — unprocessed rows, 90 days.
// 89 days old survives; 91 days old is deleted. The 89-day row is also the
// guard that the two classes have DIFFERENT windows: it is 59 days past the
// processed cutoff and must still be here.
func TestPrune_UnprocessedNinetyDayBoundary(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	survives := insertEvent(t, db, "received", now.AddDate(0, 0, -89))
	deleted := insertEvent(t, db, "received", now.AddDate(0, 0, -91))

	stats := runPrune(t, db, 0)
	require.Equal(t, int64(1), stats.UnprocessedDeleted)
	require.Equal(t, int64(0), stats.ProcessedDeleted,
		"no processed rows were seeded, so the processed class must delete nothing")

	require.Equal(t, int64(1), countRows(t, db, survives),
		"an unprocessed row 89 days old is inside the 90 day window and must survive "+
			"— it is well past the 30 day processed window, so this also asserts the two classes differ")
	require.Equal(t, int64(0), countRows(t, db, deleted),
		"an unprocessed row 91 days old is past the 90 day window and must be deleted")
}

func TestPrune_UnprocessedExactlyAtCutoffSurvives(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	exact := insertEvent(t, db, "received", now.AddDate(0, 0, -90))

	runPrune(t, db, 0)

	require.Equal(t, int64(1), countRows(t, db, exact),
		"created_at < cutoff is strict, and equal is not less-than")
}

// THE STATUS DISTINCTION, asserted in one pass. A processed row and an
// unprocessed row of the SAME age — 60 days, between the two windows — must
// get opposite outcomes. Drop the status distinction in either direction and
// this test goes red.
func TestPrune_SameAgeDifferentStatusGetOppositeOutcomes(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	sixtyDays := now.AddDate(0, 0, -60)
	processed := insertEvent(t, db, "processed", sixtyDays)
	unprocessed := insertEvent(t, db, "received", sixtyDays)

	stats := runPrune(t, db, 0)
	require.Equal(t, int64(1), stats.ProcessedDeleted)
	require.Equal(t, int64(0), stats.UnprocessedDeleted)

	require.Equal(t, int64(0), countRows(t, db, processed),
		"60 days is past the 30 day processed window")
	require.Equal(t, int64(1), countRows(t, db, unprocessed),
		"60 days is inside the 90 day unprocessed window — the same age must NOT decide both classes")
}

// The long-retention class is defined by EXCLUSION, not by matching
// 'received'. This table writes only 'received' and 'processed' today, but a
// future provider integration adding a third value must inherit the safer,
// longer window rather than silently falling outside the prune (or worse,
// into the short one).
func TestPrune_UnknownStatusTakesTheLongWindow(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	young := insertEvent(t, db, "quarantined", now.AddDate(0, 0, -60))
	old := insertEvent(t, db, "quarantined", now.AddDate(0, 0, -91))

	runPrune(t, db, 0)

	require.Equal(t, int64(1), countRows(t, db, young),
		"a non-'processed' status must get the 90 day window, not the 30 day one")
	require.Equal(t, int64(0), countRows(t, db, old),
		"a non-'processed' status must still be pruned once past 90 days")
}

// The batch loop must terminate and delete everything eligible, not just one
// batch's worth.
func TestPrune_DeletesBeyondOneBatch(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	old := now.AddDate(0, 0, -31)
	for i := 0; i < 5; i++ {
		insertEvent(t, db, "processed", old)
	}

	stats := runPrune(t, db, 2) // batchSize 2
	require.Equal(t, int64(5), stats.ProcessedDeleted,
		"the loop must continue past the first batch")
}

// A cancelled context stops the pass without erroring out of Run — shutdown
// is not a prune failure.
func TestPrune_CancelledContextIsNotAFailure(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	insertEvent(t, db, "processed", now.AddDate(0, 0, -31))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cron := webhookprune.NewPruneCron(db, nil, func() time.Time { return now }, 0)
	stats, err := cron.Run(ctx)
	require.NoError(t, err, "Run swallows per-class failure by design")
	require.Equal(t, int64(0), stats.RowsDeleted())
	require.NotEmpty(t, stats.ErrorsByClass)
}

// The counter hook fires once per class with the class's own label.
func TestPrune_CounterHookReportsPerClass(t *testing.T) {
	db := testdb.NewDB(t, "webhook_events")

	insertEvent(t, db, "processed", now.AddDate(0, 0, -31))
	insertEvent(t, db, "received", now.AddDate(0, 0, -91))

	seen := map[string]int64{}
	cron := webhookprune.NewPruneCron(db, nil, func() time.Time { return now }, 0).
		WithCounter(func(label string, n int64) { seen[label] += n })
	_, err := cron.Run(context.Background())
	require.NoError(t, err)

	require.Equal(t, map[string]int64{
		webhookprune.ProcessedMetricLabel:   1,
		webhookprune.UnprocessedMetricLabel: 1,
	}, seen)
}
