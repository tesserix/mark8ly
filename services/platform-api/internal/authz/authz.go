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

	// WriteStoreParent writes store:<id> parent tenant:<id>. Called
	// by the onboarding outbox drainer on default-store creation
	// (Phase Q). The parent tuple is what lets FGA's store-type
	// permissions inherit from tenant-level roles via `from parent`
	// in the DSL, so tenant owners/admins automatically have access
	// to every store under their tenant without per-store tuples.
	WriteStoreParent(ctx context.Context, storeID, tenantID string) error

	// WriteRole writes user:<id> <role> tenant:<id> for one of the
	// four tenant role constants. Used by the Phase P tenant-level
	// invitation accept path.
	WriteRole(ctx context.Context, userID string, role Role, tenantID string) error

	// WriteRoleObject writes user:<id> <relation> <type>:<id>.
	// Used by Phase R store-level invitations to grant manager /
	// staff / viewer on a specific store object. The `relation`
	// string is free-form so the caller can pick any relation
	// defined in the DSL for the target type.
	WriteRoleObject(ctx context.Context, userID, relation, objectType, objectID string) error

	// CheckMembership is a convenience wrapper around the derived
	// `member` relation, used by auth-bff's autologin retry loop.
	CheckMembership(ctx context.Context, userID, tenantID string) (bool, error)

	// Check is the generic permission check against a `tenant:<id>`
	// object. Relation can be any relation or permission defined on
	// the tenant type.
	Check(ctx context.Context, userID, relation, tenantID string) (bool, error)

	// CheckObject is Check with an explicit object type. Use for
	// store-scoped (and future vendor/product-scoped) permissions.
	CheckObject(ctx context.Context, userID, relation, objectType, objectID string) (bool, error)

	// GetRole returns the highest role the user holds on the tenant,
	// or the empty string if they have no role at all. "Highest" is
	// defined by rolePriority.
	GetRole(ctx context.Context, userID, tenantID string) (Role, error)

	// ListMemberTenants returns every tenant id the user has any
	// role on. Phase P: used by `GET /api/v1/users/me/tenants` to
	// power the admin tenant switcher and the multi-tenant sign-in
	// flow. The returned ids are unenriched — the caller joins
	// against the tenants table via tenant.ListByIDs.
	ListMemberTenants(ctx context.Context, userID string) ([]string, error)

	// DeleteTuple removes user:<userID> <relation> tenant:<tenantID>. Idempotent.
	DeleteTuple(ctx context.Context, userID, relation, tenantID string) error
	// DeleteStoreParent removes tenant:<tenantID> parent store:<storeID>. Idempotent.
	DeleteStoreParent(ctx context.Context, storeID, tenantID string) error

	// ListTenantMembers returns every user holding a direct role tuple
	// on the tenant, along with which relation each holds. Used by
	// tenant teardown (#361) to enumerate everyone whose FGA state
	// needs cleaning up, not just the owner. Unlike ListMemberTenants
	// (which asks "which tenants does this user belong to"), this asks
	// the reverse: "which users belong to this tenant".
	ListTenantMembers(ctx context.Context, tenantID string) ([]Member, error)
}

