package carriersecrets

import (
	"context"
	"errors"

	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// Scope uniquely identifies a single carrier credential slot. A
// (tenant, domain, provider, field) tuple maps 1:1 to one GCP SM
// secret.
type Scope struct {
	// TenantID is the mark8ly tenant UUID. Required.
	TenantID string
	// Domain is the carrier family: "shipping" | "payment" | "tax".
	Domain string
	// Provider is the carrier identifier: "delhivery" | "razorpay" |
	// "shipengine" | "taxjar" | ... Lower-cased.
	Provider string
	// Field is the credential slot: "api_key" | "secret_key" | ...
	// Lower-cased.
	Field string
}

// Validate reports the first missing required component. Callers
// (settings upsert, shipments, storefront rates) check this before
// writing so malformed scopes never reach GCP SM where they would
// produce opaque "InvalidArgument" errors.
func (s Scope) Validate() error {
	switch {
	case s.TenantID == "":
		return errors.New("carriersecrets: scope.TenantID is required")
	case s.Domain == "":
		return errors.New("carriersecrets: scope.Domain is required")
	case s.Provider == "":
		return errors.New("carriersecrets: scope.Provider is required")
	case s.Field == "":
		return errors.New("carriersecrets: scope.Field is required")
	}
	return nil
}

// SecretClient is the minimal secret-backend surface a Store depends on.
// The only production implementation left is *BaoClient (see bao.go),
// wrapping OpenBao; tests wire in a FakeClient (in-memory, deterministic).
// GCP Secret Manager's implementation (*GCPStore) was retired in
// mark8ly#621 — see chain.go's explicit gsm:// error.
type SecretClient interface {
	// CreateOrAddVersion creates the secret (idempotent) and appends a
	// new version. name is the backend's canonical resource identifier
	// (a GCP resource path historically; an OpenBao KV path now).
	CreateOrAddVersion(ctx context.Context, name string, payload []byte) error
	// AccessLatest returns the latest version's payload, mapping
	// "doesn't exist" to ErrSecretNotFound so the Store can distinguish
	// "never existed" from "transient failure".
	AccessLatest(ctx context.Context, name string) ([]byte, error)
	// DeleteSecret removes the secret and all its versions. Idempotent:
	// "already gone" is treated as success.
	DeleteSecret(ctx context.Context, name string) error
}

// ErrSecretNotFound is the sentinel returned by AccessLatest when the
// secret (or its latest version) doesn't exist.
var ErrSecretNotFound = errors.New("carriersecrets: secret not found")

// Store is the public surface every handler depends on. Main wires up
// either a ChainStore (OpenBao, see chain.go) or an InlineStore
// (crypto.Encryptor only — dev without OpenBao creds) at boot time.
type Store interface {
	// Put writes plaintext under scope and returns an opaque reference
	// the caller persists to the DB. Safe to invoke on a fresh scope
	// (creates the underlying secret) or on an existing scope (adds
	// a new version and returns the same canonical reference).
	Put(ctx context.Context, scope Scope, plaintext string) (reference string, err error)

	// Get resolves reference back to plaintext. Accepts both canonical
	// "gsm://..." references and legacy inline "noop:/aes:" ciphertext
	// so existing rows keep working until the next save migrates
	// them. An empty reference returns an empty plaintext.
	Get(ctx context.Context, reference string) (plaintext string, err error)

	// Destroy deletes the underlying secret. No-op for inline
	// references (there is no detached resource to clean up).
	Destroy(ctx context.Context, reference string) error
}

// Rewrapper is an optional extension satisfied by ChainStore. When a
// handler's read path hits a legacy reference (inline, or a stray
// pre-cutover gsm://) it can call MaybeRewrap to migrate the value to the
// primary backend and persist the new reference in place of the old
// column value.
//
// The store deliberately does NOT own the DB row — that would require
// a passthrough of gorm.DB and turn this package into a DB writer
// instead of a secret resolver. Handlers UPDATE the row themselves.
type Rewrapper interface {
	// MaybeRewrap returns the new canonical reference when oldRef is a
	// legacy inline value AND a rewrap was performed. Otherwise it
	// returns ("", false) so the caller skips the DB UPDATE entirely.
	// The plaintext argument is the already-decoded value from Get —
	// passed in so we avoid decoding twice.
	MaybeRewrap(ctx context.Context, oldRef string, scope Scope, plaintext string) (newRef string, changed bool)
}

// ─────────────────────────────────────────────────────────────────────
// InlineStore — pure envelope-encryption, no external secret backend.
// ─────────────────────────────────────────────────────────────────────

// InlineStore is the fallback Store used when OpenBao is unavailable
// (local dev without credentials, CI integration tests, emergency
// boot after an IAM misconfiguration). It never talks to an external
// secret backend — every Put returns an inline ciphertext, and Get
// decodes the same.
type InlineStore struct {
	enc crypto.Encryptor
}

// NewInlineStore wraps an Encryptor. A nil Encryptor panics —
// callers must always supply one (NoopEncryptor in dev, AES in prod
// fallback).
func NewInlineStore(enc crypto.Encryptor) *InlineStore {
	if enc == nil {
		panic("carriersecrets: InlineStore requires an Encryptor")
	}
	return &InlineStore{enc: enc}
}

// Put encrypts the plaintext and returns the ciphertext as the
// reference. The caller persists that ciphertext directly.
func (s *InlineStore) Put(_ context.Context, scope Scope, plaintext string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	return s.enc.Encrypt(plaintext)
}

// Get decodes an inline ciphertext. Empty reference -> empty
// plaintext; unknown prefixes are returned as-is (legacy plaintext).
func (s *InlineStore) Get(_ context.Context, reference string) (string, error) {
	if reference == "" {
		return "", nil
	}
	if IsGSMRef(reference) {
		return "", errors.New("carriersecrets: InlineStore cannot resolve gsm:// references — wire ChainStore")
	}
	return s.enc.Decrypt(reference)
}

// Destroy is a no-op — the caller already removed the DB row that
// held the ciphertext.
func (s *InlineStore) Destroy(_ context.Context, _ string) error { return nil }
