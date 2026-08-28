package platformadmin_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// recordingRepo is a stub audit.Repository that records every entry handed
// to Create, so tests can prove an event was (or was not) enqueued without
// a database. Access is guarded by a mutex because the emitter writes from
// a worker goroutine.
type recordingRepo struct {
	mu      sync.Mutex
	created []audit.Entry
}

func (r *recordingRepo) Create(_ context.Context, _ *gorm.DB, e *audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, *e)
	return nil
}

func (r *recordingRepo) List(context.Context, *gorm.DB, audit.ListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

func (r *recordingRepo) Stream(context.Context, *gorm.DB, audit.ListFilter, func(*audit.Entry) error) error {
	return nil
}

func (r *recordingRepo) ListPlatform(context.Context, *gorm.DB, audit.PlatformListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

func (r *recordingRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.created)
}

func (r *recordingRepo) first() audit.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created[0]
}

func newTestEmitter(t *testing.T, repo audit.Repository) *audit.Emitter {
	t.Helper()
	em, err := audit.NewEmitter(audit.EmitterConfig{
		Repo:   repo,
		Logger: slog.New(slog.NewTextHandler(httptest.NewRecorder().Body, nil)),
	})
	require.NoError(t, err)
	t.Cleanup(func() { em.Stop(context.Background()) })
	return em
}

// TestEmitOperatorAction_NilTenant_RejectsAndEmitsNothing pins #310's
// compile-time-safety requirement: a caller that forgets to supply a
// tenant gets an error back, and — proven via a recording repository,
// not merely the error return — nothing is ever enqueued for write.
func TestEmitOperatorAction_NilTenant_RejectsAndEmitsNothing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &recordingRepo{}
	em := newTestEmitter(t, repo)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)

	err := platformadmin.EmitOperatorAction(c, em, uuid.Nil, audit.Event{
		Action:       "tenant.suspended",
		ResourceType: "tenant",
	})

	require.ErrorIs(t, err, platformadmin.ErrMissingTenant)

	// Give the (nonexistent) async write every chance to happen before
	// asserting it didn't: if EmitOperatorAction had called em.Emit
	// despite the nil tenant, the worker would have written it well
	// within this window.
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 0, repo.count(), "no audit row should ever be enqueued for a nil tenant")
}

// TestEmitOperatorAction_ValidTenant_EnqueuesWithTenantID proves the
// happy path populates Event.TenantID before delegating to Emit, so the
// gin-context lookup (which nothing sets on this surface) is never relied
// on.
func TestEmitOperatorAction_ValidTenant_EnqueuesWithTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &recordingRepo{}
	em := newTestEmitter(t, repo)

	tenantID := uuid.New()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)

	err := platformadmin.EmitOperatorAction(c, em, tenantID, audit.Event{
		Action:       "tenant.suspended",
		ResourceType: "tenant",
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool { return repo.count() == 1 }, time.Second, 5*time.Millisecond,
		"expected exactly one audit row to be written")
	entry := repo.first()
	require.Equal(t, tenantID, entry.TenantID)
	require.Equal(t, "tenant.suspended", entry.Action)
}

// TestEmitOperatorAction_NilEmitter_NoPanicNoError matches Emit's own
// nil-receiver tolerance: wiring that opts out of auditing (em == nil)
// must not panic or surface an error.
func TestEmitOperatorAction_NilEmitter_NoPanicNoError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)

	var em *audit.Emitter
	require.NotPanics(t, func() {
		err := platformadmin.EmitOperatorAction(c, em, uuid.New(), audit.Event{
			Action:       "tenant.suspended",
			ResourceType: "tenant",
		})
		require.NoError(t, err)
	})
}

// TestEmitOperatorAction_OperatorContextFlowsThrough proves the operator
// and capability context keys set by platform middleware still reach the
// written entry through EmitOperatorAction — i.e. it doesn't bypass
// buildEntry's context-derived attribution, it only forces TenantID.
func TestEmitOperatorAction_OperatorContextFlowsThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &recordingRepo{}
	em := newTestEmitter(t, repo)

	tenantID := uuid.New()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)
	c.Set(platformadmin.CtxOperatorID, "op_7f3a")
	c.Set(platformadmin.CtxCapability, "tenant.suspend")

	err := platformadmin.EmitOperatorAction(c, em, tenantID, audit.Event{
		Action:       "tenant.suspended",
		ResourceType: "tenant",
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool { return repo.count() == 1 }, time.Second, 5*time.Millisecond,
		"expected exactly one audit row to be written")
	entry := repo.first()
	require.Equal(t, tenantID, entry.TenantID)
	require.Equal(t, audit.ActorOperator, entry.ActorType)
	require.NotNil(t, entry.ActorOperatorID)
	require.Equal(t, "op_7f3a", *entry.ActorOperatorID)
	require.NotNil(t, entry.Capability)
	require.Equal(t, "tenant.suspend", *entry.Capability)
}
