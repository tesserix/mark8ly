package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/depsfill"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
)

// recordingClient remembers the answer to the LAST permission check of a
// request, so the test can tell "the authz gate refused" apart from "a
// handler ran and happened to answer 404".
type recordingClient struct {
	*authz.FakeClient
	mu       sync.Mutex
	checks   int
	lastDeny bool
}

func (r *recordingClient) Check(ctx context.Context, userID, relation, tenantID string) (bool, error) {
	ok, err := r.FakeClient.Check(ctx, userID, relation, tenantID)
	r.mu.Lock()
	r.checks++
	r.lastDeny = !ok
	r.mu.Unlock()
	return ok, err
}

func (r *recordingClient) reset() {
	r.mu.Lock()
	r.checks, r.lastDeny = 0, false
	r.mu.Unlock()
}

// Adding cashier to the FGA `member` relation (#1104) lets a cashier past
// every gate that checks membership alone. Every admin route outside the
// in-store sale routes must refuse a cashier at its role gate.
func TestCashierReachesOnlyInStoreRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const userID, tenantID = "cashier-1", "tenant-1"

	fake := authz.NewFakeClient()
	fake.Grant(userID, authz.RoleCashier, tenantID)
	rec := &recordingClient{FakeClient: fake}

	var deps admin.Deps
	depsfill.Struct(&deps)
	deps.AuthzMiddleware = authz.NewMiddleware(rec, nil)
	deps.StoresMiddleware = func(c *gin.Context) { c.Next() }
	deps.SubscriptionStatusLoader = nil
	deps.SubscriptionReadOnlyGate = nil
	deps.TenantGate = nil
	deps.InternalSecret = ""

	build := func() *gin.Engine {
		engine := gin.New()
		engine.Use(gin.Recovery())
		admin.RegisterAdmin(engine.Group(""), deps)
		return engine
	}
	if n := requireCashierRefused(t, build, rec, userID, tenantID); n < 100 {
		t.Fatalf("only %d admin routes exercised; the route table did not mount", n)
	}
}

// The mobile admin API admits any bound member at group level and relies on
// per-route role gates, so it needs the same proof.
func TestCashierReachesNoMobileAdminRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const userID, tenantID = "cashier-1", "tenant-1"

	fake := authz.NewFakeClient()
	fake.Grant(userID, authz.RoleCashier, tenantID)
	rec := &recordingClient{FakeClient: fake}

	var deps admin.MobileDeps
	depsfill.Struct(&deps)
	deps.AuthzMiddleware = authz.NewMiddleware(rec, nil)
	deps.TokenVerifier = &auth.FakeVerifier{UserID: userID}
	deps.TenantMembershipChecker = fake
	deps.TenantMembershipLogger = nil
	deps.StoresMiddleware = func(c *gin.Context) { c.Next() }
	deps.TenantGate = nil

	// Rebuilt per request: the per-user rate limiter (burst 10) would
	// otherwise answer 429 from the eleventh route on.
	build := func() *gin.Engine {
		engine := gin.New()
		engine.Use(gin.Recovery())
		admin.RegisterAdminMobile(engine.Group(""), deps)
		return engine
	}
	if n := requireCashierRefused(t, build, rec, userID, tenantID); n < 10 {
		t.Fatalf("only %d mobile admin routes exercised; the route table did not mount", n)
	}
}

// userScoped routes act on the signed-in person, not the store: their own
// support chat with mark8ly, the tenants they belong to, deleting their own
// account. Any member may use them, cashiers included.
var userScoped = map[string]bool{
	"/mobile/admin/me/tenants":                                   true,
	"/mobile/admin/account":                                      true,
	"/mobile/admin/platform-support/resume":                      true,
	"/mobile/admin/platform-support/conversations":               true,
	"/mobile/admin/platform-support/conversations/:id":           true,
	"/mobile/admin/platform-support/conversations/:id/messages":  true,
	"/mobile/admin/platform-support/conversations/:id/close":     true,
	"/mobile/admin/platform-support/conversations/:id/feedback":  true,
	"/mobile/admin/platform-support/conversations/:id/ws-ticket": true,
	"/mobile/admin/platform-support/conversations/:id/queue":     true,
}

func requireCashierRefused(t *testing.T, build func() *gin.Engine, rec *recordingClient, userID, tenantID string) int {
	t.Helper()
	exempt := func(path string) bool {
		return strings.Contains(path, "/admin/stores/:storeId/in-store/") ||
			strings.HasPrefix(path, "/internal/") || // shared-secret callers, no user
			strings.Contains(path, "break-glass") || // pre-auth by design
			strings.HasPrefix(path, "/mobile/admin/auth/") || // sign-in, pre-auth by design
			userScoped[path]
	}

	checked := 0
	for _, r := range build().Routes() {
		if exempt(r.Path) {
			continue
		}
		path := r.Path
		for _, seg := range strings.Split(r.Path, "/") {
			if strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "*") {
				path = strings.Replace(path, seg, uuid.NewString(), 1)
			}
		}
		rec.reset()
		req := httptest.NewRequest(r.Method, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", userID)
		req.Header.Set("X-Tenant-Id", tenantID)
		req.Header.Set(auth.ActingTenantHeader, tenantID)
		req.Header.Set("Authorization", "Bearer test")
		w := httptest.NewRecorder()
		build().ServeHTTP(w, req)
		checked++

		if rec.checks == 0 || !rec.lastDeny || w.Code != http.StatusNotFound {
			t.Errorf("%s %s: cashier not refused by a role gate (status %d, checks %d, last denied %v)",
				r.Method, r.Path, w.Code, rec.checks, rec.lastDeny)
		}
	}
	return checked
}
