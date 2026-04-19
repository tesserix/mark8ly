// Package apple wraps the App Store Connect API for the two lifecycle
// operations the white-label app teardown sequence needs (spec §13.5):
//
//   - Day 30: BlockDownloads — set the app's availability to "not
//     available in any territory" via the App availability endpoint.
//   - Day 60: PullApp — remove the public listing (soft unpublish).
//
// Auth is ES256-JWT against Apple's ASC issuer/key credentials held
// in Secret Manager (spec §18.9). Tokens have a 20-minute TTL; we sign
// per-request rather than caching.
//
// This package intentionally uses only the Go standard library + the
// existing `crypto/ecdsa` primitives. `go-jose/v4` was considered but
// avoided to keep the go.mod direct dep graph lean; JWT assembly is
// small enough to inline (see SignJWT below).
package apple

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"
)

// ClientAPI is what lifecycle/advancer depends on; tests use FakeClient.
type ClientAPI interface {
	// BlockDownloads marks appID "not available" in all territories.
	// Idempotent against Apple ASC (re-applying the same availability
	// state is a no-op).
	BlockDownloads(ctx context.Context, appleAppID string) error

	// PullApp removes the public listing for appID. Idempotent.
	PullApp(ctx context.Context, appleAppID string) error
}

// Credentials is the bundle read from appcreds.Service.Load at call
// time — never stored on the Client struct. Enforces "fetch fresh
// creds per op" so a revoked key rotates through without a restart.
type Credentials struct {
	P8       []byte
	IssuerID string
	KeyID    string
}

// Client is the production ASC client backed by credentials fetched
// per-call from appcreds.Service. The CredsFetcher closure injects
// whatever lookup strategy the caller wants (tenant → appcreds.Load).
type Client struct {
	baseURL       string
	http          *http.Client
	credsFetcher  func(ctx context.Context) (Credentials, error)
	tokenLifetime time.Duration
}

// Config groups Client construction params.
type Config struct {
	BaseURL       string                                            // default: https://api.appstoreconnect.apple.com
	HTTP          *http.Client                                      // default: http.Client{Timeout: 30s}
	CredsFetcher  func(ctx context.Context) (Credentials, error)    // required
	TokenLifetime time.Duration                                     // default: 15 * time.Minute (Apple caps at 20)
}

// New constructs a production Client. The CredsFetcher is required;
// tests should use FakeClient instead of constructing a real Client
// with a stub fetcher.
func New(cfg Config) (*Client, error) {
	if cfg.CredsFetcher == nil {
		return nil, errors.New("apple: Config.CredsFetcher is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.appstoreconnect.apple.com"
	}
	h := cfg.HTTP
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	ttl := cfg.TokenLifetime
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	return &Client{baseURL: base, http: h, credsFetcher: cfg.CredsFetcher, tokenLifetime: ttl}, nil
}

// BlockDownloads PATCHes /v1/apps/{id}/availability with all territories
// set to "not available". Returns nil on 200/204; wrapped error on
// anything else.
func (c *Client) BlockDownloads(ctx context.Context, appleAppID string) error {
	body := map[string]any{
		"data": map[string]any{
			"type": "appAvailabilities",
			"attributes": map[string]any{
				"availableInNewTerritories": false,
			},
			"relationships": map[string]any{
				// Empty territory list → not available anywhere. Apple
				// treats an explicit empty array as "remove all".
				"availableTerritories": map[string]any{
					"data": []any{},
				},
			},
		},
	}
	path := fmt.Sprintf("/v1/apps/%s/availability", appleAppID)
	return c.call(ctx, http.MethodPatch, path, body)
}

// PullApp PATCHes /v1/apps/{id} to set state=REMOVED_FROM_SALE. Apple
// accepts this transition any time after initial approval; the listing
// stops serving new downloads but existing users keep access until
// their install is uninstalled (standard ASC semantics).
func (c *Client) PullApp(ctx context.Context, appleAppID string) error {
	body := map[string]any{
		"data": map[string]any{
			"type": "apps",
			"id":   appleAppID,
			"attributes": map[string]any{
				"state": "REMOVED_FROM_SALE",
			},
		},
	}
	path := fmt.Sprintf("/v1/apps/%s", appleAppID)
	return c.call(ctx, http.MethodPatch, path, body)
}

// call wraps request assembly + auth + response classification.
func (c *Client) call(ctx context.Context, method, path string, body any) error {
	creds, err := c.credsFetcher(ctx)
	if err != nil {
		return fmt.Errorf("apple: fetch creds: %w", err)
	}
	token, err := SignJWT(creds.P8, creds.IssuerID, creds.KeyID, c.tokenLifetime)
	if err != nil {
		return fmt.Errorf("apple: sign jwt: %w", err)
	}

	var reader *jsonReader
	if body != nil {
		reader, err = newJSONReader(body)
		if err != nil {
			return fmt.Errorf("apple: marshal body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("apple: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusCreated:
		return nil
	case http.StatusNotFound:
		// Idempotent — treat "already removed / gone" as success.
		return nil
	default:
		return fmt.Errorf("apple: %s %s: unexpected status %d", method, path, resp.StatusCode)
	}
}

// ─── JWT signing (ES256, stdlib-only) ─────────────────────────────────

// SignJWT produces the short-lived App Store Connect API JWT per
// Apple docs: ES256-signed, iss=issuerID, aud="appstoreconnect-v1",
// exp=now+ttl, header kid=keyID, typ="JWT".
//
// Exposed for testing. Production callers go through Client.
func SignJWT(p8PEM []byte, issuerID, keyID string, ttl time.Duration) (string, error) {
	priv, err := parseP8(p8PEM)
	if err != nil {
		return "", err
	}

	header := map[string]string{
		"alg": "ES256",
		"kid": keyID,
		"typ": "JWT",
	}
	claims := map[string]any{
		"iss": issuerID,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(ttl).Unix(),
		"aud": "appstoreconnect-v1",
	}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := b64url(hb) + "." + b64url(cb)
	sum := sha256.Sum256([]byte(signingInput))

	// ECDSA signature — Go returns (r, s). Apple wants fixed-width 64-byte
	// encoding: 32 bytes r, 32 bytes s, left-padded. DO NOT emit the ASN.1
	// DER form; ASC rejects it.
	r, s, err := ecdsa.Sign(rand.Reader, priv, sum[:])
	if err != nil {
		return "", fmt.Errorf("apple: ecdsa sign: %w", err)
	}
	sig := make([]byte, 64)
	rb := r.Bytes()
	sb := s.Bytes()
	copy(sig[32-len(rb):32], rb)
	copy(sig[64-len(sb):], sb)

	return signingInput + "." + b64url(sig), nil
}

func parseP8(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("apple: no PEM block in p8")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apple: parse pkcs8: %w", err)
	}
	priv, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("apple: expected ECDSA, got %T", parsed)
	}
	return priv, nil
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// parseECDSASig decodes an ASN.1 DER ECDSA signature into (r, s). Unused
// in the happy path (we emit fixed-width) but retained for parsing
// signatures received from elsewhere if the client grows.
//
//nolint:unused
type ecdsaSig struct{ R, S *big.Int }

//nolint:unused
func parseECDSASig(der []byte) (r, s *big.Int, err error) {
	var sig ecdsaSig
	if _, err := asn1.Unmarshal(der, &sig); err != nil {
		return nil, nil, err
	}
	return sig.R, sig.S, nil
}
