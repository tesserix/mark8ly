//go:build integration

package emailevents_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/emailevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

type quiet struct{}

func (quiet) Write(p []byte) (int, error) { return len(p), nil }

func router(t *testing.T, db *gorm.DB, secret string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	emailevents.NewHandler(
		emailevents.NewApplier(db, slog.New(slog.NewTextHandler(quiet{}, nil))),
		secret, slog.New(slog.NewTextHandler(quiet{}, nil)),
	).Register(r.Group(""))
	return r
}

func post(t *testing.T, r *gin.Engine, body []byte, id, ts, sig string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/resend", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(emailevents.HeaderID, id)
	req.Header.Set(emailevents.HeaderTimestamp, ts)
	req.Header.Set(emailevents.HeaderSignature, sig)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func deliveredBody(t *testing.T, sendID uuid.UUID) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":       "email.delivered",
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"data":       map[string]any{"tags": map[string]string{"send_id": sendID.String()}},
	})
	require.NoError(t, err)
	return b
}

// The endpoint is a public trust boundary: an unsigned POST must change
// nothing, whatever it claims.
func TestIntegration_Webhook_UnsignedRequestIsRejectedAndChangesNothing(t *testing.T) {
	db := testdb.NewTx(t)
	id := seedSend(t, db, "sent")
	secret := testSecret(t)

	rec := post(t, router(t, db, secret), deliveredBody(t, id), "msg_x", "1", "v1,bogus")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "sent", statusOf(t, db, id), "an unverified event must not touch the row")
}

func TestIntegration_Webhook_SignedRequestAppliesTheEvent(t *testing.T) {
	db := testdb.NewTx(t)
	sendID := seedSend(t, db, "sent")
	secret := testSecret(t)
	body := deliveredBody(t, sendID)
	now := time.Now()
	id, ts, sig := signLikeResend(t, secret, "msg_ok", now, body)

	rec := post(t, router(t, db, secret), body, id, ts, sig)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "delivered", statusOf(t, db, sendID))
}

// A retry of an accepted delivery must be a no-op that still answers 2xx,
// or the provider keeps redelivering forever.
func TestIntegration_Webhook_ReplayIsAcceptedAndIdempotent(t *testing.T) {
	db := testdb.NewTx(t)
	sendID := seedSend(t, db, "sent")
	secret := testSecret(t)
	body := deliveredBody(t, sendID)
	now := time.Now()
	id, ts, sig := signLikeResend(t, secret, "msg_same", now, body)
	r := router(t, db, secret)

	require.Equal(t, http.StatusOK, post(t, r, body, id, ts, sig).Code)
	require.Equal(t, http.StatusOK, post(t, r, body, id, ts, sig).Code)

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM email_send_events WHERE send_id = ?`, sendID).Scan(&n).Error)
	require.EqualValues(t, 1, n)
}

// Unconfigured answers 503 — plainly, so a missing secret is diagnosable
// rather than looking like a signature failure.
func TestIntegration_Webhook_UnconfiguredSaysSoInsteadOfRejecting(t *testing.T) {
	db := testdb.NewTx(t)
	rec := post(t, router(t, db, ""), []byte(`{}`), "msg_1", "1", "v1,x")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "not_configured")
}

// Signed but unrecognisable payloads are ACCEPTED, so the provider stops
// retrying something that will never parse or match.
func TestIntegration_Webhook_SignedButUnusablePayloadIsAccepted(t *testing.T) {
	db := testdb.NewTx(t)
	secret := testSecret(t)
	for _, body := range [][]byte{
		[]byte(`not json at all`),
		[]byte(`{"type":"email.delivered","data":{}}`),
	} {
		now := time.Now()
		id, ts, sig := signLikeResend(t, secret, "msg_"+string(body[:3]), now, body)
		rec := post(t, router(t, db, secret), body, id, ts, sig)
		require.Equal(t, http.StatusOK, rec.Code,
			"a signed payload we cannot use must not be retried at us forever")
	}
	_ = context.Background()
}
