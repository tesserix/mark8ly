package testimonial

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/stretchr/testify/require"
)

const testStoreID = "00000000-0000-4000-8000-000000000099"

func TestMerchantRejectsOtherStoreAndMissingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fga := authz.NewFake()
	h := NewHandler(nil, fga)
	r := gin.New()
	h.RegisterInternal(r.Group("/internal"))
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, uid := range []string{"", "another-tenant-user"} {
			req := httptest.NewRequest(method, "/internal/stores/"+testStoreID+"/testimonial", bytes.NewBufferString(`{}`))
			req.Header.Set("X-User-ID", uid)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		}
	}
}

func TestMerchantValidatesReviewBeforeWriting(t *testing.T) {
	fga := authz.NewFake()
	require.NoError(t, fga.WriteRoleObject(context.Background(), "owner", "manager", "store", testStoreID))
	r := gin.New()
	NewHandler(nil, fga).RegisterInternal(r.Group("/internal"))
	for _, body := range []string{
		`{"author_name":"A","quote":"Helpful","rating":0,"publish":true}`,
		`{"author_name":"A","quote":"Helpful","rating":6,"publish":true}`,
		`{"author_name":"A","quote":" ","rating":5,"publish":true}`,
		`{"author_name":"A","quote":"Helpful","rating":5,"publish":true,"consent":false}`,
		`{"display_name":"` + strings.Repeat("a", 81) + `","quote":"Helpful","rating":5,"publish":true,"consent":true}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/internal/stores/"+testStoreID+"/testimonial", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", "owner")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestMerchantFailsClosedWithoutAuthorization(t *testing.T) {
	r := gin.New()
	NewHandler(nil, nil).RegisterInternal(r.Group("/internal"))
	req := httptest.NewRequest(http.MethodGet, "/internal/stores/"+testStoreID+"/testimonial", nil)
	req.Header.Set("X-User-ID", "owner")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestMerchantFailsClosedWhenAuthorizationServiceFails(t *testing.T) {
	fga := authz.NewFake()
	fga.FailNextChecks(1)
	r := gin.New()
	NewHandler(nil, fga).RegisterInternal(r.Group("/internal"))
	req := httptest.NewRequest(http.MethodGet, "/internal/stores/"+testStoreID+"/testimonial", nil)
	req.Header.Set("X-User-ID", "owner")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}
