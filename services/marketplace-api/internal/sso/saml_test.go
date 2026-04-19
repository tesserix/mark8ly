package sso_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/sso"
)

// loadTestKeypair generates a fresh RSA 2048-bit keypair for use in tests.
// The returned tls.Certificate has Leaf populated so BuildSAMLServiceProvider
// does not need to re-parse the DER bytes.
func loadTestKeypair(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generate RSA key")

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-sp"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err, "create certificate")

	leaf, err := x509.ParseCertificate(derBytes)
	require.NoError(t, err, "parse leaf cert")

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  key,
		Leaf:        leaf,
	}
}

// testCertPEM returns a PEM-encoded self-signed certificate (used as IDP cert
// in tests).  Re-uses loadTestKeypair internally.
func testCertPEM(t *testing.T) string {
	t.Helper()
	kp := loadTestKeypair(t)
	block := &pem.Block{Type: "CERTIFICATE", Bytes: kp.Certificate[0]}
	return string(pem.EncodeToMemory(block))
}

func TestSAML_BuildSP_HappyPath(t *testing.T) {
	certPEM := testCertPEM(t)
	cfg := &sso.Config{
		Provider: sso.ProviderSAML,
		Metadata: map[string]any{
			sso.SAMLKeyIDPEntityID: "https://idp.example.com/entity",
			sso.SAMLKeyIDPACSURL:   "https://idp.example.com/sso",
			sso.SAMLKeyIDPCertPEM:  certPEM,
			sso.SAMLKeySPEntityID:  "https://api.mark8ly.com/sso/acme/metadata",
			sso.SAMLKeySPACSURL:    "https://api.mark8ly.com/sso/acme/callback",
		},
	}

	sp, err := sso.BuildSAMLServiceProvider(cfg, loadTestKeypair(t))
	require.NoError(t, err)
	require.Equal(t, "https://api.mark8ly.com/sso/acme/callback",
		sp.ServiceProvider.AcsURL.String())
}

func TestSAML_BuildSP_MissingIDPCert(t *testing.T) {
	cfg := &sso.Config{
		Provider: sso.ProviderSAML,
		Metadata: map[string]any{
			sso.SAMLKeyIDPEntityID: "https://idp.example.com/entity",
			sso.SAMLKeyIDPACSURL:   "https://idp.example.com/sso",
			// SAMLKeyIDPCertPEM intentionally omitted
			sso.SAMLKeySPEntityID: "https://api.mark8ly.com/sso/acme/metadata",
			sso.SAMLKeySPACSURL:   "https://api.mark8ly.com/sso/acme/callback",
		},
	}

	_, err := sso.BuildSAMLServiceProvider(cfg, loadTestKeypair(t))
	require.Error(t, err)
	// Must mention the missing key name or wrap ErrInvalidMetadata.
	require.ErrorIs(t, err, sso.ErrInvalidMetadata)
}

func TestSAML_BuildSP_MalformedCert(t *testing.T) {
	cfg := &sso.Config{
		Provider: sso.ProviderSAML,
		Metadata: map[string]any{
			sso.SAMLKeyIDPEntityID: "https://idp.example.com/entity",
			sso.SAMLKeyIDPACSURL:   "https://idp.example.com/sso",
			sso.SAMLKeyIDPCertPEM:  "not-valid-pem",
			sso.SAMLKeySPEntityID:  "https://api.mark8ly.com/sso/acme/metadata",
			sso.SAMLKeySPACSURL:    "https://api.mark8ly.com/sso/acme/callback",
		},
	}

	_, err := sso.BuildSAMLServiceProvider(cfg, loadTestKeypair(t))
	require.Error(t, err)
	require.ErrorIs(t, err, sso.ErrInvalidMetadata)
}

func TestSAML_BuildSP_MissingRequiredKeys(t *testing.T) {
	certPEM := testCertPEM(t)
	baseMetadata := map[string]any{
		sso.SAMLKeyIDPEntityID: "https://idp.example.com/entity",
		sso.SAMLKeyIDPACSURL:   "https://idp.example.com/sso",
		sso.SAMLKeyIDPCertPEM:  certPEM,
		sso.SAMLKeySPEntityID:  "https://api.mark8ly.com/sso/acme/metadata",
		sso.SAMLKeySPACSURL:    "https://api.mark8ly.com/sso/acme/callback",
	}

	requiredKeys := []string{
		sso.SAMLKeyIDPEntityID,
		sso.SAMLKeyIDPACSURL,
		sso.SAMLKeyIDPCertPEM,
		sso.SAMLKeySPEntityID,
		sso.SAMLKeySPACSURL,
	}

	for _, key := range requiredKeys {
		key := key // capture
		t.Run("missing_"+key, func(t *testing.T) {
			meta := make(map[string]any, len(baseMetadata))
			for k, v := range baseMetadata {
				meta[k] = v
			}
			delete(meta, key)

			cfg := &sso.Config{Provider: sso.ProviderSAML, Metadata: meta}
			_, err := sso.BuildSAMLServiceProvider(cfg, loadTestKeypair(t))
			require.Error(t, err)
			require.ErrorIs(t, err, sso.ErrInvalidMetadata)
		})
	}
}
