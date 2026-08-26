//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func makeEvent(tenantID string) *outbox.OutboxEvent {
	return &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventProductCreated,
		Payload:     datatypes.JSON([]byte(`{"store_id":"11111111-1111-1111-1111-111111111111"}`)),
	}
}

func enqueueCommitted(t *testing.T, db *gorm.DB, evt *outbox.OutboxEvent) {
	t.Helper()
	if err := db.Create(evt).Error; err != nil {
		t.Fatalf("enqueue: %v", err)
	}
}

func countUnpublished(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT count(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&n).Error; err != nil {
		t.Fatalf("count unpublished: %v", err)
	}
	return n
}

func TestIntegration_EnqueueInTx_RollbackDropsRow(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)
	ctx := context.Background()

	forced := errors.New("forced rollback")
	err := db.Transaction(func(tx *gorm.DB) error {
		evt := makeEvent("00000000-0000-0000-0000-000000000001")
		if err := repo.EnqueueInTx(ctx, tx, evt); err != nil {
			return err
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("expected forced rollback err, got %v", err)
	}

	var n int64
	if err := db.Raw(`SELECT count(*) FROM outbox_events`).Scan(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", n)
	}
}

func TestIntegration_ProcessBatch_HappyPath(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)
	ctx := context.Background()

	tenantID := "00000000-0000-0000-0000-000000000001"
	wantIDs := map[string]bool{}
	for i := 0; i < 3; i++ {
		evt := makeEvent(tenantID)
		enqueueCommitted(t, db, evt)
		wantIDs[evt.ID] = true
	}

	var seenIDs []string
	count, err := repo.ProcessBatch(ctx, 10, func(tx *gorm.DB, rows []outbox.OutboxEvent) error {
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		seenIDs = ids
		return repo.MarkPublishedInTx(tx, ids)
	})
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count=3, got %d", count)
	}
	if len(seenIDs) != 3 {
		t.Fatalf("expected 3 seen ids, got %d", len(seenIDs))
	}
	for _, id := range seenIDs {
		if !wantIDs[id] {
			t.Fatalf("unexpected id %s", id)
		}
	}
	if n := countUnpublished(t, db); n != 0 {
		t.Fatalf("expected 0 unpublished, got %d", n)
	}
}

func TestIntegration_ProcessBatch_SkipLocked_Concurrent(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)
	ctx := context.Background()

	tenantID := "00000000-0000-0000-0000-000000000001"
	allIDs := map[string]bool{}
	for i := 0; i < 4; i++ {
		evt := makeEvent(tenantID)
		enqueueCommitted(t, db, evt)
		allIDs[evt.ID] = true
	}

	var (
		mu        sync.Mutex
		perGR     = make(map[int][]string)
		wg        sync.WaitGroup
		countsErr error
	)

	run := func(gid int) {
		defer wg.Done()
		_, err := repo.ProcessBatch(ctx, 2, func(tx *gorm.DB, rows []outbox.OutboxEvent) error {
			ids := make([]string, 0, len(rows))
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			time.Sleep(100 * time.Millisecond)
			if err := repo.MarkPublishedInTx(tx, ids); err != nil {
				return err
			}
			mu.Lock()
			perGR[gid] = ids
			mu.Unlock()
			return nil
		})
		if err != nil {
			mu.Lock()
			countsErr = err
			mu.Unlock()
		}
	}

	wg.Add(2)
	go run(1)
	go run(2)
	wg.Wait()

	if countsErr != nil {
		t.Fatalf("ProcessBatch goroutine err: %v", countsErr)
	}

	if len(perGR[1]) != 2 {
		t.Fatalf("gr1 expected 2 rows, got %d", len(perGR[1]))
	}
	if len(perGR[2]) != 2 {
		t.Fatalf("gr2 expected 2 rows, got %d", len(perGR[2]))
	}

	union := map[string]int{}
	for _, id := range perGR[1] {
		union[id]++
	}
	for _, id := range perGR[2] {
		union[id]++
	}
	if len(union) != 4 {
		t.Fatalf("expected union of 4 unique ids, got %d", len(union))
	}
	for id, c := range union {
		if c != 1 {
			t.Fatalf("id %s seen %d times, expected 1", id, c)
		}
		if !allIDs[id] {
			t.Fatalf("unexpected id %s in union", id)
		}
	}
}

