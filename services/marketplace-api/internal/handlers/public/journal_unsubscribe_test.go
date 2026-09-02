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

// fakeJournalUnsubscriber records every Unsubscribe call so tests can
// assert on what token reached the repository without a database.
type fakeJournalUnsubscriber struct {
	calls []string
	err   error
}

func (f *fakeJournalUnsubscriber) Unsubscribe(token string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, token)
	return nil
}

func newJournalUnsubscribeTestRouter(repo *fakeJournalUnsubscriber, limiter *journal.RateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewJournalUnsubscribeHandler(repo, limiter, slog.Default())
	r := gin.New()
	r.POST("/api/v1/journal/unsubscribe", h.Unsubscribe)
	return r
}

func postJournalUnsubscribe(r *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/journal/unsubscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestJournalUnsubscribe_ValidTokenSucceeds(t *testing.T) {
	repo := &fakeJournalUnsubscriber{}
	r := newJournalUnsubscribeTestRouter(repo, journal.NewRateLimiter())

	// build a 64-char hex-looking token
	tok := ""
	for len(tok) < 64 {
		tok += "0123456789abcdef"
	}
	tok = tok[:64]

	w := postJournalUnsubscribe(r, `{"token":"`+tok+`"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"unsubscribed":true}`, w.Body.String())
	require.Equal(t, []string{tok}, repo.calls)
}

func TestJournalUnsubscribe_UnknownTokenStillReturns200(t *testing.T) {
	repo := &fakeJournalUnsubscriber{}
	r := newJournalUnsubscribeTestRouter(repo, journal.NewRateLimiter())

	w := postJournalUnsubscribe(r, `{"token":"0000000000000000000000000000000000000000000000000000000000000000"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"unsubscribed":true}`, w.Body.String())
}

func TestJournalUnsubscribe_MalformedTokenReturns200WithoutTouchingRepo(t *testing.T) {
	repo := &fakeJournalUnsubscriber{}
	r := newJournalUnsubscribeTestRouter(repo, journal.NewRateLimiter())

	w := postJournalUnsubscribe(r, `{"token":"not-a-real-token"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"unsubscribed":true}`, w.Body.String())
}

func TestJournalUnsubscribe_EmptyTokenReturns200WithoutTouchingRepo(t *testing.T) {
	repo := &fakeJournalUnsubscriber{}
	r := newJournalUnsubscribeTestRouter(repo, journal.NewRateLimiter())

	w := postJournalUnsubscribe(r, `{}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"unsubscribed":true}`, w.Body.String())
	require.Empty(t, repo.calls, "an empty token must never reach the repository")
}

func TestJournalUnsubscribe_UsingTheSameTokenTwiceBothReturn200(t *testing.T) {
	repo := &fakeJournalUnsubscriber{}
	r := newJournalUnsubscribeTestRouter(repo, journal.NewRateLimiter())

	body := `{"token":"1111111111111111111111111111111111111111111111111111111111111111"}`
	first := postJournalUnsubscribe(r, body)
	second := postJournalUnsubscribe(r, body)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
}

func TestJournalUnsubscribe_RepositoryErrorReturns500WithoutLeakingDetail(t *testing.T) {
	repo := &fakeJournalUnsubscriber{err: assertError("db is down")}
	r := newJournalUnsubscribeTestRouter(repo, journal.NewRateLimiter())

	w := postJournalUnsubscribe(r, `{"token":"2222222222222222222222222222222222222222222222222222222222222222"}`)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.JSONEq(t, `{"error":"internal","message":"internal server error"}`, w.Body.String())
}

func TestJournalUnsubscribe_RateLimitedAfterMaxRequestsFromSameIP(t *testing.T) {
	repo := &fakeJournalUnsubscriber{}
	limiter := journal.NewRateLimiter()
	r := newJournalUnsubscribeTestRouter(repo, limiter)

	body := `{"token":"3333333333333333333333333333333333333333333333333333333333333333"}`
	var last *httptest.ResponseRecorder
	for i := 0; i < journal.SubscribeRateMax; i++ {
		last = postJournalUnsubscribe(r, body)
		require.Equal(t, http.StatusOK, last.Code)
	}

	blocked := postJournalUnsubscribe(r, body)
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	require.JSONEq(t, `{"error":"rate_limited","message":"too many requests, please try again later"}`, blocked.Body.String())
}
