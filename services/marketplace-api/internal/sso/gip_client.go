// Package sso — GIP (Google Identity Platform) tenant-pool admin client.
//
// GIPClient manages SAML and OIDC provider configurations inside a GIP
// tenant pool (e.g. "mp-internal") on behalf of Mark8ly tenants.
//
// Provider IDs are derived deterministically from (tenantID, Provider) so
// that re-uploading a config is idempotent — GIP's upsert semantics are
// create-or-update anyway, but having a stable ID prevents orphaned
// providers accumulating in the pool.
//
// The production adapter (NewFirebaseGIPAdminClient) wraps
// firebase.google.com/go/v4/auth.TenantClient which is obtained from the
// Firebase Admin SDK via app.Auth(ctx).TenantManager.TenantClient(poolID).
// See firebase.google.com/go/v4/auth for the full TenantClient API.
package sso

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	firebaseAuth "firebase.google.com/go/v4/auth"
	"github.com/google/uuid"
)

// GIPProviderID returns a deterministic, GIP-compatible provider ID for the
// (tenantID, provider) pair.
//
// Format: "<provider>.mark8ly-<4-byte-hex>"
//
//   - The prefix ("saml." or "oidc.") is required by GIP to identify the
//     provider type.
//   - The 4-byte hex suffix is the first 4 bytes of SHA-256(tenantID bytes
//     || provider[0]).  This gives 2^32 (~4 billion) distinct values per
//     provider type — far more than needed for a single pool.
//
// The function is idempotent: identical inputs always produce identical output.
func GIPProviderID(tenantID uuid.UUID, provider Provider) string {
	h := sha256.New()
	h.Write(tenantID[:])
	h.Write([]byte{provider[0]})
	suffix := hex.EncodeToString(h.Sum(nil)[:4])
	return fmt.Sprintf("%s.mark8ly-%s", string(provider), suffix)
}

// SAMLProviderPayload carries the IDP-side attributes needed to register a
// SAML provider in GIP.  These come from the tenant's SSO Config after
// validation by Validate() / BuildSAMLServiceProvider().
type SAMLProviderPayload struct {
	IDPEntityID string
	IDPACSURL   string // IDP's SSO endpoint (SSOURL in GIP nomenclature)
	IDPCertPEM  string // Raw PEM — GIP accepts the full PEM string
	DisplayName string
}

// OIDCProviderPayload carries the OP-side attributes needed to register an
// OIDC provider in GIP.  ClientSecret is resolved from Secret Manager by the
// caller — it is never persisted in the SSO Config row.
type OIDCProviderPayload struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	DisplayName  string
}

// gipAdminClient is the internal interface that GIPClient delegates to.
// The production implementation is firebaseGIPAdminClient (below);
// tests use fakeGIPAdminClient.
type gipAdminClient interface {
	UpsertSAMLProvider(ctx context.Context, poolID, providerID string, payload SAMLProviderPayload) error
	UpsertOIDCProvider(ctx context.Context, poolID, providerID string, payload OIDCProviderPayload) error
	DeleteProvider(ctx context.Context, poolID, providerID string) error
}

// GIPClient manages provider configs inside a single GIP tenant pool.
type GIPClient struct {
	client gipAdminClient
	poolID string
}

// NewGIPClient returns a GIPClient backed by the given admin client and pool.
func NewGIPClient(client gipAdminClient, poolID string) *GIPClient {
	return &GIPClient{client: client, poolID: poolID}
}

// UploadSAMLProvider creates or updates the SAML provider config for the
// given tenant in the pool.  The providerID is derived deterministically so
// repeated calls are safe.
func (g *GIPClient) UploadSAMLProvider(ctx context.Context, tenantID uuid.UUID, payload SAMLProviderPayload) error {
	providerID := GIPProviderID(tenantID, ProviderSAML)
	if err := g.client.UpsertSAMLProvider(ctx, g.poolID, providerID, payload); err != nil {
		return fmt.Errorf("gip: upsert SAML provider %s: %w", providerID, err)
	}
	return nil
}

// UploadOIDCProvider creates or updates the OIDC provider config for the
// given tenant in the pool.
func (g *GIPClient) UploadOIDCProvider(ctx context.Context, tenantID uuid.UUID, payload OIDCProviderPayload) error {
	providerID := GIPProviderID(tenantID, ProviderOIDC)
	if err := g.client.UpsertOIDCProvider(ctx, g.poolID, providerID, payload); err != nil {
		return fmt.Errorf("gip: upsert OIDC provider %s: %w", providerID, err)
	}
	return nil
}

