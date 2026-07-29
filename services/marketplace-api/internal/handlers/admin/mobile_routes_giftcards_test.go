package admin

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// The mobile app can only reach `/api/v1/mobile/admin/stores/{id}/...` —
// `createApiClient` in packages/mobile-shared/api/client.ts builds every URL
// from that prefix and has no escape hatch. So a route that exists only on
// the WEB admin table (`/api/v1/admin/stores/:storeId/...`) is, from the
// phone, a 404.
//
// Gift-card Disable/Enable shipped on the web table alone. The mobile
// gift-card group was a verbatim copy of the web one made BEFORE those two
// routes existed and was never re-synced, so the mobile Gift cards screen
// would have armed two gestures that could only ever 404 — with `RespondErr`
// never even reached, because gin refuses the request at the tree.
//
// This test pins the mobile table itself rather than the handler: the
// handler was already correct and already covered
// (gift_cards_status_integration_test.go). What was missing was the wiring,
// and only a route-table assertion can see that.
func TestRegisterAdminMobile_GiftCardStatusRoutesAreReachable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	RegisterAdminMobile(r.Group("/api/v1"), MobileDeps{
		Deps: Deps{
			// Registration only touches these; no request is served, so the
			// handler needs no service and the middleware no FGA client.
			GiftCardHandler: &GiftCardHandler{},
			AuthzMiddleware: authz.NewMiddleware(nil, nil),
			StoresMiddleware: func(c *gin.Context) {
				c.Next()
			},
		},
		TokenVerifier: &auth.FakeVerifier{},
	})

	registered := make(map[string]bool, len(r.Routes()))
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	const base = "/api/v1/mobile/admin/stores/:storeId/gift-cards"
	for _, want := range []string{
		"GET " + base,
		"POST " + base,
		"GET " + base + "/:id",
		"POST " + base + "/:id/disable",
		"POST " + base + "/:id/enable",
	} {
		if !registered[want] {
			t.Errorf("mobile route table is missing %q — the phone cannot reach it", want)
		}
	}
}
