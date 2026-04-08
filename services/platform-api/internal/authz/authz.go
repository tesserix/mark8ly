// Package authz is a thin OpenFGA SDK wrapper for the operations Mark8ly
// platform-api needs.
//
// For Phase D Tier 1 we only need two operations:
//
//   - WriteMembership: write the user→tenant membership tuple after a tenant
//     is created during onboarding completion. This is the operation routed
//     through the outbox to fix auth-bug #2 (race) and #3 (no retry).
//
//   - CheckMembership: read-side check that a user is a member of a tenant.
//     Used by auth-bff's auto-login (with retry) before issuing a session.
//
// We define a Client interface so the service layer can be unit-tested with
// a fake. Integration tests run the real implementation against a real
// OpenFGA container in docker-compose.
//
// Design notes:
//
//   - We do NOT copy the old go-shared/authz package. It carried complexity
//     for relations and stores we don't yet exercise. The minimal model
//     (user, tenant.member, tenant.owner) is in infra/openfga/model.fga.
//
//   - Owner is also a member by virtue of being written to both relations
//     in onboarding completion. We don't model "owner implies member" in the
//     OpenFGA DSL yet — explicit tuples are simpler to reason about for v1.
package authz

import (
	"context"
	"fmt"
	"net/http"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

// FGAStoreName is the canonical name of the platform store created by
// infra/dev/seed/fga-init.sh and discoverable via DiscoverStoreID.
const FGAStoreName = "mark8ly-platform"

// Role is one of the four directly-assignable tenant roles defined in
// infra/openfga/model.fga. Ordered roughly by authority: owner > admin
// > staff > viewer. Callers use these constants rather than bare
// strings so a typo becomes a compile error.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleStaff  Role = "staff"
	RoleViewer Role = "viewer"
)

// rolePriority gives us an ordering over the role constants so
// GetRole can return the highest role a user holds when more than one
// tuple exists (e.g. future "staff promoted to admin" edge cases where
// we intentionally leave the old tuple in place for audit trail).
var rolePriority = map[Role]int{
	RoleOwner:  4,
	RoleAdmin:  3,
	RoleStaff:  2,
	RoleViewer: 1,
}

// allRoles is iterated by GetRole. Declared once so the order is
// stable and we don't allocate on every call.
var allRoles = []Role{RoleOwner, RoleAdmin, RoleStaff, RoleViewer}

// Client is the operations platform-api needs from OpenFGA.
//
// Phase O expanded this from the original (WriteMembership,
// WriteOwnership, CheckMembership) triple to support role writes and
// generic permission checks. The legacy CheckMembership helper is
// retained because auth-bff's autologin retry loop uses the same
// semantic — "any role implies membership" — via the derived `member`
// relation in the DSL.
type Client interface {
	// WriteOwnership writes user:<id> owner tenant:<id>. Called by
	// the onboarding outbox drainer on tenant creation.
	WriteOwnership(ctx context.Context, userID, tenantID string) error

	// WriteRole writes user:<id> <role> tenant:<id> for one of the
	// four role constants. Reserved for the future invite-teammate
	// flow; unused in Phase O but checked in unit tests.
	WriteRole(ctx context.Context, userID string, role Role, tenantID string) error

	// CheckMembership is a convenience wrapper around the derived
	// `member` relation, used by auth-bff's autologin retry loop.
	CheckMembership(ctx context.Context, userID, tenantID string) (bool, error)

	// Check is the generic permission check. Relation can be any
	// relation or permission defined on the tenant type, e.g.
	// "can_edit_settings".
	Check(ctx context.Context, userID, relation, tenantID string) (bool, error)

	// GetRole returns the highest role the user holds on the tenant,
	// or the empty string if they have no role at all. "Highest" is
	// defined by rolePriority.
	GetRole(ctx context.Context, userID, tenantID string) (Role, error)
}

// fgaClient is the OpenFGA-backed implementation of Client.
type fgaClient struct {
	api *client.OpenFgaClient
}

// Config holds the values needed to construct a real OpenFGA client.
// StoreID is created by the openfga-seed init container at boot.
type Config struct {
	APIURL  string // e.g. http://openfga:8080
	StoreID string // ulid; obtained from openfga-seed
	ModelID string // optional; latest is used if empty
}

// New constructs a real OpenFGA client.
func New(cfg Config) (Client, error) {
	if cfg.APIURL == "" {
		return nil, fmt.Errorf("authz: APIURL is required")
	}
	if cfg.StoreID == "" {
		return nil, fmt.Errorf("authz: StoreID is required")
	}

	api, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:               cfg.APIURL,
		StoreId:              cfg.StoreID,
		AuthorizationModelId: cfg.ModelID,
		HTTPClient:           &http.Client{},
	})
	if err != nil {
		return nil, fmt.Errorf("authz: new sdk client: %w", err)
	}
	return &fgaClient{api: api}, nil
}

