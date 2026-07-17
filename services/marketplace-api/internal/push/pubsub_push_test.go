package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/pushevents"
)

func testHandler(t *testing.T, verify OIDCVerifier) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/pubsub/merchant-push", NewPubsubPushHandler(PubsubPushConfig{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Audience:       "https://api.example/pubsub/merchant-push",
		ServiceAccount: "push@proj.iam.gserviceaccount.com",
		Verify:         verify,
	}))
	return r
}

func pubsubBody(t *testing.T, ev pushevents.Event) string {
	t.Helper()
	raw, _ := json.Marshal(ev)
	env := map[string]any{"message": map[string]any{"data": base64.StdEncoding.EncodeToString(raw)}}
	b, _ := json.Marshal(env)
	return string(b)
}

func do(t *testing.T, h http.Handler, auth, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/pubsub/merchant-push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

func TestPubsubPush_RejectsMissingToken(t *testing.T) {
	h := testHandler(t, func(context.Context, string, string) (string, error) {
		t.Fatal("verify must not run without a bearer token")
		return "", nil
	})
	if code := do(t, h, "", "{}"); code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", code)
	}
}

func TestPubsubPush_RejectsInvalidToken(t *testing.T) {
	h := testHandler(t, func(context.Context, string, string) (string, error) {
		return "", errors.New("bad signature")
	})
	if code := do(t, h, "Bearer x", "{}"); code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", code)
	}
}

func TestPubsubPush_RejectsWrongServiceAccount(t *testing.T) {
	h := testHandler(t, func(context.Context, string, string) (string, error) {
		return "attacker@evil.iam.gserviceaccount.com", nil
	})
	body := pubsubBody(t, pushevents.Event{StoreID: "not-a-uuid"})
	if code := do(t, h, "Bearer x", body); code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", code)
	}
}

func TestPubsubPush_AcksBadStoreID(t *testing.T) {
	// Authorized principal but malformed producer data → 200 so Pub/Sub stops
	// redelivering. Reaches neither repo nor sender (both nil here).
	h := testHandler(t, func(context.Context, string, string) (string, error) {
		return "push@proj.iam.gserviceaccount.com", nil
	})
	body := pubsubBody(t, pushevents.Event{StoreID: "not-a-uuid", Title: "New order"})
	if code := do(t, h, "Bearer good", body); code != http.StatusOK {
		t.Fatalf("want 200 ignored, got %d", code)
	}
}
