package bao

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// loginResponse builds a minimal Kubernetes-auth login response body.
func loginResponse(leaseSeconds int) string {
	body, _ := json.Marshal(map[string]any{
		"auth": map[string]any{
			"client_token":   "test-token",
			"lease_duration": leaseSeconds,
		},
	})
	return string(body)
}

// writeServiceAccountToken writes a fake projected JWT to a temp file and
// returns its path.
func writeServiceAccountToken(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("fake-jwt\n"), 0o600); err != nil {
		t.Fatalf("write fake service account token: %v", err)
	}
	return path
}

func TestClient_ReusesTokenWithinLease(t *testing.T) {
	var logins int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/kubernetes/login" {
			atomic.AddInt32(&logins, 1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, loginResponse(3600))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := New(Config{
		Address:             srv.URL,
		KubernetesRole:      "marketplace-api",
		ServiceAccountToken: writeServiceAccountToken(t),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.authenticate(t.Context()); err != nil {
		t.Fatalf("first authenticate: %v", err)
	}
	if err := c.authenticate(t.Context()); err != nil {
		t.Fatalf("second authenticate: %v", err)
	}

	if got := atomic.LoadInt32(&logins); got != 1 {
		t.Fatalf("expected exactly 1 login request within the lease, got %d", got)
	}
}

func TestClient_RenewsBeforeExpiry(t *testing.T) {
	var logins int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/kubernetes/login" {
			atomic.AddInt32(&logins, 1)
			w.Header().Set("Content-Type", "application/json")
			// Lease duration shorter than the 1-minute renewal margin: a
			// second authenticate must trigger a fresh login, not reuse.
			fmt.Fprint(w, loginResponse(30))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := New(Config{
		Address:             srv.URL,
		KubernetesRole:      "marketplace-api",
		ServiceAccountToken: writeServiceAccountToken(t),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.authenticate(t.Context()); err != nil {
		t.Fatalf("first authenticate: %v", err)
	}
	if err := c.authenticate(t.Context()); err != nil {
		t.Fatalf("second authenticate: %v", err)
	}

	if got := atomic.LoadInt32(&logins); got != 2 {
		t.Fatalf("expected re-login when token is within a minute of expiry, got %d login(s)", got)
	}
}

func TestClient_TranslatesForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/kubernetes/login":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, loginResponse(3600))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"errors":["permission denied"]}`)
		}
	}))
	defer srv.Close()

	c, err := New(Config{
		Address:             srv.URL,
		KubernetesRole:      "marketplace-api",
		ServiceAccountToken: writeServiceAccountToken(t),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, _, err = c.ReadSecret(t.Context(), "razorpay/store-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestClient_TranslatesNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/kubernetes/login":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, loginResponse(3600))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := New(Config{
		Address:             srv.URL,
		KubernetesRole:      "marketplace-api",
		ServiceAccountToken: writeServiceAccountToken(t),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, _, err = c.ReadSecret(t.Context(), "razorpay/store-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestClient_MissingServiceAccountToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := New(Config{
		Address:             srv.URL,
		KubernetesRole:      "marketplace-api",
		ServiceAccountToken: filepath.Join(t.TempDir(), "does-not-exist"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.authenticate(t.Context()); err == nil {
		t.Fatal("expected an error for a missing service account token file")
	}
}