// Delete removes the provider config for the given (tenantID, provider) pair
// from the pool.  It is a no-op if the provider does not exist (GIP returns
// 404 which is surfaced as an error here — callers should decide whether to
// treat it as benign).
func (g *GIPClient) Delete(ctx context.Context, tenantID uuid.UUID, provider Provider) error {
	providerID := GIPProviderID(tenantID, provider)
	if err := g.client.DeleteProvider(ctx, g.poolID, providerID); err != nil {
		return fmt.Errorf("gip: delete provider %s: %w", providerID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Production Firebase adapter
// ---------------------------------------------------------------------------

// firebaseGIPAdminClient adapts firebase.google.com/go/v4/auth.TenantClient
// to the gipAdminClient interface.
//
// Firebase TenantClient method mapping:
//
//	UpsertSAMLProvider → try CreateSAMLProviderConfig; on "already exists"
//	                     fall back to UpdateSAMLProviderConfig.
//	UpsertOIDCProvider → try CreateOIDCProviderConfig; on "already exists"
//	                     fall back to UpdateOIDCProviderConfig.
//	DeleteProvider     → DeleteSAMLProviderConfig or DeleteOIDCProviderConfig
//	                     depending on the providerID prefix.
//
// Note: firebase.google.com/go/v4/auth does not expose a generic
// "upsert" method — we implement create-then-update ourselves.
// The poolID parameter is carried in the TenantClient instance itself
// (obtained via app.Auth(ctx).TenantManager.TenantClient(poolID)), so
// the pool-level routing is already baked in; we accept poolID here
// solely to match the interface signature.
//
// TODO: if firebase.google.com/go/v4/auth adds an explicit Upsert method,
// replace the create/update dance below.
type firebaseGIPAdminClient struct {
	tenantClient *firebaseAuth.TenantClient
}

// NewFirebaseGIPAdminClient wraps a Firebase TenantClient as a gipAdminClient.
// The TenantClient must already be scoped to the target pool (e.g.
// "mp-internal").  Obtain one via:
//
//	tc, err := firebaseApp.Auth(ctx)
//	pool, err := tc.TenantManager.TenantClient("mp-internal")
func NewFirebaseGIPAdminClient(tenantClient *firebaseAuth.TenantClient) gipAdminClient {
	return &firebaseGIPAdminClient{tenantClient: tenantClient}
}

// UpsertSAMLProvider creates the SAML provider config, or updates it if the
// provider ID already exists.  poolID is ignored — the TenantClient is
// already pool-scoped.
func (f *firebaseGIPAdminClient) UpsertSAMLProvider(ctx context.Context, _, providerID string, payload SAMLProviderPayload) error {
	toCreate := (&firebaseAuth.SAMLProviderConfigToCreate{}).
		ID(providerID).
		IDPEntityID(payload.IDPEntityID).
		SSOURL(payload.IDPACSURL).
		X509Certificates([]string{payload.IDPCertPEM}).
		DisplayName(payload.DisplayName).
		Enabled(true)

	_, err := f.tenantClient.CreateSAMLProviderConfig(ctx, toCreate)
	if err == nil {
		return nil
	}

	// GIP returns an error with code "PROVIDER_CONFIG_EXISTS" on duplicate.
	// Firebase SDK surfaces this as an error whose message contains "ALREADY_EXISTS"
	// or similar.  We attempt an update as the safe fallback.
	// TODO: use errors.Is / firebaseAuth.IsProviderConfigExists if the SDK
	// adds a typed sentinel for this condition.
	toUpdate := (&firebaseAuth.SAMLProviderConfigToUpdate{}).
		IDPEntityID(payload.IDPEntityID).
		SSOURL(payload.IDPACSURL).
		X509Certificates([]string{payload.IDPCertPEM}).
		DisplayName(payload.DisplayName).
		Enabled(true)

	_, updateErr := f.tenantClient.UpdateSAMLProviderConfig(ctx, providerID, toUpdate)
	if updateErr != nil {
		// Return the original creation error so callers see the root cause.
		return fmt.Errorf("gip firebase: create SAML %s: %w; update also failed: %v", providerID, err, updateErr)
	}
	return nil
}

// UpsertOIDCProvider creates the OIDC provider config, or updates it if the
// provider ID already exists.
func (f *firebaseGIPAdminClient) UpsertOIDCProvider(ctx context.Context, _, providerID string, payload OIDCProviderPayload) error {
	toCreate := (&firebaseAuth.OIDCProviderConfigToCreate{}).
		ID(providerID).
		Issuer(payload.Issuer).
		ClientID(payload.ClientID).
		ClientSecret(payload.ClientSecret).
		DisplayName(payload.DisplayName).
		Enabled(true)

	_, err := f.tenantClient.CreateOIDCProviderConfig(ctx, toCreate)
	if err == nil {
		return nil
	}

	// Fallback to update on duplicate (see SAML upsert comment above).
	toUpdate := (&firebaseAuth.OIDCProviderConfigToUpdate{}).
		Issuer(payload.Issuer).
		ClientID(payload.ClientID).
		ClientSecret(payload.ClientSecret).
		DisplayName(payload.DisplayName).
		Enabled(true)

	_, updateErr := f.tenantClient.UpdateOIDCProviderConfig(ctx, providerID, toUpdate)
	if updateErr != nil {
		return fmt.Errorf("gip firebase: create OIDC %s: %w; update also failed: %v", providerID, err, updateErr)
	}
	return nil
}

// DeleteProvider removes a provider config by its ID.  GIP provider IDs are
// prefixed with "saml." or "oidc." which determines which delete method to call.
//
// TODO: firebase.google.com/go/v4/auth does not expose a generic
// DeleteProviderConfig(ctx, id) — we dispatch based on the prefix.
func (f *firebaseGIPAdminClient) DeleteProvider(ctx context.Context, _, providerID string) error {
	switch {
	case len(providerID) > 5 && providerID[:5] == "saml.":
		return f.tenantClient.DeleteSAMLProviderConfig(ctx, providerID)
	case len(providerID) > 5 && providerID[:5] == "oidc.":
		return f.tenantClient.DeleteOIDCProviderConfig(ctx, providerID)
	default:
		return fmt.Errorf("gip firebase: cannot infer provider type from ID %q", providerID)
	}
}
