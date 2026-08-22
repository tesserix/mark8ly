package admin_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
)

// stubAuditRepository is a no-DB stand-in for audit.Repository. Only List is
// exercised by AuditLogsHandler.List; Create and Stream exist solely to
// satisfy the interface.
type stubAuditRepository struct{}

func (stubAuditRepository) List(_ context.Context, _ *gorm.DB, _ audit.ListFilter) (audit.ListResult, error) {
	return audit.ListResult{Entries: nil, Total: 0}, nil
}

func (stubAuditRepository) Create(_ context.Context, _ *gorm.DB, _ *audit.Entry) error {
	return nil
}

func (stubAuditRepository) Stream(_ context.Context, _ *gorm.DB, _ audit.ListFilter, _ func(*audit.Entry) error) error {
	return nil
}

func (stubAuditRepository) ListPlatform(_ context.Context, _ *gorm.DB, _ audit.PlatformListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

// The merchant Settings -> Audit Logs page consumes the exact envelope shape
// produced by admin.AuditLogsHandler.List via
// apps/admin/lib/api/settings-tier2-api.ts. The platform console requires a
// DIFFERENT shape ({data, pagination:{page, limit, total}}) and is served by
// internal/handlers/platformadmin. This test runs the real merchant handler
// (with a stub repository, no DB needed) and asserts on its own marshalled
// response body — not on a hand-written JSON literal — so a future change
// that merges the two presenters actually breaks this test. If it fails,
// someone has merged the two presenters — don't "fix" it by changing the
// assertion.
func TestMerchantAuditLogsEnvelopeIsStable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := admin.NewAuditLogsHandler(nil, stubAuditRepository{}, slog.Default())

	tenantID := uuid.New()
	storeID := uuid.New()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Next()
	})
	r.GET("/admin/stores/:storeId/audit-logs", h.List)

	req := httptest.NewRequest(http.MethodGet, "/admin/stores/"+storeID.String()+"/audit-logs", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	require.Contains(t, got, "meta", "merchant envelope uses meta, not pagination")
	require.NotContains(t, got, "pagination", "pagination belongs to the platform surface")

	var meta map[string]any
	require.NoError(t, json.Unmarshal(got["meta"], &meta))
	for _, k := range []string{"page", "page_size", "total", "total_pages"} {
		require.Contains(t, meta, k)
	}
}
