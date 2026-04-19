package windowguard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/tax/windowguard"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeRepo satisfies windowguard.SubscriptionLoader with a single in-memory row.
type fakeRepo struct {
	sub *subscription.StoreSubscription
	err error
}

func (f *fakeRepo) GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*subscription.StoreSubscription, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sub, nil
}

func subFixture(country string, validated bool, createdAt time.Time, fastPath *time.Time) *subscription.StoreSubscription {
	c := country
	return &subscription.StoreSubscription{
		ID:                     uuid.New(),
		TenantID:               uuid.New(),
		StoreID:                uuid.New(),
		Status:                 subscription.StatusTrialing,
		Plan:                   subscription.PlanTrial,
		TaxIDCountry:           &c,
		TaxIDValidated:         validated,
		TaxIDWindowShortenedAt: fastPath,
		CreatedAt:              createdAt,
	}
}

func runRequest(t *testing.T, mw gin.HandlerFunc, sub *subscription.StoreSubscription) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", sub.TenantID.String())
		c.Next()
	})
	r.POST("/admin/stores/:storeId/storefront/publish", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "published"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+sub.StoreID.String()+"/storefront/publish", nil)
	r.ServeHTTP(rec, req)
	return rec
}

func TestWindowGuard_BeforeDay14_AllowsPublish(t *testing.T) {
	sub := subFixture("GB", false, time.Now().Add(-7*24*time.Hour), nil)
	mw := windowguard.RequirePublishable(windowguard.Config{Repo: &fakeRepo{sub: sub}})
	rec := runRequest(t, mw, sub)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestWindowGuard_PastDay14_Unvalidated_NotPaused_Blocks(t *testing.T) {
	sub := subFixture("GB", false, time.Now().Add(-15*24*time.Hour), nil)
	mw := windowguard.RequirePublishable(windowguard.Config{Repo: &fakeRepo{sub: sub}})
	rec := runRequest(t, mw, sub)
	require.Equal(t, http.StatusForbidden, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "tax_validation_window_expired", body["error"])
}

func TestWindowGuard_Validated_AlwaysAllows(t *testing.T) {
	sub := subFixture("GB", true, time.Now().Add(-30*24*time.Hour), nil)
	mw := windowguard.RequirePublishable(windowguard.Config{Repo: &fakeRepo{sub: sub}})
	rec := runRequest(t, mw, sub)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestWindowGuard_FastPathShortensTo48h(t *testing.T) {
	now := time.Now()
	fp := now.Add(-3 * 24 * time.Hour)
	sub := subFixture("GB", false, now.Add(-3*24*time.Hour), &fp)
	mw := windowguard.RequirePublishable(windowguard.Config{Repo: &fakeRepo{sub: sub}})
	rec := runRequest(t, mw, sub)
	require.Equal(t, http.StatusForbidden, rec.Code, "3 days > 48h fast-path window")
}

func TestWindowGuard_FastPathStillInside48h_Allows(t *testing.T) {
	now := time.Now()
	fp := now.Add(-12 * time.Hour)
	sub := subFixture("GB", false, now.Add(-12*time.Hour), &fp)
	mw := windowguard.RequirePublishable(windowguard.Config{Repo: &fakeRepo{sub: sub}})
	rec := runRequest(t, mw, sub)
	require.Equal(t, http.StatusOK, rec.Code)
}