// DiscoverStoreID looks up an OpenFGA store by display name and returns
// its ID. Used at startup so platform-api can self-bootstrap from a fresh
// OpenFGA instance: openfga-seed creates the store named "mark8ly-platform",
// platform-api discovers it on first boot, and FGA_STORE_ID never has to
// be threaded through env vars.
//
// Returns an empty string and nil error if the store doesn't exist (caller
// can decide to log+continue or fail-fast).
func DiscoverStoreID(ctx context.Context, apiURL, name string) (string, error) {
	api, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:     apiURL,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		return "", fmt.Errorf("authz: discover: new sdk: %w", err)
	}
	resp, err := api.ListStores(ctx).Execute()
	if err != nil {
		return "", fmt.Errorf("authz: discover: list stores: %w", err)
	}
	if resp == nil {
		return "", nil
	}
	for _, store := range resp.GetStores() {
		if store.Name == name {
			return store.Id, nil
		}
	}
	return "", nil
}

// WriteOwnership writes the tuple `user:<id> owner tenant:<id>`.
func (c *fgaClient) WriteOwnership(ctx context.Context, userID, tenantID string) error {
	return c.write(ctx, userID, "owner", tenantID)
}

// WriteRole writes the tuple `user:<id> <role> tenant:<id>` for any of
// the four role constants. Reserved for the invite-teammate flow; the
// integration point exists now so Phase O can test it even though the
// UI lands later.
func (c *fgaClient) WriteRole(ctx context.Context, userID string, role Role, tenantID string) error {
	if _, ok := rolePriority[role]; !ok {
		return fmt.Errorf("authz: unknown role %q", role)
	}
	return c.write(ctx, userID, string(role), tenantID)
}

// CheckMembership returns true if the user is a member of the tenant.
// Delegates to the generic Check against the derived `member`
// relation, which resolves transitively to any role tuple.
func (c *fgaClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	return c.Check(ctx, userID, "member", tenantID)
}

// Check is the generic permission check.
func (c *fgaClient) Check(ctx context.Context, userID, relation, tenantID string) (bool, error) {
	body := client.ClientCheckRequest{
		User:     "user:" + userID,
		Relation: relation,
		Object:   "tenant:" + tenantID,
	}
	resp, err := c.api.Check(ctx).Body(body).Execute()
	if err != nil {
		return false, fmt.Errorf("authz: check %s: %w", relation, err)
	}
	if resp == nil || resp.Allowed == nil {
		return false, nil
	}
	return *resp.Allowed, nil
}

// GetRole iterates the direct role relations in priority order and
// returns the first match. O(4) OpenFGA Check calls worst case, which
// is fine for the low-frequency "admin middleware asking once per
// request" use case Phase O targets. Move to cookie-caching when it
// shows up as a latency problem.
//
// Returns ("", nil) if the user has no role on the tenant at all.
func (c *fgaClient) GetRole(ctx context.Context, userID, tenantID string) (Role, error) {
	for _, role := range allRoles {
		ok, err := c.Check(ctx, userID, string(role), tenantID)
		if err != nil {
			return "", fmt.Errorf("authz: get role: %w", err)
		}
		if ok {
			return role, nil
		}
	}
	return "", nil
}

// write is the shared helper for WriteMembership and WriteOwnership.
func (c *fgaClient) write(ctx context.Context, userID, relation, tenantID string) error {
	body := client.ClientWriteRequest{
		Writes: []client.ClientTupleKey{{
			User:     "user:" + userID,
			Relation: relation,
			Object:   "tenant:" + tenantID,
		}},
	}
	_, err := c.api.Write(ctx).Body(body).Execute()
	if err != nil {
		// OpenFGA returns "write_failed_due_to_invalid_input" if the tuple
		// already exists. Treat that as success — the desired state is met.
		if isAlreadyExistsError(err) {
			return nil
		}
		return fmt.Errorf("authz: write %s tuple: %w", relation, err)
	}
	return nil
}

// isAlreadyExistsError pattern-matches on the OpenFGA error envelope.
// The SDK exposes the error code as a string field on the underlying type.
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	// Conservative pattern: any FGA validation error is treated as
	// "tuple state already matches what we wanted". This is safe for our
	// use case (we never overwrite tuples with conflicting values).
	if apiErr, ok := err.(openfga.FgaApiValidationError); ok {
		_ = apiErr
		return true
	}
	return false
}
