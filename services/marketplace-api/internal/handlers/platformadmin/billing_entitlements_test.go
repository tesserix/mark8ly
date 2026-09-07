package platformadmin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// entitlementsBody is the wire shape as a CONSUMER sees it — decoded from
// JSON rather than read off the handler's own struct, so a field renamed in
// the response type fails here instead of passing against itself.
type entitlementsBody struct {
	Source      string                    `json:"source"`
	CatalogMode string                    `json:"catalog_mode"`
	Features    []string                  `json:"features"`
	Plans       map[string]map[string]int `json:"plans"`
}

func decodeEntitlements(t *testing.T, catalogMode string) (int, entitlementsBody) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	h := NewBillingEntitlementsHandler(catalogMode, slog.Default())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/billing/entitlements", nil)

	h.list(c)

	var body entitlementsBody
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	}
	return w.Code, body
}

func TestEntitlementsHandlerReportsTheCompiledMatrix(t *testing.T) {
	code, body := decodeEntitlements(t, "test")

	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "mark8ly", body.Source)
	require.Equal(t, "test", body.CatalogMode)
	require.Len(t, body.Features, len(plangate.AllFeatures()))
	require.Len(t, body.Plans, 4)

	// DERIVED, never restated: assert against plangate itself, so a matrix
	// change moves this test rather than leaving it asserting a stale copy.
	for _, p := range []subscription.SubscriptionPlan{"trial", "starter", "studio", "pro"} {
		require.Equal(t, plangate.AllFeatureLimits(p), body.Plans[string(p)])
	}
}

// TestEntitlementsHandlerPublishesExactlyTheMatrixPlans is the guard against
// the plan list becoming a fourth hand-written copy. It compares the keys of
// `plans` against plangate.AllPlans() — which is itself the matrix's own key
// set — rather than against the four names spelled out above, so a plan added
// to or removed from the matrix moves this assertion automatically.
func TestEntitlementsHandlerPublishesExactlyTheMatrixPlans(t *testing.T) {
	_, body := decodeEntitlements(t, "test")

	want := make(map[string]bool, len(plangate.AllPlans()))
	for _, p := range plangate.AllPlans() {
		want[string(p)] = true
	}
	got := make(map[string]bool, len(body.Plans))
	for name := range body.Plans {
		got[name] = true
	}
	require.Equal(t, want, got)

	// PlanMarketplace exists as a subscription plan but is deliberately
	// absent from the matrix (hidden platform tier; platform routes bypass
	// plangate). Publishing it would report "a plan on which nothing is
	// enabled", which is a different and wrong claim.
	require.NotContains(t, got, string(subscription.PlanMarketplace))
}

// TestEntitlementsHandlerFeaturesAreTheCanonicalList pins the feature array to
// plangate.AllFeatures() including its ORDER — the response is a parity
// target, and a consumer diffing it positionally must not see churn that the
// gate did not produce.
func TestEntitlementsHandlerFeaturesAreTheCanonicalList(t *testing.T) {
	_, body := decodeEntitlements(t, "test")

	want := make([]string, 0, len(plangate.AllFeatures()))
	for _, f := range plangate.AllFeatures() {
		want = append(want, string(f))
	}
	require.Equal(t, want, body.Features)
}

// TestEntitlementsHandlerReportsTheConfiguredCatalogMode covers the reason the
// field exists at all: the console cannot see CONSOLE_CATALOG_MODE, and the
// value moves at the Stripe live-key swap. Reporting a constant would go
// silently wrong on that day.
func TestEntitlementsHandlerReportsTheConfiguredCatalogMode(t *testing.T) {
	for _, mode := range []string{"test", "live", ""} {
		t.Run("mode="+mode, func(t *testing.T) {
			_, body := decodeEntitlements(t, mode)
			require.Equal(t, mode, body.CatalogMode)
		})
	}
}

// TestEntitlementsRouteMounts proves Register puts the handler on the path the
// console federates, not merely that list() works when called directly.
func TestEntitlementsRouteMounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewBillingEntitlementsHandler("test", slog.Default()).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/billing/entitlements", nil))
	require.Equal(t, http.StatusOK, w.Code)
}