func TestIntegration_ProcessBatch_Ordering(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)
	ctx := context.Background()

	tenant1 := "00000000-0000-0000-0000-000000000001"
	tenant2 := "00000000-0000-0000-0000-000000000002"

	now := time.Now().UTC()

	// Backdate helper: create the row, then UPDATE its created_at.
	backdate := func(evt *outbox.OutboxEvent, ts time.Time) {
		enqueueCommitted(t, db, evt)
		if err := db.Exec(`UPDATE outbox_events SET created_at = ? WHERE id = ?`, ts, evt.ID).Error; err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	// Tenant 2 first, to prove ordering isn't insertion order.
	t2a := makeEvent(tenant2)
	backdate(t2a, now.Add(-1*time.Hour))
	t2b := makeEvent(tenant2)
	backdate(t2b, now.Add(-30*time.Minute))

	t1a := makeEvent(tenant1)
	backdate(t1a, now.Add(-5*time.Minute))
	t1b := makeEvent(tenant1)
	backdate(t1b, now.Add(-10*time.Minute))
	t1c := makeEvent(tenant1)
	backdate(t1c, now.Add(-15*time.Minute))

	var captured []outbox.OutboxEvent
	count, err := repo.ProcessBatch(ctx, 10, func(tx *gorm.DB, rows []outbox.OutboxEvent) error {
		captured = make([]outbox.OutboxEvent, len(rows))
		copy(captured, rows)
		return nil // don't mark published
	})
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected 5 rows, got %d", count)
	}

	wantOrder := []string{t1c.ID, t1b.ID, t1a.ID, t2a.ID, t2b.ID}
	if len(captured) != len(wantOrder) {
		t.Fatalf("captured len %d, want %d", len(captured), len(wantOrder))
	}
	for i, w := range wantOrder {
		if captured[i].ID != w {
			t.Fatalf("position %d: got id=%s tenant=%s created=%s, want id=%s",
				i, captured[i].ID, captured[i].TenantID, captured[i].CreatedAt, w)
		}
	}
}

func TestIntegration_MarkFailedInTx_SetsReasonAndLeavesUnpublished(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	bad := makeEvent(tenantID)
	good := makeEvent(tenantID)
	enqueueCommitted(t, db, bad)
	enqueueCommitted(t, db, good)

	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: bad.ID, Reason: outbox.ReasonPayloadUnparseable},
		})
	})
	if err != nil {
		t.Fatalf("MarkFailedInTx: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", bad.ID).Error; err != nil {
		t.Fatalf("reload failed row: %v", err)
	}
	if got.Error == nil {
		t.Fatalf("error is nil; want %q", outbox.ReasonPayloadUnparseable)
	}
	// Assert the EXACT code, not merely non-nil: a stub returns the zero
	// value for a field nobody set.
	if *got.Error != outbox.ReasonPayloadUnparseable {
		t.Fatalf("error = %q, want %q", *got.Error, outbox.ReasonPayloadUnparseable)
	}
	if got.PublishedAt != nil {
		t.Fatalf("a failed row must stay unpublished, got published_at=%v", got.PublishedAt)
	}

	var untouched outbox.OutboxEvent
	if err := db.First(&untouched, "id = ?", good.ID).Error; err != nil {
		t.Fatalf("reload untouched row: %v", err)
	}
	if untouched.Error != nil {
		t.Fatalf("unrelated row was marked failed: %q", *untouched.Error)
	}
}

func TestIntegration_MarkFailedInTx_GroupsDistinctReasons(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	unparseable := makeEvent(tenantID)
	missingStore := makeEvent(tenantID)
	enqueueCommitted(t, db, unparseable)
	enqueueCommitted(t, db, missingStore)

	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: unparseable.ID, Reason: outbox.ReasonPayloadUnparseable},
			{ID: missingStore.ID, Reason: outbox.ReasonPayloadMissingStoreID},
		})
	})
	if err != nil {
		t.Fatalf("MarkFailedInTx: %v", err)
	}

	for _, tc := range []struct {
		id   string
		want string
	}{
		{unparseable.ID, outbox.ReasonPayloadUnparseable},
		{missingStore.ID, outbox.ReasonPayloadMissingStoreID},
	} {
		var got outbox.OutboxEvent
		if err := db.First(&got, "id = ?", tc.id).Error; err != nil {
			t.Fatalf("reload %s: %v", tc.id, err)
		}
		if got.Error == nil || *got.Error != tc.want {
			t.Fatalf("row %s error = %v, want %q", tc.id, got.Error, tc.want)
		}
	}
}

