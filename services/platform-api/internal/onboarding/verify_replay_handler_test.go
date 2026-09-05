package onboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/verification"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// replaySessionRepo is a Repository whose only interesting behaviour is
// GetByID; the rest satisfy the interface.
type replaySessionRepo struct{ sess *Session }

func (r *replaySessionRepo) Create(context.Context, *Session) error { return nil }
func (r *replaySessionRepo) GetByID(_ context.Context, id string) (*Session, error) {
	if r.sess == nil || r.sess.ID != id {
		return nil, apperrors.NotFound("session_not_found", "no such session")
	}
	return r.sess, nil
}
func (r *replaySessionRepo) UpdateDraft(context.Context, string, json.RawMessage) error { return nil }
func (r *replaySessionRepo) MarkEmailVerified(context.Context, string) error            { return nil }
func (r *replaySessionRepo) CompleteInTx(context.Context, *gorm.DB, string, string) error {
	return nil
}
func (r *replaySessionRepo) GetFunnel(context.Context, FunnelFilter) (*FunnelStats, error) {
	return &FunnelStats{}, nil
}
func (r *replaySessionRepo) ListSessions(context.Context, FunnelFilter) ([]SessionRow, int64, error) {
	return nil, 0, nil
}

// replayTokenRepo serves one already-consumed, unexpired token.
type replayTokenRepo struct{ tok *verification.Token }

func (r *replayTokenRepo) Create(context.Context, *verification.Token) error { return nil }
func (r *replayTokenRepo) GetByHash(_ context.Context, hash string) (*verification.Token, error) {
	if r.tok == nil || r.tok.CodeHash != hash {
		return nil, apperrors.NotFound("token_not_found", "verification link is invalid or expired")
	}
	return r.tok, nil
}
func (r *replayTokenRepo) MarkConsumed(context.Context, string) error         { return nil }
func (r *replayTokenRepo) InvalidateForSession(context.Context, string) error { return nil }

type noopSender struct{}

func (noopSender) Send(context.Context, notification.Email) error { return nil }

// postVerify drives the real route with a consumed token, for a session in
// the given verified state, and returns the response.
func postVerify(t *testing.T, sessionVerified bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Mint the token the way the service does: the stored value is the
	// hex SHA-256 of the raw token, so the test can build the pair itself
	// without exporting production helpers just for a test.
	raw := "replay-token-fixture-not-a-real-credential"
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])
	now := time.Now()
	tokRepo := &replayTokenRepo{tok: &verification.Token{
		ID: "tok-1", SessionID: "sess-1", Email: "user@example.com",
		CodeHash: hash, ExpiresAt: now.Add(verification.TokenLifetime), ConsumedAt: &now,
	}}

	sess := &Session{ID: "sess-1", Email: "user@example.com"}
	if sessionVerified {
		sess.EmailVerifiedAt = &now
	}

	h := NewHandler(
		NewService(Config{Repo: &replaySessionRepo{sess: sess}}),
		verification.NewService(tokRepo, verification.Config{
			Sender: noopSender{}, EmailFrom: "n@m.com",
			SupportEmail: "h@m.com", VerifyURLBase: "https://o.test",
		}),
	)

	r := gin.New()
	h.Register(r.Group(""))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/onboarding/verify-token",
		strings.NewReader(`{"token":"`+raw+`"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// #710: opening a verification link a second time rendered a failure page
// ("We couldn't verify your link") with a "Start over" button that would
// discard a session which had already progressed. Mail scanners and users
// both replay links, so this is ordinary traffic, not misuse.
func TestReplayOfAConsumedLinkSucceedsWhenTheSessionIsVerified(t *testing.T) {
	rec := postVerify(t, true)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			Verified  bool   `json:"verified"`
			SessionID string `json:"session_id"`
			Email     string `json:"email"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Data.Verified)
	require.Equal(t, "sess-1", body.Data.SessionID)
	require.Equal(t, "user@example.com", body.Data.Email)
}

// The boundary. A consumed token whose session was never verified is a
// genuinely dead link and must keep failing — otherwise this change would
// hand a session id and email to anyone replaying a spent token.
func TestReplayStillFailsWhenTheSessionIsNotVerified(t *testing.T) {
	rec := postVerify(t, false)
	require.NotEqual(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "token_consumed")
}
