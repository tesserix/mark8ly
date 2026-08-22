package migration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeReviewStore is a test double for the reviewStore interface.
type fakeReviewStore struct {
	createResult *migration.Review
	createErr    error
	approveErr   error
	rejectErr    error
}

func (f *fakeReviewStore) CreatePending(_ context.Context, _ migration.CreatePendingInput) (*migration.Review, error) {
	return f.createResult, f.createErr
}

func (f *fakeReviewStore) Approve(_ context.Context, _, _ uuid.UUID, _ string) error {
	return f.approveErr
}

func (f *fakeReviewStore) Reject(_ context.Context, _, _ uuid.UUID, _ string) error {
	return f.rejectErr
}

// fakeValidator is a test double for PriorPlatformValidator.
type fakeValidator struct{ err error }

func (f *fakeValidator) ValidateWhoisAge(_ context.Context, _ string, _ int) error {
	return f.err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const (
	validTenantID = "11111111-1111-1111-1111-111111111111"
	validStoreID  = "22222222-2222-2222-2222-222222222222"
	validReviewID = "33333333-3333-3333-3333-333333333333"
	validUserID   = "44444444-4444-4444-4444-444444444444"
)

func newSubmitRouter(store *fakeReviewStore, v migration.PriorPlatformValidator, injectTenant bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := migration.NewHandler(store, v, nil)
	r.Use(func(c *gin.Context) {
		if injectTenant {
			c.Set("tenant_id", validTenantID)
		}
		c.Next()
	})
	r.POST("/admin/stores/:storeId/migration-fast-path/submit", h.Submit)
	return r
}

func newReviewRouter(store *fakeReviewStore, injectUser bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := migration.NewHandler(store, nil, nil)
	r.Use(func(c *gin.Context) {
		if injectUser {
			c.Set("user_id", validUserID)
		}
		c.Next()
	})
	r.POST("/internal/csm/migration-fast-path/:id/review", h.Review)
	return r
}

func doPost(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

// ---------------------------------------------------------------------------
// Submit tests
// ---------------------------------------------------------------------------

func TestSubmit_Success_Returns200_WithReviewID(t *testing.T) {
	reviewID := uuid.MustParse(validReviewID)
	store := &fakeReviewStore{
		createResult: &migration.Review{
			ID:     reviewID,
			Status: "pending",
		},
	}
	r := newSubmitRouter(store, nil, true)
	w := doPost(t, r, "/admin/stores/"+validStoreID+"/migration-fast-path/submit", map[string]any{
		"evidence_type":  "platform_screenshot",
		"evidence_url":   "https://example.com/screenshot.png",
		"prior_platform": "shopify",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, validReviewID, resp["review_id"])
	assert.Equal(t, "pending", resp["status"])
}

func TestSubmit_WhoisMissing_Returns400(t *testing.T) {
	store := &fakeReviewStore{}
	r := newSubmitRouter(store, nil, true)
	// evidence_type=whois_domain but no whois_domain field
	w := doPost(t, r, "/admin/stores/"+validStoreID+"/migration-fast-path/submit", map[string]any{
		"evidence_type": "whois_domain",
		"evidence_url":  "https://example.com/whois.png",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "whois_domain_required", resp["error"])
}

func TestSubmit_WhoisTooYoung_Returns400(t *testing.T) {
	store := &fakeReviewStore{}
	v := &fakeValidator{err: migration.ErrNotFound} // any non-nil error triggers the check
	r := newSubmitRouter(store, v, true)
	w := doPost(t, r, "/admin/stores/"+validStoreID+"/migration-fast-path/submit", map[string]any{
		"evidence_type": "whois_domain",
		"evidence_url":  "https://example.com/whois.png",
		"whois_domain":  "newdomain.example.com",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "whois_too_young", resp["error"])
}

func TestSubmit_AlreadyPending_Returns409(t *testing.T) {
	store := &fakeReviewStore{createErr: migration.ErrAlreadyPending}
	r := newSubmitRouter(store, nil, true)
	w := doPost(t, r, "/admin/stores/"+validStoreID+"/migration-fast-path/submit", map[string]any{
		"evidence_type": "platform_screenshot",
		"evidence_url":  "https://example.com/screenshot.png",
	})

	assert.Equal(t, http.StatusConflict, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "already_pending", resp["error"])
}

func TestSubmit_InvalidBody_Returns400(t *testing.T) {
	store := &fakeReviewStore{}
	r := newSubmitRouter(store, nil, true)
	// Missing required evidence_url
	w := doPost(t, r, "/admin/stores/"+validStoreID+"/migration-fast-path/submit", map[string]any{
		"evidence_type": "platform_screenshot",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "invalid_request", resp["error"])
}

func TestSubmit_InvalidStoreID_Returns400(t *testing.T) {
	store := &fakeReviewStore{}
	r := newSubmitRouter(store, nil, true)
	w := doPost(t, r, "/admin/stores/not-a-uuid/migration-fast-path/submit", map[string]any{
		"evidence_type": "platform_screenshot",
		"evidence_url":  "https://example.com/screenshot.png",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "invalid_store_id", resp["error"])
}

// ---------------------------------------------------------------------------
// Review tests
// ---------------------------------------------------------------------------

func TestReview_Approve_Returns200(t *testing.T) {
	store := &fakeReviewStore{}
	r := newReviewRouter(store, true)
	w := doPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "approve",
		"notes":    "Verified Shopify export CSV — domain age confirmed.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "approved", resp["status"])
}

func TestReview_Reject_Returns200(t *testing.T) {
	store := &fakeReviewStore{}
	r := newReviewRouter(store, true)
	w := doPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "reject",
		"notes":    "Evidence URL is a broken link; cannot verify.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "rejected", resp["status"])
}

func TestReview_NotFound_Returns404(t *testing.T) {
	store := &fakeReviewStore{approveErr: migration.ErrNotFound}
	r := newReviewRouter(store, true)
	w := doPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "approve",
		"notes":    "Looks good.",
	})

	assert.Equal(t, http.StatusNotFound, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "not_found", resp["error"])
}

func TestReview_InvalidDecision_Returns400(t *testing.T) {
	store := &fakeReviewStore{}
	r := newReviewRouter(store, true)
	w := doPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "maybe",
		"notes":    "Not sure.",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "invalid_request", resp["error"])
}

func TestReview_MissingReviewer_Returns401(t *testing.T) {
	store := &fakeReviewStore{}
	// injectUser=false — user_id not set on context
	r := newReviewRouter(store, false)
	w := doPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "approve",
		"notes":    "Looks good.",
	})

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "missing_reviewer", resp["error"])
}
