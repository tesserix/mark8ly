package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

type stubBreakGlassLister struct {
	res   breakglass.PlatformListResult
	err   error
	gotF  breakglass.PlatformListFilter
	calls int
}

func (s *stubBreakGlassLister) ListPlatform(_ context.Context, _ *gorm.DB,
	f breakglass.PlatformListFilter, _ time.Time) (breakglass.PlatformListResult, error) {
	s.calls++
	s.gotF = f
	return s.res, s.err
}

type stubBreakGlassDirectory struct {
	res *tenantdirectory.ListResult
	err error
}

func (s *stubBreakGlassDirectory) List(_ context.Context, _ tenantdirectory.ListParams) (*tenantdirectory.ListResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.res != nil {
		return s.res, nil
	}
	return &tenantdirectory.ListResult{}, nil
}

func (s *stubBreakGlassDirectory) Get(_ context.Context, _ string) (*tenantdirectory.TenantDetail, error) {
	return nil, errors.New("not implemented")
}

func (s *stubBreakGlassDirectory) FindByOwnerEmail(_ context.Context, _ string) (*tenantdirectory.Tenant, error) {
	return nil, errors.New("not implemented")
}

func breakGlassRouter(t *testing.T, repo platformadmin.BreakGlassLister, dir platformadmin.TenantDirectory) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewBreakGlassHandler(nil, repo, dir, nil).Register(r.Group(""))
	return r
}

