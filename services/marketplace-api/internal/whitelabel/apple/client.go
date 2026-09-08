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
	"net/url"
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

	// ListApps returns every app the credentials can see. Read-only —
	// it never mutates anything at Apple. Used to discover the Apple
	// app id at teardown time from credentials already stored (#702).
	ListApps(ctx context.Context) ([]App, error)
}

// App is one resource from GET /v1/apps. ID is the Apple app id that
// BlockDownloads and PullApp take. BundleID and Name are carried for
// diagnostics: a caller refusing an ambiguous result needs to say which
// apps it saw, and an opaque numeric id alone is not a useful log line.
type App struct {
	ID       string
	BundleID string
	Name     string
}

// ErrUnauthorized reports that App Store Connect rejected the
// credentials (401/403) — typically a revoked or expired .p8 key. It is
// deliberately a distinct sentinel from "the account has no apps": the
// first means we do not know what exists, the second means we do know
// and the answer is nothing. A caller that conflates them would refuse
// teardown for the wrong reason (#702).
var ErrUnauthorized = errors.New("apple: App Store Connect rejected the credentials")

// ErrTooManyAppPages reports that /v1/apps had more pages than
// listAppsMaxPages. See the pagination note on ListApps.
var ErrTooManyAppPages = errors.New("apple: /v1/apps exceeded the page cap")

const (
	// listAppsPageLimit is Apple's documented maximum page size for
	// /v1/apps; asking for it keeps the common case to one request.
	listAppsPageLimit = 200

	// listAppsMaxPages bounds paging at 2000 apps.
	listAppsMaxPages = 10
)

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
	BaseURL       string                                         // default: https://api.appstoreconnect.apple.com
	HTTP          *http.Client                                   // default: http.Client{Timeout: 30s}
	CredsFetcher  func(ctx context.Context) (Credentials, error) // required
	TokenLifetime time.Duration                                  // default: 15 * time.Minute (Apple caps at 20)
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

// ListApps returns every app visible to the stored App Store Connect
// credentials, via GET /v1/apps.
//
// This is a READ. It issues only GET requests and mutates nothing at
// Apple; BlockDownloads and PullApp remain the only write paths.
//
// PAGINATION — deliberate choice: we follow links.next, capped at
// listAppsMaxPages pages of listAppsPageLimit each, and return
// ErrTooManyAppPages rather than truncating. Reading only the first page
// would silently omit apps once an account crosses 200, and the caller
// (teardown discovery) decides based on how many apps it saw — a
// truncated list can turn an ambiguous account into a falsely
// unambiguous one and pull the wrong listing. Unbounded paging is the
// opposite failure, so the cap is explicit and loud when hit.
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("apple: parse base url: %w", err)
	}

	next := fmt.Sprintf("%s/v1/apps?limit=%d", c.baseURL, listAppsPageLimit)
	var apps []App

	for page := 0; next != ""; page++ {
		if page >= listAppsMaxPages {
			return nil, fmt.Errorf("%w (%d pages)", ErrTooManyAppPages, listAppsMaxPages)
		}

		var doc appsDocument
		if err := c.getJSON(ctx, next, &doc); err != nil {
			return nil, err
		}
		for _, r := range doc.Data {
			apps = append(apps, App{
				ID:       r.ID,
				BundleID: r.Attributes.BundleID,
				Name:     r.Attributes.Name,
			})
		}

		next, err = sameHostNext(base, doc.Links.Next)
		if err != nil {
			return nil, err
		}
	}

	return apps, nil
}

// sameHostNext validates a links.next value before we follow it. Apple
// returns an absolute URL; refusing one that points at a different host
// keeps a compromised or unexpected response from redirecting an
// authenticated request (we send the Bearer token on every hop).
func sameHostNext(base *url.URL, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("apple: parse links.next: %w", err)
	}
	if u.Scheme != base.Scheme || u.Host != base.Host {
		return "", fmt.Errorf("apple: links.next points off-host (%s)", u.Host)
	}
	return u.String(), nil
}

// appsDocument is the JSON:API document returned by GET /v1/apps.
type appsDocument struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			BundleID string `json:"bundleId"`
			Name     string `json:"name"`
		} `json:"attributes"`
	} `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// getJSON is the read counterpart to call. It exists as a separate
// helper because call is shaped for the two write operations: it
// discards the response body, and it maps 404 to nil because
// re-removing an already-removed app is a success. Neither is correct
// for a read — a read needs the body, and a 404 is an answer we do not
// have rather than an answer of "none". Auth and request assembly are
// the same and stay in one place per caller by construction.
func (c *Client) getJSON(ctx context.Context, rawURL string, out any) error {
	creds, err := c.credsFetcher(ctx)
	if err != nil {
		return fmt.Errorf("apple: fetch creds: %w", err)
	}
	token, err := SignJWT(creds.P8, creds.IssuerID, creds.KeyID, c.tokenLifetime)
	if err != nil {
		return fmt.Errorf("apple: sign jwt: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("apple: GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("apple: decode GET %s: %w", rawURL, err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: GET %s: status %d", ErrUnauthorized, rawURL, resp.StatusCode)
	default:
		return fmt.Errorf("apple: GET %s: unexpected status %d", rawURL, resp.StatusCode)
	}
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
