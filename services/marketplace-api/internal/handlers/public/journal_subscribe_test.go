package public

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/journal"
)

// fakeJournalRepo records every Subscribe call so tests can assert on
// normalization and idempotency without a database.
type fakeJournalRepo struct {
	calls []string
	err   error
}

func (f *fakeJournalRepo) Subscribe(email, source string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, email+"|"+source)
	return nil
}

func newJournalTestRouter(repo *fakeJournalRepo, limiter *journal.RateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewJournalSubscribeHandler(repo, limiter, slog.Default())
	r := gin.New()
	r.POST("/api/v1/journal/subscribe", h.Subscribe)
	return r
}

func postJournalSubscribe(r *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/journal/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestJournalSubscribe_ValidEmailSucceeds(t *testing.T) {
	repo := &fakeJournalRepo{}
	r := newJournalTestRouter(repo, journal.NewRateLimiter())

	w := postJournalSubscribe(r, `{"email":"Ada@Example.COM"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"subscribed":true}`, w.Body.String())
	// Normalization (trim + lowercase) happens in the repository layer
	// (see journal.NormalizeEmail and repository_integration_test.go) —
	// the handler's job is just to pass the validated email through with
	// the right source.
	require.Equal(t, []string{"Ada@Example.COM|journal"}, repo.calls)
}

func TestJournalSubscribe_MalformedEmailRejected(t *testing.T) {
	repo := &fakeJournalRepo{}
	r := newJournalTestRouter(repo, journal.NewRateLimiter())

	w := postJournalSubscribe(r, `{"email":"not-an-email"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.JSONEq(t, `{"error":"validation_failed","message":"a valid email address is required"}`, w.Body.String())
	require.Empty(t, repo.calls, "malformed input must never reach the repository")
}

func TestJournalSubscribe_MissingEmailRejected(t *testing.T) {
	repo := &fakeJournalRepo{}
	r := newJournalTestRouter(repo, journal.NewRateLimiter())

	w := postJournalSubscribe(r, `{}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Empty(t, repo.calls)
}

func TestJournalSubscribe_OversizedEmailRejected(t *testing.T) {
	repo := &fakeJournalRepo{}
	r := newJournalTestRouter(repo, journal.NewRateLimiter())

	longLocal := make([]byte, journal.MaxEmailLength)
	for i := range longLocal {
		longLocal[i] = 'a'
	}
	body := `{"email":"` + string(longLocal) + `@example.com"}`

	w := postJournalSubscribe(r, body)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Empty(t, repo.calls, "an absurdly long email must be rejected before it reaches storage")
}

// Idempotency at the handler layer: two identical submissions both
// return 200. The "one row" guarantee itself is proven by the
// repository's ON CONFLICT DO NOTHING (see repository_integration_test.go);
// this test proves the handler never turns a duplicate into a 409/500.
func TestJournalSubscribe_ResubmittingSameEmailStillReturns200(t *testing.T) {
	repo := &fakeJournalRepo{}
	r := newJournalTestRouter(repo, journal.NewRateLimiter())

	first := postJournalSubscribe(r, `{"email":"ada@example.com"}`)
	second := postJournalSubscribe(r, `{"email":"ada@example.com"}`)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
}

func TestJournalSubscribe_RepositoryErrorReturns500WithoutLeakingDetail(t *testing.T) {
	repo := &fakeJournalRepo{err: assertError("db is down")}
	r := newJournalTestRouter(repo, journal.NewRateLimiter())

	w := postJournalSubscribe(r, `{"email":"ada@example.com"}`)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.JSONEq(t, `{"error":"internal","message":"internal server error"}`, w.Body.String())
}

func TestJournalSubscribe_RateLimitedAfterMaxRequestsFromSameIP(t *testing.T) {
	repo := &fakeJournalRepo{}
	limiter := journal.NewRateLimiter()
	r := newJournalTestRouter(repo, limiter)

	var last *httptest.ResponseRecorder
	for i := 0; i < journal.SubscribeRateMax; i++ {
		last = postJournalSubscribe(r, `{"email":"ada@example.com"}`)
		require.Equal(t, http.StatusOK, last.Code)
	}

	blocked := postJournalSubscribe(r, `{"email":"ada@example.com"}`)
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	require.JSONEq(t, `{"error":"rate_limited","message":"too many requests, please try again later"}`, blocked.Body.String())
}

type assertError string

func (e assertError) Error() string { return string(e) }
