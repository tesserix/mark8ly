package admin_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// fakePlanResolver implements the resolver method RequireBreakGlassFeature
// needs (structurally — the middleware's parameter type is an unexported
// interface, satisfied by any type with a matching ResolveByTenant method,
// per Go's structural interface typing) WITHOUT touching a *gorm.DB. It
// records the tenant it was asked to resolve so tests can assert the
// middleware actually called it with the tenant id peeked from the body.
type fakePlanResolver struct {
	plan   subscription.SubscriptionPlan
	called uuid.UUID
}

func (f *fakePlanResolver) ResolveByTenant(_ context.Context, tenantID uuid.UUID) plangate.Plan {
	f.called = tenantID
	return f.plan
}

func breakGlassGateRouter(resolver *fakePlanResolver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/admin/break-glass/login",
		admin.RequireBreakGlassFeature(resolver, plangate.FeatureSSO, nil),
		func(c *gin.Context) {
			// Stand-in for BreakGlassLoginHandler.Login: proves the body
			// the gate peeked is STILL fully readable downstream.
			body, _ := readAll(c)
			c.JSON(http.StatusOK, gin.H{"received_body_len": len(body)})
		})
	return r
}

func readAll(c *gin.Context) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(c.Request.Body)
	return buf.Bytes(), err
}

// TestRequireBreakGlassFeature_RefusesTenantWithoutFeature pins the plan
// brief's Task 6 gate: a tenant on a plan below FeatureSSO's minimum gets
// the SAME 403 the SSO config endpoints give, even though break-glass
// login runs with no upstream auth middleware to have set tenant_id on
// the Gin context — the tenant id here comes from the request body.
func TestRequireBreakGlassFeature_RefusesTenantWithoutFeature(t *testing.T) {
	tenantID := uuid.New()
	resolver := &fakePlanResolver{plan: subscription.PlanStudio} // below Pro
	r := breakGlassGateRouter(resolver)

	body := []byte(`{"tenant_id":"` + tenantID.String() + `","password":"x","totp_code":"000000"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/break-glass/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "plan_required")
	require.Equal(t, tenantID, resolver.called, "gate must resolve the plan for the tenant_id in the body")
}

// TestRequireBreakGlassFeature_AllowsTenantWithFeature is the mirror case:
// a Pro-plan tenant passes through to the handler, and the body it sent
// is still intact for the handler to bind (the gate's body-peek must not
// consume the stream).
func TestRequireBreakGlassFeature_AllowsTenantWithFeature(t *testing.T) {
	tenantID := uuid.New()
	resolver := &fakePlanResolver{plan: subscription.PlanPro}
	r := breakGlassGateRouter(resolver)

	body := []byte(`{"tenant_id":"` + tenantID.String() + `","password":"x","totp_code":"000000"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/break-glass/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"received_body_len": `+strconv.Itoa(len(body))+`}`, rec.Body.String())
}

// TestRequireBreakGlassFeature_MalformedBodyPassesThrough documents the
// deliberate choice in RequireBreakGlassFeature's doc comment: a missing
// or malformed tenant_id is NOT rejected by the gate. It falls through so
// BreakGlassLoginHandler.Login's own shape validation produces the SAME
// uniform {"error":"invalid_credentials"} a wrong password gets — the
// endpoint's documented invariant that its response must never reveal
// which check failed.
func TestRequireBreakGlassFeature_MalformedBodyPassesThrough(t *testing.T) {
	resolver := &fakePlanResolver{plan: subscription.PlanStudio} // would be refused if resolved
	r := breakGlassGateRouter(resolver)

	req := httptest.NewRequest(http.MethodPost, "/admin/break-glass/login", bytes.NewReader([]byte(`not json`)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "malformed body must fall through to the handler, not be blocked by the gate")
	require.Equal(t, uuid.Nil, resolver.called, "resolver must never be called when no tenant_id could be peeked")
}
