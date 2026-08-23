package onboarding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubFunnelRepo records the filter it received and returns canned results.
// Embeds Repository so it only needs to implement the two methods the
// analytics handlers actually call.
type stubFunnelRepo struct {
	Repository
	gotFunnelFilter  FunnelFilter
	funnelStats      *FunnelStats
	funnelErr        error
	gotSessionFilter FunnelFilter
	sessionRows      []SessionRow
	sessionTotal     int64
	sessionErr       error
}

func (s *stubFunnelRepo) GetFunnel(_ context.Context, f FunnelFilter) (*FunnelStats, error) {
	s.gotFunnelFilter = f
	return s.funnelStats, s.funnelErr
}

func (s *stubFunnelRepo) ListSessions(_ context.Context, f FunnelFilter) ([]SessionRow, int64, error) {
	s.gotSessionFilter = f
	return s.sessionRows, s.sessionTotal, s.sessionErr
}

// analyticsRouter mounts only RegisterAnalytics, for tests that don't care
// about route collisions with the public wizard routes.
func analyticsRouter(t *testing.T, repo Repository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(NewService(Config{Repo: repo}), nil)
	h.RegisterAnalytics(r.Group(""))
	return r
}

func TestFunnel_ReturnsCountersNoPaginationKey(t *testing.T) {
	repo := &stubFunnelRepo{funnelStats: &FunnelStats{
		FunnelCounts: FunnelCounts{Started: 10, EmailVerified: 8, Completed: 5, InFlight: 3, Abandoned: 2},
	}}
	rec := httptest.NewRecorder()
	analyticsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding/funnel", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	_, hasData := body["data"]
	require.True(t, hasData)
	_, hasPagination := body["pagination"]
	require.False(t, hasPagination, "funnel response must not have a pagination key")

	var data FunnelCounts
	require.NoError(t, json.Unmarshal(body["data"], &data))
	require.Equal(t, int64(10), data.Started)
}

// TestFunnel_MedianNullSerializesJSONNull asserts on the raw JSON body,
// not a decoded struct — decoding makes null and 0 hard to tell apart,
// and that distinction is the whole point (#283).
func TestFunnel_MedianNullSerializesJSONNull(t *testing.T) {
	repo := &stubFunnelRepo{funnelStats: &FunnelStats{
		MedianCompletionSeconds: nil,
	}}
	rec := httptest.NewRecorder()
	analyticsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding/funnel", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"median_completion_seconds":null`)
}

func TestFunnel_AsOfIsNeverSetFromQueryParams(t *testing.T) {
	repo := &stubFunnelRepo{funnelStats: &FunnelStats{}}
	rec := httptest.NewRecorder()
	analyticsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/onboarding/funnel?as_of=2020-01-01T00:00:00Z", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.gotFunnelFilter.AsOf.IsZero(), "AsOf must never be settable from a query param")
}

func TestSessions_EmptyIsArrayNotNull(t *testing.T) {
	repo := &stubFunnelRepo{sessionRows: nil, sessionTotal: 0}
	rec := httptest.NewRecorder()
	analyticsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding/sessions", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}

func TestSessions_LimitClamps500(t *testing.T) {
	repo := &stubFunnelRepo{sessionRows: []SessionRow{}, sessionTotal: 0}
	rec := httptest.NewRecorder()
	analyticsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding/sessions?limit=9999", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, MaxFunnelPageSize, repo.gotSessionFilter.Limit)

	var body struct {
		Pagination struct {
			Limit int `json:"limit"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 500, body.Pagination.Limit)
}

func TestSessions_MissingParamsTakeDefaults(t *testing.T) {
	repo := &stubFunnelRepo{sessionRows: []SessionRow{}, sessionTotal: 0}
	rec := httptest.NewRecorder()
	analyticsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding/sessions", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, DefaultFunnelPageSize, repo.gotSessionFilter.Limit)
	require.Equal(t, 1, max(repo.gotSessionFilter.Page, 1))

	var body struct {
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, DefaultFunnelPageSize, body.Pagination.Limit)
}

func TestSessions_GarbageParamsDoNotError(t *testing.T) {
	repo := &stubFunnelRepo{sessionRows: []SessionRow{}, sessionTotal: 0}
	rec := httptest.NewRecorder()
	analyticsRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/onboarding/sessions?limit=abc&page=-4", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, DefaultFunnelPageSize, repo.gotSessionFilter.Limit)
}

// fullRouter mounts the onboarding handler exactly the way cmd/server/
// main.go does: Register on the public /api/v1 group, RegisterAnalytics on
// a SEPARATE strict internal group. This is the real two-group shape —
// verified not to panic at router-build time and not to shadow either
// sibling route, the #287 class of bug: /onboarding/sessions (static,
// analytics) and /onboarding/sessions/:id (public wizard) sit at the same
// path position.
func fullRouter(t *testing.T, repo Repository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(NewService(Config{Repo: repo}), nil)

	public := r.Group("/api/v1")
	h.Register(public)

	strictInternal := r.Group("/internal")
	h.RegisterAnalytics(strictInternal)

	return r
}

func TestRoute_SessionsStaticSiblingDoesNotShadowPublicWildcard(t *testing.T) {
	repo := &stubFunnelRepo{
		sessionRows:  []SessionRow{{ID: "row-1"}},
		sessionTotal: 1,
		funnelStats:  &FunnelStats{},
	}

	require.NotPanics(t, func() {
		fullRouter(t, repo)
	})

	r := fullRouter(t, repo)

	// New static analytics route resolves to its own handler.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/onboarding/sessions", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var listBody struct {
		Data []SessionRow `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listBody))
	require.Len(t, listBody.Data, 1)
	require.Equal(t, "row-1", listBody.Data[0].ID)

	// Public wizard's GET /api/v1/onboarding/sessions/:id (wildcard) still
	// resolves to its own handler, not the static analytics route.
	sessRepo := &sessionByIDStub{sess: &Session{ID: "sess-abc"}}
	sessRouter := gin.New()
	gin.SetMode(gin.TestMode)
	sh := NewHandler(NewService(Config{Repo: sessRepo}), nil)
	pub := sessRouter.Group("/api/v1")
	sh.Register(pub)
	strict := sessRouter.Group("/internal")
	sh.RegisterAnalytics(strict)

	rec2 := httptest.NewRecorder()
	sessRouter.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/v1/onboarding/sessions/sess-abc", nil))
	require.Equal(t, http.StatusOK, rec2.Code)
	var getBody struct {
		Data Session `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &getBody))
	require.Equal(t, "sess-abc", getBody.Data.ID)
}

// sessionByIDStub satisfies Repository just enough for GetByID, used to
// prove the public /:id wildcard still resolves after RegisterAnalytics
// adds the static sibling.
type sessionByIDStub struct {
	Repository
	sess *Session
}

func (s *sessionByIDStub) GetByID(_ context.Context, _ string) (*Session, error) {
	return s.sess, nil
}
