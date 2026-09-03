//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func reloadRow(t *testing.T, db *gorm.DB, id string) outbox.OutboxEvent {
	t.Helper()
	var got outbox.OutboxEvent
	require.NoError(t, db.First(&got, "id = ?", id).Error)
	return got
}

// This is the single most important test in the ticket. The outbox exists
// to guarantee at-least-once delivery; a requeue that republishes an
// already-delivered event converts a delivery failure into a data
// corruption problem. published_at is the ONLY marker that a row was
// delivered — there is no status column and no attempt counter — so
// RequeueOne MUST refuse any row whose published_at is non-nil, and must
// leave that row completely untouched when it does.
func TestIntegration_RequeueOne_RefusesPublishedRow_DoublePublishGuard(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	pubAt := time.Now().UTC()
	require.NoError(t, db.Exec(`UPDATE outbox_events SET published_at = ? WHERE id = ?`, pubAt, evt.ID).Error)

	_, err := outbox.RequeueOne(context.Background(), db, evt.ID)
	require.Error(t, err)
	require.True(t, errors.Is(err, outbox.ErrAlreadyPublished),
		"requeuing a published row must be refused with ErrAlreadyPublished, got %v", err)

	got := reloadRow(t, db, evt.ID)
	require.NotNil(t, got.PublishedAt, "the row must remain published")
	require.Nil(t, got.Error)
	require.Nil(t, got.DeadLetteredAt)
	require.Nil(t, got.DeadLetterReason)
}

// The dead-letter counterpart to the double-publish guard: a delivered row
// cannot be dead-lettered either.
func TestIntegration_DeadLetterOne_RefusesPublishedRow(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	pubAt := time.Now().UTC()
	require.NoError(t, db.Exec(`UPDATE outbox_events SET published_at = ? WHERE id = ?`, pubAt, evt.ID).Error)

	_, err := outbox.DeadLetterOne(context.Background(), db, evt.ID, "operator says so")
	require.Error(t, err)
	require.True(t, errors.Is(err, outbox.ErrAlreadyPublished))

	got := reloadRow(t, db, evt.ID)
	require.Nil(t, got.DeadLetteredAt, "a published row must not become dead-lettered")
	require.Nil(t, got.DeadLetterReason)
}

// ProcessBatch's poll must skip a dead-lettered row EXPLICITLY (via
// dead_lettered_at IS NULL), not merely because it also has error set. A
// dead-letter written with a NULL error would otherwise be picked back up
// and delivered, defeating the whole operation.
func TestIntegration_ProcessBatch_SkipsDeadLetteredRows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)
	tenantID := uuid.NewString()

	deadLettered := makeEvent(tenantID)
	fresh := makeEvent(tenantID)
	enqueueCommitted(t, db, deadLettered)
	enqueueCommitted(t, db, fresh)

	// dead_lettered_at set, error deliberately left NULL: this is exactly
	// the shape the poll guard must catch on its own.
	require.NoError(t, db.Exec(
		`UPDATE outbox_events SET dead_lettered_at = now(), dead_letter_reason = 'test' WHERE id = ?`,
		deadLettered.ID).Error)

	var seen []string
	count, err := repo.ProcessBatch(context.Background(), 10,
		func(tx *gorm.DB, rows []outbox.OutboxEvent) error {
			for _, r := range rows {
				seen = append(seen, r.ID)
			}
			return nil
		})
	require.NoError(t, err)
	require.Equal(t, 1, count, "ProcessBatch must skip the dead-lettered row even though its error is NULL")
	require.Equal(t, []string{fresh.ID}, seen)
}

// Requeue clears error, clears BOTH dead-letter columns, and bumps
// created_at so the monotonic watermark actually moves — the original
// created_at is returned to the caller because requeue overwrites it and
// this is the only place it survives (the audit event carries it).
func TestIntegration_RequeueOne_ClearsColumnsAndBumpsCreatedAt(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	original := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	reason := outbox.ReasonPayloadMissingStoreID
	deadAt := time.Now().UTC().Add(-1 * time.Hour)
	deadReason := "test dead-letter"
	require.NoError(t, db.Exec(
		`UPDATE outbox_events SET created_at = ?, error = ?, dead_lettered_at = ?, dead_letter_reason = ? WHERE id = ?`,
		original, reason, deadAt, deadReason, evt.ID).Error)

	res, err := outbox.RequeueOne(context.Background(), db, evt.ID)
	require.NoError(t, err)
	require.Equal(t, evt.ID, res.ID)
	require.WithinDuration(t, original, res.OriginalCreatedAt, time.Second,
		"the ORIGINAL created_at must be returned so the audit event can carry it")

	got := reloadRow(t, db, evt.ID)
	require.Nil(t, got.Error, "error must be cleared")
	require.Nil(t, got.DeadLetteredAt, "dead_lettered_at must be cleared — dead-letter is reversible")
	require.Nil(t, got.DeadLetterReason, "dead_letter_reason must be cleared")
	require.Nil(t, got.PublishedAt)
	// created_at must be bumped forward from the seeded original (48h in
	// the past) to "now" — compared as a DURATION, not against a
	// test-process clock reading, since the DB server's clock need not
	// agree with the test host's to the millisecond.
	require.True(t, got.CreatedAt.UTC().After(original.Add(47*time.Hour)),
		"created_at must be bumped to (approximately) now() so the monotonic watermark actually "+
			"moves; got %s, original was %s", got.CreatedAt.UTC(), original)
}

