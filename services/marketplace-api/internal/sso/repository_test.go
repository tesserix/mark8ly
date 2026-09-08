package sso

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// These tests exercise the pure-Go validation layer. Tenant-isolation
// integration tests against a real Postgres live under an //go:build
// integration file (added separately as integration infra lands).

func TestValidate_RejectsNilConfig(t *testing.T) {
	require.ErrorIs(t, Validate(nil), ErrInvalidMetadata)
}

func TestValidate_RejectsZeroTenant(t *testing.T) {
	err := Validate(&Config{
		TenantID: uuid.Nil,
		Provider: ProviderSAML,
		Metadata: map[string]any{SAMLKeyIDPEntityID: "x", SAMLKeyIDPACSURL: "x", SAMLKeyIDPCertPEM: "x"},
	})
	require.ErrorIs(t, err, ErrInvalidMetadata)
}

func TestValidate_RejectsUnknownProvider(t *testing.T) {
	err := Validate(&Config{
		TenantID: uuid.New(),
		Provider: Provider("magic"),
		Metadata: map[string]any{},
	})
	require.ErrorIs(t, err, ErrInvalidProvider)
}

func TestValidate_SAML_RequiresCoreKeys(t *testing.T) {
	base := &Config{TenantID: uuid.New(), Provider: ProviderSAML, Metadata: map[string]any{}}
	require.Error(t, Validate(base))

	base.Metadata = map[string]any{
		SAMLKeyIDPEntityID: "https://idp.example.com/entity",
		SAMLKeyIDPACSURL:   "https://idp.example.com/sso",
		// missing cert
	}
	require.Error(t, Validate(base))

	base.Metadata[SAMLKeyIDPCertPEM] = "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----"
	require.NoError(t, Validate(base))
}

// Renamed and corrected by mark8ly#820. It used to require discovery_url,
// which nothing reads — BuildOIDCRelyingParty discovers from the issuer — and
// did not require client_secret_ref, without which no relying party can be
// built. A config could therefore save cleanly and then fail every login.
func TestValidate_OIDC_RequiresWhatALoginActuallyNeeds(t *testing.T) {
	base := &Config{TenantID: uuid.New(), Provider: ProviderOIDC, Metadata: map[string]any{}}
	require.Error(t, Validate(base))

	base.Metadata = map[string]any{
		OIDCKeyIssuer:          "https://accounts.example.com",
		OIDCKeyClientID:        "client-abc",
		OIDCKeyClientSecretRef: "kv/test/idp",
	}
	require.NoError(t, Validate(base), "the three fields a login needs were rejected")
}

// The field the old rule demanded is now optional — accepted and stored for a
// caller that wants to record it, never required for a config that works
// without it.
func TestValidate_OIDC_DiscoveryURLIsOptional(t *testing.T) {
	cfg := &Config{TenantID: uuid.New(), Provider: ProviderOIDC, Metadata: map[string]any{
		OIDCKeyIssuer:          "https://accounts.example.com",
		OIDCKeyClientID:        "client-abc",
		OIDCKeyClientSecretRef: "kv/test/idp",
		OIDCKeyDiscoveryURL:    "https://accounts.example.com/.well-known/openid-configuration",
	}}
	require.NoError(t, Validate(cfg))
}

// The specific regression: a config missing the client secret reference must
// NOT validate. It saved cleanly before, and then no one could sign in.
func TestValidate_OIDC_RefusesAConfigThatCannotBuildARelyingParty(t *testing.T) {
	cfg := &Config{TenantID: uuid.New(), Provider: ProviderOIDC, Metadata: map[string]any{
		OIDCKeyIssuer:       "https://accounts.example.com",
		OIDCKeyClientID:     "client-abc",
		OIDCKeyDiscoveryURL: "https://accounts.example.com/.well-known/openid-configuration",
	}}
	require.Error(t, Validate(cfg), "a config with no client_secret_ref validated; every login would fail")
}

func TestValidate_OIDC_EmptyStringsFail(t *testing.T) {
	cfg := &Config{
		TenantID: uuid.New(),
		Provider: ProviderOIDC,
		Metadata: map[string]any{
			OIDCKeyIssuer:       "",
			OIDCKeyClientID:     "client-abc",
			OIDCKeyDiscoveryURL: "https://x/.well-known/openid-configuration",
		},
	}
	err := Validate(cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidMetadata))
}

func TestProvider_Valid(t *testing.T) {
	require.True(t, ProviderSAML.Valid())
	require.True(t, ProviderOIDC.Valid())
	require.False(t, Provider("").Valid())
	require.False(t, Provider("kerberos").Valid())
}
