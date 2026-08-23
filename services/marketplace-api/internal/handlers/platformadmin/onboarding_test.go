package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

// errBoom is a generic non-sentinel error used to prove the handler's
// default branch (-> 500 internal_error) rather than the ErrUnavailable
// branch (-> 503).
var errBoom = errors.New("boom")

// stubFunnelClient records params and returns canned results.
type stubFunnelClient struct {
	gotFunnelParams   onboardingfunnel.FunnelParams
	gotSessionsParams onboardingfunnel.SessionsParams
	funnel            *onboardingfunnel.FunnelStats
	sessions          *onboardingfunnel.SessionsResult
	err               error
}

func (s *stubFunnelClient) GetFunnel(_ context.Context, p onboardingfunnel.FunnelParams) (*onboardingfunnel.FunnelStats, error) {
	s.gotFunnelParams = p
	if s.err != nil {
		return nil, s.err
	}
	if s.funnel == nil {
		s.funnel = &onboardingfunnel.FunnelStats{}
	}
	return s.funnel, nil
}

func (s *stubFunnelClient) ListSessions(_ context.Context, p onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error) {
	s.gotSessionsParams = p
	if s.err != nil {
		return nil, s.err
	}
	if s.sessions == nil {
		s.sessions = &onboardingfunnel.SessionsResult{Sessions: []onboardingfunnel.Session{}}
	}
	return s.sessions, nil
}

func funnelRouter(t *testing.T, client platformadmin.OnboardingFunnel) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewOnboardingFunnelHandler(client, nil).Register(r.Group(""))
	return r
}

func ptrFloat64(v float64) *float64 { return &v }

func mustParseTime(t *testing.T, v string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, v)
	require.NoError(t, err)
	return tm
}

// THE test. Real handler output compared to the committed contract.
func TestOnboardingFunnelMatchesContract(t *testing.T) {
	client := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{
		FunnelCounts: onboardingfunnel.FunnelCounts{
			Started: 412, EmailVerified: 301, Completed: 188, InFlight: 34, Abandoned: 190,
		},
		MedianCompletionSeconds: ptrFloat64(743),
		Last24h: onboardingfunnel.FunnelCounts{
			Started: 12, Completed: 5,
		},
		Window: onboardingfunnel.FunnelWindow{
			From: "2026-08-01T00:00:00Z", To: "2026-08-23T00:00:00Z",
		},
	}}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/funnel", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/onboarding_funnel_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// median_completion_seconds must serialise as JSON null, not omitted and not
// 0. Asserted against the raw body: decoding null and 0 into a *float64
// collapses the very distinction this test exists to catch.
func TestOnboardingFunnelMedianNullWhenNoCompletions(t *testing.T) {
	client := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{
		MedianCompletionSeconds: nil,
		Window:                  onboardingfunnel.FunnelWindow{From: "2026-08-01T00:00:00Z", To: "2026-08-23T00:00:00Z"},
	}}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/funnel", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	raw, ok := body.Data["median_completion_seconds"]
	require.True(t, ok, "median_completion_seconds key must be present")
	require.Equal(t, "null", string(raw))
}

// The funnel response is a single object with no pagination key.
func TestOnboardingFunnelHasNoPaginationKey(t *testing.T) {
	client := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{}}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/funnel", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "pagination")
}

func TestOnboardingFunnelPassesWindowParams(t *testing.T) {
	client := &stubFunnelClient{funnel: &onboardingfunnel.FunnelStats{}}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/funnel?created_from=2026-08-01T00:00:00Z&created_to=2026-08-23T00:00:00Z", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, client.gotFunnelParams.CreatedFrom.Equal(mustParseTime(t, "2026-08-01T00:00:00Z")))
	require.True(t, client.gotFunnelParams.CreatedTo.Equal(mustParseTime(t, "2026-08-23T00:00:00Z")))
}

// An unreachable upstream must NOT look like an empty funnel — that would
// tell a console operator "no activity" when the truth is "we could not
// ask". The body must carry no counters at all.
func TestOnboardingFunnelUpstreamDownIs503(t *testing.T) {
	client := &stubFunnelClient{err: onboardingfunnel.ErrUnavailable}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/funnel", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))
	require.NotContains(t, rec.Body.String(), "started")
	require.NotContains(t, rec.Body.String(), "median_completion_seconds")
}

func TestOnboardingFunnelOtherErrorIs500(t *testing.T) {
	client := &stubFunnelClient{err: errBoom}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/funnel", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
}

