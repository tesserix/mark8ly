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

func TestValidate_OIDC_RequiresIssuerClientDiscovery(t *testing.T) {
	base := &Config{TenantID: uuid.New(), Provider: ProviderOIDC, Metadata: map[string]any{}}
	require.Error(t, Validate(base))

	base.Metadata = map[string]any{
		OIDCKeyIssuer:       "https://accounts.example.com",
		OIDCKeyClientID:     "client-abc",
		OIDCKeyDiscoveryURL: "https://accounts.example.com/.well-known/openid-configuration",
	}
	require.NoError(t, Validate(base))
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
