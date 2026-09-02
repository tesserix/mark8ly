package carriersecrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/bao"
)

// baoValueField is the single KV v2 data field every reference written by
// BaoClient stores its payload under. SecretClient's contract is a single
// opaque []byte per name; KV v2 secrets are maps, so we keep the shape
// trivial rather than inventing a multi-field schema nothing else needs.
const baoValueField = "value"

// BaoClient adapts *bao.Client (Task 2's authenticated OpenBao client) to
// the existing SecretClient interface, so a later ChainStore can hold an
// OpenBao backend and a GCPStore side by side without any backend-specific
// branching.
//
// name, on every method here, is always the logical KV path — e.g.
// "kv/mark8ly/marketplace-api/tenants/<id>/payment/razorpay/api_key" —
// produced by carriersecrets.BaoPath (write path) or
// carriersecrets.ParseBaoReference (read path). Both yield the identical
// string; BaoClient does not care which one a caller used.
type BaoClient struct {
	client *bao.Client
	mount  string
}

// NewBaoClient wraps an already-authenticated *bao.Client. The mount used to
// strip the logical-path prefix is derived from c.Mount(), not taken as a
// parameter: bao.Client re-adds its own configured mount when building the
// data/metadata URL, so accepting a separate mount argument here would let a
// caller pass one that disagrees with what c actually talks to — silently
// routing writes to the wrong mount. Deriving it from c makes that
// impossible to express.
func NewBaoClient(c *bao.Client) *BaoClient {
	return &BaoClient{client: c, mount: c.Mount()}
}

// relativePath strips this client's mount prefix from a logical KV path,
// returning the remainder bao.Client's ReadSecret/WriteSecret/DestroySecret
// expect (they add the mount back themselves via dataPath/metadataPath).
func (b *BaoClient) relativePath(name string) (string, error) {
	prefix := b.mount + "/"
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok || rest == "" {
		return "", fmt.Errorf("carriersecrets: bao path %q does not start with mount %q", name, b.mount)
	}
	return rest, nil
}

// CreateOrAddVersion writes payload to the KV v2 DATA path at name. KV v2
// has no separate "create" step: an unconditional write either creates the
// path (if it never existed) or appends a new version (if it did), which is
// exactly GCP's create-or-add semantics.
func (b *BaoClient) CreateOrAddVersion(ctx context.Context, name string, payload []byte) error {
	rel, err := b.relativePath(name)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(payload)
	if _, err := b.client.WriteSecret(ctx, rel, map[string]string{baoValueField: encoded}, 0); err != nil {
		return fmt.Errorf("carriersecrets: bao write %s: %w", name, err)
	}
	return nil
}

// AccessLatest reads the latest version from the KV v2 DATA path at name.
// A path that was never written maps to carriersecrets.ErrSecretNotFound —
// not bao.ErrNotFound and not a bare error — so callers can use the same
// sentinel regardless of which backend is behind the SecretClient.
func (b *BaoClient) AccessLatest(ctx context.Context, name string) ([]byte, error) {
	rel, err := b.relativePath(name)
	if err != nil {
		return nil, err
	}
	data, _, err := b.client.ReadSecret(ctx, rel)
	if err != nil {
		if errors.Is(err, bao.ErrNotFound) {
			return nil, ErrSecretNotFound
		}
		return nil, fmt.Errorf("carriersecrets: bao read %s: %w", name, err)
	}
	encoded, ok := data[baoValueField]
	if !ok {
		return nil, ErrSecretNotFound
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("carriersecrets: bao decode %s: %w", name, err)
	}
	return payload, nil
}

// DeleteSecret removes all versions of name via the KV v2 METADATA path
// (bao.Client.DestroySecret), not the DATA path. A DATA-path delete in KV v2
// is a soft, restorable delete — the credential would still be recoverable
// while this call appeared to succeed, which would not match GCP's
// irreversible DeleteSecret. Not-found is treated as success, matching
// GCP's idempotent DeleteSecret.
func (b *BaoClient) DeleteSecret(ctx context.Context, name string) error {
	rel, err := b.relativePath(name)
	if err != nil {
		return err
	}
	if err := b.client.DestroySecret(ctx, rel); err != nil {
		if errors.Is(err, bao.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("carriersecrets: bao delete %s: %w", name, err)
	}
	return nil
}

var _ SecretClient = (*BaoClient)(nil)
