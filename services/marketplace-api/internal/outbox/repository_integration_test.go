//go:build integration

package outbox_test

import (
	"context"
	"errors"
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
