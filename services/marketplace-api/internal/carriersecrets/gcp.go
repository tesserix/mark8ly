package carriersecrets

import (
	"context"
	"errors"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SecretClient is the minimal GCP Secret Manager surface the Store
// depends on. Production implementations wrap
// cloud.google.com/go/secretmanager/apiv1; tests wire in a
// FakeClient (in-memory, deterministic).
type SecretClient interface {
	// CreateOrAddVersion creates the secret (idempotent on
	// AlreadyExists) and appends a new enabled version. name is the
	// canonical resource path: projects/{project}/secrets/{id}.
	CreateOrAddVersion(ctx context.Context, name string, payload []byte) error
	// AccessLatest returns the latest enabled version payload. Maps
	// the NotFound gRPC code to ErrSecretNotFound so the Store can
	// distinguish "never existed" from "transient failure".
	AccessLatest(ctx context.Context, name string) ([]byte, error)
	// DeleteSecret removes the secret and all its versions.
	// Idempotent: NotFound is treated as success.
	DeleteSecret(ctx context.Context, name string) error
}

// ErrSecretNotFound is the sentinel returned by AccessLatest when
// the secret (or its latest version) doesn't exist.
var ErrSecretNotFound = errors.New("carriersecrets: secret not found")

// ─────────────────────────────────────────────────────────────────────
// GCPStore — wraps *secretmanager.Client.
// ─────────────────────────────────────────────────────────────────────

// GCPStore is the production SecretClient backed by the Google SDK.
type GCPStore struct {
	client    *secretmanager.Client
	projectID string
}

// NewGCPStore wraps an already-constructed *secretmanager.Client.
// main.go owns the client lifecycle and defers Close().
func NewGCPStore(c *secretmanager.Client, projectID string) *GCPStore {
	return &GCPStore{client: c, projectID: projectID}
}

// CreateOrAddVersion creates the secret if missing and appends a new
// version. Secret Manager returns AlreadyExists on the create leg
// when the secret was provisioned on a previous call — we tolerate
// that so Put is idempotent.
func (g *GCPStore) CreateOrAddVersion(ctx context.Context, name string, payload []byte) error {
	secretID, parent, err := splitSecretName(name, g.projectID)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("carriersecrets: create secret %s: %w", name, err)
	}
	_, err = g.client.AddSecretVersion(ctx, &smpb.AddSecretVersionRequest{
		Parent:  name,
		Payload: &smpb.SecretPayload{Data: payload},
	})
	if err != nil {
		return fmt.Errorf("carriersecrets: add version %s: %w", name, err)
	}
	return nil
}

// AccessLatest fetches /versions/latest under name.
func (g *GCPStore) AccessLatest(ctx context.Context, name string) ([]byte, error) {
	versioned := name + "/versions/latest"
	resp, err := g.client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{Name: versioned})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrSecretNotFound
		}
		return nil, fmt.Errorf("carriersecrets: access %s: %w", versioned, err)
	}
	return resp.Payload.Data, nil
}

// DeleteSecret removes the secret. NotFound is success (idempotent).
func (g *GCPStore) DeleteSecret(ctx context.Context, name string) error {
	err := g.client.DeleteSecret(ctx, &smpb.DeleteSecretRequest{Name: name})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("carriersecrets: delete %s: %w", name, err)
	}
	return nil
}

// splitSecretName parses "projects/{project}/secrets/{secretID}"
// back into ("{secretID}", "projects/{project}"). Returns an error
// for malformed input so callers can surface a 500 rather than an
// opaque gRPC InvalidArgument.
func splitSecretName(name, projectID string) (secretID, parent string, err error) {
	parent = "projects/" + projectID
	prefix := parent + "/secrets/"
	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return "", "", fmt.Errorf("carriersecrets: malformed secret name %q (want prefix %q)", name, prefix)
	}
	return name[len(prefix):], parent, nil
}
