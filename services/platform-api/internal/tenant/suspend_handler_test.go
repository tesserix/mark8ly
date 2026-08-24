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
	suspend      *SuspendResult
	suspendErr   error
	unsuspend    *SuspendResult
	unsuspendErr error
}

func (s *stubLifecycleRepo) Suspend(_ context.Context, _ string) (*SuspendResult, error) {
	return s.suspend, s.suspendErr
}

func (s *stubLifecycleRepo) Unsuspend(_ context.Context, _ string) (*SuspendResult, error) {
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
