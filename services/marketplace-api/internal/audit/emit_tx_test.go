package audit_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// txRecordingRepo records the two arguments EmitTx is responsible for
// forwarding — the *gorm.DB handle and the context.Context — alongside the
// Entry. recordingRepo (emitter_test.go) deliberately discards both, which is
// right for the tests that use it and useless here: every assertion below is
// about WHICH handle and WHICH context reached the repository.
//
// ctxErr captures ctx.Err() at call time rather than retaining ctx, because a
// context is only meaningfully inspectable while the call is in flight.
type txRecordingRepo struct {
	calls     int
	gotDB     *gorm.DB
	gotCtxErr error
	created   []*audit.Entry
	createErr error
	// ctxErrAsCreateErr makes Create fail when the caller's context is
	// already cancelled, the way a real driver does. Without it a
	// forwarded-but-cancelled context is indistinguishable from a fresh one
	// at the call site.
	ctxErrAsCreateErr bool
}

func (r *txRecordingRepo) Create(ctx context.Context, db *gorm.DB, e *audit.Entry) error {
	r.calls++
	r.gotDB = db
	r.gotCtxErr = ctx.Err()
	if r.ctxErrAsCreateErr && ctx.Err() != nil {
		return ctx.Err()
	}
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, e)
	return nil
}