// THE test for sessions. Real handler output compared to the committed
// contract. idle_hours comes straight from the upstream Session field —
// the handler no longer computes it against its own clock — so the stub
// sets it directly, deterministically.
func TestOnboardingSessionsMatchesContract(t *testing.T) {
	completedAt := mustParseTime(t, "2026-08-21T03:00:00Z")
	tenantID := "3f2504e0-4f89-11d3-9a0c-0305e82c3301"

	client := &stubFunnelClient{sessions: &onboardingfunnel.SessionsResult{
		Total: 2, Page: 1, Limit: 50,
		Sessions: []onboardingfunnel.Session{
			{
				ID:             "9f2504e0-4f89-11d3-9a0c-0305e82c9901",
				Email:          "founder@acme.example",
				Status:         "in_progress",
				CreatedAt:      mustParseTime(t, "2026-08-20T09:00:00Z"),
				LastActivityAt: mustParseTime(t, "2026-08-21T02:00:00Z"),
				Abandoned:      true,
				IdleHours:      32,
			},
			{
				ID:             "9f2504e0-4f89-11d3-9a0c-0305e82c9902",
				Email:          "owner@beta.example",
				Status:         "completed",
				CreatedAt:      mustParseTime(t, "2026-08-19T08:00:00Z"),
				LastActivityAt: mustParseTime(t, "2026-08-19T09:00:00Z"),
				CompletedAt:    &completedAt,
				TenantID:       &tenantID,
				Abandoned:      false,
				IdleHours:      73,
			},
		},
	}}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/sessions", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/onboarding_sessions_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// draft must never reach the console. onboardingfunnel.Session carries no
// Draft field at all — Task 3's client already refuses to decode it — so
// this is a permanent regression guard: it fails only if a future edit
// either adds Draft back to Session and passes it through by embedding, or
// stops projecting sessions field by field.
func TestOnboardingSessionsDraftAbsentFromRawBody(t *testing.T) {
	client := &stubFunnelClient{sessions: &onboardingfunnel.SessionsResult{
		Sessions: []onboardingfunnel.Session{
			{
				ID:             "9f2504e0-4f89-11d3-9a0c-0305e82c9901",
				Email:          "founder@acme.example",
				Status:         "in_progress",
				CreatedAt:      mustParseTime(t, "2026-08-20T09:00:00Z"),
				LastActivityAt: mustParseTime(t, "2026-08-21T02:00:00Z"),
				Abandoned:      true,
			},
		},
		Total: 1, Page: 1, Limit: 50,
	}}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/sessions", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, strings.Contains(strings.ToLower(rec.Body.String()), "draft"),
		"raw response body must never contain a draft key")
}

func TestOnboardingSessionsEmptyIsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	funnelRouter(t, &stubFunnelClient{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/sessions", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}

func TestOnboardingSessionsPassesFilters(t *testing.T) {
	client := &stubFunnelClient{}
	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/sessions?status=in_progress&abandoned=true&limit=25&page=2", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "in_progress", client.gotSessionsParams.Status)
	require.NotNil(t, client.gotSessionsParams.Abandoned)
	require.True(t, *client.gotSessionsParams.Abandoned)
	require.Equal(t, 25, client.gotSessionsParams.Limit)
	require.Equal(t, 2, client.gotSessionsParams.Page)
}

// pagination.limit must reflect what the client reported (the clamped,
// effective limit), not what the caller requested.
func TestOnboardingSessionsPaginationReflectsClientNotRequest(t *testing.T) {
	client := &stubFunnelClient{sessions: &onboardingfunnel.SessionsResult{
		Sessions: []onboardingfunnel.Session{}, Total: 0, Page: 1, Limit: 500,
	}}

	rec := httptest.NewRecorder()
	funnelRouter(t, client).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/onboarding/sessions?limit=9999", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Pagination struct {
			Limit int `json:"limit"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 500, body.Pagination.Limit)
}

// An unreachable upstream must NOT look like an empty session list.
func TestOnboardingSessionsUpstreamDownIs503(t *testing.T) {
	rec := httptest.NewRecorder()
	funnelRouter(t, &stubFunnelClient{err: onboardingfunnel.ErrUnavailable}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/onboarding/sessions", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))
}

func TestOnboardingSessionsOtherErrorIs500(t *testing.T) {
	rec := httptest.NewRecorder()
	funnelRouter(t, &stubFunnelClient{err: errBoom}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/onboarding/sessions", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", errorCode(t, rec))
}
