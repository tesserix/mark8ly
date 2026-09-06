package authbffclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ProviderGoogle is the only federated provider the mobile admin app signs
// in with today. auth-bff pins the IDP by this name and refuses any other
// value outright, so passing anything else is a client bug, not a feature
// flag.
const ProviderGoogle = "google"

// IDPError carries auth-bff's own stable error code for the IDP routes.
//
// The password login path collapses everything into ErrInvalidCredentials
// because telling a wrong password from an unknown account is an
// enumeration oracle. The IDP path has the opposite requirement: Google
// already authenticated the person, so the reason a sign-in was refused
// ("this Google account has no admin account here", "the provider never
// verified the email") is actionable copy the merchant needs, and
// flattening it produces the generic dead end #493 was.
type IDPError struct {
	// Code is auth-bff's own `error` field, verbatim.
	Code string
	// Status is the HTTP status auth-bff answered with, kept so a caller
	// can distinguish a refusal from an upstream outage without having to
	// enumerate every code.
	Status int
}

func (e *IDPError) Error() string {
	return fmt.Sprintf("authbffclient: auth-bff idp returned %d: %s", e.Status, e.Code)
}

// IDPFinishResult is what auth-bff's mobile idp/finish answers with.
//
// TenantRequired is the ONLY outcome the mobile flow expects in practice:
// which tenant a Google-authenticated merchant belongs to is unknowable
// until the identity has been resolved, so marketplace-api never sends a
// workspace_tenant on finish and always resolves it by email afterwards.
// The token fields are still decoded so a finish that somehow completes
// outright is not silently dropped.
type IDPFinishResult struct {
	TenantRequired bool
	SessionID      string
	SessionToken   string
	// LoginName is the VERIFIED email auth-bff resolved from Zitadel — not
	// anything a client asserted. It is what the tenant lookup keys on.
	LoginName string

	// Login carries the completed-login fields for the (unexpected) case
	// where finish completes without needing a tenant.
	Login LoginResult
}

type idpStartWire struct {
	AuthURL string `json:"auth_url"`
}

type idpFinishWire struct {
	TenantRequired bool   `json:"tenant_required"`
	SessionID      string `json:"session_id"`
	SessionToken   string `json:"session_token"`
	LoginName      string `json:"login_name"`
}

// IDPStart opens a Zitadel IDP intent and returns the URL the app must
// open in an authentication session.
//
// returnURL is built by the CALLER from configuration, never from client
// input: Zitadel does not validate successUrl at all, so auth-bff's
// allowlist is the only control against handing a finished sign-in to
// somebody else's domain, and forwarding a device-supplied URL would make
// that allowlist the sole reviewer of attacker input.
func (c *MobileLoginClient) IDPStart(ctx context.Context, provider, returnURL string) (string, error) {
	raw, err := c.postIDP(ctx, "/auth/zitadel/mobile/idp/start", map[string]any{
		"provider":   provider,
		"return_url": returnURL,
	})
	if err != nil {
		return "", err
	}
	var wire idpStartWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return "", fmt.Errorf("authbffclient: decode idp start: %w", err)
	}
	if wire.AuthURL == "" {
		// Returning "" would send the app to an empty browser session and
		// look like a user cancellation.
		return "", fmt.Errorf("authbffclient: idp start returned no auth_url")
	}
	return wire.AuthURL, nil
}

// IDPFinish exchanges the intent id/token the app carried back from the
// bridge page for a Zitadel session.
//
// auth_request_id is deliberately NOT sent: auth-bff mints one on the
// mobile route, for the same reason it does on mobile login — a native
// client has no browser round trip to obtain one.
func (c *MobileLoginClient) IDPFinish(ctx context.Context, provider, intentID, intentToken string) (IDPFinishResult, error) {
	raw, err := c.postIDP(ctx, "/auth/zitadel/mobile/idp/finish", map[string]any{
		"provider":     provider,
		"intent_id":    intentID,
		"intent_token": intentToken,
	})
	if err != nil {
		return IDPFinishResult{}, err
	}

	var wire idpFinishWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return IDPFinishResult{}, fmt.Errorf("authbffclient: decode idp finish: %w", err)
	}
	login, err := decodeLoginWire(raw)
	if err != nil {
		return IDPFinishResult{}, err
	}
	return IDPFinishResult{
		TenantRequired: wire.TenantRequired,
		SessionID:      wire.SessionID,
		SessionToken:   wire.SessionToken,
		LoginName:      wire.LoginName,
		Login:          login,
	}, nil
}

// IDPComplete finishes a Google sign-in once the tenant has been resolved,
// and is what actually yields tokens (or a step-up).
func (c *MobileLoginClient) IDPComplete(ctx context.Context, loginName, sessionID, sessionToken, workspaceTenant string) (LoginResult, error) {
	raw, err := c.postIDP(ctx, "/auth/zitadel/mobile/idp/complete", map[string]any{
		"login_name":       loginName,
		"session_id":       sessionID,
		"session_token":    sessionToken,
		"workspace_tenant": workspaceTenant,
	})
	if err != nil {
		return LoginResult{}, err
	}
	return decodeLoginWire(raw)
}

// postIDP is post()'s sibling for the IDP routes. It differs in exactly
// one way — every non-2xx answer keeps auth-bff's own error code, rather
// than 401 collapsing into ErrInvalidCredentials — see IDPError.
func (c *MobileLoginClient) postIDP(ctx context.Context, path string, body map[string]any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("authbffclient: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("authbffclient: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authbffclient: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errWire struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &errWire)
		if errWire.Error == "" {
			errWire.Error = "upstream_error"
		}
		return nil, &IDPError{Code: errWire.Error, Status: resp.StatusCode}
	}
	return raw, nil
}

// decodeLoginWire reuses the login response shape so the IDP path cannot
// drift from it — auth-bff answers a completed IDP sign-in with the exact
// same body a completed password sign-in produces, and duplicating the
// decode would be a second place for that contract to rot.
func decodeLoginWire(raw []byte) (LoginResult, error) {
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
