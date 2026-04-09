// Package authz is marketplace-api's read-only OpenFGA client for tenant-
// scoped permission checks. Per spec §13.1.1, marketplace-api NEVER writes
// tuples — all writes happen in platform-api during onboarding and
// invitation accept. This package exposes only Check / CheckMembership /
// GetRole. The Write* methods that platform-api's authz package exposes
// are intentionally absent so accidental tuple writes from marketplace-api
// are a compile error.
//
// The middleware that consumes this Client lives in the same package
// (middleware.go). Tests use the FakeClient in fake.go to drive the
// middleware without a real OpenFGA instance.
package authz

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openfga/go-sdk/client"
)

// FGAStoreName is the canonical OpenFGA store name marketplace-api reads
// from. The same store is written to by platform-api — there is no
// separate "marketplace" store in slice 1 (spec §13.1.1).
const FGAStoreName = "mark8ly-platform"

// Role names match the relations defined in the OpenFGA model.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleStaff  Role = "staff"
	RoleViewer Role = "viewer"
)

var rolePriority = map[Role]int{
	RoleOwner:  4,
	RoleAdmin:  3,
	RoleStaff:  2,
	RoleViewer: 1,
}

// HigherOrEqual reports whether r outranks (or equals) other in the role
// priority order owner > admin > staff > viewer.
func (r Role) HigherOrEqual(other Role) bool {
	return rolePriority[r] >= rolePriority[other]
}

// allRoles is iterated by GetRole. Stable order so the highest match
// wins on the first hit.
var allRoles = []Role{RoleOwner, RoleAdmin, RoleStaff, RoleViewer}

// Client is the read-only operations marketplace-api needs from OpenFGA.
type Client interface {
	// Check is the generic permission check against a tenant:<id>
	// object. Relation can be any role or derived relation defined on
	// the tenant type in the FGA model.
	Check(ctx context.Context, userID, relation, tenantID string) (bool, error)

	// CheckMembership is a convenience wrapper for the derived
	// `member` relation — true iff the user holds any role on the
	// tenant.
	CheckMembership(ctx context.Context, userID, tenantID string) (bool, error)

	// GetRole returns the highest direct role the user holds on the
	// tenant, or "" if they have no role. Iterates the four roles in
	// priority order; worst case 4 Check calls.
	GetRole(ctx context.Context, userID, tenantID string) (Role, error)
}

// Config holds the values needed to construct a real OpenFGA client.
// StoreID is obtained at startup via DiscoverStoreID.
type Config struct {
	APIURL  string // e.g. http://openfga:8080
	StoreID string // ulid; from DiscoverStoreID
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
// its ID. Returns ("", nil) if no store with that name exists; callers
// fail-fast on empty.
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
	for _, s := range resp.GetStores() {
		if s.Name == name {
			return s.Id, nil
		}
	}
	return "", nil
}

type fgaClient struct {
	api *client.OpenFgaClient
}

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

func (c *fgaClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	return c.Check(ctx, userID, "member", tenantID)
}

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
