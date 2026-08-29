package tenant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

const testTenantID = "11111111-1111-1111-1111-111111111111"

// stubLifecycleRepo returns canned results for Suspend/Unsuspend.
type stubLifecycleRepo struct {
	Repository
	suspend         *SuspendResult
	suspendErr      error
	suspendCalled   bool
	unsuspend       *SuspendResult
	unsuspendErr    error
	unsuspendCalled bool
}

func (s *stubLifecycleRepo) Suspend(_ context.Context, _ string) (*SuspendResult, error) {
	s.suspendCalled = true
	return s.suspend, s.suspendErr
}

func (s *stubLifecycleRepo) Unsuspend(_ context.Context, _ string) (*SuspendResult, error) {
	s.unsuspendCalled = true
	return s.unsuspend, s.unsuspendErr
}

func newTestHandler(repo Repository) *Handler {
	return NewHandler(NewService(repo, nil), nil)
}

func doPost(t *testing.T, h *Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterLifecycle(r.Group(""))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

type lifecycleBody struct {
	Data struct {
		TenantID       string `json:"tenant_id"`
		Status         string `json:"status"`
		StoresAffected int    `json:"stores_affected"`
		Changed        bool   `json:"changed"`
	} `json:"data"`
}

func TestSuspendHandler_ReturnsChangedAndCount(t *testing.T) {
	h := newTestHandler(&stubLifecycleRepo{suspend: &SuspendResult{
		Status: StatusSuspended, StoresAffected: 2, Changed: true,
	}})
	rec := doPost(t, h, "/tenants/"+testTenantID+"/suspend")

	require.Equal(t, http.StatusOK, rec.Code)
	var body lifecycleBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, testTenantID, body.Data.TenantID)
	require.Equal(t, "suspended", body.Data.Status)
	require.Equal(t, 2, body.Data.StoresAffected)
	require.True(t, body.Data.Changed)
}

// A no-op must be 200 with changed:false — NOT an error. #287's acceptance
// says suspending an already-suspended tenant returns current state.
func TestSuspendHandler_AlreadySuspendedIsOKNotError(t *testing.T) {
	h := newTestHandler(&stubLifecycleRepo{suspend: &SuspendResult{
		Status: StatusSuspended, StoresAffected: 0, Changed: false,
	}})
	rec := doPost(t, h, "/tenants/"+testTenantID+"/suspend")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"changed":false`)
}

func TestSuspendHandler_UnknownTenantIs404(t *testing.T) {
	h := newTestHandler(&stubLifecycleRepo{
		suspendErr: apperrors.NotFound("tenant_not_found", "no such tenant"),
	})
	rec := doPost(t, h, "/tenants/"+testTenantID+"/suspend")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSuspendHandler_ArchivedIs409(t *testing.T) {
	h := newTestHandler(&stubLifecycleRepo{
		suspendErr: apperrors.Conflict("tenant_suspend_conflict", "tenant is archived"),
	})
	rec := doPost(t, h, "/tenants/"+testTenantID+"/suspend")
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestUnsuspendHandler_ReturnsChangedAndCount(t *testing.T) {
	h := newTestHandler(&stubLifecycleRepo{unsuspend: &SuspendResult{
		Status: StatusActive, StoresAffected: 3, Changed: true,
	}})
	rec := doPost(t, h, "/tenants/"+testTenantID+"/unsuspend")

	require.Equal(t, http.StatusOK, rec.Code)
	var body lifecycleBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, testTenantID, body.Data.TenantID)
	require.Equal(t, "active", body.Data.Status)
	require.Equal(t, 3, body.Data.StoresAffected)
	require.True(t, body.Data.Changed)
}

// A no-op must be 200 with changed:false — NOT an error.
func TestUnsuspendHandler_AlreadyActiveIsOKNotError(t *testing.T) {
	h := newTestHandler(&stubLifecycleRepo{unsuspend: &SuspendResult{
		Status: StatusActive, StoresAffected: 0, Changed: false,
	}})
	rec := doPost(t, h, "/tenants/"+testTenantID+"/unsuspend")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"changed":false`)
}

func TestUnsuspendHandler_UnknownTenantIs404(t *testing.T) {
	h := newTestHandler(&stubLifecycleRepo{
		unsuspendErr: apperrors.NotFound("tenant_not_found", "no such tenant"),
	})
	rec := doPost(t, h, "/tenants/"+testTenantID+"/unsuspend")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// A malformed :id is a caller error. Without validation it reaches Postgres,
// which raises 22P02 (invalid input syntax for type uuid), and that surfaces
// as a 500 that pages someone. marketplace-api's console-facing handler
// already rejects it (internal/handlers/platformadmin/tenant_lifecycle.go,
// uuid.Parse -> 400); the two halves of the same operation must agree.
//
// The repository assertion is the point of the test: 400 alone could be
// produced by the same query failing differently. The id must not reach the
// database at all.
func TestSuspendHandler_MalformedIDIs400AndNeverReachesRepo(t *testing.T) {
	repo := &stubLifecycleRepo{suspend: &SuspendResult{
		Status: StatusSuspended, StoresAffected: 1, Changed: true,
	}}
	rec := doPost(t, newTestHandler(repo), "/tenants/not-a-uuid/suspend")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, repo.suspendCalled,
		"a malformed id must be rejected before the repository sees it")
	require.Contains(t, rec.Body.String(), `"error":"invalid_tenant_id"`)
}

func TestUnsuspendHandler_MalformedIDIs400AndNeverReachesRepo(t *testing.T) {
	repo := &stubLifecycleRepo{unsuspend: &SuspendResult{
		Status: StatusActive, StoresAffected: 1, Changed: true,
	}}
	rec := doPost(t, newTestHandler(repo), "/tenants/not-a-uuid/unsuspend")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, repo.unsuspendCalled,
		"a malformed id must be rejected before the repository sees it")
	require.Contains(t, rec.Body.String(), `"error":"invalid_tenant_id"`)
}
