package onboarding

// google_verify.go: server-side validation of a GIP id_token presented by
// the onboarding "Continue with Google" flow.
//
// We use Identity Toolkit's REST `accounts:lookup` because:
//   - It validates the JWT (signature, expiry, audience, tenant) for us —
//     no need to ship a JWKS verifier in platform-api just for this one path.
//   - It returns the verified email + email_verified bool, which is exactly
//     what we need to bypass the magic-link verification step.
//
// The `accounts:lookup` call requires the public GIP API key. The same key
// is used by the frontend; on the server we read it from GIP_API_KEY env.
//
// Trust model: the lookup proves the holder controls a valid token issued
// by our GIP tenant pool for this email. That's the same proof a magic
// link gives us, so it's a sound replacement for the magic-link step in
// the Google sign-up path.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// GoogleVerifier validates a GIP id_token via Identity Toolkit accounts:lookup.
type GoogleVerifier struct {
	apiKey   string
	tenantID string
	client   *http.Client
	endpoint string // overridable for tests
}

// NewGoogleVerifier constructs a GoogleVerifier. Returns nil if apiKey is
// empty (the caller should treat that as "feature disabled").
func NewGoogleVerifier(apiKey, tenantID string) *GoogleVerifier {
	if apiKey == "" {
		return nil
	}
	return &GoogleVerifier{
		apiKey:   apiKey,
		tenantID: tenantID,
		client:   &http.Client{Timeout: 5 * time.Second},
		endpoint: "https://identitytoolkit.googleapis.com/v1/accounts:lookup",
	}
}

// VerifiedUser is the subset of accounts:lookup response we care about.
type VerifiedUser struct {
	LocalID       string
	Email         string
	EmailVerified bool
}

// Verify exchanges an id_token for the underlying user record. Returns an
// AppError on any failure path so the handler can map it to a clean status.
func (v *GoogleVerifier) Verify(ctx context.Context, idToken string) (*VerifiedUser, error) {
	if idToken == "" {
		return nil, apperrors.BadRequest("invalid_id_token", "id_token is required")
	}

	body, _ := json.Marshal(map[string]string{"idToken": idToken})
	url := fmt.Sprintf("%s?key=%s", v.endpoint, v.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("google verify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := v.client.Do(req)
	if err != nil {
		return nil, apperrors.Wrap(err, http.StatusServiceUnavailable, "gip_unreachable", "identity toolkit unreachable")
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		// 400 from Identity Toolkit means the token is invalid/expired/wrong tenant.
		return nil, apperrors.Unauthorized("invalid_id_token", "google id_token is invalid or expired")
	}

	var parsed struct {
		Users []struct {
			LocalID       string `json:"localId"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"emailVerified"`
			TenantID      string `json:"tenantId"`
		} `json:"users"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("google verify: decode: %w", err)
	}
	if len(parsed.Users) == 0 {
		return nil, apperrors.Unauthorized("invalid_id_token", "google id_token did not resolve to a user")
	}
	u := parsed.Users[0]

	// Defense in depth: even though Identity Toolkit verified the JWT for
	// us, double-check the tenant matches what we configured. Stops a
	// stolen token from another tenant pool from sneaking through if the
	// API key is shared across pools.
	if v.tenantID != "" && u.TenantID != "" && !strings.EqualFold(u.TenantID, v.tenantID) {
		return nil, apperrors.Unauthorized("tenant_mismatch", "id_token is for a different tenant pool")
	}

	return &VerifiedUser{
		LocalID:       u.LocalID,
		Email:         strings.ToLower(strings.TrimSpace(u.Email)),
		EmailVerified: u.EmailVerified,
	}, nil
}
