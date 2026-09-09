package appcreds

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
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

// ErrInvalidGooglePackageName is returned when a supplied Android
// applicationId is not a legal package name.
var ErrInvalidGooglePackageName = errors.New("appcreds: invalid Google Play package name")

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

// googlePackageNameRe is Android's applicationId grammar: two or more
// dot-separated segments, each a legal Java identifier — a leading letter
// followed by letters, digits or underscores.
//
// ANCHORED AT BOTH ENDS, and `\A`/`\z` rather than `^`/`$`, because Go's
// `$` matches before a trailing newline. A package name pasted out of a
// build file arrives as "com.example.app\n" more often than not, and `^...$`
// would accept it — storing a name the Android Publisher API then rejects.
var googlePackageNameRe = regexp.MustCompile(`\A[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+\z`)

// maxGooglePackageNameLen matches white_label_app_state.google_package
// (varchar(255), migration 000076). Validating shorter than the column
// would reject legal names; validating longer would accept one that
// truncates on write, and a truncated package is a package Play does not
// know — the advancer would call it nightly and fail.
const maxGooglePackageNameLen = 255

// ValidateGooglePackageName checks a merchant-supplied Android
// applicationId.
//
// # Why this is validated at all, when it is only an identifier
//
// It is the one white-label credential with no self-describing format: a
// .p8 is parsed as a key and a service-account JSON is parsed as JSON, so
// both fail loudly on garbage. A package name is just a string, and a
// wrong one is INDISTINGUISHABLE FROM A RIGHT ONE until it reaches Play.
//
// That matters because of where it is used. `lifecycle/advancer.go` guards
// both Google calls on `r.GooglePackage != ""` — a typo is non-empty, so it
// passes the guard, reaches `edits.insert` inside the nightly cron, and
// surfaces only as a skipped teardown step for one merchant, months later,
// at the moment the teardown was supposed to happen. Refusing it at the
// upload boundary is the only place a human is present to fix it.
//
// It deliberately does NOT verify the package exists in Play. That would
// need an API call with the merchant's credential at upload time, and a
// merchant may legitimately supply the name before the app is published.
func ValidateGooglePackageName(name string) error {
	if len(name) > maxGooglePackageNameLen {
		return fmt.Errorf("%w: %d bytes exceeds the %d-byte column",
			ErrInvalidGooglePackageName, len(name), maxGooglePackageNameLen)
	}
	if !googlePackageNameRe.MatchString(name) {
		// The name is NOT echoed into the error. It reaches an HTTP response
		// and a log line, and while a package name is not a secret it is
		// attacker-controlled input on an authenticated endpoint — the same
		// reason the sibling validators return a fixed sentinel.
		return fmt.Errorf("%w: expected two or more dot-separated segments, "+
			"each starting with a letter (e.g. com.example.app)", ErrInvalidGooglePackageName)
	}
	return nil
}
