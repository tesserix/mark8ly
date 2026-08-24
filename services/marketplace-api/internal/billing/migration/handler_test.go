package migration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/migration"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeReviewStore is a test double for the reviewStore interface.
type fakeReviewStore struct {
	createResult  *migration.Review
	createErr     error
	approveResult *migration.Review
	approveErr    error
	rejectResult  *migration.Review
	rejectErr     error
}

func (f *fakeReviewStore) CreatePending(_ context.Context, _ migration.CreatePendingInput) (*migration.Review, error) {
	return f.createResult, f.createErr
}

func (f *fakeReviewStore) Approve(_ context.Context, id, _ uuid.UUID, _ string) (*migration.Review, error) {
	if f.approveErr != nil {
		return nil, f.approveErr
	}
	if f.approveResult != nil {
		return f.approveResult, nil
	}
	return &migration.Review{ID: id, TenantID: uuid.MustParse(validTenantID), StoreID: uuid.MustParse(validStoreID)}, nil
}

func (f *fakeReviewStore) Reject(_ context.Context, id, _ uuid.UUID, _ string) (*migration.Review, error) {
	if f.rejectErr != nil {
		return nil, f.rejectErr
	}
	if f.rejectResult != nil {
		return f.rejectResult, nil
	}
	return &migration.Review{ID: id, TenantID: uuid.MustParse(validTenantID), StoreID: uuid.MustParse(validStoreID)}, nil
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

// ---------------------------------------------------------------------------
// RegisterInternalRoutes tests (#281 — the route was implemented but never
// mounted; these pin the mount itself, not just the handler in isolation).
// ---------------------------------------------------------------------------

const internalSecret = "test-internal-secret"

// recordingAuditRepo is a stub audit.Repository that records every entry
// handed to Create, so tests can prove an event was (or was not) enqueued
// without a database. Guarded by a mutex — the emitter writes from a worker
// goroutine.
type recordingAuditRepo struct {
	mu      sync.Mutex
	created []audit.Entry
}

func (r *recordingAuditRepo) Create(_ context.Context, _ *gorm.DB, e *audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, *e)
	return nil
}

func (r *recordingAuditRepo) List(context.Context, *gorm.DB, audit.ListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

func (r *recordingAuditRepo) Stream(context.Context, *gorm.DB, audit.ListFilter, func(*audit.Entry) error) error {
	return nil
}

func (r *recordingAuditRepo) ListPlatform(context.Context, *gorm.DB, audit.PlatformListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

func (r *recordingAuditRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.created)
}

func (r *recordingAuditRepo) first() audit.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created[0]
}

func newTestEmitter(t *testing.T, repo audit.Repository) *audit.Emitter {
	t.Helper()
	em := audit.NewEmitter(audit.EmitterConfig{
		Repo:   repo,
		Logger: slog.New(slog.NewTextHandler(httptest.NewRecorder().Body, nil)),
	})
	t.Cleanup(func() { em.Stop(context.Background()) })
	return em
}

// waitForAuditWrite polls the recording repo briefly — Emit is async
// fire-and-forget, so the write may land a beat after the HTTP response.
func waitForAuditWrite(t *testing.T, repo *recordingAuditRepo, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if repo.count() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.GreaterOrEqual(t, repo.count(), want, "audit event was not written in time")
}

// newInternalRouter builds a router with the route mounted exactly as
// production does — via RegisterInternalRoutes, behind auth.HeaderTrustAuth
// — rather than binding Review directly. This is what proves the route is
// actually reachable end to end, not just that the handler method works in
// isolation.
func newInternalRouter(store *fakeReviewStore, em *audit.Emitter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := migration.NewHandler(store, nil, nil)
	if em != nil {
		h.WithAudit(em)
	}
	h.RegisterInternalRoutes(r.Group("/internal"), internalSecret)
	return r
}

