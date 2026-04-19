package breakglass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Blob is the decrypted payload stored in GCP Secret Manager under
// /projects/tesserix-prod/secrets/break-glass-{tenant_id}. The TOTP
// secret NEVER leaves this blob; only `bcrypt(Password)` lands in the
// DB row.
type Blob struct {
	Password    string    `json:"password"`
	TOTPSecret  string    `json:"totp_secret"`
	GeneratedAt time.Time `json:"generated_at"`
}

// SecretClient is the minimal Secret Manager surface this package
// needs. Production implementations wrap
// cloud.google.com/go/secretmanager/apiv1.Client; tests swap in a
// FakeSecretClient.
type SecretClient interface {
	// AddVersion writes a new version of the secret at `path`. Payload
	// is opaque bytes from this package's perspective — always a
	// JSON-encoded Blob.
	AddVersion(ctx context.Context, path string, payload []byte) error
	// AccessLatest returns the current payload at `path`.
	AccessLatest(ctx context.Context, path string) ([]byte, error)
}

// SecretManager wraps SecretClient with Blob-typed read/write helpers.
type SecretManager struct {
	client SecretClient
}

// NewSecretManager constructs a SecretManager around a SecretClient.
func NewSecretManager(c SecretClient) *SecretManager { return &SecretManager{client: c} }

// Upsert JSON-marshals blob and writes it as a new version at path.
// Callers (Bootstrapper + Rotator) MUST call this before touching DB
// state so a partial failure never leaves a password_hash on disk
// that doesn't match the blob.
func (s *SecretManager) Upsert(ctx context.Context, path string, b Blob) error {
	if b.GeneratedAt.IsZero() {
		b.GeneratedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("%w: marshal: %v", ErrSecretManagerFailed, err)
	}
	if err := s.client.AddVersion(ctx, path, payload); err != nil {
		return fmt.Errorf("%w: add version: %v", ErrSecretManagerFailed, err)
	}
	return nil
}

// Fetch reads the current blob at path. Returns ErrSecretManagerFailed
// wrapping any underlying error — the login handler surfaces this as
// a 500, NOT a 401, so ops sees the real cause.
func (s *SecretManager) Fetch(ctx context.Context, path string) (*Blob, error) {
	payload, err := s.client.AccessLatest(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("%w: access: %v", ErrSecretManagerFailed, err)
	}
	var b Blob
	if err := json.Unmarshal(payload, &b); err != nil {
		return nil, fmt.Errorf("%w: unmarshal: %v", ErrSecretManagerFailed, err)
	}
	if b.Password == "" || b.TOTPSecret == "" {
		return nil, errors.New("breakglass: secret blob is missing password or totp_secret")
	}
	return &b, nil
}

// SecretPathFor returns the canonical Secret Manager resource path
// for a tenant's break-glass blob under `projectID`.
func SecretPathFor(projectID, tenantID string) string {
	return fmt.Sprintf("/projects/%s/secrets/break-glass-%s", projectID, tenantID)
}

// FakeSecretClient is an in-memory SecretClient for unit tests.
// Versions beyond the latest are discarded — the real GCP API keeps
// history, but this package only ever reads `latest`, so replaying
// that surface is enough.
type FakeSecretClient struct {
	data map[string][]byte
}

// NewFakeSecretClient returns an empty FakeSecretClient.
func NewFakeSecretClient() *FakeSecretClient {
	return &FakeSecretClient{data: map[string][]byte{}}
}

// AddVersion stores payload at path (overwriting any prior value).
func (f *FakeSecretClient) AddVersion(_ context.Context, path string, payload []byte) error {
	if f.data == nil {
		f.data = map[string][]byte{}
	}
	// Copy so callers can reuse the slice.
	cp := make([]byte, len(payload))
	copy(cp, payload)
	f.data[path] = cp
	return nil
}

// AccessLatest returns the payload at path.
func (f *FakeSecretClient) AccessLatest(_ context.Context, path string) ([]byte, error) {
	v, ok := f.data[path]
	if !ok {
		return nil, fmt.Errorf("fake secret client: no version at %q", path)
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}
