// Package authz wraps OpenFGA's check-side API for auth-bff.
//
// auth-bff only ever READS from OpenFGA (it never writes tuples — that's
// platform-api's job via the outbox). The single operation we need is
// CheckMembership, used by the autologin service to verify a user is a
// member of a tenant before issuing a session cookie.
//
// This is the bug-fix landing point for auth-bug #2 (FGA tuple race) on
// the auth-bff side. The full fix has two halves:
//
//   1. platform-api writes the tenant + outbox row in one tx, then the
//      drainer ships the FGA tuple. (See platform-api's outbox + onboarding
//      packages.)
//
//   2. auth-bff's autologin does CheckMembership with retry-on-not-found
//      so the user can't get a session before the tuple is visible. The
//      retry waits up to ~2 seconds, which is plenty for the drainer to
//      catch up in normal operation.
//
// Together they close the race from both ends.
package authz

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openfga/go-sdk/client"
)

// Client is the read-side OpenFGA operations auth-bff needs.
type Client interface {
	CheckMembership(ctx context.Context, userID, tenantID string) (bool, error)
}

// fgaClient is the OpenFGA-backed implementation.
type fgaClient struct {
	api *client.OpenFgaClient
}

// Config holds the values needed to construct a real Client.
type Config struct {
	APIURL  string
	StoreID string
	ModelID string
}

// New constructs a real OpenFGA client wired to the platform store.
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

// CheckMembership returns true if the user is a member of the tenant.
func (c *fgaClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	body := client.ClientCheckRequest{
		User:     "user:" + userID,
		Relation: "member",
		Object:   "tenant:" + tenantID,
	}
	resp, err := c.api.Check(ctx).Body(body).Execute()
	if err != nil {
		return false, fmt.Errorf("authz: check membership: %w", err)
	}
	if resp == nil || resp.Allowed == nil {
		return false, nil
	}
	return *resp.Allowed, nil
}

// DiscoverStoreID looks up an OpenFGA store by display name and returns
// its ID. Mirrors platform-api's authz.DiscoverStoreID so auth-bff can
// self-bootstrap from a fresh OpenFGA without any hand-managed env vars.
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

// FGAStoreName is the canonical name of the platform store, matching
// infra/dev/seed/fga-init.sh.
const FGAStoreName = "mark8ly-platform"
