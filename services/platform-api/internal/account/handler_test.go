package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// fakeSvc is a Service double that records the tenantID/actorUID it was
// called with and returns whatever err is configured, so tests can drive
// the handler without a real DB/FGA/GIP.
type fakeSvc struct {
	err            error
	calledTenantID string
	calledActorUID string
	called         bool
}

func (f *fakeSvc) DeleteAccount(_ context.Context, tenantID, actorUID string) error {
	f.called = true
	f.calledTenantID = tenantID
	f.calledActorUID = actorUID
	return f.err
}

func newTestRouter(svc accountDeleter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	internal := r.Group("/internal")
	NewHandler(svc).Register(internal)
	return r
}

func TestAccountHandler_Delete_204(t *testing.T) {
	svc := &fakeSvc{}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`{"uid":"owner-1"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !svc.called {
		t.Fatal("expected service.DeleteAccount to be called")
	}
	if svc.calledTenantID != "t1" {
		t.Fatalf("tenantID=%q, want t1", svc.calledTenantID)
	}
	if svc.calledActorUID != "owner-1" {
		t.Fatalf("actorUID=%q, want owner-1", svc.calledActorUID)
	}
}

func TestAccountHandler_Delete_MissingUID_400(t *testing.T) {
	svc := &fakeSvc{}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`{"uid":""}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing_uid") {
		t.Fatalf("body=%s, want missing_uid error code", rec.Body.String())
	}
	if svc.called {
		t.Fatal("service.DeleteAccount should not be called on validation failure")
	}
}

func TestAccountHandler_Delete_InvalidJSON_400(t *testing.T) {
	svc := &fakeSvc{}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`not-json`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.called {
		t.Fatal("service.DeleteAccount should not be called on validation failure")
	}
}

func TestAccountHandler_Delete_Forbidden_403(t *testing.T) {
	svc := &fakeSvc{err: apperrors.Forbidden("not_a_member", "actor has no role on this tenant")}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`{"uid":"stranger-1"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.calledTenantID != "t1" || svc.calledActorUID != "stranger-1" {
		t.Fatalf("unexpected call args: tenantID=%q actorUID=%q", svc.calledTenantID, svc.calledActorUID)
	}
}

func TestAccountHandler_Delete_NotFound_404(t *testing.T) {
	svc := &fakeSvc{err: apperrors.NotFound("tenant_not_found", "tenant does not exist")}
	r := newTestRouter(svc)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/missing-tenant/account",
		strings.NewReader(`{"uid":"owner-1"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.calledTenantID != "missing-tenant" {
		t.Fatalf("tenantID=%q, want missing-tenant", svc.calledTenantID)
	}
}