func (r *txRecordingRepo) List(_ context.Context, _ *gorm.DB, _ audit.ListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

func (r *txRecordingRepo) Stream(_ context.Context, _ *gorm.DB, _ audit.ListFilter, _ func(*audit.Entry) error) error {
	return nil
}

func (r *txRecordingRepo) ListPlatform(_ context.Context, _ *gorm.DB, _ audit.PlatformListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

// newTxEmitter builds an Emitter whose cfg.DB is a distinct, non-nil sentinel
// so a test can tell "wrote on the handle I passed" apart from "fell back to
// the emitter's own handle". The sentinel is never dereferenced:
// txRecordingRepo only compares the pointer, and NewEmitter's doc comment
// states the Emitter treats cfg.DB as opaque data it forwards to Repo.Create.
func newTxEmitter(t *testing.T, repo audit.Repository) (*audit.Emitter, *gorm.DB) {
	t.Helper()
	emitterDB := &gorm.DB{}
	e, err := audit.NewEmitter(audit.EmitterConfig{DB: emitterDB, Repo: repo, Logger: slog.Default()})
	require.NoError(t, err)
	t.Cleanup(func() { e.Stop(context.Background()) })
	return e, emitterDB
}

// TestEmitTx_WritesOnTheSuppliedHandleNotTheEmittersOwn is the whole point of
// EmitTx: the insert must go through the caller's transaction, so the audit
// row and the state change it describes share a fate. Passing e.db instead
// would produce a row that commits independently — the exact failure
// tesserix-home#331 asks this function to prevent — and would still pass a
// test that only asserted "a row was created".
//
// It also asserts the operator attribution, which is not incidental: it is
// the evidence that EmitTx reuses buildEntry rather than assembling an Entry
// of its own. Actor type, operator id and capability all come from there.
func TestEmitTx_WritesOnTheSuppliedHandleNotTheEmittersOwn(t *testing.T) {
	repo := &txRecordingRepo{}
	e, emitterDB := newTxEmitter(t, repo)

	tx := &gorm.DB{}
	tenantID := uuid.New()

	err := e.EmitTx(context.Background(), tx, ginContextWithOperator(t, "op-7", "billing.discount"), audit.Event{
		Action: "tenant.discount_applied", ResourceType: "subscription",
		TenantID: tenantID, Metadata: map[string]any{"coupon_id": "co_123"},
	})

	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	require.Same(t, tx, repo.gotDB, "EmitTx must insert on the caller's transaction handle")
	require.NotSame(t, emitterDB, repo.gotDB, "EmitTx must not fall back to the Emitter's own handle")

	require.Equal(t, tenantID, repo.created[0].TenantID)
	require.Equal(t, audit.ActorOperator, repo.created[0].ActorType)
	require.Equal(t, "op-7", *repo.created[0].ActorOperatorID)
	require.Equal(t, "billing.discount", *repo.created[0].Capability)
}

// TestEmitTx_UsesTheCallersContext pins the deliberate divergence from
// EmitSync and write(), both of which substitute a fresh
// context.Background() with a timeout. An insert on someone else's
// transaction must share that transaction's cancellation; substituting a
// background context here would leave the insert running against a handle
// whose transaction is already being torn down.
//
// The assertion is on ctx.Err() observed INSIDE Create: a test that merely
// passed a live context would pass unchanged if EmitTx swapped in a
// background one.
func TestEmitTx_UsesTheCallersContext(t *testing.T) {
	repo := &txRecordingRepo{ctxErrAsCreateErr: true}
	e, _ := newTxEmitter(t, repo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := e.EmitTx(ctx, &gorm.DB{}, ginContextWithOperator(t, "op-7", "billing.discount"), audit.Event{
		Action: "tenant.discount_applied", ResourceType: "subscription", TenantID: uuid.New(),
	})

	require.Equal(t, 1, repo.calls, "EmitTx must still reach the repository; the context is the caller's to cancel")
	require.ErrorIs(t, repo.gotCtxErr, context.Canceled,
		"the caller's cancelled context must reach Create, not a fresh context.Background()")
	require.ErrorIs(t, err, context.Canceled, "the cancellation must be surfaced to the caller")
}

// TestEmitTx_NilTxIsRefusedWithoutFallingBackToTheEmittersDB guards the
// quietest way to get this wrong: a caller who forgets the transaction gets a
// non-transactional audit row that looks entirely correct in the table. An
// error is the only outcome that surfaces the mistake.
func TestEmitTx_NilTxIsRefusedWithoutFallingBackToTheEmittersDB(t *testing.T) {
	repo := &txRecordingRepo{}
	e, _ := newTxEmitter(t, repo)

	err := e.EmitTx(context.Background(), nil, ginContextWithOperator(t, "op-7", "billing.discount"), audit.Event{
		Action: "tenant.discount_applied", ResourceType: "subscription", TenantID: uuid.New(),
	})

	require.Error(t, err)
	require.Zero(t, repo.calls, "a nil transaction must not be silently replaced by the Emitter's own handle")
	require.Empty(t, repo.created)
}

// TestEmitTx_NilReceiverIsAnError mirrors EmitSync: the documented opt-out
// (a nil *Emitter) must stay safe to call, but a TRANSACTIONAL audit that
// quietly does nothing is the precise failure this function exists to
// prevent, so the nil receiver reports rather than returns silently.
func TestEmitTx_NilReceiverIsAnError(t *testing.T) {
	var e *audit.Emitter
	require.Error(t, e.EmitTx(context.Background(), &gorm.DB{}, nil,
		audit.Event{Action: "a", ResourceType: "b", TenantID: uuid.New()}))
}

// TestEmitTx_PropagatesTheRepositoryError: the caller decides what a failed
// audit insert means for its transaction, so the error has to reach it
// intact. Swallowing it would commit the state change unattributed.
func TestEmitTx_PropagatesTheRepositoryError(t *testing.T) {
	repo := &txRecordingRepo{createErr: errors.New("boom")}
	e, _ := newTxEmitter(t, repo)

	err := e.EmitTx(context.Background(), &gorm.DB{}, ginContextWithOperator(t, "op-7", "billing.discount"),
		audit.Event{Action: "tenant.discount_applied", ResourceType: "subscription", TenantID: uuid.New()})

	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

// TestEmitTx_MissingTenantIsAnError: buildEntry returns nil rather than write
// a tenant-unscoped row. EmitSync turns that into an error and so must this,
// or the caller commits its change believing it was audited.
func TestEmitTx_MissingTenantIsAnError(t *testing.T) {
	repo := &txRecordingRepo{}
	e, _ := newTxEmitter(t, repo)

	err := e.EmitTx(context.Background(), &gorm.DB{}, ginContextWithOperator(t, "op-7", "billing.discount"),
		audit.Event{Action: "tenant.discount_applied", ResourceType: "subscription"}) // no TenantID

	require.Error(t, err)
	require.Empty(t, repo.created, "a tenant-less row must never be written")
}

// TestEmitTx_MissingActionOrResourceTypeIsAnError keeps the validation floor
// level with EmitSync's; these two columns are NOT NULL and a row missing
// either is not a usable trail entry.
func TestEmitTx_MissingActionOrResourceTypeIsAnError(t *testing.T) {
	repo := &txRecordingRepo{}
	e, _ := newTxEmitter(t, repo)

	require.Error(t, e.EmitTx(context.Background(), &gorm.DB{}, nil,
		audit.Event{ResourceType: "subscription", TenantID: uuid.New()}), "missing action")
	require.Error(t, e.EmitTx(context.Background(), &gorm.DB{}, nil,
		audit.Event{Action: "tenant.discount_applied", TenantID: uuid.New()}), "missing resource type")
	require.Empty(t, repo.created)
}
