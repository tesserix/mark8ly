package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
)

// fakeTrialSubscriber is a test double for admin.TrialSubscriber.
type fakeTrialSubscriber struct {
	result *trial.SubscribeResult
	err    error
}

func (f *fakeTrialSubscriber) Subscribe(_ context.Context, _ trial.SubscribeInput) (*trial.SubscribeResult, error) {
	return f.result, f.err
}

// newTrialTestRouter builds a minimal gin engine with tenant context injected,
// mounting the TrialBillingHandler at the path used by RegisterAdmin.
func newTrialTestRouter(fake admin.TrialSubscriber) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := admin.NewTrialBillingHandler(fake, nil)
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", "11111111-1111-1111-1111-111111111111")
		c.Next()
	})
	r.POST("/admin/stores/:storeId/billing/subscription", h.Subscribe)
	return r
}

func postTrialSubscribe(t *testing.T, r *gin.Engine, storeID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+storeID+"/billing/subscription",
		bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const validTrialStoreID = "33333333-3333-3333-3333-333333333333"

func validTrialBody() map[string]any {
	return map[string]any{
		"plan":     "starter",
		"period":   "monthly",
		"currency": "USD",
	}
}

func TestTrialSubscribe_Success(t *testing.T) {
	fake := &fakeTrialSubscriber{
		result: &trial.SubscribeResult{
			StripeSubscriptionID: "sub_ok_123",
			TrialEndUnix:         1_800_000_000,
		},
	}
	r := newTrialTestRouter(fake)
	w := postTrialSubscribe(t, r, validTrialStoreID, validTrialBody())

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "sub_ok_123", resp["stripe_subscription_id"])
	assert.Equal(t, float64(1_800_000_000), resp["trial_end_unix"])
}

func TestTrialSubscribe_AlreadyActive_Returns409(t *testing.T) {
	fake := &fakeTrialSubscriber{err: trial.ErrSubscriptionAlreadyActive}
	r := newTrialTestRouter(fake)
	w := postTrialSubscribe(t, r, validTrialStoreID, validTrialBody())

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "already_active", resp["error"])
}

func TestTrialSubscribe_MissingCustomer_Returns412(t *testing.T) {
	fake := &fakeTrialSubscriber{err: trial.ErrMissingStripeCustomer}
	r := newTrialTestRouter(fake)
	w := postTrialSubscribe(t, r, validTrialStoreID, validTrialBody())

	assert.Equal(t, http.StatusPreconditionFailed, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "missing_stripe_customer", resp["error"])
}

func TestTrialSubscribe_InvalidBody_Returns400(t *testing.T) {
	fake := &fakeTrialSubscriber{}
	r := newTrialTestRouter(fake)

	// Missing required fields — binding should reject.
	w := postTrialSubscribe(t, r, validTrialStoreID, map[string]any{"plan": "starter"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTrialSubscribe_InvalidStoreID_Returns400(t *testing.T) {
	fake := &fakeTrialSubscriber{}
	r := newTrialTestRouter(fake)
	w := postTrialSubscribe(t, r, "not-a-uuid", validTrialBody())

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_failed", resp["error"])
}
