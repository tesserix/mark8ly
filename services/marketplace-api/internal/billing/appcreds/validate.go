package appcreds

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
)

// ErrInvalidP8 is returned when a .p8 payload is not a PEM-wrapped,
// PKCS#8-encoded ECDSA P-256 private key. Apple only accepts ES256-signed
// JWTs (NIST P-256), so any other key material is a guaranteed auth failure
// at run time — we catch it at upload time instead.
var ErrInvalidP8 = errors.New("appcreds: invalid .p8 (expected PEM PKCS#8 ECDSA P-256)")

// ErrInvalidGooglePlayJSON is returned when the Google Play payload is not
// a service-account JSON (e.g. user OAuth credentials, invalid JSON, or
// missing required fields).
var ErrInvalidGooglePlayJSON = errors.New("appcreds: invalid Google Play service-account JSON")

// ValidateP8 asserts the payload is a PEM-wrapped PKCS#8 ECDSA P-256
// private key. Returns a wrapped ErrInvalidP8 on failure. Does NOT persist
// anything — pure byte-level validation.
func ValidateP8(payload []byte) error {
	block, _ := pem.Decode(payload)
	if block == nil {
		return fmt.Errorf("%w: no PEM block", ErrInvalidP8)
	}
	// Apple's ".p8" is specifically PKCS#8. Reject anything labelled
	// "RSA PRIVATE KEY" / "EC PRIVATE KEY" etc.
	if block.Type != "PRIVATE KEY" {
		return fmt.Errorf("%w: PEM type %q; want PRIVATE KEY", ErrInvalidP8, block.Type)
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("%w: parse PKCS8: %v", ErrInvalidP8, err)
	}
	priv, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return fmt.Errorf("%w: key type %T; want *ecdsa.PrivateKey", ErrInvalidP8, parsed)
	}

	// Apple ASC uses ES256 (NIST P-256 / prime256v1). Other curves (P-384,
	// P-521) would parse but fail at signing time.
	if priv.Curve.Params().Name != "P-256" {
		return fmt.Errorf("%w: curve %q; want P-256", ErrInvalidP8, priv.Curve.Params().Name)
	}
	return nil
}

// serviceAccount is the subset of a Google service-account JSON that this
// package reads. Both ValidateGooglePlayJSON and GooglePlayProjectID go
// through parseServiceAccount so there is exactly one definition of what
// a well-formed payload is; a second inline json.Unmarshal elsewhere would
// be free to disagree with this one about what "valid" means.
type serviceAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
}

// parseServiceAccount decodes and validates a Google Play service-account
// JSON. Returns a wrapped ErrInvalidGooglePlayJSON on any failure.
func parseServiceAccount(payload []byte) (serviceAccount, error) {
	var sa serviceAccount
	if err := json.Unmarshal(payload, &sa); err != nil {
		return serviceAccount{}, fmt.Errorf("%w: parse json: %v", ErrInvalidGooglePlayJSON, err)
	}
	if sa.Type != "service_account" {
		return serviceAccount{}, fmt.Errorf("%w: type %q; want service_account", ErrInvalidGooglePlayJSON, sa.Type)
	}
	if sa.ProjectID == "" {
		return serviceAccount{}, fmt.Errorf("%w: missing project_id", ErrInvalidGooglePlayJSON)
	}
	if sa.PrivateKey == "" {
		return serviceAccount{}, fmt.Errorf("%w: missing private_key", ErrInvalidGooglePlayJSON)
	}
	if sa.ClientEmail == "" {
		return serviceAccount{}, fmt.Errorf("%w: missing client_email", ErrInvalidGooglePlayJSON)
	}
	return sa, nil
}

// ValidateGooglePlayJSON asserts the payload is a Google service-account
// JSON (not a user-authorized OAuth credential). Checks:
//   - valid JSON
//   - "type" == "service_account"
//   - required fields present: project_id, private_key, client_email
//
// Returns a wrapped ErrInvalidGooglePlayJSON on any failure.
func ValidateGooglePlayJSON(payload []byte) error {
	_, err := parseServiceAccount(payload)
	return err
}

// GooglePlayProjectID returns the "project_id" of a Google Play
// service-account JSON, running the same validation ValidateGooglePlayJSON
// does (same parser, same wrapped ErrInvalidGooglePlayJSON).
//
// WHAT THIS IDENTIFIER ACTUALLY IS: the GCP project that owns the service
// account, which is not by definition the merchant's Firebase project.
// Firebase projects ARE GCP projects and the Play publisher service
// account is conventionally created inside the same one, so for a
// Firebase-backed white-label app the two are the same string in
// practice — but nothing enforces that, and a merchant who created the
// publisher SA in a separate project will yield a project id that has no
// Firebase resources at all. A caller recording this as a Firebase
// project id (#702 teardown discovery) is recording a strong inference,
// not a verified fact.
func GooglePlayProjectID(payload []byte) (string, error) {
	sa, err := parseServiceAccount(payload)
	if err != nil {
		return "", err
	}
	return sa.ProjectID, nil
}
