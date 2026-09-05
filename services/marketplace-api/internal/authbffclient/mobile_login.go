package authbffclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrInvalidCredentials is auth-bff's enumeration-safe rejection: a wrong
// password and a non-existent account produce this identical error, and
// callers must not try to tell them apart.
var ErrInvalidCredentials = errors.New("authbffclient: invalid credentials")

// MobileLoginClient calls auth-bff's mobile login routes.
//
// # Why marketplace-api proxies this rather than the app calling auth-bff
//
// auth-bff is internet-reachable at auth.mark8ly.com, and the ONLY thing
// protecting /auth/zitadel/login from credential stuffing is the
// X-Internal-Auth secret that its trusted server-side callers hold. A
// device cannot hold that secret. Exposing an unauthenticated login route
// on auth-bff would remove that protection for every surface at once.
//
// marketplace-api is already the app's public backend, already holds the
// same secret (MARKETPLACE_INTERNAL_AUTH_SECRET — the identical GCP secret
// auth-bff checks against), and already has a per-user rate limiter. So
// the public front door lives there and auth-bff keeps its gate.
type MobileLoginClient struct {
	baseURL string
	secret  string
	http    *http.Client
}

func NewMobileLoginClient(baseURL, secret string, httpClient *http.Client) *MobileLoginClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &MobileLoginClient{baseURL: baseURL, secret: secret, http: httpClient}
}

// LoginResult carries every outcome auth-bff can report. Exactly one of
// {tokens present, EmailOTPRequired, TOTPRequired} is true on success.
type LoginResult struct {
	UID          string
	Email        string
	TenantID     string
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int

	// EmailOTPRequired is the COMMON first-login case on mobile, not an
	// edge case: a fresh install is by definition an unrecognised device.
	EmailOTPRequired bool
	MFARequired      bool

	TOTPRequired bool
	SessionID    string
	SessionToken string

	// PendingToken is the sealed state the email-OTP challenge resumes
	// from. Opaque to us and to the client; handed straight back.
	PendingToken string
}

type loginWire struct {
	Data *struct {
		UID              string `json:"uid"`
		Email            string `json:"email"`
		TenantID         string `json:"tenant_id"`
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		TokenType        string `json:"token_type"`
		ExpiresIn        int    `json:"expires_in"`
		EmailOTPRequired bool   `json:"email_otp_required"`
		MFARequired      bool   `json:"mfa_required"`
		PendingToken     string `json:"pending_token"`
	} `json:"data"`
	TOTPRequired bool   `json:"totp_required"`
	SessionID    string `json:"session_id"`
	SessionToken string `json:"session_token"`
}

// Login posts credentials to auth-bff's mobile route.
//
// auth_request_id is deliberately NOT sent: auth-bff mints one for the
// mobile route, because a native client has no browser redirect through
// /oauth/v2/authorize to obtain one.
//
// A step-up (email OTP or TOTP) is a normal successful outcome, not an
// error — treating it as failure is what produced the silent redirect loop
// in #493, where every layer logged success and the user saw nothing.
func (c *MobileLoginClient) Login(ctx context.Context, email, password, workspaceTenant string) (LoginResult, error) {
	return c.post(ctx, "/auth/zitadel/mobile/login", map[string]any{
		"login_name":       email,
		"password":         password,
		"workspace_tenant": workspaceTenant,
	})
}

// VerifyOTP completes a login that stopped at the email-OTP gate — the
// common first sign-in on a fresh install, since such a device is by
// definition unrecognised.
//
// The email is deliberately not sent: auth-bff derives identity from the
// sealed pending token, and offering one here would invite a later change
// to start trusting it, which is exactly the binding the challenge exists
// to protect.
func (c *MobileLoginClient) VerifyOTP(ctx context.Context, pendingToken, code string) (LoginResult, error) {
	return c.post(ctx, "/auth/zitadel/mobile/otp/verify", map[string]any{
		"pending_token": pendingToken,
		"code":          code,
	})
}

// VerifyTOTP completes a login that stopped at the TOTP gate.
func (c *MobileLoginClient) VerifyTOTP(ctx context.Context, authRequestID, email, sessionID, sessionToken, code, workspaceTenant string) (LoginResult, error) {
	return c.post(ctx, "/auth/zitadel/mobile/totp", map[string]any{
		"auth_request_id":  authRequestID,
		"login_name":       email,
		"session_id":       sessionID,
		"session_token":    sessionToken,
		"code":             code,
		"workspace_tenant": workspaceTenant,
	})
}

func (c *MobileLoginClient) post(ctx context.Context, path string, body map[string]any) (LoginResult, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return LoginResult{}, fmt.Errorf("authbffclient: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return LoginResult{}, fmt.Errorf("authbffclient: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return LoginResult{}, fmt.Errorf("authbffclient: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode == http.StatusUnauthorized {
		// Preserved as a distinct error so the handler can answer 401
		// rather than flattening it into an upstream failure — a merchant
		// typing the wrong password must not be told the service is down.
		return LoginResult{}, ErrInvalidCredentials
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return LoginResult{}, fmt.Errorf("authbffclient: auth-bff returned %d: %s", resp.StatusCode, string(raw))
	}

	var wire loginWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return LoginResult{}, fmt.Errorf("authbffclient: decode: %w", err)
	}

	out := LoginResult{
		TOTPRequired: wire.TOTPRequired,
		SessionID:    wire.SessionID,
		SessionToken: wire.SessionToken,
	}
	if wire.Data != nil {
		out.UID = wire.Data.UID
		out.Email = wire.Data.Email
		out.TenantID = wire.Data.TenantID
		out.AccessToken = wire.Data.AccessToken
		out.RefreshToken = wire.Data.RefreshToken
		out.TokenType = wire.Data.TokenType
		out.ExpiresIn = wire.Data.ExpiresIn
		out.EmailOTPRequired = wire.Data.EmailOTPRequired
		out.MFARequired = wire.Data.MFARequired
		out.PendingToken = wire.Data.PendingToken
	}
	return out, nil
}
