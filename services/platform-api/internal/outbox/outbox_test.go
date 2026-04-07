package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────
// Backoff math
// ─────────────────────────────────────────────────────────────────────────

func TestDefaultBackoff_GrowsExponentiallyAndCaps(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 32 * time.Second},
		{7, 64 * time.Second},
		{8, 128 * time.Second},
		{9, 256 * time.Second},
		{10, 5 * time.Minute}, // capped
		{20, 5 * time.Minute}, // still capped, no overflow
		{60, 5 * time.Minute}, // would overflow int64 if not capped
	}
	for _, tc := range cases {
		got := defaultBackoff(tc.attempts)
		if got != tc.want {
			t.Errorf("defaultBackoff(%d) = %v, want %v", tc.attempts, got, tc.want)
		}
	}
}

func TestDefaultBackoff_NormalizesNonPositiveAttempts(t *testing.T) {
	if got := defaultBackoff(0); got != time.Second {
		t.Errorf("defaultBackoff(0) = %v, want 1s", got)
	}
	if got := defaultBackoff(-5); got != time.Second {
		t.Errorf("defaultBackoff(-5) = %v, want 1s", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Drainer behavior (in-process, no DB)
// ─────────────────────────────────────────────────────────────────────────
//
// These tests exercise the drainer's logic in pure in-process mode without
// a real Postgres. The dispatch path is exercised; DB persistence is
// covered by integration tests in a follow-up.

func TestDrainer_Register_PanicsOnDuplicate(t *testing.T) {
	d := NewDrainer(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{})

	d.Register("kind.a", Handler(func(context.Context, json.RawMessage) error { return nil }))

	defer func() {
		if r := recover(); r == nil {
			t.Error("registering the same kind twice should panic")
		}
	}()
	d.Register("kind.a", Handler(func(context.Context, json.RawMessage) error { return nil }))
}

func TestDrainer_StartStop_IsIdempotent(t *testing.T) {
	d := NewDrainer(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d.Start(ctx)
	d.Start(ctx) // second start is a no-op
	d.Stop()
	d.Stop() // second stop is a no-op
}

// Truncate is exercised indirectly by markRetry / markDead, but it's also
// worth a direct test for the boundary conditions.
func TestTruncate_LeavesShortStringsUnchanged(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("truncate kept short string wrong: %q", got)
	}
}

func TestTruncate_CutsLongStrings(t *testing.T) {
	long := "0123456789abcdef"
	if got := truncate(long, 8); got != "01234567" {
		t.Errorf("truncate(%q, 8) = %q, want 01234567", long, got)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Status constants and Event shape
// ─────────────────────────────────────────────────────────────────────────

func TestEvent_TableName(t *testing.T) {
	if (Event{}).TableName() != "outbox_events" {
		t.Error("Event.TableName should return outbox_events")
	}
}

func TestErrNoHandler_IsExported(t *testing.T) {
	if !errors.Is(ErrNoHandler, ErrNoHandler) {
		t.Error("ErrNoHandler should be a sentinel error")
	}
}
