package autologin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/emailotp"
)

// TestAutoLogin_RateLimitedChallenge_Returns429 asserts a rate-limited
// IssueChallenge failure is reported as 429 rate_limited, matching what
// loginotp's classify() returns for the identical emailotp condition —
// not the generic 503 challenge_send_failed that told users to retry
// straight into the limit. It exercises the real service.AutoLogin ->
// respondError path so both the error-wrap fix and the new 429 case are
// covered together.
func TestAutoLogin_RateLimitedChallenge_Returns429(t *testing.T) {
	gin.SetMode(gin.TestMode)

	issuer := &stubIssuer{err: emailotp.ErrRateLimited}
	svc := newOTPService(t, &stubDevices{isNew: true}, issuer)

	_, err := svc.AutoLogin(context.Background(), httptest.NewRecorder(), loginReq())
	if err == nil {
		t.Fatal("expected an error when the challenge is rate limited")
	}
	if !errors.Is(err, emailotp.ErrRateLimited) {
		t.Fatalf("errors.Is(err, emailotp.ErrRateLimited) = false; err = %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	respondError(c, err)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
	if body := w.Body.String(); !strings.Contains(body, `"error":"rate_limited"`) {
		t.Errorf("body = %s, want error code rate_limited", body)
	}
}

// TestAutoLogin_GenericChallengeFailure_Returns503 asserts the old
// behaviour survives for every non-rate-limit send failure: this is the
// regression guard that fix A/B must not disturb.
func TestAutoLogin_GenericChallengeFailure_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)

	issuer := &stubIssuer{err: errors.New("smtp: connection refused")}
	svc := newOTPService(t, &stubDevices{isNew: true}, issuer)

	_, err := svc.AutoLogin(context.Background(), httptest.NewRecorder(), loginReq())
	if err == nil {
		t.Fatal("expected an error when the challenge send fails")
	}
	if errors.Is(err, emailotp.ErrRateLimited) {
		t.Fatal("a generic send failure must not match ErrRateLimited")
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	respondError(c, err)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error":"challenge_send_failed"`) {
		t.Errorf("body = %s, want error code challenge_send_failed", body)
	}
	if !strings.Contains(body, "could not send your sign-in code; please retry") {
		t.Errorf("body = %s, want the original 503 message preserved", body)
	}
}

// TestErrorChain_RateLimitSurvivesWrap is the narrow unit-level property
// that fix A exists for: errors.Is must see through the
// ErrChallengeSendFail wrap down to the emailotp sentinel, independent
// of any HTTP-layer behaviour.
func TestErrorChain_RateLimitSurvivesWrap(t *testing.T) {
	issuer := &stubIssuer{err: emailotp.ErrRateLimited}
	svc := newOTPService(t, &stubDevices{isNew: true}, issuer)

	_, err := svc.AutoLogin(context.Background(), httptest.NewRecorder(), loginReq())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrChallengeSendFail) {
		t.Error("errors.Is(err, ErrChallengeSendFail) = false")
	}
	if !errors.Is(err, emailotp.ErrRateLimited) {
		t.Error("errors.Is(err, emailotp.ErrRateLimited) = false")
	}
}
