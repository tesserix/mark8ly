package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeSubmitService captures Submit calls and returns a configured error.
type fakeSubmitService struct {
	err     error
	lastIn  tax.SubmitInput
	calls   int
}

func (f *fakeSubmitService) Submit(_ context.Context, in tax.SubmitInput) error {
	f.calls++
	f.lastIn = in
	return f.err
}

func runSubmit(t *testing.T, svc admin.SubmitService, body any) *httptest.ResponseRecorder {
	t.Helper()
	h := admin.NewTaxHandler(nil, svc, nil)

	r := gin.New()
	tenantID := uuid.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Next()
	})
	r.POST("/admin/stores/:storeId/tax/submit", h.Submit)

	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+uuid.New().String()+"/tax/submit", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestTaxHandler_Submit_Success_Returns200(t *testing.T) {
	rec := runSubmit(t, &fakeSubmitService{}, map[string]any{
		"country":       "GB",
		"tax_id":        "GB123456789",
		"business_name": "Acme Ltd",
	})
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestTaxHandler_Submit_ManualReview_Returns202(t *testing.T) {
	rec := runSubmit(t, &fakeSubmitService{err: tax.ErrManualReviewRequired}, map[string]any{
		"country":       "MY",
		"tax_id":        "C12345678901",
		"business_name": "Acme Sdn Bhd",
	})
	require.Equal(t, http.StatusAccepted, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	data, _ := body["data"].(map[string]any)
	require.Equal(t, "manual_review_queued", data["status"])
}

func TestTaxHandler_Submit_RegistryUnavailable_Returns202(t *testing.T) {
	rec := runSubmit(t, &fakeSubmitService{err: tax.ErrRegistryUnavailable}, map[string]any{
		"country":       "GB",
		"tax_id":        "GB123456789",
		"business_name": "Acme Ltd",
	})
	require.Equal(t, http.StatusAccepted, rec.Code)
}

func TestTaxHandler_Submit_NZDisabled_Returns503(t *testing.T) {
	rec := runSubmit(t, &fakeSubmitService{err: tax.ErrValidatorDisabled}, map[string]any{
		"country":       "NZ",
		"tax_id":        "123-456-789",
		"business_name": "Kiwi Co",
	})
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "awaiting legal sign-off")
}

func TestTaxHandler_Submit_InvalidFormat_Returns400(t *testing.T) {
	rec := runSubmit(t, &fakeSubmitService{err: tax.ErrInvalidFormat}, map[string]any{
		"country":       "GB",
		"tax_id":        "garbage",
		"business_name": "Acme Ltd",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTaxHandler_Submit_NotFound_Returns422(t *testing.T) {
	rec := runSubmit(t, &fakeSubmitService{err: tax.ErrNotFound}, map[string]any{
		"country":       "GB",
		"tax_id":        "GB000000000",
		"business_name": "Ghost Ltd",
	})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestTaxHandler_Submit_MissingFields_Returns400(t *testing.T) {
	svc := &fakeSubmitService{}
	rec := runSubmit(t, svc, map[string]any{"country": "GB"}) // missing tax_id + business_name
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, svc.calls, "service must not be called for malformed body")
}
