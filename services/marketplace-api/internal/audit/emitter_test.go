package audit_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// TestEmit_MissingTenant_LogsDrop guards against the silent-attribution-loss
// defect this series exists to eliminate: a caller that forgets to populate
// Event.TenantID (or set tenant_id on the gin context) must produce a
// visible warning, not a silent no-op. See emitter.go's Emit/buildEntry.
func TestEmit_MissingTenant_LogsDrop(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	em := audit.NewEmitter(audit.EmitterConfig{
		Repo:   audit.NewRepository(),
		Logger: logger,
	})
	t.Cleanup(func() { em.Stop(t.Context()) })

	em.Emit(nil, audit.Event{
		Action:       "tenant.suspend",
		ResourceType: "tenant",
	})

	out := buf.String()
	require.Contains(t, out, "dropping event")
	require.Contains(t, out, "no tenant")
	require.True(t, strings.Contains(out, "tenant.suspend"), "log should identify the action: %s", out)
	require.True(t, strings.Contains(out, "tenant"), "log should identify the resource type: %s", out)
}

// recordingRepo is a Repository test double that records every Entry
// passed to Create and never touches a real DB. createErr, when set, is
// returned by Create without recording the entry.
type recordingRepo struct {
	created   []*audit.Entry
	createErr error
}

func (r *recordingRepo) Create(_ context.Context, _ *gorm.DB, e *audit.Entry) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, e)
	return nil
}

func (r *recordingRepo) List(_ context.Context, _ *gorm.DB, _ audit.ListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

func (r *recordingRepo) Stream(_ context.Context, _ *gorm.DB, _ audit.ListFilter, _ func(*audit.Entry) error) error {
	return nil
}

func (r *recordingRepo) ListPlatform(_ context.Context, _ *gorm.DB, _ audit.PlatformListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

// ginContextWithOperator builds a *gin.Context carrying the two context
// keys buildEntry reads for a platform console operator claim:
// "platform_operator_id" and "platform_capability" (see emitter.go's
// buildEntry, around lines 220-221).
func ginContextWithOperator(t *testing.T, operatorID, capability string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("platform_operator_id", operatorID)
	c.Set("platform_capability", capability)
	return c
}

func TestEmitSync_WritesBeforeReturning(t *testing.T) {
	repo := &recordingRepo{}
	e := audit.NewEmitter(audit.EmitterConfig{DB: nil, Repo: repo, Logger: slog.Default()})
	t.Cleanup(func() { e.Stop(context.Background()) })

	c := ginContextWithOperator(t, "op-7", "tenants.purge")
	tenantID := uuid.New()

	err := e.EmitSync(c, audit.Event{
		Action: "tenant.purged", ResourceType: "tenant",
		TenantID: tenantID, Metadata: map[string]any{"total_rows": 42},
	})

	require.NoError(t, err)
	// The row is present on RETURN, with no sleep and no queue drain. An
	// async emitter passes an "eventually" assertion and fails this one.
	require.Len(t, repo.created, 1)
	require.Equal(t, tenantID, repo.created[0].TenantID)
	require.Equal(t, audit.ActorOperator, repo.created[0].ActorType)
	require.Equal(t, "op-7", *repo.created[0].ActorOperatorID)
	require.Equal(t, "tenants.purge", *repo.created[0].Capability)
}

func TestEmitSync_ReturnsTheRepositoryError(t *testing.T) {
	repo := &recordingRepo{createErr: errors.New("boom")}
	e := audit.NewEmitter(audit.EmitterConfig{DB: nil, Repo: repo, Logger: slog.Default()})
	t.Cleanup(func() { e.Stop(context.Background()) })

	err := e.EmitSync(ginContextWithOperator(t, "op-7", "tenants.purge"),
		audit.Event{Action: "tenant.purged", ResourceType: "tenant", TenantID: uuid.New()})

	require.Error(t, err, "an unrecorded irreversible action must be surfaced, never swallowed")
	require.Contains(t, err.Error(), "boom")
}

func TestEmitSync_MissingTenantIsAnError(t *testing.T) {
	repo := &recordingRepo{}
	e := audit.NewEmitter(audit.EmitterConfig{DB: nil, Repo: repo, Logger: slog.Default()})
	t.Cleanup(func() { e.Stop(context.Background()) })

	err := e.EmitSync(ginContextWithOperator(t, "op-7", "c"),
		audit.Event{Action: "tenant.purged", ResourceType: "tenant"}) // no TenantID

	require.Error(t, err)
	require.Empty(t, repo.created, "a tenant-less row must never be written")
}

func TestEmitSync_NilReceiverIsAnError(t *testing.T) {
	var e *audit.Emitter
	require.Error(t, e.EmitSync(nil, audit.Event{Action: "a", ResourceType: "b", TenantID: uuid.New()}))
}