func doInternalPost(t *testing.T, r *gin.Engine, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func validInternalHeaders() map[string]string {
	return map[string]string{
		"X-Internal-Auth": internalSecret,
		"X-User-Id":       validUserID,
		"X-Tenant-Id":     validTenantID, // HeaderTrustAuth requires this present; CSM doesn't need it to be the review's real tenant.
	}
}

// TestRegisterInternalRoutes_MountsExpectedPath pins requirement 1: the
// route is registered at POST /internal/csm/migration-fast-path/:id/review
// by RegisterInternalRoutes, not hand-wired at each call site.
func TestRegisterInternalRoutes_MountsExpectedPath(t *testing.T) {
	store := &fakeReviewStore{}
	r := newInternalRouter(store, nil)
	w := doInternalPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "approve",
		"notes":    "Verified via RegisterInternalRoutes mount.",
	}, validInternalHeaders())

	assert.Equal(t, http.StatusOK, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "approved", resp["status"])
}

// TestRegisterInternalRoutes_NoInternalAuthHeader_Returns401BeforeHandler
// proves HeaderTrustAuth actually gates the route: an otherwise-valid
// request missing X-Internal-Auth never reaches Review — the fake store's
// Approve is never called by construction (a 401 from the middleware means
// c.Next() was never called).
func TestRegisterInternalRoutes_NoInternalAuthHeader_Returns401BeforeHandler(t *testing.T) {
	store := &fakeReviewStore{}
	r := newInternalRouter(store, nil)
	headers := validInternalHeaders()
	delete(headers, "X-Internal-Auth")
	w := doInternalPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "approve",
		"notes":    "Should never reach the handler.",
	}, headers)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestRegisterInternalRoutes_MissingUserID_Returns401MissingReviewer proves
// the mounted route (not just the bare handler) surfaces missing_reviewer
// when X-User-Id is absent, even though HeaderTrustAuth itself requires
// X-User-Id to be non-empty to pass at all — so this exercises the same
// contract requirement 3 of the task calls out.
func TestRegisterInternalRoutes_MissingUserID_Returns401MissingReviewer(t *testing.T) {
	store := &fakeReviewStore{}
	r := newInternalRouter(store, nil)
	headers := validInternalHeaders()
	delete(headers, "X-User-Id")
	w := doInternalPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "approve",
		"notes":    "Missing user id entirely.",
	}, headers)

	// HeaderTrustAuth itself rejects a request with no X-User-Id before the
	// handler runs, so this also comes back 401 — but via the middleware's
	// generic "unauthorized", not the handler's "missing_reviewer". Both are
	// 401s the caller cannot get past without a real trust header.
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestRegisterInternalRoutes_NonUUIDUserID_Returns401MissingReviewer covers
// the case HeaderTrustAuth itself does not: a syntactically present but
// non-UUID X-User-Id passes the middleware (it only checks non-empty) and
// must be rejected by the handler's own uuid.Parse check.
func TestRegisterInternalRoutes_NonUUIDUserID_Returns401MissingReviewer(t *testing.T) {
	store := &fakeReviewStore{}
	r := newInternalRouter(store, nil)
	headers := validInternalHeaders()
	headers["X-User-Id"] = "not-a-uuid"
	w := doInternalPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "approve",
		"notes":    "Non-UUID reviewer id.",
	}, headers)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := parseJSON(t, w)
	assert.Equal(t, "missing_reviewer", resp["error"])
}

// ---------------------------------------------------------------------------
// Audit emission tests (requirement 2 — #310: an event with no tenant is
// silently dropped, so the tenant id must be asserted explicitly, not just
// "an event was written").
// ---------------------------------------------------------------------------

