package platformadmin_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

func conversionsRouter(t *testing.T, dir platformadmin.TenantDirectory) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewConversionsHandler(dir, nil).Register(r.Group(""))
	return r
}

func conversionsRequest(email string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/admin/conversions?email="+url.QueryEscape(email), nil)
}

// THE test. Real handler output compared to the committed contract.
func TestConversionsMatchesContract(t *testing.T) {
	dir := &stubDirectory{conversion: &tenantdirectory.Tenant{
		ID: "3f2504e0-4f89-11d3-9a0c-0305e82c3301", Name: "Acme Trading",
		OwnerEmail: "founder@acme.example", Status: "active",
		CreatedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
	}}

	rec := httptest.NewRecorder()
	conversionsRouter(t, dir).ServeHTTP(rec, conversionsRequest("founder@acme.example"))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/conversions_converted.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// A miss is a definite, positive answer, never a 404 — a 404 on the wire is
// indistinguishable from a route that does not exist, so it can never carry
// the answer "not converted".
func TestConversionsMissIsExactlyStateNone(t *testing.T) {
	dir := &stubDirectory{err: tenantdirectory.ErrNotFound}

	rec := httptest.NewRecorder()
	conversionsRouter(t, dir).ServeHTTP(rec, conversionsRequest("nobody@example.com"))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEqual(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"state":"none"}`, rec.Body.String())
}

func TestConversionsMissingEmailIs400(t *testing.T) {
	rec := httptest.NewRecorder()
	conversionsRouter(t, &stubDirectory{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/conversions", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "validation_error", errorCode(t, rec))
}

func TestConversionsBlankEmailIs400(t *testing.T) {
	rec := httptest.NewRecorder()
	conversionsRouter(t, &stubDirectory{}).ServeHTTP(rec, conversionsRequest(""))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "validation_error", errorCode(t, rec))
}

func TestConversionsWhitespaceEmailIs400(t *testing.T) {
	rec := httptest.NewRecorder()
	conversionsRouter(t, &stubDirectory{}).ServeHTTP(rec, conversionsRequest("  "))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "validation_error", errorCode(t, rec))
}

// "We could not ask" is not "not converted", and the console renders the two
// differently. A 503 body must never also carry a definite state.
func TestConversionsUpstreamDownIs503(t *testing.T) {
	dir := &stubDirectory{err: tenantdirectory.ErrUnavailable}

	rec := httptest.NewRecorder()
	conversionsRouter(t, dir).ServeHTTP(rec, conversionsRequest("a@example.com"))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))
	require.NotContains(t, rec.Body.String(), `"state"`)
}

func TestConversionsUnexpectedErrorIs500(t *testing.T) {
	dir := &stubDirectory{err: errUnexpected}

	rec := httptest.NewRecorder()
	conversionsRouter(t, dir).ServeHTTP(rec, conversionsRequest("a@example.com"))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
}

// The email reaches the client verbatim after trimming, so a future
// normalisation change here cannot silently diverge from platform-api's.
func TestConversionsTrimsEmailBeforeCallingClient(t *testing.T) {
	dir := &stubDirectory{err: tenantdirectory.ErrNotFound}

	rec := httptest.NewRecorder()
	conversionsRouter(t, dir).ServeHTTP(rec, conversionsRequest("  Founder@Acme.example  "))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "Founder@Acme.example", dir.gotEmail)
}

var errUnexpected = errors.New("boom")
