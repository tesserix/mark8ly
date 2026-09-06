package audit_test

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// countWorkerFrames reports how many goroutines currently have an
// audit.(*Emitter).worker frame on their stack. It parses a full stack
// dump (runtime.Stack(buf, all=true)) rather than comparing
// runtime.NumGoroutine() snapshots: a raw goroutine count is a global
// counter that any unrelated goroutine (GC, finalizers, another test in
// the package, a future t.Parallel) can perturb, making a
// before/after-count comparison a coin flip under any of those
// conditions. Grepping the dump for the specific frame this test cares
// about is immune to all of that — it is a deterministic zero-or-nonzero
// check, not a statistical one.
func countWorkerFrames(t *testing.T) int {
	t.Helper()
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	return strings.Count(string(buf), "audit.(*Emitter).worker(")
}

// TestNewEmitter_NilRepo_ReturnsErrorAndStartsNoWorkers pins the actual
// defect from #318: NewEmitter stored a nil Repo unguarded and had
// already started cfg.Workers goroutines that would panic on first
// dereference. An error return alone doesn't prove the fix — a version
// that errored *after* starting the workers would also pass an
// error-only test. This asserts no audit.(*Emitter).worker frame exists
// after the call, using a large Workers value so a leak is unmistakable.
func TestNewEmitter_NilRepo_ReturnsErrorAndStartsNoWorkers(t *testing.T) {
	em, err := audit.NewEmitter(audit.EmitterConfig{
		Repo:    nil,
		Logger:  slog.Default(),
		Workers: 8,
	})

	require.Error(t, err)
	require.Nil(t, em)

	require.Equal(t, 0, countWorkerFrames(t),
		"NewEmitter with a nil Repo must not leave any worker goroutines running")
}

// TestNilEmitter_AllExportedMethodsAreSafe pins the documented opt-out
// contract: a nil *Emitter is safe to call every exported method on —
// all twelve, enumerated by grepping the PACKAGE (every "func (e
// *Emitter)" across every file in internal/audit), not just emitter.go.
// A narrower enumeration is exactly how #318's follow-up review found
// EmitCredentialAccess, EmitPromoApplied, EmitPromoCancelled,
// EmitRefundIssued and EmitBillingArchived were pinned nowhere even
// though they live outside emitter.go and share the same delegation
// pattern.
//
// Emit, EmitSync, EmitTx and Stop guard the receiver explicitly; the other eight
// are safe only because they delegate to Emit without dereferencing e
// themselves — an implementation detail nothing else currently pins. If
// a future refactor made any of them touch e directly before calling
// Emit, this test is what would catch it.
func TestNilEmitter_AllExportedMethodsAreSafe(t *testing.T) {
	var e *audit.Emitter

	require.NotPanics(t, func() {
		e.Emit(nil, audit.Event{Action: "a", ResourceType: "b"})
	}, "Emit on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		err := e.EmitSync(nil, audit.Event{Action: "a", ResourceType: "b", TenantID: uuid.New()})
		require.Error(t, err, "EmitSync on a nil *Emitter must return an error, not silently succeed")
	}, "EmitSync on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		err := e.EmitTx(context.Background(), &gorm.DB{}, nil,
			audit.Event{Action: "a", ResourceType: "b", TenantID: uuid.New()})
		require.Error(t, err, "EmitTx on a nil *Emitter must return an error, not silently succeed")
	}, "EmitTx on a nil *Emitter must not panic")

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

	// The five below live outside emitter.go (credential_events.go,
	// promo_events.go, refund_events.go) and were the ones missed by the
	// original "AllSix" enumeration.

	require.NotPanics(t, func() {
		e.EmitCredentialAccess(nil, audit.CredentialAccess{
			TenantID: uuid.New(), StoreID: uuid.New(),
			CredentialType: "firebase_service_account", Operation: "read",
			Actor: "system:cron:lifecycle",
		})
	}, "EmitCredentialAccess on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		e.EmitPromoApplied(nil, audit.PromoApplied{
			TenantID: uuid.New(), StoreID: uuid.New(),
			Code: "LAUNCH20", Actor: "user:" + uuid.New().String(), Accepted: true,
		})
	}, "EmitPromoApplied on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		e.EmitPromoCancelled(nil, audit.PromoCancelled{
			TenantID: uuid.New(), StoreID: uuid.New(), PromoCodeID: uuid.New(),
			Actor: "user:" + uuid.New().String(),
		})
	}, "EmitPromoCancelled on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		e.EmitRefundIssued(nil, audit.RefundIssued{
			TenantID: uuid.New(), StoreID: uuid.New(),
			StripeChargeID: "ch_123", Actor: "user:" + uuid.New().String(), Accepted: true,
		})
	}, "EmitRefundIssued on a nil *Emitter must not panic")

	require.NotPanics(t, func() {
		e.EmitBillingArchived(nil, audit.BillingArchived{
			TenantID: uuid.New(), StoreID: uuid.New(), ArchiveID: uuid.New(),
			Op: "created", Actor: "system:cron:archive",
		})
	}, "EmitBillingArchived on a nil *Emitter must not panic")
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

