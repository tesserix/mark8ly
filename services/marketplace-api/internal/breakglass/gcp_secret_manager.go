package breakglass

import (
	"context"
	"fmt"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// GCPSecretClient is the production SecretClient backed by
// cloud.google.com/go/secretmanager. Kept in this package so
// FakeSecretClient stays the only test dependency callers need to
// know about.
type GCPSecretClient struct {
	Client *secretmanager.Client
}

// NewGCPSecretClient wraps an already-constructed GCP client. The
// caller owns lifecycle (usually main.go builds it once and defers
// client.Close()).
func NewGCPSecretClient(c *secretmanager.Client) *GCPSecretClient {
	return &GCPSecretClient{Client: c}
}

// AddVersion creates a new SecretVersion at path. `path` is the
// secret resource (`projects/{project}/secrets/{name}`); if the
// caller passed our canonical "/projects/…" form we strip the
// leading slash before handing it to Google.
func (g *GCPSecretClient) AddVersion(ctx context.Context, path string, payload []byte) error {
	parent := normalizeSecretPath(path)
	_, err := g.Client.AddSecretVersion(ctx, &smpb.AddSecretVersionRequest{
		Parent:  parent,
		Payload: &smpb.SecretPayload{Data: payload},
	})
	if err != nil {
		return fmt.Errorf("secretmanager: add version at %s: %w", parent, err)
	}
	return nil
}

// AccessLatest returns the latest-enabled version payload at path.
func (g *GCPSecretClient) AccessLatest(ctx context.Context, path string) ([]byte, error) {
	name := normalizeSecretPath(path) + "/versions/latest"
	resp, err := g.Client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{Name: name})
	if err != nil {
		return nil, fmt.Errorf("secretmanager: access %s: %w", name, err)
	}
	return resp.Payload.Data, nil
}

// normalizeSecretPath accepts both canonical Mark8ly form
// (`/projects/{p}/secrets/{s}`) and the raw GCP form. Google's API
// rejects the leading slash.
func normalizeSecretPath(p string) string {
	return strings.TrimPrefix(p, "/")
}
