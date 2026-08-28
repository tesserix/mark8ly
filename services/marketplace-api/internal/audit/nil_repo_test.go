package audit_test

import (
	"context"
	"log/slog"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// waitForGoroutineCountSettle polls runtime.NumGoroutine() until two
// consecutive samples agree, so a baseline/after comparison isn't thrown
// off by unrelated goroutines (GC, finalizers, test runner bookkeeping)
// that happen to be mid-exit. Goroutine counts are inherently racy; this
// does not eliminate that, it just avoids sampling mid-churn.
func waitForGoroutineCountSettle(t *testing.T) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	last := runtime.NumGoroutine()
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		cur := runtime.NumGoroutine()
		if cur == last {
			return cur
		}
		last = cur
	}
	return last
}

// TestNewEmitter_NilRepo_ReturnsErrorAndStartsNoWorkers pins the actual
// defect from #318: NewEmitter stored a nil Repo unguarded and had
// already started cfg.Workers goroutines that would panic on first
// dereference. An error return alone doesn't prove the fix — a version
// that errored *after* starting the workers would also pass a
// error-only test. This asserts the goroutine count as well, using a
// large Workers value so a leak is unmistakable.
func TestNewEmitter_NilRepo_ReturnsErrorAndStartsNoWorkers(t *testing.T) {
	before := waitForGoroutineCountSettle(t)

	em, err := audit.NewEmitter(audit.EmitterConfig{
		Repo:    nil,
		Logger:  slog.Default(),
		Workers: 8,
	})

	require.Error(t, err)
	require.Nil(t, em)

	after := waitForGoroutineCountSettle(t)
	require.Equal(t, before, after,
		"NewEmitter with a nil Repo must not leave any worker goroutines running")
}

// TestNilEmitter_AllSixExportedMethodsAreSafe pins the documented
// opt-out contract: a nil *Emitter is safe to call every exported
// method on. Emit, EmitSync and Stop guard the receiver explicitly;
// EmitStateTransition, EmitAPIKeyEvent and EmitPlanChange are safe only
// because they delegate to Emit without dereferencing e themselves — an
// implementation detail nothing else currently pins. If a future
// refactor made any of the three delegating methods touch e directly
// before calling Emit, this test is what would catch it.
func TestNilEmitter_AllSixExportedMethodsAreSafe(t *testing.T) {
	var e *audit.Emitter

	require.NotPanics(t, func() {
		e.Emit(nil, audit.Event{Action: "a", ResourceType: "b"})
	}, "Emit on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		err := e.EmitSync(nil, audit.Event{Action: "a", ResourceType: "b", TenantID: uuid.New()})
		require.Error(t, err, "EmitSync on a nil *Emitter must return an error, not silently succeed")
	}, "EmitSync on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		e.Stop(ctx)
	}, "Stop on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		e.EmitStateTransition(nil, audit.StateTransition{
			StoreID: uuid.New(), TenantID: uuid.New(),
			From: "trial", To: "active", Actor: "system:webhook:stripe",
		})
	}, "EmitStateTransition on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		e.EmitAPIKeyEvent(nil, audit.APIKeyEvent{
			TenantID: uuid.New(), StoreID: uuid.New(), KeyID: uuid.New(),
			KeyPrefix: "pfx", Action: "created",
		})
	}, "EmitAPIKeyEvent on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		e.EmitPlanChange(nil, audit.PlanChange{
			TenantID: uuid.New(), StoreID: uuid.New(),
			FromPlan: "starter", ToPlan: "growth", Subaction: "upgrade_committed",
			Actor: "system:cron:downgrade_recheck",
		})
	}, "EmitPlanChange on a nil *Emitter must not panic")
}

// TestNewEmitter_ValidConfig_StartsWorkersAndWrites is the happy-path
// guard: a valid Repo constructs without error, the worker(s) actually
// drain the queue, and a write reaches the repository. No database
// involved — recordingRepo is an in-memory test double.
func TestNewEmitter_ValidConfig_StartsWorkersAndWrites(t *testing.T) {
	repo := &recordingRepo{}
	e, err := audit.NewEmitter(audit.EmitterConfig{Repo: repo, Logger: slog.Default()})
	require.NoError(t, err)
	require.NotNil(t, e)

	tenantID := uuid.New()
	e.Emit(nil, audit.Event{
		Action:       "tenant.suspend",
		ResourceType: "tenant",
		TenantID:     tenantID,
	})

	// Emit is async; Stop drains the queue before returning, so calling
	// it (once, here) gives a deterministic point at which the write is
	// guaranteed to have landed. Stop is not safe to call twice (it
	// closes a channel), so this replaces rather than supplements a
	// t.Cleanup call.
	e.Stop(context.Background())

	require.Len(t, repo.created, 1)
	require.Equal(t, tenantID, repo.created[0].TenantID)
	require.Equal(t, "tenant.suspend", repo.created[0].Action)
}
