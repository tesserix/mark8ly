// Package teamproxy is a thin marketplace-api client for platform-api's
// INTERNAL team endpoints (/internal/tenants/:id/{members,invitations,
// members/role}). Team membership is owned by platform-api and is
// tenant-scoped; the mobile admin app only talks to marketplace-api, so
// this package lets the mobile team screens reach that data without the
// app needing a second base URL or an internal-auth secret.
//
// Auth mirrors internal/stores/platform_http.go: the shared X-Internal-Auth
// secret rides every request. Both are wired from the SAME cfg fields
// (PlatformAPIURL / PlatformAPISecret) that already power the storefront's
// store lookups in prod, so exposing team needs no new chart wiring.
package teamproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrPlatformUnavailable signals the platform-api could not be reached.
var ErrPlatformUnavailable = errors.New("teamproxy: platform-api unavailable")

// APIError carries a platform-api 4xx/5xx through to the caller so the
// handler can surface the real status + message (e.g. 403 "an admin cannot
// change another admin's role") instead of a generic 500.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("teamproxy: platform-api %d: %s", e.Status, e.Message)
}

// Client calls platform-api's internal team endpoints.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewClient constructs a Client. httpClient may be nil (defaults to a
// 5-second timeout). The secret is sent as X-Internal-Auth when non-empty.
func NewClient(baseURL, secret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{baseURL: baseURL, secret: secret, http: httpClient}
}

// Member mirrors platform-api invitation.Member. Kind is "owner" | "invited".
type Member struct {
	Email      string  `json:"email"`
	Role       string  `json:"role"`
	Kind       string  `json:"kind"`
	AcceptedAt *string `json:"accepted_at,omitempty"`
}

// Invitation mirrors the subset of platform-api invitation.Invitation the
// mobile team screen needs. TokenHash and internal UID columns are omitted.
type Invitation struct {
	ID         string  `json:"id"`
	StoreID    *string `json:"store_id,omitempty"`
	Email      string  `json:"email"`
	Role       string  `json:"role"`
	ExpiresAt  string  `json:"expires_at"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
	AcceptedAt *string `json:"accepted_at,omitempty"`
}

// RoleResult is the echo returned by the role-change endpoint.
type RoleResult struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// do performs the request, attaches internal auth, and decodes the
// platform-api `{data: ...}` envelope into out (when out is non-nil).
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("teamproxy: marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("teamproxy: new req: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("teamproxy: %w", errors.Join(ErrPlatformUnavailable, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return &APIError{Status: resp.StatusCode, Code: e.Error, Message: e.Message}
	}

	if out == nil {
		return nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("teamproxy: decode envelope: %w", err)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("teamproxy: decode data: %w", err)
	}
	return nil
}

// ListMembers returns the tenant's members (owner + accepted invitees).
func (c *Client) ListMembers(ctx context.Context, tenantID string) ([]Member, error) {
	var members []Member
	if err := c.do(ctx, http.MethodGet, "/internal/tenants/"+tenantID+"/members", nil, &members); err != nil {
		return nil, err
	}
	return members, nil
}

// ListInvitations returns the tenant's pending invitations.
func (c *Client) ListInvitations(ctx context.Context, tenantID string) ([]Invitation, error) {
	var invs []Invitation
	if err := c.do(ctx, http.MethodGet, "/internal/tenants/"+tenantID+"/invitations", nil, &invs); err != nil {
		return nil, err
	}
	return invs, nil
}

// CreateInvitation invites email at role. actorUID is the inviting user's GIP UID.
func (c *Client) CreateInvitation(ctx context.Context, tenantID, email, role, actorUID string) (*Invitation, error) {
	body := map[string]string{"email": email, "role": role, "uid": actorUID}
	var inv Invitation
	if err := c.do(ctx, http.MethodPost, "/internal/tenants/"+tenantID+"/invitations", body, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// RevokeInvitation cancels a pending invitation. actorUID is the actor's GIP UID.
func (c *Client) RevokeInvitation(ctx context.Context, tenantID, invID, actorUID string) error {
	body := map[string]string{"uid": actorUID}
	return c.do(ctx, http.MethodDelete, "/internal/tenants/"+tenantID+"/invitations/"+invID, body, nil)
}

// DeleteTenantAccount asks platform-api to delete the actor's account. For an
// owner this tears down the tenant; for staff it removes only their membership.
// platform-api is authoritative for the owner-vs-staff decision.
func (c *Client) DeleteTenantAccount(ctx context.Context, tenantID, actorUID string) error {
	body := map[string]string{"uid": actorUID}
	return c.do(ctx, http.MethodDelete, "/internal/tenants/"+tenantID+"/account", body, nil)
}

// UpdateMemberRole changes a member's tenant role. Target identified by email.
func (c *Client) UpdateMemberRole(ctx context.Context, tenantID, email, newRole, actorUID string) (*RoleResult, error) {
	body := map[string]string{"email": email, "new_role": newRole, "uid": actorUID}
	var res RoleResult
	if err := c.do(ctx, http.MethodPatch, "/internal/tenants/"+tenantID+"/members/role", body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// TenantMembership is one tenant the caller has a role on, as returned by
// platform-api's GET /api/v1/users/me/tenants.
type TenantMembership struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// ListMyTenants returns every tenant the given identity has a role on.
//
// # Why this is on the mobile critical path (#686)
//
// After a Zitadel sign-in the mobile app knows who the user is and
// nothing else. Zitadel mints no tenant claim (unlike GIP), every mobile
// admin route is behind RequireBoundTenant, and the stores response
// carries no tenant id — so without this lookup the client has no value
// to put in X-Acting-Tenant-Id and cannot make a single successful call.
// This is the mobile equivalent of the web's resolveWorkspaceTenant.
//
// # Why it goes through marketplace-api rather than being called directly
//
// platform-api mounts this on its PUBLIC /api/v1 group with no
// authentication — it takes the identity as a query parameter, because
// its only caller so far is the admin BFF passing a uid from an already
// validated session. A device calling it directly would be an
// unauthenticated membership lookup for any identity. Routing it through
// marketplace-api keeps that endpoint in-cluster and lets the caller's
// identity come from a VERIFIED bearer token instead of a query string.
//
// # Identity key
//
// id may be a provider uid OR an email: platform-api resolves either,
// because mark8ly writes both email- and uid-keyed FGA tuples for the
// same human. It is passed through verbatim — platform-api owns the
// email lowercasing, and provider uids are case-sensitive.
//
// An empty result is not an error: a signed-in user who has not onboarded
// genuinely has zero tenants, and the client shows a different screen for
// that than for a failed lookup.
func (c *Client) ListMyTenants(ctx context.Context, id string) ([]TenantMembership, error) {
	var out []TenantMembership
	path := "/api/v1/users/me/tenants?uid=" + url.QueryEscape(id)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
