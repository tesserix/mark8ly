package public_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/public"
)

// The mount test #820 asked for.
//
// Deliberately NOT the 404-vs-405 probe the issue text suggests:
// HandleMethodNotAllowed is never set in any service in this estate, so gin
// answers 404 whether or not a route is mounted and that probe distinguishes
// nothing. Instead it compares a router built WITHOUT the handler against one
// built WITH it, through the real RegisterPublic — an unmounted route 404s, a
// mounted one answers its own error.
func TestRegisterPublic_SSORoutesExistOnlyWhenTheHandlerIsSupplied(t *testing.T) {
	gin.SetMode(gin.TestMode)

	bare := gin.New()
	public.RegisterPublic(bare.Group("/"), public.PublicDeps{})

	// A handler with no dependencies at all still MOUNTS; it answers 503 from
	// its own nil checks. That difference is the whole point: "the route is
	// not there" and "the route is there and cannot serve" are different
	// failures with different fixes, and #820 exists because they were
	// indistinguishable.
	mounted := gin.New()
	public.RegisterPublic(mounted.Group("/"), public.PublicDeps{
		SSOLoginHandler: public.NewSSOLoginHandler(nil, nil, nil, nil, nil, nil, nil, nil),
	})

	for _, route := range []struct {
		method, path string
	}{
		{http.MethodGet, "/sso/acme/login"},
		{http.MethodPost, "/sso/acme/callback"},
		{http.MethodPost, "/sso/acme/logout"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			bareRec := httptest.NewRecorder()
			bare.ServeHTTP(bareRec, httptest.NewRequest(route.method, route.path, nil))
			require.Equal(t, http.StatusNotFound, bareRec.Code,
				"the route exists without an SSO handler")

			rec := httptest.NewRecorder()
			mounted.ServeHTTP(rec, httptest.NewRequest(route.method, route.path, nil))
			require.NotEqual(t, http.StatusNotFound, rec.Code,
				"the route is missing even though a handler was supplied")
		})
	}
}
