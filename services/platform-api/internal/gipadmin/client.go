// GIP-708 ORDER: this package is DELETE, but its sentinel errors
// (ErrUserNotFound and friends) are reused ~20x by internal/zitadeladmin and
// internal/auth/handler.go. Relocate the sentinels to a shared home FIRST or
// deleting this package breaks the build in two other packages.
// See docs/auth/2026-09-07-gip-removal-audit.md.
//
// Package gipadmin is a thin wrapper over the Google Identity Toolkit
// admin REST API. It is used by platform-api to drive password reset
// flows without exposing the default Firebase action handler UI to end
// users.
//
// Auth model:
//   - sendPasswordResetOobCode uses an OAuth 2.0 bearer token obtained
//     from Application Default Credentials. In-cluster the platform-api
//     GSA is bound to the pod via Workload Identity; locally developers
//     run `gcloud auth application-default login`. This is required
//     because the `returnOobLink=true` flag only works on the admin
//     `projects/{id}/accounts:sendOobCode` endpoint, which rejects
//     API-key auth.
//   - resetPassword uses the public Identity Toolkit endpoint with an
//     API key — this is the same endpoint the Firebase JS SDK calls
//     when a user submits a new password, and does not require OAuth.
//
// No Firebase Admin SDK is pulled in: the SDK drags ~25 transitive
// dependencies for a flow that is two plain HTTPS POSTs.
package gipadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Sentinel errors surfaced to callers.
var (
	ErrUserNotFound    = errors.New("gipadmin: user not found")
	ErrInvalidOobCode  = errors.New("gipadmin: invalid or expired oob code")
	ErrWeakPassword    = errors.New("gipadmin: password does not meet complexity rules")
	ErrUnauthenticated = errors.New("gipadmin: admin credentials unavailable")
	ErrTooManyAttempts = errors.New("gipadmin: too many attempts, try again later")
	ErrUnavailable     = errors.New("gipadmin: upstream unavailable")
)

// Config holds the values needed to construct an AdminClient.
type Config struct {
	// ProjectID is the GCP project that owns the Identity Platform
	// instance (e.g. "tesseracthub-480811"). Maps to "projects/{id}"
	// in the admin REST path.
	ProjectID string
	// TenantID is the GIP tenant pool the merchant admin logs into
	// (e.g. "MP-Internal-e986p"). Sent as `tenantId` on every call.
	TenantID string
	// WebAPIKey is the Firebase Web API key for the same project.
	// Used ONLY for the public resetPassword call; it is not a secret
	// and is safe to ship to the browser, but lives here because
	// platform-api is the one performing the reset.
	WebAPIKey string
}

// AdminClient is a thin Identity Toolkit admin client for password flows.
type AdminClient struct {
	cfg         Config
	httpClient  *http.Client
	tokenSource oauth2.TokenSource
}

// New constructs an AdminClient. Returns an error if ADC cannot be
// resolved or required config fields are missing. The returned client
// is safe for concurrent use.
func New(ctx context.Context, cfg Config) (*AdminClient, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("gipadmin: ProjectID is required")
	}
	if cfg.TenantID == "" {
		return nil, fmt.Errorf("gipadmin: TenantID is required")
	}
	if cfg.WebAPIKey == "" {
		return nil, fmt.Errorf("gipadmin: WebAPIKey is required")
	}
	ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("gipadmin: default token source: %w", err)
	}
	return &AdminClient{
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		tokenSource: ts,
	}, nil
}

// SendPasswordResetOobCode asks GIP to mint an out-of-band reset code
// for the given email without sending GIP's default email. The returned
// oobCode is the opaque token the caller embeds in their own email; it
// is later passed back to ResetPassword to actually change the password.
//
// Returns ErrUserNotFound if the email is not registered in the tenant.
// Callers that want to avoid enumeration should treat this as success.
func (c *AdminClient) SendPasswordResetOobCode(ctx context.Context, email string) (string, error) {
	url := fmt.Sprintf(
		"https://identitytoolkit.googleapis.com/v1/projects/%s/accounts:sendOobCode",
		c.cfg.ProjectID,
	)
	body := map[string]any{
		"requestType":   "PASSWORD_RESET",
		"email":         email,
		"returnOobLink": true,
		"tenantId":      c.cfg.TenantID,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("gipadmin: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("gipadmin: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	tok, err := c.tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var parsed struct {
			OobCode string `json:"oobCode"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return "", fmt.Errorf("gipadmin: decode sendOobCode response: %w", err)
		}
		if parsed.OobCode == "" {
			return "", fmt.Errorf("gipadmin: empty oobCode in response")
		}
		return parsed.OobCode, nil
	}

	// Identity Toolkit returns {"error":{"message":"...","code":...}} on
	// failure. Map the well-known strings to sentinel errors so the
	// service layer can decide what to do (and critically, so the
	// handler can choose whether to leak user-existence to the caller).
	errMsg := parseIdentityToolkitError(respBody)
	switch errMsg {
	case "EMAIL_NOT_FOUND":
		return "", ErrUserNotFound
	case "TOO_MANY_ATTEMPTS_TRY_LATER":
		return "", ErrTooManyAttempts
	}
	return "", fmt.Errorf("gipadmin: sendOobCode %d: %s", resp.StatusCode, errMsg)
}

// ResetPassword submits a new password for the given oob code. Uses the
// public API-key endpoint because the oob code itself is the proof of
// possession — no admin bearer token required.
func (c *AdminClient) ResetPassword(ctx context.Context, oobCode, newPassword string) error {
	url := fmt.Sprintf(
		"https://identitytoolkit.googleapis.com/v1/accounts:resetPassword?key=%s",
		c.cfg.WebAPIKey,
	)
	body := map[string]any{
		"oobCode":     oobCode,
		"newPassword": newPassword,
		// The code was minted inside this tenant (SendPasswordResetOobCode
		// sends the same tenantId), and Identity Toolkit only resolves an
		// oobCode within the tenant that issued it. Omitting this looks the
		// code up at project level, where it does not exist — GIP answers
		// INVALID_OOB_CODE and a freshly-issued link reads as expired.
		"tenantId": c.cfg.TenantID,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("gipadmin: marshal reset request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("gipadmin: build reset request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	errMsg := parseIdentityToolkitError(respBody)
	switch errMsg {
	case "EXPIRED_OOB_CODE", "INVALID_OOB_CODE":
		return ErrInvalidOobCode
	case "WEAK_PASSWORD : Password should be at least 6 characters",
		"WEAK_PASSWORD":
		return ErrWeakPassword
	}
	return fmt.Errorf("gipadmin: resetPassword %d: %s", resp.StatusCode, errMsg)
}

// parseIdentityToolkitError extracts the "message" field from the
// standard Identity Toolkit error envelope. Falls back to the raw body
// when the shape is unexpected so logs still have something to chew on.
func parseIdentityToolkitError(body []byte) string {
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Message != "" {
		return env.Error.Message
	}
	return string(body)
}
