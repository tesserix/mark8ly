// Package sso — SAML 2.0 SP wiring via github.com/crewjam/saml.
//
// BuildSAMLServiceProvider constructs a crewjam/samlsp.Middleware from the
// per-tenant Config.  The shared Mark8ly SP keypair (loaded by the caller
// from Secret Manager at startup) is passed in as a tls.Certificate so that
// plaintext private keys never flow through this library.
package sso

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
)

// BuildSAMLServiceProvider constructs a crewjam samlsp.Middleware from the
// per-tenant SSO Config and the shared SP TLS keypair.
//
// The following metadata keys must be present in cfg.Metadata:
//   - SAMLKeyIDPEntityID — IDP entity ID URI
//   - SAMLKeyIDPACSURL   — IDP SSO endpoint (HTTP-POST binding location)
//   - SAMLKeyIDPCertPEM  — IDP signing certificate in PEM format
//   - SAMLKeySPEntityID  — SP entity ID URI (typically .../sso/<slug>/metadata)
//   - SAMLKeySPACSURL    — SP ACS URL (typically .../sso/<slug>/callback)
//
// kp is the SP's TLS certificate.  The private key must be *rsa.PrivateKey —
// the SAML library only supports RSA.
func BuildSAMLServiceProvider(cfg *Config, kp tls.Certificate) (*samlsp.Middleware, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: cfg is nil", ErrInvalidMetadata)
	}

	idpEntityID, ok := nonEmpty(cfg.Metadata, SAMLKeyIDPEntityID)
	if !ok {
		return nil, fmt.Errorf("%w: %s required", ErrInvalidMetadata, SAMLKeyIDPEntityID)
	}

	idpACSURL, ok := nonEmpty(cfg.Metadata, SAMLKeyIDPACSURL)
	if !ok {
		return nil, fmt.Errorf("%w: %s required", ErrInvalidMetadata, SAMLKeyIDPACSURL)
	}

	idpCertPEM, ok := nonEmpty(cfg.Metadata, SAMLKeyIDPCertPEM)
	if !ok {
		return nil, fmt.Errorf("%w: %s required", ErrInvalidMetadata, SAMLKeyIDPCertPEM)
	}

	spEntityID, ok := nonEmpty(cfg.Metadata, SAMLKeySPEntityID)
	if !ok {
		return nil, fmt.Errorf("%w: %s required", ErrInvalidMetadata, SAMLKeySPEntityID)
	}

	spACSURL, ok := nonEmpty(cfg.Metadata, SAMLKeySPACSURL)
	if !ok {
		return nil, fmt.Errorf("%w: %s required", ErrInvalidMetadata, SAMLKeySPACSURL)
	}

	// Parse IDP certificate PEM — must be a CERTIFICATE block.
	certDER, err := parseCertPEM(idpCertPEM)
	if err != nil {
		return nil, fmt.Errorf("%w: idp_cert_pem: %v", ErrInvalidMetadata, err)
	}

	// Base64-encode the raw DER for the SAML X509Certificate Data field.
	certB64 := base64.StdEncoding.EncodeToString(certDER)

	// Build an inline EntityDescriptor that represents the IDP.
	idpMeta := &saml.EntityDescriptor{
		EntityID: idpEntityID,
		IDPSSODescriptors: []saml.IDPSSODescriptor{
			{
				SSODescriptor: saml.SSODescriptor{
					RoleDescriptor: saml.RoleDescriptor{
						ProtocolSupportEnumeration: "urn:oasis:names:tc:SAML:2.0:protocol",
						KeyDescriptors: []saml.KeyDescriptor{
							{
								Use: "signing",
								KeyInfo: saml.KeyInfo{
									X509Data: saml.X509Data{
										X509Certificates: []saml.X509Certificate{
											{Data: certB64},
										},
									},
								},
							},
						},
					},
				},
				SingleSignOnServices: []saml.Endpoint{
					{
						Binding:  saml.HTTPPostBinding,
						Location: idpACSURL,
					},
				},
			},
		},
	}

	// Validate the SP ACS URL so we fail fast on bad config.
	acsURL, err := url.Parse(spACSURL)
	if err != nil {
		return nil, fmt.Errorf("%w: sp_acs_url invalid: %v", ErrInvalidMetadata, err)
	}

	// Derive the SP base URL from the ACS URL (strip the path).
	spBase := &url.URL{
		Scheme: acsURL.Scheme,
		Host:   acsURL.Host,
	}

	// Extract the RSA private key from the leaf certificate.
	rsaKey, ok := kp.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: SP keypair must use an RSA private key", ErrInvalidMetadata)
	}

	leafCert, err := spLeafCert(kp)
	if err != nil {
		return nil, err
	}

	mw, err := samlsp.New(samlsp.Options{
		EntityID:          spEntityID,
		URL:               *spBase,
		Key:               rsaKey,
		Certificate:       leafCert,
		IDPMetadata:       idpMeta,
		AllowIDPInitiated: false,
	})
	if err != nil {
		return nil, fmt.Errorf("samlsp.New: %w", err)
	}

	// Override the auto-derived ACS URL with the explicit one from config
	// so per-tenant callback paths are honoured.
	mw.ServiceProvider.AcsURL = *acsURL

	return mw, nil
}

// spLeafCert returns the parsed x509 leaf certificate from a tls.Certificate.
// tls.LoadX509KeyPair does not populate Leaf automatically; we parse it
// on-demand from the raw DER bytes when it is nil.
func spLeafCert(kp tls.Certificate) (*x509.Certificate, error) {
	if kp.Leaf != nil {
		return kp.Leaf, nil
	}
	if len(kp.Certificate) == 0 {
		return nil, fmt.Errorf("%w: SP keypair has no certificate", ErrInvalidMetadata)
	}
	cert, err := x509.ParseCertificate(kp.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("%w: SP certificate parse error: %v", ErrInvalidMetadata, err)
	}
	return cert, nil
}

// parseCertPEM decodes the first CERTIFICATE PEM block and returns the raw
// DER bytes.  Returns an error if the input contains no valid block.
func parseCertPEM(pemStr string) ([]byte, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("expected CERTIFICATE block, got %q", block.Type)
	}
	return block.Bytes, nil
}