// Batch requeue must return a per-row outcome so one bad id does not fail
// the rest of the set.
func TestIntegration_RequeueBatch_OneInvalidIDDoesNotFailTheRest(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()

	a := makeEvent(tenantID)
	b := makeEvent(tenantID)
	enqueueCommitted(t, db, a)
	enqueueCommitted(t, db, b)

	reason := outbox.ReasonPayloadUnparseable
	require.NoError(t, db.Exec(`UPDATE outbox_events SET error = ? WHERE id IN ?`, reason, []string{a.ID, b.ID}).Error)

	missingID := uuid.NewString()
	outcomes := outbox.RequeueBatch(context.Background(), db, []string{a.ID, missingID, b.ID})
	require.Len(t, outcomes, 3)

	byID := map[string]outbox.RequeueOutcome{}
	for _, o := range outcomes {
		byID[o.ID] = o
	}

	require.True(t, byID[a.ID].OK, "a must have been requeued")
	require.True(t, byID[b.ID].OK, "b must have been requeued despite the bad id in the same batch")
	require.False(t, byID[missingID].OK)
	require.Equal(t, "not_found", byID[missingID].Err)

	gotA := reloadRow(t, db, a.ID)
	require.Nil(t, gotA.Error)
	gotB := reloadRow(t, db, b.ID)
	require.Nil(t, gotB.Error)
}

// A batch requeue is refused per-row, not batch-wide, for an already
// published row too — that row's outcome must be false with the
// already_published code, and its siblings must still succeed.
func TestIntegration_RequeueBatch_PublishedRowOutcomeIsRefusedNotFatal(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()

	published := makeEvent(tenantID)
	pending := makeEvent(tenantID)
	enqueueCommitted(t, db, published)
	enqueueCommitted(t, db, pending)

	pubAt := time.Now().UTC()
	require.NoError(t, db.Exec(`UPDATE outbox_events SET published_at = ? WHERE id = ?`, pubAt, published.ID).Error)
	reason := outbox.ReasonPayloadUnparseable
	require.NoError(t, db.Exec(`UPDATE outbox_events SET error = ? WHERE id = ?`, reason, pending.ID).Error)

	outcomes := outbox.RequeueBatch(context.Background(), db, []string{published.ID, pending.ID})
	byID := map[string]outbox.RequeueOutcome{}
	for _, o := range outcomes {
		byID[o.ID] = o
	}

	require.False(t, byID[published.ID].OK)
	require.Equal(t, "already_published", byID[published.ID].Err)
	require.True(t, byID[pending.ID].OK)

	got := reloadRow(t, db, published.ID)
	require.NotNil(t, got.PublishedAt)
	require.Nil(t, got.Error, "published row untouched by the batch's error-clearing")
}

// Dead-letter rejects an empty reason: a dead-letter with no human
// explanation is exactly the gap this operation exists to close.
func TestIntegration_DeadLetterOne_RejectsEmptyReason(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	for _, reason := range []string{"", "   "} {
		_, err := outbox.DeadLetterOne(context.Background(), db, evt.ID, reason)
		require.Error(t, err, "reason=%q", reason)
		require.True(t, errors.Is(err, outbox.ErrReasonRequired), "reason=%q, got %v", reason, err)
	}

	got := reloadRow(t, db, evt.ID)
	require.Nil(t, got.DeadLetteredAt, "an empty reason must not dead-letter the row")
}

// DeadLetterOne's happy path: dead_lettered_at and the reason are set,
// error is untouched (dead-letter does not imply a failure reason), and
// published_at stays nil.
func TestIntegration_DeadLetterOne_SetsColumnsAndReason(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	before := time.Now().UTC()
	res, err := outbox.DeadLetterOne(context.Background(), db, evt.ID, "  confirmed duplicate order  ")
	require.NoError(t, err)
	require.Equal(t, evt.ID, res.ID)
	require.True(t, !res.DeadLetteredAt.Before(before))

	got := reloadRow(t, db, evt.ID)
	require.NotNil(t, got.DeadLetteredAt)
	require.NotNil(t, got.DeadLetterReason)
	require.Equal(t, "confirmed duplicate order", *got.DeadLetterReason, "reason must be trimmed")
	require.Nil(t, got.PublishedAt)
}

// Requeuing a not-found id is refused, not a silent no-op.
func TestIntegration_RequeueOne_NotFound(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	_, err := outbox.RequeueOne(context.Background(), db, uuid.NewString())
	require.Error(t, err)
	require.True(t, errors.Is(err, outbox.ErrNotFound))
}
