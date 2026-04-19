package apple_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
)

func genP8(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen p256: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal p8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestSignJWT_ShapeAndHeaders(t *testing.T) {
	p8 := genP8(t)
	tok, err := apple.SignJWT(p8, "iss-abc", "kid-xyz", 15*time.Minute)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT must have 3 parts; got %d", len(parts))
	}

	// Decode header — verify alg=ES256, kid, typ.
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var hdr map[string]string
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if hdr["alg"] != "ES256" {
		t.Errorf("alg = %q, want ES256", hdr["alg"])
	}
	if hdr["kid"] != "kid-xyz" {
		t.Errorf("kid = %q, want kid-xyz", hdr["kid"])
	}
	if hdr["typ"] != "JWT" {
		t.Errorf("typ = %q, want JWT", hdr["typ"])
	}

	// Decode claims — verify iss, aud, exp in window.
	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims["iss"] != "iss-abc" {
		t.Errorf("iss = %v, want iss-abc", claims["iss"])
	}
	if claims["aud"] != "appstoreconnect-v1" {
		t.Errorf("aud = %v, want appstoreconnect-v1", claims["aud"])
	}

	// Signature is 64 bytes (32 r + 32 s, fixed-width).
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if len(sig) != 64 {
		t.Errorf("sig length = %d, want 64 (fixed-width r||s)", len(sig))
	}
}

func TestSignJWT_RejectsNonECKey(t *testing.T) {
	// Empty bytes → no PEM block → error.
	_, err := apple.SignJWT(nil, "iss", "kid", time.Minute)
	if err == nil {
		t.Error("SignJWT(nil) = no error; want error")
	}
}

func TestClient_BlockDownloads_CallsAvailabilityEndpoint(t *testing.T) {
	p8 := genP8(t)
	var gotMethod, gotPath string
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.ReadAll(r.Body) // drain
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cli, err := apple.New(apple.Config{
		BaseURL: srv.URL,
		CredsFetcher: func(context.Context) (apple.Credentials, error) {
			return apple.Credentials{P8: p8, IssuerID: "iss", KeyID: "kid"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := cli.BlockDownloads(context.Background(), "app-id-42"); err != nil {
		t.Fatalf("BlockDownloads: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if !strings.Contains(gotPath, "/v1/apps/app-id-42/availability") {
		t.Errorf("path = %s, want contains /v1/apps/app-id-42/availability", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("auth = %q, want Bearer <jwt>", gotAuth)
	}
}

func TestClient_PullApp_SetsRemovedFromSale(t *testing.T) {
	p8 := genP8(t)
	var gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli, err := apple.New(apple.Config{
		BaseURL: srv.URL,
		CredsFetcher: func(context.Context) (apple.Credentials, error) {
			return apple.Credentials{P8: p8, IssuerID: "iss", KeyID: "kid"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := cli.PullApp(context.Background(), "app-id-99"); err != nil {
		t.Fatalf("PullApp: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/v1/apps/app-id-99") {
		t.Errorf("path = %s, want /v1/apps/app-id-99", gotPath)
	}
	data, _ := gotBody["data"].(map[string]any)
	attrs, _ := data["attributes"].(map[string]any)
	if attrs["state"] != "REMOVED_FROM_SALE" {
		t.Errorf("attributes.state = %v, want REMOVED_FROM_SALE", attrs["state"])
	}
}

func TestClient_404_IsTreatedAsSuccess(t *testing.T) {
	p8 := genP8(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cli, err := apple.New(apple.Config{
		BaseURL: srv.URL,
		CredsFetcher: func(context.Context) (apple.Credentials, error) {
			return apple.Credentials{P8: p8, IssuerID: "iss", KeyID: "kid"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := cli.BlockDownloads(context.Background(), "gone-app"); err != nil {
		t.Errorf("BlockDownloads on 404 = %v; want nil (idempotent)", err)
	}
}

func TestClient_Non2xx_NotFoundReturnsError(t *testing.T) {
	p8 := genP8(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cli, err := apple.New(apple.Config{
		BaseURL: srv.URL,
		CredsFetcher: func(context.Context) (apple.Credentials, error) {
			return apple.Credentials{P8: p8, IssuerID: "iss", KeyID: "kid"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := cli.BlockDownloads(context.Background(), "x"); err == nil {
		t.Error("BlockDownloads on 500 = nil, want error")
	}
}

func TestNew_RequiresCredsFetcher(t *testing.T) {
	_, err := apple.New(apple.Config{})
	if err == nil {
		t.Error("New(empty config) = nil, want error")
	}
}

func TestFakeClient_Counts(t *testing.T) {
	f := apple.NewFakeClient()
	_ = f.BlockDownloads(context.Background(), "a")
	_ = f.BlockDownloads(context.Background(), "b")
	_ = f.PullApp(context.Background(), "a")

	if f.BlockDownloadsCallCount != 2 {
		t.Errorf("BlockDownloadsCallCount = %d, want 2", f.BlockDownloadsCallCount)
	}
	if f.PullAppCallCount != 1 {
		t.Errorf("PullAppCallCount = %d, want 1", f.PullAppCallCount)
	}
}
