package gipadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// tenantClaimKey is the custom-claim key marketplace-api reads out of the
// GIP ID token to decide which tenant a mobile caller belongs to
// (services/marketplace-api/internal/auth/gip_verifier.go).
const tenantClaimKey = "tenant_id"

// EnsureTenantClaim adds tenantID to the user's `tenant_id` custom claim,
// preserving every other claim already on the account.
//
// Why this exists: marketplace-api's mobile bearer auth reads `tenant_id`
// from the ID token, but until now nothing in the codebase ever WROTE it.
// Every tenant created through the onboarding wizard therefore had an owner
// who could sign in to GIP but was rejected by the mobile API — the app read
// that rejection as a dead session and bounced back to the login screen.
//
// The claim is written as an ARRAY, which is the shape the verifier's
// tenantIDFromClaims prefers, so a user can own more than one tenant later
// without a migration.
//
// Merge, don't clobber: accounts are known to carry unrelated claims (e.g.
// {"admin": true}). We read the current attributes, add to them, and write
// the union back. GIP has no partial-update for customAttributes — the field
// is replaced wholesale — so read-modify-write is the only correct approach.
//
// Idempotent: if tenantID is already present the call is a no-op and no
// write is issued, so retries from the outbox drainer are safe.
func (c *AdminClient) EnsureTenantClaim(ctx context.Context, uid, tenantID string) error {
	if uid == "" || tenantID == "" {
		return fmt.Errorf("gipadmin: uid and tenantID are required")
	}

	attrs, err := c.lookupCustomAttributes(ctx, uid)
	if err != nil {
		return err
	}

	tenants := existingTenantIDs(attrs[tenantClaimKey])
	for _, t := range tenants {
		if t == tenantID {
			return nil // already present — nothing to do
		}
	}
	attrs[tenantClaimKey] = append(tenants, tenantID)

	encoded, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("gipadmin: marshal custom attributes: %w", err)
	}

	// customAttributes is a JSON *string* field, not a nested object.
	return c.postAdmin(ctx, "accounts:update", map[string]any{
		"localId":          uid,
		"customAttributes": string(encoded),
	})
}

// existingTenantIDs normalises whatever shape the claim is currently in.
// Accepts the array form we write, plus the scalar form the verifier also
// tolerates, so an account written by any other tool still merges cleanly.
func existingTenantIDs(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		// The shape after a JSON round-trip.
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		// The shape while still in memory, before marshalling. Omitting
		// this silently dropped previously-held tenants on a second
		// append. Mirrors tenantIDFromClaims in marketplace-api, which
		// accepts the same three shapes.
		out := make([]string, 0, len(t))
		for _, s := range t {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// lookupCustomAttributes returns the account's current custom claims as a
// map. An account with no claims yields an empty (non-nil) map.
func (c *AdminClient) lookupCustomAttributes(ctx context.Context, uid string) (map[string]any, error) {
	raw, err := c.doAdmin(ctx, "accounts:lookup", map[string]any{
		"localId": []string{uid},
	})
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Users []struct {
			CustomAttributes string `json:"customAttributes"`
		} `json:"users"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("gipadmin: decode lookup response: %w", err)
	}
	if len(parsed.Users) == 0 {
		return nil, ErrUserNotFound
	}

	attrs := map[string]any{}
	if s := parsed.Users[0].CustomAttributes; s != "" {
		if err := json.Unmarshal([]byte(s), &attrs); err != nil {
			// Corrupt claims would otherwise be silently replaced,
			// destroying whatever was there. Fail loudly instead.
			return nil, fmt.Errorf("gipadmin: existing customAttributes is not valid JSON: %w", err)
		}
	}
	return attrs, nil
}

// postAdmin issues a tenant-scoped admin call and discards the body.
func (c *AdminClient) postAdmin(ctx context.Context, method string, body map[string]any) error {
	_, err := c.doAdmin(ctx, method, body)
	return err
}

// doAdmin issues an OAuth-authenticated call against the tenant-scoped
// Identity Toolkit admin API and returns the raw response body.
func (c *AdminClient) doAdmin(ctx context.Context, method string, body map[string]any) ([]byte, error) {
	url := fmt.Sprintf(
		"https://identitytoolkit.googleapis.com/v1/projects/%s/tenants/%s/%s",
		c.cfg.ProjectID, c.cfg.TenantID, method,
	)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gipadmin: marshal %s request: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("gipadmin: build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	tok, err := c.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return respBody, nil
	}

	errMsg := parseIdentityToolkitError(respBody)
	if errMsg == "USER_NOT_FOUND" {
		return nil, ErrUserNotFound
	}
	return nil, fmt.Errorf("gipadmin: %s %d: %s", method, resp.StatusCode, errMsg)
}
