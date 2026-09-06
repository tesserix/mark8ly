package admin_test

// branding_support_email_test.go — pins the request → UpdateInput wiring
// for support_email (#749).
//
// The service-layer tests in internal/branding prove the field is merged
// and validated correctly once it arrives. They cannot prove it arrives:
// drop `SupportEmail: req.SupportEmail` from the UpdateInput literal in
// Update and every one of them still passes, because in.SupportEmail is
// simply always nil and the nil guard preserves whatever was there. What
// the merchant then sees is a form that saves without error and changes
// nothing. These tests drive the real HTTP handler so that line is load-
// bearing.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// recordingBrandingRepo implements branding.Repository over memory.
type recordingBrandingRepo struct {
	existing *branding.StoreBranding
	upserted *branding.StoreBranding
}

func (r *recordingBrandingRepo) GetByStoreID(context.Context, *gorm.DB, uuid.UUID) (*branding.StoreBranding, error) {
	if r.existing == nil {
		return nil, apperrors.NotFound("branding")
	}
	clone := *r.existing
	return &clone, nil
}

func (r *recordingBrandingRepo) Upsert(_ context.Context, _ *gorm.DB, b *branding.StoreBranding) error {
	clone := *b
	r.upserted = &clone
	return nil
}

func brandingPUT(t *testing.T, repo *recordingBrandingRepo, storeID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := branding.NewService(branding.ServiceConfig{Repo: repo})
	h := admin.NewBrandingHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil)))

	r := gin.New()
	r.PUT("/admin/stores/:storeId/branding", func(c *gin.Context) {
		c.Set("tenant_id", uuid.New().String())
		h.Update(c)
	})

	req := httptest.NewRequest(http.MethodPut,
		"/admin/stores/"+storeID.String()+"/branding", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestBrandingUpdate_SupportEmailReachesTheService(t *testing.T) {
	storeID := uuid.New()
	repo := &recordingBrandingRepo{}

	rec := brandingPUT(t, repo, storeID, `{"support_email":"hello@nadiasceramics.com"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.NotNil(t, repo.upserted)
	require.Equal(t, "hello@nadiasceramics.com", repo.upserted.SupportEmail,
		"the address in the request body must reach the row that gets written")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "hello@nadiasceramics.com", resp["support_email"],
		"and must be echoed back, or the admin form reloads showing it blank")
}

func TestBrandingUpdate_SupportEmailCanBeCleared(t *testing.T) {
	storeID := uuid.New()
	existing := &branding.StoreBranding{
		StoreID: storeID, SupportEmail: "hello@nadiasceramics.com",
		ColorBackground: "#F7F6F2", ColorText: "#0E0E0C", ColorAccent: "#2D4A2B",
		ColorButtonBg: "#0E0E0C", ColorButtonText: "#F7F6F2",
	}
	repo := &recordingBrandingRepo{existing: existing}

	rec := brandingPUT(t, repo, storeID, `{"support_email":""}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "", repo.upserted.SupportEmail)
}

// The admin form PUTs only the keys the merchant changed, so this is the
// shape of almost every real save.
func TestBrandingUpdate_UnrelatedSaveKeepsSupportEmail(t *testing.T) {
	storeID := uuid.New()
	existing := &branding.StoreBranding{
		StoreID: storeID, SupportEmail: "hello@nadiasceramics.com",
		ColorBackground: "#F7F6F2", ColorText: "#0E0E0C", ColorAccent: "#2D4A2B",
		ColorButtonBg: "#0E0E0C", ColorButtonText: "#F7F6F2",
	}
	repo := &recordingBrandingRepo{existing: existing}

	rec := brandingPUT(t, repo, storeID, `{"tagline":"Wheel-thrown in Margate"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "hello@nadiasceramics.com", repo.upserted.SupportEmail,
		"a save that never mentions support_email must not blank it")
}

func TestBrandingUpdate_RejectsUnusableSupportEmail(t *testing.T) {
	storeID := uuid.New()
	repo := &recordingBrandingRepo{}

	rec := brandingPUT(t, repo, storeID, `{"support_email":"Nadia <nadia@nadiasceramics.com>"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Nil(t, repo.upserted, "nothing may be written when validation fails")
}
