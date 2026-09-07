package breakglass

import (
	"context"
	"fmt"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

// baoBreakGlassPrefix is the namespace-scoped root for every break-glass
// secret blob, mirroring carriersecrets' baoPathPrefix convention
// ("kv/mark8ly/marketplace-api/tenants/...", see internal/carriersecrets/refs.go).
// The leading "kv" segment is the OpenBao KV v2 mount name:
// carriersecrets.BaoClient.relativePath strips exactly `mount + "/"` off the
// front of whatever name it's given before building the underlying
// data/metadata path (internal/carriersecrets/bao.go:48), and this estate's
// mount is pinned to "kv" (bao.Client defaults to it, and
// pkg/config.ErrOpenBaoKVMountUnsupported enforces it for carrier secrets).
// A path that doesn't start with this prefix fails loudly in relativePath
// rather than silently landing somewhere unexpected.
const baoBreakGlassPrefix = "kv/mark8ly/marketplace-api/break-glass"

// BaoSecretPathFor returns the OpenBao logical KV path for a tenant's
// break-glass blob — the OpenBao analogue of SecretPathFor (the GCP Secret
// Manager path builder that shipped before mark8ly#621 retired GCP SM).
// Both simply produce an opaque `path` string that Bootstrapper stores
// verbatim on Account.SecretPath and Rotator replays unchanged
// (bootstrap.go:74, rotation.go:63) — neither cares about its shape, so
// swapping the builder is enough to move the backend.
func BaoSecretPathFor(tenantID string) string {
	return fmt.Sprintf("%s/%s", baoBreakGlassPrefix, tenantID)
}

// BaoSecretClient adapts carriersecrets.BaoClient (via the
// carriersecrets.SecretClient interface it already implements) to this
// package's SecretClient interface, so break-glass secrets live in
// OpenBao instead of GCP Secret Manager. GCP SM was retired service-wide in
// mark8ly#621 and this service account's IAM was revoked with it — see
// this same commit's deletion of gcp_secret_manager.go.
//
// Accepting the interface rather than the concrete *carriersecrets.BaoClient
// keeps this adapter testable against carriersecrets.FakeClient without a
// live OpenBao, while a production caller wires in a real *BaoClient, which
// satisfies the same interface (bao.go:131).
type BaoSecretClient struct {
	client carriersecrets.SecretClient
}

// NewBaoSecretClient wraps an already-authenticated carriersecrets
// SecretClient (normally a *carriersecrets.BaoClient).
func NewBaoSecretClient(c carriersecrets.SecretClient) *BaoSecretClient {
	return &BaoSecretClient{client: c}
}

// AddVersion writes payload to path via the underlying client's KV v2
// write. path must be mount-prefixed — see BaoSecretPathFor — otherwise
// carriersecrets.BaoClient.relativePath rejects it before any network call.
func (c *BaoSecretClient) AddVersion(ctx context.Context, path string, payload []byte) error {
	if err := c.client.CreateOrAddVersion(ctx, path, payload); err != nil {
		return fmt.Errorf("breakglass: bao add version at %s: %w", path, err)
	}
	return nil
}

// AccessLatest returns the latest version's payload at path.
func (c *BaoSecretClient) AccessLatest(ctx context.Context, path string) ([]byte, error) {
	payload, err := c.client.AccessLatest(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("breakglass: bao access latest at %s: %w", path, err)
	}
	return payload, nil
}

var _ SecretClient = (*BaoSecretClient)(nil)
