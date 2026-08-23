package estate

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

// stubRepo returns a canned result or error for Get.
type stubRepo struct {
	counts *Counts
	err    error
}

func (s *stubRepo) Get(_ context.Context) (*Counts, error) {
	return s.counts, s.err
}

func router(repo Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(repo).Register(r.Group(""))
	return r
}

func TestGet_EnvelopeHasDataWithBothKeys(t *testing.T) {
	repo := &stubRepo{counts: &Counts{TenantsActive: 3, StoresActive: 5}}
	rec := httptest.NewRecorder()
	router(repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/estate/counts", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data Counts `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, int64(3), body.Data.TenantsActive)
	require.Equal(t, int64(5), body.Data.StoresActive)
}

func TestGet_ZeroCountsStillPresentInEnvelope(t *testing.T) {
	repo := &stubRepo{counts: &Counts{TenantsActive: 0, StoresActive: 0}}
	rec := httptest.NewRecorder()
	router(repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/estate/counts", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, string(body["data"]), `"tenants_active":0`)
	require.Contains(t, string(body["data"]), `"stores_active":0`)
}

// A repository error must map through the package's error convention
// (apperrors -> respondError), not come back as a bare 500 string.
func TestGet_RepositoryErrorMapsThroughAppErrors(t *testing.T) {
	repo := &stubRepo{err: apperrors.Internal("estate_count_failed", "could not count the estate")}
	rec := httptest.NewRecorder()
	router(repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/estate/counts", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "estate_count_failed", body.Error)
	require.Equal(t, "could not count the estate", body.Message)
}

// An unwrapped/unknown error still maps to a safe generic 500 body, not a
// leaked error string.
func TestGet_UnknownErrorMapsToGenericInternalError(t *testing.T) {
	repo := &stubRepo{err: context.DeadlineExceeded}
	rec := httptest.NewRecorder()
	router(repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/estate/counts", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "internal_error", body.Error)
}
