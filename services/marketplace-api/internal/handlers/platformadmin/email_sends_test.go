package platformadmin_test

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

	"github.com/mark8ly/marketplace-api/internal/emaillog"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

type stubSendLister struct {
	res   emaillog.PlatformListResult
	err   error
	gotF  emaillog.PlatformListFilter
	calls int
}

func (s *stubSendLister) ListPlatform(_ context.Context, _ *gorm.DB,
	f emaillog.PlatformListFilter, _ time.Time) (emaillog.PlatformListResult, error) {
	s.calls++
	s.gotF = f
	return s.res, s.err
}

func sendsRouter(t *testing.T, repo platformadmin.EmailSendLister) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewEmailSendsHandler(nil, repo, nil).Register(r.Group(""))
	return r
}

// The contract's whole point: a governance surface that answers "did it
// arrive" without carrying what was said. Asserted against the SERIALISED
// row, not the struct — a field added later would slip past a struct check.
func TestEmailSendsCarriesNoCustomerContent(t *testing.T) {
	tid, sid := uuid.New(), uuid.New()
	sentAt := time.Now().UTC()
	repo := &stubSendLister{res: emaillog.PlatformListResult{
		Total: 1,
		Sends: []emaillog.PlatformRow{{
			ID: uuid.New(), TenantID: &tid, StoreID: &sid,
			Recipient: "buyer@example.com", Kind: "orderdoc",
			Status: "delivered", CreatedAt: sentAt, SentAt: &sentAt,
		}},
	}}

	rec := httptest.NewRecorder()
	sendsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/email-sends", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	for _, forbidden := range []string{"subject", "body", "html_body", "text_body", "content"} {
		require.NotContainsf(t, body.Data[0], forbidden,
			"%q is interpolated customer content and must never reach this surface", forbidden)
	}
	require.Equal(t, "orderdoc", body.Data[0]["kind"])
	require.Equal(t, "delivered", body.Data[0]["status"])
}

// A settled row omits age_seconds entirely. A number growing forever beside a
// genuinely stuck row would read as stuck.
func TestEmailSendsOmitsAgeForSettledRows(t *testing.T) {
	age := int64(4200)
	repo := &stubSendLister{res: emaillog.PlatformListResult{
		Total: 2,
		Sends: []emaillog.PlatformRow{
			{ID: uuid.New(), Recipient: "a@example.com", Kind: "ticket",
				Status: "delivered", CreatedAt: time.Now().UTC()},
			{ID: uuid.New(), Recipient: "b@example.com", Kind: "ticket",
				Status: "sending", CreatedAt: time.Now().UTC(), AgeSeconds: &age},
		},
	}}

	rec := httptest.NewRecorder()
	sendsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/email-sends", nil))

	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotContains(t, body.Data[0], "age_seconds", "a settled row has no waiting time")
	require.EqualValues(t, 4200, body.Data[1]["age_seconds"])
}

// Filters must reach the query rather than being applied to an already-paged
// response — the defect #406 was about.
func TestEmailSendsForwardsFilters(t *testing.T) {
	repo := &stubSendLister{}
	tid := uuid.New()
	rec := httptest.NewRecorder()
	sendsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/admin/email-sends?tenant_id="+tid.String()+
			"&kind=giftcard&status=bounced&stuck_minutes=30&page=2&limit=25", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.gotF.TenantID)
	require.Equal(t, tid, *repo.gotF.TenantID)
	require.Equal(t, "giftcard", repo.gotF.Kind)
	require.Equal(t, "bounced", repo.gotF.Status)
	require.Equal(t, 30, repo.gotF.StuckMinutes)
	require.Equal(t, 2, repo.gotF.Page)
	require.Equal(t, 25, repo.gotF.Limit)
}

// A malformed tenant_id takes the default (no filter) rather than erroring,
// matching every other read on this surface — and must NOT silently become a
// zero uuid, which would filter to nothing and read as "no mail".
func TestEmailSendsMalformedTenantIsIgnoredNotZeroed(t *testing.T) {
	repo := &stubSendLister{}
	rec := httptest.NewRecorder()
	sendsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/email-sends?tenant_id=not-a-uuid", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, repo.gotF.TenantID)
}

// pagination.limit reports the EFFECTIVE limit, so total/limit is a correct
// page count even when the caller asks for more than the ceiling.
func TestEmailSendsReportsTheClampedLimit(t *testing.T) {
	repo := &stubSendLister{res: emaillog.PlatformListResult{Total: 900}}
	rec := httptest.NewRecorder()
	sendsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/email-sends?limit=100000", nil))

	var body struct {
		Pagination struct {
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, emaillog.MaxPlatformPageSize, body.Pagination.Limit)
	require.EqualValues(t, 900, body.Pagination.Total)
}

func TestEmailSendsEmptyIsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	sendsRouter(t, &stubSendLister{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/email-sends", nil))

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}
