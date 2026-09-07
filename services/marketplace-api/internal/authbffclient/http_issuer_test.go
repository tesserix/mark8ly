package authbffclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/authbffclient"
)

func TestHTTPIssuer_Success(t *testing.T) {
	var gotHeader string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Internal-Auth")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"set_cookie": "mk8_session=abc123; HttpOnly; Secure; SameSite=Lax; Path=/",
		})
	}))
	defer srv.Close()

	issuer := authbffclient.NewHTTPIssuer(srv.URL, "test-secret", nil)

	tenantID := uuid.New()
	userID := uuid.New()
	cookie, err := issuer.Issue(t.Context(), tenantID, userID, "break_glass", 2*time.Hour)

	require.NoError(t, err)
	assert.Equal(t, "mk8_session=abc123; HttpOnly; Secure; SameSite=Lax; Path=/", cookie)

	// The request actually carried the shared secret header.
	assert.Equal(t, "test-secret", gotHeader)

	// The body carries auth_context: "break_glass", not just a 200.
	assert.Equal(t, "break_glass", gotBody["auth_context"])
	assert.Equal(t, tenantID.String(), gotBody["tenant_id"])
	assert.Equal(t, userID.String(), gotBody["user_id"])
	assert.EqualValues(t, 7200, gotBody["ttl_seconds"])
}

func TestHTTPIssuer_Non200_ReturnsErrorAndEmptyCookie(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":"unauthorized"}`},
		{"server_error", http.StatusInternalServerError, `{"error":"internal_error"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			issuer := authbffclient.NewHTTPIssuer(srv.URL, "test-secret", nil)
			cookie, err := issuer.Issue(t.Context(), uuid.New(), uuid.New(), "break_glass", time.Hour)

			require.Error(t, err)
			assert.Empty(t, cookie)
		})
	}
}

func TestHTTPIssuer_MalformedBody_ReturnsErrorAndEmptyCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	issuer := authbffclient.NewHTTPIssuer(srv.URL, "test-secret", nil)
	cookie, err := issuer.Issue(t.Context(), uuid.New(), uuid.New(), "break_glass", time.Hour)

	require.Error(t, err)
	assert.Empty(t, cookie)
}

func TestHTTPIssuer_ConnectionFailure_ReturnsErrorRatherThanHanging(t *testing.T) {
	// A closed server: connection refused, not a hang.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	issuer := authbffclient.NewHTTPIssuer(srv.URL, "test-secret", nil)

	done := make(chan struct{})
	var cookie string
	var err error
	go func() {
		cookie, err = issuer.Issue(t.Context(), uuid.New(), uuid.New(), "break_glass", time.Hour)
		close(done)
	}()

	select {
	case <-done:
		require.Error(t, err)
		assert.Empty(t, cookie)
	case <-time.After(10 * time.Second):
		t.Fatal("Issue did not return within 10s — appears to hang")
	}
}

func TestNewSessionIssuer_AbsentConfig_YieldsNoopIssuer(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		secret  string
	}{
		{"both empty", "", ""},
		{"missing base URL", "", "secret"},
		{"missing secret", "http://auth-bff.internal", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issuer := authbffclient.NewSessionIssuer(tc.baseURL, tc.secret, nil)
			_, ok := issuer.(authbffclient.NoopIssuer)
			require.True(t, ok, "expected NoopIssuer, got %T", issuer)

			cookie, err := issuer.Issue(t.Context(), uuid.New(), uuid.New(), "break_glass", time.Hour)
			require.ErrorIs(t, err, authbffclient.ErrIssuerUnavailable)
			assert.Empty(t, cookie)
		})
	}
}

func TestNewSessionIssuer_FullConfig_YieldsHTTPIssuer(t *testing.T) {
	issuer := authbffclient.NewSessionIssuer("http://auth-bff.internal", "secret", nil)
	_, ok := issuer.(*authbffclient.HTTPIssuer)
	require.True(t, ok, "expected *HTTPIssuer, got %T", issuer)
}

func TestHTTPIssuer_Timeout_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"set_cookie":"nope"}`))
	}))
	defer srv.Close()

	issuer := authbffclient.NewHTTPIssuerWithClient(srv.URL, "test-secret", &http.Client{Timeout: 50 * time.Millisecond})
	cookie, err := issuer.Issue(t.Context(), uuid.New(), uuid.New(), "break_glass", time.Hour)

	require.Error(t, err)
	assert.Empty(t, cookie)
}