func TestIntegration_MarkFailedInTx_EmptyIsNoOp(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, nil)
	})
	if err != nil {
		t.Fatalf("empty MarkFailedInTx must be a no-op, got: %v", err)
	}
}

func TestIntegration_MarkFailedInTx_CoercesUnrecognisedReason(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	// Shaped like a real encoding/json error: it quotes the offending input,
	// which is exactly how customer payload data would reach this column.
	raw := `json: cannot unmarshal string into Go value of type map[string]interface {} "acme-secret-sku"`

	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{{ID: evt.ID, Reason: raw}})
	}); err != nil {
		t.Fatalf("MarkFailedInTx: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Error == nil {
		t.Fatalf("error is nil; want %q", outbox.ReasonUnknown)
	}
	if *got.Error != outbox.ReasonUnknown {
		t.Fatalf("error = %q, want %q", *got.Error, outbox.ReasonUnknown)
	}
	if strings.Contains(*got.Error, "acme-secret-sku") {
		t.Fatalf("payload fragment leaked into outbox_events.error: %q", *got.Error)
	}
	if got.PublishedAt != nil {
		t.Fatalf("a failed row must stay unpublished, got published_at=%v", got.PublishedAt)
	}
}

func TestIntegration_MarkFailedInTx_DuplicateIDFirstReasonWins(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: evt.ID, Reason: outbox.ReasonPayloadUnparseable},
			{ID: evt.ID, Reason: outbox.ReasonPayloadMissingStoreID},
		})
	}); err != nil {
		t.Fatalf("MarkFailedInTx: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	// Deterministic: the FIRST entry wins, regardless of map iteration order.
	if got.Error == nil || *got.Error != outbox.ReasonPayloadUnparseable {
		t.Fatalf("error = %v, want %q (first occurrence wins)", got.Error, outbox.ReasonPayloadUnparseable)
	}
}

// A row with error set is TERMINAL. This is the poison-pill proof: nothing
// else in the suite exercises the poll's error IS NULL term, which exists
// precisely so a permanently-failing row cannot be re-selected forever and
// starve real events out of the batch window.
func TestIntegration_ProcessBatch_SkipsFailedRows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	failed := makeEvent(tenantID)
	fresh := makeEvent(tenantID)
	enqueueCommitted(t, db, failed)
	enqueueCommitted(t, db, fresh)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: failed.ID, Reason: outbox.ReasonPayloadUnparseable},
		})
	}); err != nil {
		t.Fatalf("seed failed row: %v", err)
	}

	var seen []string
	count, err := repo.ProcessBatch(context.Background(), 10,
		func(tx *gorm.DB, rows []outbox.OutboxEvent) error {
			for _, r := range rows {
				seen = append(seen, r.ID)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if count != 1 {
		t.Fatalf("ProcessBatch saw %d rows, want 1 (the failed row must be skipped)", count)
	}
	if len(seen) != 1 || seen[0] != fresh.ID {
		t.Fatalf("ProcessBatch saw %v, want only the un-failed row %s", seen, fresh.ID)
	}
}

// Clearing error is the documented requeue path for an operator. It must
// actually work, or "terminal" means "lost".
func TestIntegration_ProcessBatch_ClearingErrorRequeues(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: evt.ID, Reason: outbox.ReasonPayloadMissingStoreID},
		})
	}); err != nil {
		t.Fatalf("seed failed row: %v", err)
	}

	if err := db.Exec(`UPDATE outbox_events SET error = NULL WHERE id = ?`, evt.ID).Error; err != nil {
		t.Fatalf("clear error: %v", err)
	}

	count, err := repo.ProcessBatch(context.Background(), 10,
		func(tx *gorm.DB, rows []outbox.OutboxEvent) error { return nil })
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if count != 1 {
		t.Fatalf("ProcessBatch saw %d rows, want 1 after error was cleared", count)
	}
}

// Guard against adding a reason constant without allowlisting it in
// sanitizeReason, which would silently record "unknown" instead.
func TestIntegration_MarkFailedInTx_StoreNotFoundIsInTheVocabulary(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	evt := makeEvent(tenantID)
	enqueueCommitted(t, db, evt)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.MarkFailedInTx(tx, []outbox.Failure{
			{ID: evt.ID, Reason: outbox.ReasonStoreNotFound},
		})
	}); err != nil {
		t.Fatalf("MarkFailedInTx: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Error == nil || *got.Error != outbox.ReasonStoreNotFound {
		t.Fatalf("error = %v, want %q (is it in sanitizeReason's switch?)", got.Error, outbox.ReasonStoreNotFound)
	}
}
