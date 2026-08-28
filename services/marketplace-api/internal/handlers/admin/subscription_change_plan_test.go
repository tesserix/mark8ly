package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
)

// fakeOrch is a test double for changePlanOrchestrator.
type fakeOrch struct {
	executeOut   planchange.Output
	executeErr   error
	preflightOut planchange.PreflightReport
	preflightErr error
}

func (f *fakeOrch) Execute(_ context.Context, _ planchange.Input) (planchange.Output, error) {
	return f.executeOut, f.executeErr
}

func (f *fakeOrch) Preflight(_ context.Context, _ planchange.Input) (planchange.PreflightReport, error) {
	return f.preflightOut, f.preflightErr
}

// newTestRouter builds a minimal gin engine with tenant/user context injected,
// mounting the change-plan handler at the same paths used by RegisterAdmin.
func newTestRouter(fake admin.ChangePlanOrchestratorForTest) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := admin.NewChangePlanHandler(fake, nil)
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", "11111111-1111-1111-1111-111111111111")
		c.Set("user_id", "u-1")
		c.Next()
	})
	r.POST("/admin/stores/:storeId/subscription/change-plan", h.ChangePlan)
	r.GET("/admin/stores/:storeId/subscription/change-plan/preflight", h.ChangePlanPreflight)
	return r
}

func postChangePlan(t *testing.T, r *gin.Engine, storeID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+storeID+"/subscription/change-plan",
		bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func getPreflight(t *testing.T, r *gin.Engine, storeID, targetPlan, targetPeriod string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/admin/stores/" + storeID + "/subscription/change-plan/preflight" +
		"?target_plan=" + targetPlan + "&target_period=" + targetPeriod
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const validStoreID = "22222222-2222-2222-2222-222222222222"

func validChangePlanBody() map[string]any {
	return map[string]any{
		"target_plan":   "studio",
		"target_period": "annual",
	}
}

// --- ChangePlan tests ---

func TestChangePlan_Success_ReturnsUpgradeCommitted(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	fake := &fakeOrch{
		executeOut: planchange.Output{
			Result:        planchange.ResultUpgradeCommitted,
			EffectiveAt:   now,
			StripeUpdated: true,
		},
	}
	r := newTestRouter(fake)
	w := postChangePlan(t, r, validStoreID, validChangePlanBody())

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "upgrade_committed", resp["result"])
	assert.Equal(t, true, resp["stripe_updated"])
	assert.NotEmpty(t, resp["effective_at"])
}

func TestChangePlan_ReadOnlyStatus_Returns402(t *testing.T) {
	fake := &fakeOrch{executeErr: planchange.ErrSubscriptionReadOnly}
	r := newTestRouter(fake)
	w := postChangePlan(t, r, validStoreID, validChangePlanBody())

	assert.Equal(t, http.StatusPaymentRequired, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "subscription_read_only", resp["error"])
}

func TestChangePlan_CurrencyLocked_Returns409(t *testing.T) {
	fake := &fakeOrch{executeErr: planchange.ErrCurrencyLocked}
	r := newTestRouter(fake)
	w := postChangePlan(t, r, validStoreID, validChangePlanBody())

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "currency_locked", resp["error"])
}

func TestChangePlan_OverQuota_Returns422(t *testing.T) {
	fake := &fakeOrch{executeErr: planchange.ErrStoreCountOverQuota}
	r := newTestRouter(fake)
	w := postChangePlan(t, r, validStoreID, validChangePlanBody())

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "store_count_over_quota", resp["error"])
}

// A failed audit write must not change what the merchant is told: the
// downgrade was still refused for quota, and the frontend keys on the
// store_count_over_quota slug (#397 review finding 1).
func TestChangePlan_OverQuota_AuditWriteFailed_Still422(t *testing.T) {
	wrapped := errors.Join(
		planchange.ErrStoreCountOverQuota,
		fmt.Errorf("planchange: write blocked downgrade audit row: %w", errors.New("boom")),
	)
	fake := &fakeOrch{executeErr: wrapped}
	r := newTestRouter(fake)
	w := postChangePlan(t, r, validStoreID, validChangePlanBody())

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.Contains(t, w.Body.String(), "store_count_over_quota")
}

func TestChangePlan_NoChange_Returns409(t *testing.T) {
	fake := &fakeOrch{executeErr: planchange.ErrNoChange}
	r := newTestRouter(fake)
	w := postChangePlan(t, r, validStoreID, validChangePlanBody())

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "no_change", resp["error"])
}

func TestChangePlan_InvalidTarget_Returns400(t *testing.T) {
	fake := &fakeOrch{executeErr: planchange.ErrInvalidTargetPlan}
	r := newTestRouter(fake)
	w := postChangePlan(t, r, validStoreID, validChangePlanBody())

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "invalid_target_plan", resp["error"])
}

func TestChangePlan_InvalidJSON_Returns400(t *testing.T) {
	fake := &fakeOrch{}
	r := newTestRouter(fake)

	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+validStoreID+"/subscription/change-plan",
		bytes.NewBufferString("{not valid json}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChangePlan_InvalidStoreID_Returns400(t *testing.T) {
	fake := &fakeOrch{}
	r := newTestRouter(fake)
	w := postChangePlan(t, r, "not-a-uuid", validChangePlanBody())

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_failed", resp["error"])
}

// --- ChangePlanPreflight tests ---

func TestChangePlanPreflight_ReportsDowngradeBlock(t *testing.T) {
	fake := &fakeOrch{
		preflightOut: planchange.PreflightReport{
			Decision:         planchange.DecisionBlockStoreCount,
			StoreCount:       3,
			TargetStoreLimit: 1,
			Stores: []planchange.StoreInfo{
				{Name: "Store A", Slug: "store-a", Status: "active"},
				{Name: "Store B", Slug: "store-b", Status: "active"},
				{Name: "Store C", Slug: "store-c", Status: "active"},
			},
		},
	}
	r := newTestRouter(fake)
	w := getPreflight(t, r, validStoreID, "starter", "annual")

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "block_store_count", resp["decision"])
	assert.Equal(t, float64(3), resp["store_count"])
	stores, ok := resp["stores"].([]any)
	require.True(t, ok)
	assert.Len(t, stores, 3)
}

func TestChangePlanPreflight_AllowUpgrade_Returns200(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	fake := &fakeOrch{
		preflightOut: planchange.PreflightReport{
			Decision:    planchange.DecisionAllowUpgradeNow,
			EffectiveAt: now,
		},
	}
	r := newTestRouter(fake)
	w := getPreflight(t, r, validStoreID, "studio", "annual")

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "allow_upgrade_now", resp["decision"])
}

func TestChangePlanPreflight_InvalidStoreID_Returns400(t *testing.T) {
	fake := &fakeOrch{}
	r := newTestRouter(fake)
	w := getPreflight(t, r, "bad-uuid", "studio", "annual")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