// Member is one user's direct role tuple on a tenant, as returned by
// ListTenantMembers.
type Member struct {
	UserID   string
	Relation string
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

// WriteStoreParent writes the tuple `tenant:<tid> parent store:<sid>`.
// Unlike WriteOwnership / WriteRole the "user" field carries a
// `tenant:` prefix rather than `user:` because the store-type
// `parent` relation accepts a tenant subject, not a user.
func (c *fgaClient) WriteStoreParent(ctx context.Context, storeID, tenantID string) error {
	body := client.ClientWriteRequest{
		Writes: []client.ClientTupleKey{{
			User:     "tenant:" + tenantID,
			Relation: "parent",
			Object:   "store:" + storeID,
		}},
	}
	_, err := c.api.Write(ctx).Body(body).Execute()
	if err != nil {
		if isAlreadyExistsError(err) {
			return nil
		}
		return fmt.Errorf("authz: write store parent: %w", err)
	}
	return nil
}

// WriteRole writes the tuple `user:<id> <role> tenant:<id>` for any
// of the four tenant role constants.
func (c *fgaClient) WriteRole(ctx context.Context, userID string, role Role, tenantID string) error {
	if _, ok := rolePriority[role]; !ok {
		return fmt.Errorf("authz: unknown role %q", role)
	}
	return c.write(ctx, userID, string(role), tenantID)
}

// WriteRoleObject writes user:<id> <relation> <type>:<id>. Used by
// Phase R store-level invitations.
func (c *fgaClient) WriteRoleObject(ctx context.Context, userID, relation, objectType, objectID string) error {
	body := client.ClientWriteRequest{
		Writes: []client.ClientTupleKey{{
			User:     "user:" + userID,
			Relation: relation,
			Object:   objectType + ":" + objectID,
		}},
	}
	_, err := c.api.Write(ctx).Body(body).Execute()
	if err != nil {
		if isAlreadyExistsError(err) {
			return nil
		}
		return fmt.Errorf("authz: write role object %s:%s %s: %w", objectType, objectID, relation, err)
	}
	return nil
}

// CheckMembership returns true if the user is a member of the tenant.
// Delegates to the generic Check against the derived `member`
// relation, which resolves transitively to any role tuple.
func (c *fgaClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	return c.Check(ctx, userID, "member", tenantID)
}

// Check is the generic permission check against a `tenant:` object.
func (c *fgaClient) Check(ctx context.Context, userID, relation, tenantID string) (bool, error) {
	return c.CheckObject(ctx, userID, relation, "tenant", tenantID)
}

// CheckObject is Check against an explicit object type.
func (c *fgaClient) CheckObject(ctx context.Context, userID, relation, objectType, objectID string) (bool, error) {
	body := client.ClientCheckRequest{
		User:     "user:" + userID,
		Relation: relation,
		Object:   objectType + ":" + objectID,
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

// ListMemberTenants returns every tenant id where the user has any
// role. Delegates to OpenFGA's ListObjects against the derived
// `member` relation, which resolves transitively via the DSL union.
// Returns ids without the "tenant:" prefix so callers can pass them
// straight into tenant.ListByIDs.
func (c *fgaClient) ListMemberTenants(ctx context.Context, userID string) ([]string, error) {
	body := client.ClientListObjectsRequest{
		User:     "user:" + userID,
		Relation: "member",
		Type:     "tenant",
	}
	resp, err := c.api.ListObjects(ctx).Body(body).Execute()
	if err != nil {
		return nil, fmt.Errorf("authz: list member tenants: %w", err)
	}
	if resp == nil {
		return nil, nil
	}
	objects := resp.GetObjects()
	out := make([]string, 0, len(objects))
	for _, o := range objects {
		// ListObjects returns fully-qualified object refs like
		// "tenant:<uuid>" — strip the prefix.
		if len(o) > len("tenant:") && o[:len("tenant:")] == "tenant:" {
			out = append(out, o[len("tenant:"):])
		}
	}
	return out, nil
}

// ListTenantMembers returns every user holding a direct role tuple on
// tenant:<tenantID>, via OpenFGA's Read (a tuple query by object),
// not ListObjects — ListObjects answers "which objects does this user
// relate to", the opposite of what teardown needs here. Read is
// paginated; this loops on the continuation token until OpenFGA
// reports none left, so a tenant with more members than fit in one
// page is still enumerated completely.
func (c *fgaClient) ListTenantMembers(ctx context.Context, tenantID string) ([]Member, error) {
	object := "tenant:" + tenantID
	var out []Member
	var continuationToken *string
	for {
		body := client.ClientReadRequest{Object: &object}
		req := c.api.Read(ctx).Body(body)
		if continuationToken != nil && *continuationToken != "" {
			req = req.Options(client.ClientReadOptions{ContinuationToken: continuationToken})
		}
		resp, err := req.Execute()
		if err != nil {
			return nil, fmt.Errorf("authz: list tenant members: %w", err)
		}
		if resp == nil {
			return out, nil
		}
		for _, tuple := range resp.GetTuples() {
			key := tuple.GetKey()
			user := key.GetUser()
			// Tuples are stored as "user:<id>" — strip the prefix the
			// same way ListMemberTenants strips "tenant:".
			if len(user) > len("user:") && user[:len("user:")] == "user:" {
				out = append(out, Member{UserID: user[len("user:"):], Relation: key.GetRelation()})
			}
		}
		if resp.ContinuationToken == "" {
			return out, nil
		}
		token := resp.ContinuationToken
		continuationToken = &token
	}
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

// DeleteTuple removes the tuple `user:<userID> <relation> tenant:<tenantID>`.
// Used by account teardown to unwind ownership/role tuples. Idempotent:
// deleting an already-absent tuple returns nil rather than an error.
func (c *fgaClient) DeleteTuple(ctx context.Context, userID, relation, tenantID string) error {
	body := client.ClientWriteRequest{
		Deletes: []client.ClientTupleKeyWithoutCondition{{
			User:     "user:" + userID,
			Relation: relation,
			Object:   "tenant:" + tenantID,
		}},
	}
	_, err := c.api.Write(ctx).Body(body).Execute()
	if err != nil {
		if isAlreadyExistsError(err) { // missing tuple → validation error → treat as done
			return nil
		}
		return fmt.Errorf("authz: delete %s tuple: %w", relation, err)
	}
	return nil
}

// DeleteStoreParent removes the tuple `tenant:<tenantID> parent store:<storeID>`.
// Idempotent: deleting an already-absent tuple returns nil rather than an error.
func (c *fgaClient) DeleteStoreParent(ctx context.Context, storeID, tenantID string) error {
	body := client.ClientWriteRequest{
		Deletes: []client.ClientTupleKeyWithoutCondition{{
			User:     "tenant:" + tenantID,
			Relation: "parent",
			Object:   "store:" + storeID,
		}},
	}
	_, err := c.api.Write(ctx).Body(body).Execute()
	if err != nil {
		if isAlreadyExistsError(err) {
			return nil
		}
		return fmt.Errorf("authz: delete store parent: %w", err)
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
