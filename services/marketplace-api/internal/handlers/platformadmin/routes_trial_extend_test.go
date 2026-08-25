package platformadmin_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// Mounted when the dependency is supplied. Asserted as "not 404" with the
// secret set, matching TestRegisterTicketsMountsWhenDepPresent.
func TestRegisterMountsTrialExtendWhenSupplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:          &stubRepo{},
		TrialExtender: &stubExtender{},
		Secret:        "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/platform/admin/billing/trials/bbbbbbbb-1111-1111-1111-111111111111/extend",
		bytes.NewBufferString(`{}`)))
	require.NotEqual(t, http.StatusNotFound, rec.Code,
		"the route must be mounted when TrialExtender is set")

	// A bogus sibling under the same prefix must 404 — without this, the
	// assertion above is also satisfied by a router that answers everything.
	bogus := httptest.NewRecorder()
	r.ServeHTTP(bogus, httptest.NewRequest(http.MethodPost,
		"/api/v1/platform/admin/billing/trials/bbbbbbbb-1111-1111-1111-111111111111/extend-nope",
		bytes.NewBufferString(`{}`)))
	require.Equal(t, http.StatusNotFound, bogus.Code)
}

// Nil leaves it unmounted, matching every other optional route here.
func TestRegisterLeavesTrialExtendUnmountedWhenNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{Repo: &stubRepo{}})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/platform/admin/billing/trials/bbbbbbbb-1111-1111-1111-111111111111/extend",
		bytes.NewBufferString(`{}`)))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
