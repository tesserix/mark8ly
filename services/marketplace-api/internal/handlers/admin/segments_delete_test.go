package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
)

// segmentDeleteRepo stubs the campaign repository methods DeleteSegment
// touches so the handler can be exercised without a database. Any other
// Repository call panics on the nil embedded interface.
type segmentDeleteRepo struct {
	campaign.Repository

	seg           *campaign.CustomerSegment
	campaignCount int64
	deleteCalled  bool
}

func (f *segmentDeleteRepo) GetSegmentByID(_ context.Context, _ *gorm.DB, _ uuid.UUID) (*campaign.CustomerSegment, error) {
	return f.seg, nil
}

func (f *segmentDeleteRepo) CountCampaignsBySegment(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (int64, error) {
	return f.campaignCount, nil
}

func (f *segmentDeleteRepo) DeleteSegment(_ *gorm.DB, _ uuid.UUID) error {
	f.deleteCalled = true
	return nil
}

func runSegmentDelete(t *testing.T, campaignCount int64) (*httptest.ResponseRecorder, map[string]any, *segmentDeleteRepo) {
	t.Helper()

	segID := uuid.New()
	repo := &segmentDeleteRepo{
		seg: &campaign.CustomerSegment{
			ID:       segID,
			TenantID: uuid.New(),
			StoreID:  uuid.New(),
			Name:     "VIPs",
		},
		campaignCount: campaignCount,
	}
	svc := campaign.NewService(campaign.ServiceConfig{Repo: repo})
	h := admin.NewSegmentHandler(svc, nil)

	r := gin.New()
	r.DELETE("/admin/stores/:storeId/segments/:id", h.Delete)

	req := httptest.NewRequest(http.MethodDelete,
		"/admin/stores/"+uuid.NewString()+"/segments/"+segID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("json: %v (body %q)", err, w.Body.String())
		}
	}
	return w, body, repo
}

// TestSegmentDelete_Referenced_Returns409 is the wire-level guard: the case
// the admin Delete confirm dialog warns about must render 409 with an
// actionable code, not the 500 a raw FK violation used to produce.
func TestSegmentDelete_Referenced_Returns409(t *testing.T) {
	w, body, repo := runSegmentDelete(t, 2)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %q)", w.Code, w.Body.String())
	}
	if body["error"] != "segment_in_use" {
		t.Fatalf("error = %v, want segment_in_use", body["error"])
	}
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details object, got %v", body["details"])
	}
	if details["campaign_count"] != float64(2) {
		t.Fatalf("campaign_count = %v, want 2", details["campaign_count"])
	}
	if repo.deleteCalled {
		t.Fatal("the segment was deleted despite live campaign references")
	}
}

// TestSegmentDelete_Unreferenced_Returns204 proves the happy path is intact.
func TestSegmentDelete_Unreferenced_Returns204(t *testing.T) {
	w, _, repo := runSegmentDelete(t, 0)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body %q)", w.Code, w.Body.String())
	}
	if !repo.deleteCalled {
		t.Fatal("expected the segment to be deleted")
	}
}
