package appcreds

import (
	"context"
	"errors"
	"fmt"
	"sync"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SM is the narrow interface appcreds.Service depends on. Production wires
// in a *gcpSM wrapping cloud.google.com/go/secretmanager; tests wire in
// NewFakeSM (in-memory, deterministic).
//
// Methods are kept generic (name-addressed) so path construction stays in
// paths.go — this interface never sees project/tenant/credType separately.
type SM interface {
	// CreateOrAddVersion creates the secret if missing and appends a new
	// version with payload. Secret Manager semantics: versions are
	// monotonic; old versions become `DISABLED` but remain for audit.
	CreateOrAddVersion(ctx context.Context, name string, payload []byte) error

	// AccessLatest reads the most recent enabled version of the secret.
	// Returns ErrSMNotFound when the secret or its versions don't exist;
	// the Service layer maps that to ErrNotFound.
	AccessLatest(ctx context.Context, name string) ([]byte, error)

	// Delete removes the secret and all its versions. Idempotent:
	// returns nil if the secret didn't exist (caller can't distinguish
	// "already gone" from "just deleted" — see Service.Delete).
	Delete(ctx context.Context, name string) error
}

// ErrSMNotFound is the package-local sentinel returned by SM impls when
// a secret or version doesn't exist. Service.Load translates this to
// the exported ErrNotFound so callers depend only on this package.
var ErrSMNotFound = errors.New("appcreds/sm: not found")

// ───────────────────────────────────────────────────────────────────────
// Production adapter — wraps the Google SDK.
// ───────────────────────────────────────────────────────────────────────

type gcpSM struct {
	client    *secretmanager.Client
	projectID string
}

// NewGCPSM wraps an already-constructed *secretmanager.Client. The
// caller owns the client lifecycle — main.go builds it once at boot
// and defers Close().
//
// projectID is the GCP project that hosts all merchant_* secrets; it
// matches the projectID argument used in Path().
func NewGCPSM(c *secretmanager.Client, projectID string) SM {
	return &gcpSM{client: c, projectID: projectID}
}

func (g *gcpSM) CreateOrAddVersion(ctx context.Context, name string, payload []byte) error {
	// Secret Manager requires Create before AddVersion — idempotent via
	// AlreadyExists trap. Pattern matches internal/breakglass/gcp_secret_manager.go
	// but inlined here to keep appcreds the sole secretmanager importer (§18.9).
	secretID, parent, err := splitSecretName(name, g.projectID)
	if err != nil {
		return err
	}

	// Create secret — tolerate "already exists".
	_, err = g.client.CreateSecret(ctx, &smpb.CreateSecretRequest{
		Parent:   parent,
		SecretId: secretID,
		Secret: &smpb.Secret{
			Replication: &smpb.Replication{
				Replication: &smpb.Replication_Automatic_{
					Automatic: &smpb.Replication_Automatic{},
				},
			},
		},
	})
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return fmt.Errorf("appcreds: create secret %s: %w", name, err)
	}

	// Append new version.
	_, err = g.client.AddSecretVersion(ctx, &smpb.AddSecretVersionRequest{
		Parent:  name,
		Payload: &smpb.SecretPayload{Data: payload},
	})
	if err != nil {
		return fmt.Errorf("appcreds: add secret version %s: %w", name, err)
	}
	return nil
}

func (g *gcpSM) AccessLatest(ctx context.Context, name string) ([]byte, error) {
	versioned := name + "/versions/latest"
	resp, err := g.client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{Name: versioned})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrSMNotFound
		}
		return nil, fmt.Errorf("appcreds: access %s: %w", versioned, err)
	}
	return resp.Payload.Data, nil
}

func (g *gcpSM) Delete(ctx context.Context, name string) error {
	err := g.client.DeleteSecret(ctx, &smpb.DeleteSecretRequest{Name: name})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("appcreds: delete %s: %w", name, err)
	}
	return nil // NotFound is success (idempotent).
}

// splitSecretName parses "projects/{project}/secrets/{secretID}" back into
// ("{secretID}", "projects/{project}"). Returns error if the shape doesn't
// match — paranoia against callers passing raw secret IDs.
func splitSecretName(name, projectID string) (secretID, parent string, err error) {
	parent = "projects/" + projectID
	prefix := parent + "/secrets/"
	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return "", "", fmt.Errorf("appcreds: malformed secret name %q (want prefix %q)", name, prefix)
	}
	return name[len(prefix):], parent, nil
}

// ───────────────────────────────────────────────────────────────────────
// FakeSM — in-memory impl for unit tests. Thread-safe.
// ───────────────────────────────────────────────────────────────────────

// FakeSM is a deterministic in-memory SM impl for unit tests. Tests
// that need to assert "secret was deleted" can call Has() after the
// unit under test runs.
type FakeSM struct {
	mu   sync.Mutex
	data map[string][]byte
}

// NewFakeSM constructs an empty FakeSM ready for use in a test.
func NewFakeSM() *FakeSM {
	return &FakeSM{data: make(map[string][]byte)}
}

func (f *FakeSM) CreateOrAddVersion(ctx context.Context, name string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Copy the payload so the caller can mutate their slice without
	// affecting stored state.
	cp := make([]byte, len(payload))
	copy(cp, payload)
	f.data[name] = cp
	return nil
}

func (f *FakeSM) AccessLatest(ctx context.Context, name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[name]
	if !ok {
		return nil, ErrSMNotFound
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (f *FakeSM) Delete(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, name)
	return nil
}

// Has reports whether the fake currently holds a value at name. Exposed
// for assertion in tests.
func (f *FakeSM) Has(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[name]
	return ok
}
