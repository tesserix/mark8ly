package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/review"
)

// StoreMiddleware proves that :storeId belongs to the caller's tenant and
// nothing more. Every route below takes a SECOND id from the path, and each
// of the services behind them is keyed on that bare id — so the handler is
// the only thing that can refuse another tenant's row.
//
// store_scope_arch_test.go pins that each handler shows some scoping. These
// tests pin what that scoping does on the wire: 404, and no side effect.
// Section 1.3 of docs/GO-LIVE-PUNCHLIST.md is what they close.

// ---------------------------------------------------------------------------
// Campaigns
// ---------------------------------------------------------------------------

// campaignScopeRepo answers one campaign for any id, and records the writes
// the handler would drive. Any other Repository call panics on the nil
// embedded interface, which is what keeps this stub honest.
type campaignScopeRepo struct {
	campaign.Repository

	cmp *campaign.Campaign

	updated bool
	deleted bool
}

func (f *campaignScopeRepo) GetCampaignByID(_ context.Context, _ *gorm.DB, _ uuid.UUID) (*campaign.Campaign, error) {
	return f.cmp, nil
}

func (f *campaignScopeRepo) UpdateCampaign(_ *gorm.DB, _ *campaign.Campaign) error {
	f.updated = true
	return nil
}

func (f *campaignScopeRepo) DeleteCampaign(_ *gorm.DB, _ uuid.UUID) error {
	f.deleted = true
	return nil
}

func TestCampaignRoutes_OtherStore_Return404AndDoNotWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cmpID := uuid.New()
	repo := &campaignScopeRepo{cmp: &campaign.Campaign{
		ID:       cmpID,
		TenantID: uuid.New(),
		StoreID:  uuid.New(), // some other store's campaign
		Name:     "Spring sale",
		Status:   "draft",
	}}
	svc := campaign.NewService(campaign.ServiceConfig{Repo: repo})
	h := admin.NewCampaignHandler(svc, repo, slog.Default())

	r := gin.New()
	base := "/admin/stores/:storeId/campaigns/:id"
	r.GET(base, h.Get)
	r.PATCH(base, h.Patch)
	r.DELETE(base, h.Delete)
	r.POST(base+"/send", h.Send)
	r.POST(base+"/schedule", h.Schedule)
	r.POST(base+"/pause", h.Pause)
	r.POST(base+"/resume", h.Resume)

	caller := "/admin/stores/" + uuid.NewString() + "/campaigns/" + cmpID.String()

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"get", http.MethodGet, caller, nil},
		{"patch", http.MethodPatch, caller, map[string]any{"name": "renamed"}},
		{"delete", http.MethodDelete, caller, nil},
		// The one that reaches the outside world: a send puts mail in
		// another merchant's customers' inboxes and spends their budget.
		{"send", http.MethodPost, caller + "/send", nil},
		{"schedule", http.MethodPost, caller + "/schedule", map[string]any{"scheduled_at": "2030-01-01T00:00:00Z"}},
		{"pause", http.MethodPost, caller + "/pause", nil},
		{"resume", http.MethodPost, caller + "/resume", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, tc.method, tc.path, tc.body)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body %q)", w.Code, w.Body.String())
			}
		})
	}

	if repo.updated {
		t.Error("another store's campaign was updated")
	}
	if repo.deleted {
		t.Error("another store's campaign was deleted")
	}
}

// ---------------------------------------------------------------------------
// Reviews
// ---------------------------------------------------------------------------

type reviewScopeRepo struct {
	review.Repository

	rev *review.Review

	statusChanged bool
	featureSet    bool
	replied       bool
}

func (f *reviewScopeRepo) GetByID(_ context.Context, _ string) (*review.Review, error) {
	return f.rev, nil
}

func (f *reviewScopeRepo) UpdateStatus(_ context.Context, _, _ string) (*review.Review, error) {
	f.statusChanged = true
	return f.rev, nil
}

func (f *reviewScopeRepo) SetFeatured(_ context.Context, _ string, _ bool) (*review.Review, error) {
	f.featureSet = true
	return f.rev, nil
}

func (f *reviewScopeRepo) AddReply(_ context.Context, _ *review.ReviewReply) error {
	f.replied = true
	return nil
}

func TestReviewRoutes_OtherStore_Return404AndDoNotModerate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	revID := uuid.NewString()
	repo := &reviewScopeRepo{rev: &review.Review{
		ID:      revID,
		StoreID: uuid.NewString(), // some other store's review
		Status:  review.StatusPending,
	}}
	h := admin.NewReviewsHandler(repo, slog.Default())

	r := gin.New()
	base := "/admin/stores/:storeId/reviews/:id"
	r.GET(base, h.Get)
	r.POST(base+"/approve", h.Approve)
	r.POST(base+"/reject", h.Reject)
	r.POST(base+"/featured", h.ToggleFeatured)
	r.POST(base+"/reply", h.Reply)

	caller := "/admin/stores/" + uuid.NewString() + "/reviews/" + revID

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"get", http.MethodGet, caller, nil},
		{"approve", http.MethodPost, caller + "/approve", nil},
		{"reject", http.MethodPost, caller + "/reject", nil},
		{"featured", http.MethodPost, caller + "/featured", map[string]any{"featured": true}},
		{"reply", http.MethodPost, caller + "/reply", map[string]any{"content": "thanks!"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, tc.method, tc.path, tc.body)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body %q)", w.Code, w.Body.String())
			}
		})
	}

	if repo.statusChanged {
		t.Error("another store's review was approved or rejected")
	}
	if repo.featureSet {
		t.Error("another store's review was featured")
	}
	if repo.replied {
		t.Error("a merchant reply was published on another store's review")
	}
}

// ---------------------------------------------------------------------------
// Loyalty
// ---------------------------------------------------------------------------

type loyaltyScopeRepo struct {
	loyalty.Repository

	member *loyalty.CustomerLoyalty
}

func (f *loyaltyScopeRepo) GetCustomerByID(_ context.Context, _ *gorm.DB, _ uuid.UUID) (*loyalty.CustomerLoyalty, error) {
	return f.member, nil
}

// The service is built with a nil *gorm.DB, so this test does not merely
// assert a 404: an adjust that got past the scope check would open a
// transaction on nil and panic the test rather than pass it quietly.
func TestLoyaltyMemberRoutes_OtherStore_Return404AndDoNotAdjust(t *testing.T) {
	gin.SetMode(gin.TestMode)

	memberID := uuid.New()
	repo := &loyaltyScopeRepo{member: &loyalty.CustomerLoyalty{
		ID:       memberID,
		TenantID: uuid.New(),
		StoreID:  uuid.New(), // some other store's member
	}}
	h := admin.NewLoyaltyHandler(loyalty.NewService(nil, repo, slog.Default()), slog.Default())

	r := gin.New()
	base := "/admin/stores/:storeId/loyalty/members/:id"
	r.GET(base, withTenant(uuid.NewString(), h.GetMember))
	r.POST(base+"/adjust", withTenant(uuid.NewString(), h.AdjustPoints))

	caller := "/admin/stores/" + uuid.NewString() + "/loyalty/members/" + memberID.String()

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"get", http.MethodGet, caller, nil},
		{"adjust", http.MethodPost, caller + "/adjust", map[string]any{"points": 500, "description": "goodwill"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, tc.method, tc.path, tc.body)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body %q)", w.Code, w.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// withTenant stands in for the auth middleware, which is what puts tenant_id
// on the context in production.
func withTenant(tenantID string, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		c.Set("user_email", "staff@example.com")
		next(c)
	}
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
