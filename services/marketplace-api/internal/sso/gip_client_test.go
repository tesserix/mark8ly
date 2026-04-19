package sso_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/sso"
)

// ---------------------------------------------------------------------------
// fakeGIPAdminClient — test double with call counters
// ---------------------------------------------------------------------------

// fakeGIPAdminClient is a test-only implementation of the unexported
// gipAdminClient interface.  It is accessed via NewGIPClientFromFake.
type fakeGIPAdminClient struct {
	UpsertSAMLCallCount int
	UpsertOIDCCallCount int
	DeleteCallCount     int

	LastPoolID     string
	LastProviderID string
	LastSAMLPayload sso.SAMLProviderPayload
	LastOIDCPayload sso.OIDCProviderPayload

	// ForceError, if non-nil, is returned by every method.
	ForceError error
}

func (f *fakeGIPAdminClient) UpsertSAMLProvider(_ context.Context, poolID, providerID string, payload sso.SAMLProviderPayload) error {
	if f.ForceError != nil {
		return f.ForceError
	}
	f.UpsertSAMLCallCount++
	f.LastPoolID = poolID
	f.LastProviderID = providerID
	f.LastSAMLPayload = payload
	return nil
}

func (f *fakeGIPAdminClient) UpsertOIDCProvider(_ context.Context, poolID, providerID string, payload sso.OIDCProviderPayload) error {
	if f.ForceError != nil {
		return f.ForceError
	}
	f.UpsertOIDCCallCount++
	f.LastPoolID = poolID
	f.LastProviderID = providerID
	f.LastOIDCPayload = payload
	return nil
}

func (f *fakeGIPAdminClient) DeleteProvider(_ context.Context, poolID, providerID string) error {
	if f.ForceError != nil {
		return f.ForceError
	}
	f.DeleteCallCount++
	f.LastPoolID = poolID
	f.LastProviderID = providerID
	return nil
}

// NewGIPClientFromFake exposes the internal constructor for tests so that
// fakeGIPAdminClient (also in this _test package) can be injected.
// It delegates to sso.NewGIPClient which accepts the unexported interface via
// a package-level test-accessor alias.
//
// Because gipAdminClient is unexported, we expose this via a dedicated
// exported test function in the sso package (see gip_client_testexport_test.go
// built only during `go test`).  For this file we define a local helper that
// works through the exported NewGIPClient constructor with a compatible shim.
//
// NOTE: Go allows _test packages to satisfy unexported interfaces when the
// interface methods are all exported — which is the case here.  The
// fakeGIPAdminClient satisfies gipAdminClient because all three methods match.
// We pass it through sso.NewGIPClientFromFake (defined below in the sso package
// as a test-only exported wrapper).
func newTestGIPClient(fake *fakeGIPAdminClient, poolID string) *sso.GIPClient {
	return sso.NewGIPClientFromFake(fake, poolID)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGIPProviderID_Deterministic(t *testing.T) {
	tenantID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	id1 := sso.GIPProviderID(tenantID, sso.ProviderSAML)
	id2 := sso.GIPProviderID(tenantID, sso.ProviderSAML)

	require.Equal(t, id1, id2, "same inputs must produce identical provider ID")
	require.True(t, strings.HasPrefix(id1, "saml.mark8ly-"),
		"SAML provider ID must start with saml.mark8ly-; got %q", id1)
}

func TestGIPProviderID_DifferentProviders(t *testing.T) {
	tenantID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	samlID := sso.GIPProviderID(tenantID, sso.ProviderSAML)
	oidcID := sso.GIPProviderID(tenantID, sso.ProviderOIDC)

	require.NotEqual(t, samlID, oidcID, "SAML and OIDC IDs must differ for the same tenant")
	require.True(t, strings.HasPrefix(oidcID, "oidc.mark8ly-"),
		"OIDC provider ID must start with oidc.mark8ly-; got %q", oidcID)
}

func TestGIPProviderID_DifferentTenants(t *testing.T) {
	t1 := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	t2 := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	id1 := sso.GIPProviderID(t1, sso.ProviderSAML)
	id2 := sso.GIPProviderID(t2, sso.ProviderSAML)

	require.NotEqual(t, id1, id2, "different tenants must produce different provider IDs")
}

func TestGIPClient_UploadSAML_CallsFakeWithCorrectArgs(t *testing.T) {
	fake := &fakeGIPAdminClient{}
	client := newTestGIPClient(fake, "mp-internal")

	tenantID := uuid.New()
	payload := sso.SAMLProviderPayload{
		IDPEntityID: "https://idp.example.com/entity",
		IDPACSURL:   "https://idp.example.com/sso",
		IDPCertPEM:  "-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----",
		DisplayName: "Acme SAML",
	}

	err := client.UploadSAMLProvider(context.Background(), tenantID, payload)
	require.NoError(t, err)
	require.Equal(t, 1, fake.UpsertSAMLCallCount)
	require.Equal(t, "mp-internal", fake.LastPoolID)
	require.True(t, strings.HasPrefix(fake.LastProviderID, "saml.mark8ly-"),
		"expected saml.mark8ly- prefix; got %q", fake.LastProviderID)
	require.Equal(t, payload, fake.LastSAMLPayload)
}

func TestGIPClient_UploadOIDC_CallsFakeWithCorrectArgs(t *testing.T) {
	fake := &fakeGIPAdminClient{}
	client := newTestGIPClient(fake, "mp-internal")

	tenantID := uuid.New()
	payload := sso.OIDCProviderPayload{
		Issuer:       "https://accounts.google.com",
		ClientID:     "my-client",
		ClientSecret: "s3cr3t",
		DisplayName:  "Google OIDC",
	}

	err := client.UploadOIDCProvider(context.Background(), tenantID, payload)
	require.NoError(t, err)
	require.Equal(t, 1, fake.UpsertOIDCCallCount)
	require.Equal(t, "mp-internal", fake.LastPoolID)
	require.True(t, strings.HasPrefix(fake.LastProviderID, "oidc.mark8ly-"),
		"expected oidc.mark8ly- prefix; got %q", fake.LastProviderID)
	require.Equal(t, payload, fake.LastOIDCPayload)
}

func TestGIPClient_Delete_CallsFakeWithCorrectProviderID(t *testing.T) {
	fake := &fakeGIPAdminClient{}
	client := newTestGIPClient(fake, "mp-internal")

	tenantID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	expectedID := sso.GIPProviderID(tenantID, sso.ProviderSAML)

	err := client.Delete(context.Background(), tenantID, sso.ProviderSAML)
	require.NoError(t, err)
	require.Equal(t, 1, fake.DeleteCallCount)
	require.Equal(t, expectedID, fake.LastProviderID)
}

func TestGIPClient_PropagatesError(t *testing.T) {
	fake := &fakeGIPAdminClient{ForceError: sso.ErrInvalidProvider}
	client := newTestGIPClient(fake, "mp-internal")

	err := client.UploadSAMLProvider(context.Background(), uuid.New(), sso.SAMLProviderPayload{})
	require.Error(t, err)
}