// TestNewEmitter_NilLoggerDefaultsRatherThanPanics pins Important 4: a
// caller that omits cfg.Logger (as mustEmitter and several other
// call-sites do) must not get a nil e.logger — every write-path log call
// (Emit's drop/queue-full warnings, write()'s insert-failure error, both
// on the worker goroutine) would nil-deref and take down the process,
// the same failure shape as #318, the moment the repo returns an error.
func TestNewEmitter_NilLoggerDefaultsRatherThanPanics(t *testing.T) {
	repo := &recordingRepo{createErr: errRepoWrite{}}
	e, err := audit.NewEmitter(audit.EmitterConfig{Repo: repo}) // no Logger
	require.NoError(t, err)

	require.NotPanics(t, func() {
		e.Emit(nil, audit.Event{
			Action: "tenant.suspend", ResourceType: "tenant", TenantID: uuid.New(),
		})
		e.Stop(context.Background())
	}, "a nil cfg.Logger must default rather than panic when the worker logs an insert failure")
}

// errRepoWrite is a stand-in error so the insert-failure path (write()'s
// e.logger.Error call) actually executes in
// TestNewEmitter_NilLoggerDefaultsRatherThanPanics.
type errRepoWrite struct{}

func (errRepoWrite) Error() string { return "boom" }

// TestEmitSync_RealRepositoryNilDB_ReturnsErrorRatherThanPanics pins the
// second instance of the #318 failure shape, one layer down from the
// nil-Repo guard above: gormRepository.Create's own nil-*gorm.DB check
// (repository.go:124-131). Without it, db.WithContext(ctx) inside Create
// dereferences a nil *gorm.DB on the calling goroutine and panics.
//
// Every other "DB: nil" test in this file (TestNewEmitter_ValidConfig_...,
// TestNewEmitter_NilLoggerDefaultsRatherThanPanics) uses recordingRepo,
// an in-memory double that never touches db — they'd pass unchanged even
// if repository.go's guard were deleted. This test uses the real
// audit.NewRepository() specifically so it exercises that guard, and
// gives the event a valid TenantID so it reaches Create instead of being
// dropped earlier by buildEntry's missing-tenant check (the only other
// test using a real repository, TestEmit_MissingTenant_LogsDrop in
// emitter_test.go, never reaches Create at all).
//
// EmitSync is used rather than the async Emit path because it returns
// the Create error directly, giving a real assertion instead of relying
// on a log line.
func TestEmitSync_RealRepositoryNilDB_ReturnsErrorRatherThanPanics(t *testing.T) {
	e, err := audit.NewEmitter(audit.EmitterConfig{
		DB:     nil,
		Repo:   audit.NewRepository(),
		Logger: slog.Default(),
	})
	require.NoError(t, err)
	require.NotNil(t, e)
	t.Cleanup(func() { e.Stop(context.Background()) })

	var syncErr error
	require.NotPanics(t, func() {
		syncErr = e.EmitSync(nil, audit.Event{
			Action:       "tenant.suspend",
			ResourceType: "tenant",
			TenantID:     uuid.New(),
		})
	}, "EmitSync with a real Repository and a nil *gorm.DB must not panic")

	require.Error(t, syncErr, "EmitSync must surface the nil-db condition as an error")
	require.Contains(t, syncErr.Error(), "db is nil")
}