func TestReview_Approve_EmitsAuditEvent_WithReviewerDecisionAndTenant(t *testing.T) {
	reviewTenantID := uuid.MustParse(validTenantID)
	reviewStoreID := uuid.MustParse(validStoreID)
	reviewID := uuid.MustParse(validReviewID)
	reviewerID := uuid.MustParse(validUserID)

	repo := &recordingAuditRepo{}
	em := newTestEmitter(t, repo)

	store := &fakeReviewStore{
		approveResult: &migration.Review{
			ID:       reviewID,
			TenantID: reviewTenantID,
			StoreID:  reviewStoreID,
			Status:   "approved",
		},
	}
	r := newInternalRouter(store, em)

	// Deliberately send a different tenant on the header than the review's
	// real tenant, mirroring the task's point that a CSM caller addresses
	// the review by id and the header tenant must not be trusted as the
	// review's tenant.
	headers := validInternalHeaders()
	headers["X-Tenant-Id"] = "99999999-9999-9999-9999-999999999999"

	w := doInternalPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "approve",
		"notes":    "Verified Shopify export CSV.",
	}, headers)
	require.Equal(t, http.StatusOK, w.Code)

	waitForAuditWrite(t, repo, 1)
	entry := repo.first()

	assert.Equal(t, reviewTenantID, entry.TenantID, "audit event must carry the REVIEW's tenant, not the request header's")
	require.NotNil(t, entry.StoreID)
	assert.Equal(t, reviewStoreID, *entry.StoreID)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, reviewerID, *entry.ActorUserID)
	assert.Contains(t, entry.Action, "approve")
	require.NotNil(t, entry.ResourceID)
	assert.Equal(t, reviewID.String(), *entry.ResourceID)
	assert.Equal(t, "Verified Shopify export CSV.", entry.Metadata["notes"])
	assert.Equal(t, "approve", entry.Metadata["decision"])
}

func TestReview_Reject_EmitsAuditEvent_WithReviewerDecisionAndTenant(t *testing.T) {
	// The review's real tenant deliberately differs from the header tenant
	// sent below (same reasoning as the approve test) — this is what makes
	// the assertion actually prove the explicit-TenantID wiring rather than
	// just happening to match the context fallback.
	reviewTenantID := uuid.MustParse(validTenantID)
	reviewStoreID := uuid.MustParse(validStoreID)
	reviewID := uuid.MustParse(validReviewID)
	reviewerID := uuid.MustParse(validUserID)

	repo := &recordingAuditRepo{}
	em := newTestEmitter(t, repo)

	store := &fakeReviewStore{
		rejectResult: &migration.Review{
			ID:       reviewID,
			TenantID: reviewTenantID,
			StoreID:  reviewStoreID,
			Status:   "rejected",
		},
	}
	r := newInternalRouter(store, em)

	headers := validInternalHeaders()
	headers["X-Tenant-Id"] = "88888888-8888-8888-8888-888888888888"

	w := doInternalPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
		"decision": "reject",
		"notes":    "Evidence URL is a broken link.",
	}, headers)
	require.Equal(t, http.StatusOK, w.Code)

	waitForAuditWrite(t, repo, 1)
	entry := repo.first()

	assert.Equal(t, reviewTenantID, entry.TenantID, "audit event must carry the REVIEW's tenant, not the request header's")
	require.NotNil(t, entry.StoreID)
	assert.Equal(t, reviewStoreID, *entry.StoreID)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, reviewerID, *entry.ActorUserID)
	assert.Contains(t, entry.Action, "reject")
	assert.Equal(t, "reject", entry.Metadata["decision"])
	assert.Equal(t, "Evidence URL is a broken link.", entry.Metadata["notes"])
}

// TestReview_NilAuditEmitter_DoesNotPanic pins the nil-safety requirement:
// a Handler constructed without WithAudit (h.audit stays nil) must not
// panic on approve/reject, matching audit.Emitter.Emit's own nil-receiver
// tolerance.
func TestReview_NilAuditEmitter_DoesNotPanic(t *testing.T) {
	store := &fakeReviewStore{}
	r := newInternalRouter(store, nil) // em == nil, WithAudit never called
	assert.NotPanics(t, func() {
		w := doInternalPost(t, r, "/internal/csm/migration-fast-path/"+validReviewID+"/review", map[string]any{
			"decision": "approve",
			"notes":    "No emitter wired.",
		}, validInternalHeaders())
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
