package breakglass

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSlack_PostsToSecurityAlerts(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewSlackClient(srv.URL, SlackChannel)
	require.NoError(t, client.PostLoginAlert(context.Background(), uuid.New(), true))

	require.Equal(t, "#security-alerts", got["channel"])
	require.Equal(t, "mark8ly-security", got["username"])
	require.True(t, strings.Contains(got["text"].(string), "break-glass login"))
	require.True(t, strings.Contains(got["text"].(string), "SUCCESS"))
}

func TestSlack_FailedLogin_TextFlag(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewSlackClient(srv.URL, SlackChannel)
	require.NoError(t, client.PostLoginAlert(context.Background(), uuid.New(), false))
	require.Contains(t, got["text"].(string), "FAILED")
}

func TestSlack_ReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewSlackClient(srv.URL, SlackChannel)
	err := client.PostLoginAlert(context.Background(), uuid.New(), true)
	require.Error(t, err)
}

func TestSlack_EmptyWebhook_IsNoOp(t *testing.T) {
	client := NewSlackClient("", SlackChannel)
	require.NoError(t, client.PostLoginAlert(context.Background(), uuid.New(), true))
}

func TestSlack_RotationAlert_SuccessVsFailure(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewSlackClient(srv.URL, SlackChannel)

	require.NoError(t, client.PostRotationAlert(context.Background(), uuid.New(), true, ""))
	require.Contains(t, got["text"].(string), "rotated")

	require.NoError(t, client.PostRotationAlert(context.Background(), uuid.New(), false, "secret_manager_down"))
	require.Contains(t, got["text"].(string), "rotation FAILED")
	require.Contains(t, got["text"].(string), "secret_manager_down")
}