// TestBreakGlassResponseCannotCarryCredentialFields is the handler-side twin
// of breakglass.TestPlatformRowCannotCarryCredentialFields: it marshals a
// response built from a FULLY-POPULATED PlatformRow and asserts none of the
// three forbidden strings appears anywhere in the JSON bytes. The struct-shape
// test guards PlatformRow itself; this one guards the wire response actually
// sent, which is what a caller can observe.
func TestBreakGlassResponseCannotCarryCredentialFields(t *testing.T) {
	tid := uuid.New()
	usedAt := time.Now().UTC()
	rotatedAt := usedAt.Add(-24 * time.Hour)
	scheduledAt := usedAt.Add(24 * time.Hour)
	lockoutExpires := usedAt.Add(time.Hour)
	createdAt := usedAt.Add(-720 * time.Hour)

	repo := &stubBreakGlassLister{res: breakglass.PlatformListResult{
		Total: 1,
		Accounts: []breakglass.PlatformRow{{
			TenantID:            tid,
			TenantName:          "",
			TOTPEnrolled:        true,
			LastUsedAt:          &usedAt,
			LastRotatedAt:       rotatedAt,
			RotationScheduledAt: &scheduledAt,
			LockedOut:           true,
			LockoutExpiresAt:    &lockoutExpires,
			CreatedAt:           createdAt,
		}},
	}}

	rec := httptest.NewRecorder()
	breakGlassRouter(t, repo, &stubBreakGlassDirectory{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/break-glass", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	for _, forbidden := range []string{"password_hash", "secret_path", "totp_secret_ref"} {
		require.NotContainsf(t, body, forbidden,
			"%q must never reach the wire — break_glass_accounts holds the live credential behind it", forbidden)
	}
}

func TestBreakGlassEnvelopeShape(t *testing.T) {
	tid := uuid.New()
	usedAt := time.Now().UTC()
	rotatedAt := usedAt.Add(-24 * time.Hour)
	createdAt := usedAt.Add(-720 * time.Hour)

	repo := &stubBreakGlassLister{res: breakglass.PlatformListResult{
		Total: 1,
		Accounts: []breakglass.PlatformRow{{
			TenantID:      tid,
			TOTPEnrolled:  true,
			LastUsedAt:    &usedAt,
			LastRotatedAt: rotatedAt,
			CreatedAt:     createdAt,
		}},
	}}
	dir := &stubBreakGlassDirectory{res: &tenantdirectory.ListResult{
		Tenants: []tenantdirectory.Tenant{{ID: tid.String(), Name: "Acme Ltd"}},
	}}

	rec := httptest.NewRecorder()
	breakGlassRouter(t, repo, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/break-glass", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []struct {
			TenantID            string  `json:"tenant_id"`
			TenantName          string  `json:"tenant_name"`
			TOTPEnrolled        bool    `json:"totp_enrolled"`
			LastUsedAt          string  `json:"last_used_at"`
			LastRotatedAt       string  `json:"last_rotated_at"`
			RotationScheduledAt *string `json:"rotation_scheduled_at"`
			LockedOut           bool    `json:"locked_out"`
			LockoutExpiresAt    *string `json:"lockout_expires_at"`
			CreatedAt           string  `json:"created_at"`
		} `json:"data"`
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	row := body.Data[0]
	require.Equal(t, tid.String(), row.TenantID)
	require.Equal(t, "Acme Ltd", row.TenantName)
	require.True(t, row.TOTPEnrolled)
	require.Equal(t, usedAt.Format(time.RFC3339), row.LastUsedAt)
	require.Equal(t, rotatedAt.Format(time.RFC3339), row.LastRotatedAt)
	require.Nil(t, row.RotationScheduledAt)
	require.False(t, row.LockedOut)
	require.Nil(t, row.LockoutExpiresAt)
	require.Equal(t, createdAt.Format(time.RFC3339), row.CreatedAt)

	require.Equal(t, 1, body.Pagination.Page)
	require.Equal(t, breakglass.DefaultPlatformPageSize, body.Pagination.Limit)
	require.EqualValues(t, 1, body.Pagination.Total)

	require.NotContains(t, rec.Body.String(), `"source"`)
}

func TestBreakGlassEmptyIsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	breakGlassRouter(t, &stubBreakGlassLister{}, &stubBreakGlassDirectory{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/break-glass", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}

func TestBreakGlassForwardsFilters(t *testing.T) {
	repo := &stubBreakGlassLister{}
	tid := uuid.New()
	usedAfter := "2026-01-01T00:00:00Z"
	usedBefore := "2026-06-01T00:00:00Z"

	rec := httptest.NewRecorder()
	url := "/admin/break-glass?tenant_id=" + tid.String() +
		"&used_after=" + usedAfter + "&used_before=" + usedBefore +
		"&used=true&locked=false&page=3&limit=25"
	breakGlassRouter(t, repo, &stubBreakGlassDirectory{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, url, nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.gotF.TenantID)
	require.Equal(t, tid, *repo.gotF.TenantID)
	require.NotNil(t, repo.gotF.UsedAfter)
	require.Equal(t, usedAfter, repo.gotF.UsedAfter.UTC().Format(time.RFC3339))
	require.NotNil(t, repo.gotF.UsedBefore)
	require.Equal(t, usedBefore, repo.gotF.UsedBefore.UTC().Format(time.RFC3339))
	require.NotNil(t, repo.gotF.Used)
	require.True(t, *repo.gotF.Used)
	require.NotNil(t, repo.gotF.Locked)
	require.False(t, *repo.gotF.Locked)
	require.Equal(t, 3, repo.gotF.Page)
	require.Equal(t, 25, repo.gotF.Limit)
}

func TestBreakGlassMalformedTenantIsIgnoredNotZeroed(t *testing.T) {
	repo := &stubBreakGlassLister{}
	rec := httptest.NewRecorder()
	breakGlassRouter(t, repo, &stubBreakGlassDirectory{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/break-glass?tenant_id=not-a-uuid", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, repo.gotF.TenantID)
}

func TestBreakGlassSortValues(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantDesc bool
	}{
		{"default", "", true},
		{"explicit desc", "sort=-last_used_at", true},
		{"asc", "sort=last_used_at", false},
		{"unknown falls back to default", "sort=bogus", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubBreakGlassLister{}
			rec := httptest.NewRecorder()
			url := "/admin/break-glass"
			if tt.query != "" {
				url += "?" + tt.query
			}
			breakGlassRouter(t, repo, &stubBreakGlassDirectory{}).ServeHTTP(rec, httptest.NewRequest(
				http.MethodGet, url, nil))
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, tt.wantDesc, repo.gotF.SortDesc)
		})
	}
}

func TestBreakGlassReportsTheClampedLimit(t *testing.T) {
	repo := &stubBreakGlassLister{res: breakglass.PlatformListResult{Total: 900}}
	rec := httptest.NewRecorder()
	breakGlassRouter(t, repo, &stubBreakGlassDirectory{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/break-glass?limit=100000", nil))

	var body struct {
		Pagination struct {
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, breakglass.MaxPlatformPageSize, body.Pagination.Limit)
	require.EqualValues(t, 900, body.Pagination.Total)
}

// A directory failure must not put a fabricated name (the raw tenant uuid)
// where the console renders a display string. It degrades to the row still
// being returned, identified by tenant_id, with tenant_name OMITTED
// (dropped by omitempty) — not present as empty string, not present as the
// id — and the request still 200.
func TestBreakGlassTenantNameEnrichmentDegradesOnDirectoryError(t *testing.T) {
	tid := uuid.New()
	repo := &stubBreakGlassLister{res: breakglass.PlatformListResult{
		Total: 1,
		Accounts: []breakglass.PlatformRow{{
			TenantID:      tid,
			LastRotatedAt: time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		}},
	}}
	dir := &stubBreakGlassDirectory{err: errors.New("platform-api unreachable")}

	rec := httptest.NewRecorder()
	breakGlassRouter(t, repo, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/break-glass", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.Equal(t, tid.String(), body.Data[0]["tenant_id"])
	require.NotContains(t, body.Data[0], "tenant_name",
		"a directory outage must omit the name, never fabricate one from the uuid")
}
