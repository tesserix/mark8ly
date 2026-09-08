// Package ssousers implements sso.UsersRepo against platform-api (#820).
//
// It exists because marketplace-api cannot answer "who is this email in this
// tenant" for itself. `user_profiles` is keyed on a provider subject, is not
// tenant-scoped, and holds display preferences; membership is FGA tuples and
// identity is Zitadel. FGA can check a relation for a KNOWN id but cannot
// search by email.
//
// Getting that answer wrong is silently wrong. A merchant admin who already
// signs in with a password would, on their first SSO login, be minted a SECOND
// identity — their role would not follow them, their audit trail would split,
// and they would most likely land with no access at all.
package ssousers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// DefaultRole is the tenant role granted to a user provisioned by an SSO
// login. Staff, deliberately: an IdP assertion is a statement about WHO
// someone is, not about how much they should be trusted, and the tenant's
// owner can raise it afterwards.
const DefaultRole = "staff"

// Client calls platform-api's SSO identity routes.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewClient constructs a Client. httpClient may be nil.
//
// The timeout is longer than the stores client's 3s because these calls reach
// Zitadel and OpenFGA behind platform-api, and they sit on an interactive
// login path where a retry means starting the IdP round-trip again.
func NewClient(baseURL, secret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: baseURL, secret: secret, http: httpClient}
}

// FindByTenantAndEmail satisfies sso.UsersRepo.
//
// found is true ONLY when the email already belongs to a member of the
// tenant — someone with a role, whose identity is reused as-is. An account
// that exists but holds no role here answers false, which sends JIT to
// CreateForTenant; that resolves the same account rather than duplicating it
// and grants the role. Both paths land on one identity, which is why the JIT
// algorithm itself needed no change.
func (c *Client) FindByTenantAndEmail(ctx context.Context, tenantID uuid.UUID, email string) (uuid.UUID, bool, error) {
	var out struct {
		UserID string `json:"user_id"`
		Found  bool   `json:"found"`
	}
	err := c.post(ctx,
		fmt.Sprintf("/internal/tenants/%s/sso-users/lookup", tenantID),
		map[string]string{"email": email}, &out)
	if err != nil {
		return uuid.Nil, false, err
	}
	if !out.Found {
		return uuid.Nil, false, nil
	}
	id, err := parseUserID(out.UserID)
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

// CreateForTenant satisfies sso.UsersRepo: ensure the account exists and give
// it a role on the tenant.
//
// attrs carries whatever the IdP asserted; only the name is used, and only to
// satisfy Zitadel's requirement for a non-empty given/family name. The role
// argument from JIT is ignored in favour of DefaultRole for the reason given
// there: an IdP says who someone is, not how much to trust them.
func (c *Client) CreateForTenant(ctx context.Context, tenantID uuid.UUID, email string,
	attrs map[string]any, _ string) (uuid.UUID, error) {

	first, last := namesFrom(attrs)

	var out struct {
		UserID string `json:"user_id"`
	}
	err := c.post(ctx,
		fmt.Sprintf("/internal/tenants/%s/sso-users", tenantID),
		map[string]string{
			"email":      email,
			"first_name": first,
			"last_name":  last,
			"role":       DefaultRole,
		}, &out)
	if err != nil {
		return uuid.Nil, err
	}
	return parseUserID(out.UserID)
}

// namesFrom pulls a display name out of the resolved IdP attributes.
//
// Both halves are best-effort: platform-api derives them from the email local
// part when they arrive empty, because Zitadel rejects an empty givenName or
// familyName outright and a nameless assertion would otherwise fail the login
// rather than provision.
func namesFrom(attrs map[string]any) (first, last string) {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := attrs[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	return get("firstName", "given_name", "givenName"),
		get("lastName", "family_name", "familyName", "surname")
}

func parseUserID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		// Zitadel ids are not UUIDs in every deployment shape, and
		// sso.UsersRepo's contract is a uuid.UUID. Saying so plainly beats
		// returning uuid.Nil, which would read as a valid user and bind an
		// SSO login to the zero identity.
		return uuid.Nil, fmt.Errorf("ssousers: platform-api returned a non-UUID user id %q: %w", raw, err)
	}
	return id, nil
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("ssousers: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("ssousers: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ssousers: %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The body is not surfaced to the merchant — it can name another
		// account holding the same address — but it is worth carrying into
		// the log, where the SSO handler already sends its causes.
		return fmt.Errorf("ssousers: %s: unexpected status %d", path, resp.StatusCode)
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("ssousers: %s: decode: %w", path, err)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("ssousers: %s: decode data: %w", path, err)
	}
	return nil
}
