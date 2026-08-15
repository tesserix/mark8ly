package internalsvc_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/internalsvc"
)

// Unlike the sibling per-slug endpoints, this one enumerates every merchant's
// custom domain, so the shared secret is not optional. Both cases abort before
// the DB is touched, hence the nil handle.
func TestActiveDomainsRequiresInternalAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	internalsvc.NewActiveDomainsHandler(nil).Register(r.Group("/internal"), "s3cret")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/active-domains", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/active-domains", nil)
	req.Header.Set("X-Internal-Auth", "wrong")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
