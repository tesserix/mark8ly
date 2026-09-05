package zitadellogin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// TokenExchanger performs the OIDC authorization-code exchange against
// Zitadel and returns real OAuth tokens.
//
// # This is the first token exchange in the estate, deliberately
//
// Nothing in mark8ly exchanges an authorization code today. The web admin
// says so explicitly (apps/admin/app/auth/callback/route.ts: "It does NOT
// exchange `code` for anything — there is nothing left to exchange it
// for") because the browser rides auth-bff's own `m8_session` cookie; the
// OIDC flow is used only to drive Zitadel's session APIs.
//
// Mobile cannot do that. marketplace-api's mobile admin routes verify a
// BEARER JWT against Zitadel's JWKS, and a session cookie is not a bearer
// token — so keeping the app's own login form (rather than sending users
// to Zitadel's hosted login) requires somebody to turn the code into a
// token server-side. That is this type.
//
// The credentials it uses are the mark8ly-admin confidential client's,
// which the chart already injects into auth-bff as
// ZITADEL_ADMIN_CLIENT_ID / ZITADEL_ADMIN_CLIENT_SECRET and which,
// before this, no code read.
type TokenExchanger struct {
	issuer       string
	clientID     string
	clientSecret string
	http         *http.Client
}

func NewTokenExchanger(issuer, clientID, clientSecret string, httpClient *http.Client) *TokenExchanger {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &TokenExchanger{
		issuer:       strings.TrimSuffix(issuer, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         httpClient,
	}
}

// Tokens is the subset of the token response mobile needs.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// AuthorizeURL builds the /oauth/v2/authorize URL whose redirect carries
// the auth request id.
//
// adminProjectID is load-bearing and is the single easiest thing to get
// wrong here. marketplace-api's ZitadelVerifier pins the token's `aud` to
// the mark8ly-admin project so that a storefront token — same issuer,
// same signing key, same human — cannot be replayed as an admin
// credential. That audience is only present if the AUTHORIZE step asked
// for `urn:zitadel:iam:org:project:id:<id>:aud`. Omit it and everything
// still "works" right up until the API rejects a perfectly valid token
// with a flat 401 that reads exactly like a wrong password.
//
// offline_access is what yields a refresh token. Without it a merchant is
// silently signed out when the access token expires (~1h).
func (e *TokenExchanger) AuthorizeURL(redirectURI, adminProjectID, state string) string {
	q := url.Values{}
	q.Set("client_id", e.clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	q.Set("scope", strings.Join([]string{
		"openid", "profile", "email", "offline_access",
		"urn:zitadel:iam:org:project:id:" + adminProjectID + ":aud",
	}, " "))
	return e.issuer + "/oauth/v2/authorize?" + q.Encode()
}

// CodeFromCallbackURL extracts the authorization code from the callbackUrl
// that finalize returns.
//
// An `error` parameter is reported as an error rather than yielding an
// empty code: a caller that treated "" as success would exchange nothing
// and report a confusing failure one layer further on.
func CodeFromCallbackURL(callbackURL string) (string, error) {
	u, err := url.Parse(callbackURL)
	if err != nil {
		return "", fmt.Errorf("zitadellogin: parse callback url: %w", err)
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return "", fmt.Errorf("zitadellogin: callback returned error %q: %w", e, ErrUnavailable)
	}
	code := q.Get("code")
	if code == "" {
		return "", fmt.Errorf("zitadellogin: callback carried no code: %w", ErrUnavailable)
	}
	return code, nil
}

// ExchangeCodeForTokens trades the authorization code for tokens.
//
// redirectURI MUST byte-match the one the auth request was created with —
// Zitadel refuses the exchange otherwise, and the failure is a generic
// invalid_grant that says nothing about which value differed.
func (e *TokenExchanger) ExchangeCodeForTokens(ctx context.Context, code, redirectURI string) (Tokens, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.issuer+"/oauth/v2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("zitadellogin: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// client_secret_basic. The secret stays out of the form body so it
	// cannot end up in request-body logging.
	req.SetBasicAuth(e.clientID, e.clientSecret)

	resp, err := e.http.Do(req)
	if err != nil {
		return Tokens{}, fmt.Errorf("zitadellogin: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The upstream reason is preserved because invalid_grant vs
		// invalid_client vs unauthorized_client point at three completely
		// different misconfigurations.
		return Tokens{}, fmt.Errorf("zitadellogin: token exchange failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tok Tokens
	if err := json.Unmarshal(body, &tok); err != nil {
		return Tokens{}, fmt.Errorf("zitadellogin: decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return Tokens{}, fmt.Errorf("zitadellogin: token response carried no access_token: %w", ErrUnavailable)
	}
	return tok, nil
}

// CreateAuthRequest starts an OIDC flow server-side and returns the auth
// request id, with no browser involved.
//
// This is the precondition for keeping mark8ly's OWN login form on mobile:
// the existing /auth/zitadel/login endpoint requires an auth_request_id,
// and on web the browser obtains one by being redirected through
// /oauth/v2/authorize. A native app posting credentials to an API has no
// such redirect, so the server has to create the request itself.
//
// Zitadel answers /oauth/v2/authorize with a 302 to the login UI carrying
// ?authRequest=<id>. The redirect is deliberately NOT followed — following
// it fetches a browser-facing login page and loses the id.
func (e *TokenExchanger) CreateAuthRequest(ctx context.Context, redirectURI, adminProjectID, state string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		e.AuthorizeURL(redirectURI, adminProjectID, state), nil)
	if err != nil {
		return "", fmt.Errorf("zitadellogin: build authorize request: %w", err)
	}

	// Copy the client so the no-redirect policy is scoped to this call and
	// never mutates a shared http.Client other callers depend on.
	noFollow := *e.http
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := noFollow.Do(req)
	if err != nil {
		return "", fmt.Errorf("zitadellogin: authorize request: %w", err)
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		// A redirect_uri or client_id mismatch shows up here, and the
		// upstream text is the only thing that says which.
		return "", fmt.Errorf("zitadellogin: authorize did not redirect (%d): %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	u, err := url.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("zitadellogin: parse authorize redirect: %w", err)
	}
	id := u.Query().Get("authRequest")
	if id == "" {
		return "", fmt.Errorf("zitadellogin: authorize redirect carried no authRequest id (%s): %w", loc, ErrUnavailable)
	}
	return id, nil
}
