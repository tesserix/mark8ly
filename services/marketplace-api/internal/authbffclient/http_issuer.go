package authbffclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// internalAuthHeader carries the shared internal secret auth-bff
// validates via internalauth.Equal on its side (auth-bff/internal/internalauth).
// marketplace-api has no equivalent package to import — mobile_login.go
// uses the same literal header name for the same reason.
const internalAuthHeader = "X-Internal-Auth"

// defaultHTTPIssuerTimeout bounds a single mint-session call. Issuance
// happens synchronously inside a request handler (break-glass login,
// SSO callback) — a hung auth-bff must not hang the caller's request
// forever.
const defaultHTTPIssuerTimeout = 5 * time.Second

// HTTPIssuer is the production SessionIssuer. It POSTs to auth-bff's
// POST /internal/mint-session (internal/session/mint_session.go),
// authenticated with the shared X-Internal-Auth header — see D2 in
// docs/superpowers/plans/2026-09-07-break-glass-activation-v2.md. Not
// mTLS, not a Bearer token: both were wrong guesses recorded as errata.
type HTTPIssuer struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewHTTPIssuer builds an HTTPIssuer with a sane default timeout. Pass
// a non-nil client via NewHTTPIssuerWithClient to override it (tests
// that need a tighter timeout).
func NewHTTPIssuer(baseURL, secret string, httpClient *http.Client) *HTTPIssuer {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPIssuerTimeout}
	}
	return &HTTPIssuer{baseURL: baseURL, secret: secret, http: httpClient}
}

// NewHTTPIssuerWithClient builds an HTTPIssuer with an explicit,
// required http.Client — used by tests that need a tight timeout to
// exercise the timeout path without waiting 5s.
func NewHTTPIssuerWithClient(baseURL, secret string, httpClient *http.Client) *HTTPIssuer {
	return &HTTPIssuer{baseURL: baseURL, secret: secret, http: httpClient}
}

// NewSessionIssuer decides which SessionIssuer to wire up from config,
// so a service's main.go never has to construct a "half-configured"
// HTTPIssuer itself. baseURL and secret come from cfg.AuthBFFURL and
// cfg.InternalAuthSecret (pkg/config) — the same two settings
// MobileLoginClient already relies on, so this needs no new config
// field.
//
// If either is empty, NoopIssuer is returned: every Issue call then
// fails loudly with ErrIssuerUnavailable instead of a misconfigured
// deploy silently serving an unauthenticated route. InternalAuthSecret
// is already required outside dev (pkg/config Validate), and AuthBFFURL
// is optional there — so a break-glass/SSO deploy without AuthBFFURL
// set is a valid, if degraded, state: those routes 500 instead of
// crash-looping the whole service.
func NewSessionIssuer(baseURL, secret string, httpClient *http.Client) SessionIssuer {
	if baseURL == "" || secret == "" {
		return NoopIssuer{}
	}
	return NewHTTPIssuer(baseURL, secret, httpClient)
}

type mintSessionRequestWire struct {
	TenantID    string `json:"tenant_id"`
	UserID      string `json:"user_id"`
	AuthContext string `json:"auth_context"`
	TTLSeconds  int    `json:"ttl_seconds"`
}

type mintSessionResponseWire struct {
	SetCookie string `json:"set_cookie"`
}

// Issue implements SessionIssuer. Any non-200 response, a malformed
// body, or a transport failure (timeout, connection refused) returns
// an error and an empty cookie string — never a partially-trusted
// cookie value.
//
// Never logged here: the service key (never attached to any log
// call), the returned cookie, or an email (this issuer never sends
// one — see the SessionIssuer doc comment on why that's safe).
func (i *HTTPIssuer) Issue(ctx context.Context, tenantID, userID uuid.UUID, authContext string, ttl time.Duration) (string, error) {
	reqBody := mintSessionRequestWire{
		TenantID:    tenantID.String(),
		UserID:      userID.String(),
		AuthContext: authContext,
		TTLSeconds:  int(ttl.Seconds()),
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("authbffclient: marshal mint-session request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, i.baseURL+"/internal/mint-session", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("authbffclient: build mint-session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(internalAuthHeader, i.secret)

	resp, err := i.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("authbffclient: mint-session request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode != http.StatusOK {
		// Deliberately no response body in the error: it may echo back
		// caller-controlled fields, and the handler above maps any error
		// here to a generic 500 without leaking why — keep that true from
		// this layer up.
		return "", fmt.Errorf("authbffclient: mint-session returned status %d", resp.StatusCode)
	}

	var out mintSessionResponseWire
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("authbffclient: decode mint-session response: %w", err)
	}
	if out.SetCookie == "" {
		return "", fmt.Errorf("authbffclient: mint-session response missing set_cookie")
	}

	return out.SetCookie, nil
}
