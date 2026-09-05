package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// stubRepo records the filter it was handed and returns a canned result, so
// the tests can assert on parsing without a database.
type stubRepo struct {
	result    audit.ListResult
	gotFilter audit.PlatformListFilter
}

func (s *stubRepo) ListPlatform(_ context.Context, _ *gorm.DB, f audit.PlatformListFilter) (audit.ListResult, error) {
	s.gotFilter = f
	if s.result.Entries == nil {
		s.result.Entries = []audit.Entry{}
	}
	return s.result, nil
}

func (s *stubRepo) List(context.Context, *gorm.DB, audit.ListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}
func (s *stubRepo) Create(context.Context, *gorm.DB, *audit.Entry) error { return nil }
func (s *stubRepo) Stream(context.Context, *gorm.DB, audit.ListFilter, func(*audit.Entry) error) error {
	return nil
}

// THE test. The #276 near-miss happened because the Go tests never marshalled
// against the console's parser and the console tests mocked the response —
// both sides green, both wrong. This compares real handler output to the
// pinned contract as bytes.
func TestAuditLogsMatchesPinnedContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	storeID := uuid.New()
	operator := "op_7f3a"
	email := "merchant@example.com"
	prodID := "prod_123"

	repo := &stubRepo{result: audit.ListResult{
		Total: 2,
		Entries: []audit.Entry{
			{
				ID:           uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301"),
				TenantID:     uuid.New(),
				StoreID:      &storeID,
				ActorEmail:   &email,
				ActorType:    audit.ActorUser,
				Action:       "product.deleted",
				ResourceType: "product",
				ResourceID:   &prodID,
				CreatedAt:    time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
			},
			{
				ID:              uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3302"),
				TenantID:        uuid.New(),
				StoreID:         nil,
				ActorOperatorID: &operator,
				ActorType:       audit.ActorOperator,
				Action:          "tenant.suspended",
				ResourceType:    "tenant",
				CreatedAt:       time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC),
				// One row carries metadata and one does not, so the golden
				// file pins both the string encoding and the omission (#313).
				Metadata: audit.Metadata{"reason_code": "abuse"},
			},
		},
	}}

	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs?limit=200&since_hours=720", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/audit_logs_response.json")
	require.NoError(t, err)

	// JSONEq rather than byte equality: key order is not part of the contract,
	// but the exact set of keys and their values is.
	require.JSONEq(t, string(want), rec.Body.String())
}

// A nil Go slice marshals to {} in this codebase's shape, which defeats every
// caller's `?? []` and has already crashed a page in this estate precisely
// when it had no data.
func TestEmptyResultIsEmptyArrayNotNullOrObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{result: audit.ListResult{Total: 0, Entries: nil}}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data       json.RawMessage `json:"data"`
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
	require.Equal(t, int64(0), body.Pagination.Total)
}

// The envelope is "pagination", never "meta". "meta" belongs to the merchant
// surface — see internal/handlers/admin/audit_logs_envelope_test.go.
func TestEnvelopeUsesPaginationNotMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, &stubRepo{}, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "pagination")
	require.NotContains(t, body, "meta")
	require.NotContains(t, body, "source", "the platform API stamps source itself")
}

// Ids go out bare. The platform API namespaces every row as <slug>:<id> on
// arrival; prefixing here produces "mark8ly:mark8ly:9f2".
func TestIdsAreBare(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{result: audit.ListResult{Total: 1, Entries: []audit.Entry{{
		ID:           uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301"),
		ActorType:    audit.ActorSystem,
		Action:       "x",
		ResourceType: "y",
		CreatedAt:    time.Now().UTC(),
	}}}}

	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	require.NotContains(t, rec.Body.String(), "mark8ly:")
	require.Contains(t, rec.Body.String(), "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
}

func TestOversizedLimitIsClampedNotRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs?limit=100000", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 500, repo.gotFilter.Limit, "limit must clamp to MaxPlatformPageSize")
}

// Both parameters are always sent by the console, but a missing one must fall
// back to our default rather than error.
func TestMissingParamsUseDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, audit.DefaultPlatformPageSize, repo.gotFilter.Limit)
}

func TestSinceHoursNarrowsWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs?since_hours=24", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, repo.gotFilter.DateFrom.IsZero())
	require.WithinDuration(t, time.Now().Add(-24*time.Hour), repo.gotFilter.DateFrom, time.Minute)
}

func TestStoreIDIsOptionalNarrowingFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	storeID := uuid.New()
	repo := &stubRepo{}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs?store_id="+storeID.String(), nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, storeID, repo.gotFilter.StoreID)
}

func TestTimestampsAreRFC3339(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{result: audit.ListResult{Total: 1, Entries: []audit.Entry{{
		ID:           uuid.New(),
		ActorType:    audit.ActorSystem,
		Action:       "x",
		ResourceType: "y",
		CreatedAt:    time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
	}}}}

	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	var body struct {
		Data []struct {
			Timestamp string `json:"timestamp"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)

	_, err := time.Parse(time.RFC3339, body.Data[0].Timestamp)
	require.NoError(t, err, "timestamps must be ISO 8601 with offset")
}
