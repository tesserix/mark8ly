package carriersecrets

import (
	"context"
	"errors"
	"fmt"

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

// Store is the public surface every handler depends on. Main wires up
// either a HybridStore (GCP SM + inline fallback) or an InlineStore
// (crypto.Encryptor only — dev without GCP creds) at boot time.
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

// Rewrapper is an optional extension satisfied by HybridStore. When a
// handler's read path hits a legacy inline reference while running in
// gcpsm mode, it can call MaybeRewrap to migrate the value to GCP SM
// and persist the new reference in place of the old column value.
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
// HybridStore — GCP Secret Manager primary, inline fallback for reads.
// ─────────────────────────────────────────────────────────────────────

// HybridStore is NO LONGER the production Store on the wired path — main.go
// now wires ChainStore (see chain.go) for every SHIPPING_SECRET_STORE value
// including "gcpsm", since ChainStore is a strict superset (GCP-primary
// ChainStore behaves identically to HybridStore, plus bao:// routing for
// the rollback case — see pkg/config's OpenBao-required-in-gcpsm-mode
// validation). HybridStore is retained here DELIBERATELY for a later
// phase rather than deleted outright: it is a smaller, independently
// understandable reference implementation of the primary+read-fallback
// pattern ChainStore generalises, and removing it is not required by any
// task in this migration. Do not wire it into main.go going forward —
// ChainStore supersedes it.
//
// It writes through to GCP Secret Manager and reads from either GCP SM or
// inline ciphertext so existing rows stay readable during migration.
type HybridStore struct {
	client    SecretClient
	enc       crypto.Encryptor
	projectID string
	prefix    string
}

// HybridConfig bundles the knobs for constructing a HybridStore.
type HybridConfig struct {
	// Client is the GCP SM adapter (usually *GCPStore backed by the
	// Google SDK, or *FakeClient in tests).
	Client SecretClient
	// Encryptor decodes legacy inline values on read. Nil disables
	// inline-compat reads — use that only in tests that want to
	// assert references are GCP SM references.
	Encryptor crypto.Encryptor
	// ProjectID is the GCP project hosting the secrets. Required.
	ProjectID string
	// Prefix goes at the start of every secret ID — the cluster
	// identifier, e.g. "mark8ly-prod" or "mark8ly-test".
	Prefix string
}

// NewHybridStore constructs a HybridStore. Panics on missing
// required fields; callers must pass a usable config at boot.
func NewHybridStore(cfg HybridConfig) *HybridStore {
	if cfg.Client == nil {
		panic("carriersecrets: HybridStore requires a SecretClient")
	}
	if cfg.ProjectID == "" {
		panic("carriersecrets: HybridStore requires ProjectID")
	}
	if cfg.Prefix == "" {
		panic("carriersecrets: HybridStore requires Prefix")
	}
	return &HybridStore{
		client:    cfg.Client,
		enc:       cfg.Encryptor,
		projectID: cfg.ProjectID,
		prefix:    cfg.Prefix,
	}
}

// Put creates the secret if missing and appends a new version.
func (h *HybridStore) Put(ctx context.Context, scope Scope, plaintext string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	resource := SecretResource(h.projectID, h.prefix, scope)
	if err := h.client.CreateOrAddVersion(ctx, resource, []byte(plaintext)); err != nil {
		return "", fmt.Errorf("carriersecrets: put %s: %w", resource, err)
	}
	return GSMRefPrefix + resource, nil
}

// Get resolves a reference. Routes by prefix: GSM references go to
// the SDK; inline references go to the encryptor.
func (h *HybridStore) Get(ctx context.Context, reference string) (string, error) {
	if reference == "" {
		return "", nil
	}
	if resource, ok := ParseReference(reference); ok {
		data, err := h.client.AccessLatest(ctx, resource)
		if err != nil {
			return "", fmt.Errorf("carriersecrets: get %s: %w", resource, err)
		}
		return string(data), nil
	}
	if IsInlineRef(reference) {
		if h.enc == nil {
			return "", errors.New("carriersecrets: inline reference received but no encryptor wired")
		}
		plain, err := h.enc.Decrypt(reference)
		if err != nil {
			return "", fmt.Errorf("carriersecrets: decrypt inline: %w", err)
		}
		return plain, nil
	}
	// Unknown shape — treat as plaintext legacy value (pre-encryption
	// rows). NoopEncryptor.Decrypt has the same pass-through
	// behaviour, so this keeps dev setups working.
	return reference, nil
}

// Destroy deletes the GCP SM secret. No-op for inline references
// since there is no detached resource to clean up.
func (h *HybridStore) Destroy(ctx context.Context, reference string) error {
	if reference == "" {
		return nil
	}
	resource, ok := ParseReference(reference)
	if !ok {
		return nil
	}
	if err := h.client.DeleteSecret(ctx, resource); err != nil {
		return fmt.Errorf("carriersecrets: destroy %s: %w", resource, err)
	}
	return nil
}

// MaybeRewrap upgrades a legacy inline reference to GCP SM lazily.
// Returns ("", false) when oldRef is already a gsm:// reference or
// empty — the caller must skip the DB UPDATE in that case. A rewrap
// failure is swallowed (returns "", false) so a transient SM blip
// never fails the surrounding read path; the next read will try
// again.
func (h *HybridStore) MaybeRewrap(ctx context.Context, oldRef string, scope Scope, plaintext string) (string, bool) {
	if oldRef == "" || IsGSMRef(oldRef) {
		return "", false
	}
	if err := scope.Validate(); err != nil {
		return "", false
	}
	newRef, err := h.Put(ctx, scope, plaintext)
	if err != nil {
		return "", false
	}
	return newRef, true
}

// ─────────────────────────────────────────────────────────────────────
// InlineStore — pure envelope-encryption, no GCP dependency.
// ─────────────────────────────────────────────────────────────────────

// InlineStore is the fallback Store used when GCP SM is unavailable
// (local dev without credentials, CI integration tests, emergency
// boot after an IAM misconfiguration). It never talks to GCP — every
// Put returns an inline ciphertext, and Get decodes the same.
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
		return "", errors.New("carriersecrets: InlineStore cannot resolve gsm:// references — wire HybridStore")
	}
	return s.enc.Decrypt(reference)
}

// Destroy is a no-op — the caller already removed the DB row that
// held the ciphertext.
func (s *InlineStore) Destroy(_ context.Context, _ string) error { return nil }
