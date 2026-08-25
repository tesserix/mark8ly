//go:build integration

package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// TestIntegration_Enqueue_PersistsRow exercises the real DB path.
// pkg/testdb.NewDB truncates outbox_events on cleanup so each test runs
// against a known-empty table.
func TestIntegration_Enqueue_PersistsRow(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")

	type payload struct {
		Foo string `json:"foo"`
	}
	if err := Enqueue(db, "test.kind", payload{Foo: "bar"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var got Event
	if err := db.First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Kind != "test.kind" {
		t.Errorf("Kind = %q, want %q", got.Kind, "test.kind")
	}
	if got.Status != StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, StatusPending)
	}
	if got.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", got.Attempts)
	}

	var p payload
	if err := json.Unmarshal(got.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Foo != "bar" {
		t.Errorf("payload.Foo = %q, want bar", p.Foo)
	}
}

// TestIntegration_Drainer_DispatchesAndMarksCompleted is the happy-path
// regression test: enqueue → drain → row is marked completed.
func TestIntegration_Drainer_DispatchesAndMarksCompleted(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	var called atomic.Int32
	d := NewDrainer(db, log, Config{})
	d.Register("test.kind", func(ctx context.Context, p json.RawMessage) error {
		called.Add(1)
		return nil
	})

	if err := Enqueue(db, "test.kind", map[string]string{"key": "value"}); err != nil {
		t.Fatal(err)
	}

	if err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if called.Load() != 1 {
		t.Errorf("handler called %d times, want 1", called.Load())
	}

	var row Event
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != StatusCompleted {
		t.Errorf("Status after success = %q, want %q", row.Status, StatusCompleted)
	}
	if row.CompletedAt == nil {
		t.Error("CompletedAt should be set after success")
	}
}

// TestIntegration_Drainer_RetriesOnHandlerError exercises auth-bug #3:
// when the handler fails, the row stays pending, attempts++, next_attempt_at
// is bumped into the future.
func TestIntegration_Drainer_RetriesOnHandlerError(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	d := NewDrainer(db, log, Config{
		// Aggressive backoff so the test can verify the timestamp moved.
		Backoff: func(_ int) time.Duration { return time.Hour },
	})
	d.Register("test.kind", func(ctx context.Context, p json.RawMessage) error {
		return errors.New("simulated handler failure")
	})

	if err := Enqueue(db, "test.kind", map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}

	if err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	var row Event
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != StatusPending {
		t.Errorf("Status after failure = %q, want pending (so the drainer retries)", row.Status)
	}
	if row.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", row.Attempts)
	}
	if row.LastError == "" {
		t.Error("LastError should be populated after failure")
	}
	// next_attempt_at should be in the future (we set backoff to 1h).
	if !row.NextAttemptAt.After(time.Now().Add(30 * time.Minute)) {
		t.Errorf("NextAttemptAt should be ~1h in the future, got %v", row.NextAttemptAt)
	}
}

// TestIntegration_Drainer_MarksDeadAfterMaxAttempts asserts that after
// MaxAttempts failures the row is moved out of the retry pool so the
// drainer doesn't loop on permanently broken events.
func TestIntegration_Drainer_MarksDeadAfterMaxAttempts(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	d := NewDrainer(db, log, Config{
		MaxAttempts: 2,
		Backoff:     func(_ int) time.Duration { return 0 }, // immediate retry
	})
	d.Register("test.kind", func(ctx context.Context, p json.RawMessage) error {
		return errors.New("permanent failure")
	})

	if err := Enqueue(db, "test.kind", map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}

	// Two ticks → two attempts → second attempt hits maxAttempts → status=dead.
	for i := 0; i < 2; i++ {
		if err := d.Tick(context.Background()); err != nil {
			t.Fatalf("Tick %d: %v", i, err)
		}
	}

	var row Event
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != StatusDead {
		t.Errorf("Status after maxAttempts = %q, want %q", row.Status, StatusDead)
	}
}

// TestIntegration_Drainer_SkipsFutureAttempts confirms the drainer respects
// next_attempt_at and doesn't dispatch events scheduled for the future.
func TestIntegration_Drainer_SkipsFutureAttempts(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	var called atomic.Int32
	d := NewDrainer(db, log, Config{})
	d.Register("test.kind", func(ctx context.Context, p json.RawMessage) error {
		called.Add(1)
		return nil
	})

	// Enqueue then push next_attempt_at into the future.
	if err := Enqueue(db, "test.kind", map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Event{}).Where("1 = 1").Update("next_attempt_at", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	if err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if called.Load() != 0 {
		t.Errorf("handler called %d times, want 0 (event scheduled for the future)", called.Load())
	}
}

// TestIntegration_Enqueue_RollsBackWithTransaction is the structural
// guarantee for auth-bug #2: if the originating tx aborts, the outbox row
// must NOT exist. Otherwise we'd ship FGA writes for tenants that were
// never created.
func TestIntegration_Enqueue_RollsBackWithTransaction(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")

	tx := db.Begin()
	if err := Enqueue(tx, "test.kind", map[string]string{"k": "v"}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.Model(&Event{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("rolled-back tx left %d outbox row(s); should be 0", count)
	}
}

// TestIntegration_EnqueueAfter_IsNotDrainedBeforeItsDelay is the property the
// operator tenant purge (#288) depends on: an event enqueued as a BACKSTOP for
// work the same request also does inline must not be claimable while that
// inline work is still running.
//
// Both halves are asserted against ONE drainer and ONE event, because the
// failure this guards against is "the delay is written but Tick ignores it" —
// and a test that only checked the early Tick would pass against an event that
// never becomes claimable at all.
func TestIntegration_EnqueueAfter_IsNotDrainedBeforeItsDelay(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	var called atomic.Int32
	d := NewDrainer(db, log, Config{})
	d.Register("test.delayed", func(ctx context.Context, p json.RawMessage) error {
		called.Add(1)
		return nil
	})

	if err := EnqueueAfter(db, "test.delayed", map[string]string{"k": "v"}, 30*time.Second); err != nil {
		t.Fatalf("EnqueueAfter: %v", err)
	}

	// The row is durable immediately — the delay defers ELIGIBILITY, not the
	// write. A backstop that isn't committed with the transaction is no
	// backstop at all.
	var row Event
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.Status != StatusPending {
		t.Errorf("Status = %q, want %q", row.Status, StatusPending)
	}
	if !row.NextAttemptAt.After(time.Now().Add(25 * time.Second)) {
		t.Errorf("NextAttemptAt = %v, want ~30s in the future", row.NextAttemptAt)
	}

	// Half 1: not claimable yet.
	if err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if called.Load() != 0 {
		t.Fatalf("handler called %d times before the delay elapsed, want 0 — the drainer is racing the inline purge", called.Load())
	}

	// Half 2: once due, it drains normally. Move the row's due time into the
	// past rather than sleeping 30s.
	if err := db.Model(&Event{}).Where("id = ?", row.ID).
		Update("next_attempt_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("age the row: %v", err)
	}
	if err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after delay: %v", err)
	}
	if called.Load() != 1 {
		t.Errorf("handler called %d times after the delay elapsed, want 1 — the backstop must still fire", called.Load())
	}
}
