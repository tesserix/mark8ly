// Package appcreds is the ONLY package in marketplace-api authorized to read
// or write Secret Manager secrets for the white-label mobile-app add-on
// (spec §18.9). A CI lint step in .github/workflows/ enforces that no other
// file imports cloud.google.com/go/secretmanager.
//
// The choke-point design guarantees that every credential read + write emits
// an audit event + increments a Prometheus counter. Callers pass a
// *Service, never the raw Secret Manager client.
package appcreds

import "fmt"

// CredType enumerates the logical credentials stored per merchant tenant.
// Each logical credential maps 1:1 to a distinct Secret Manager secret name
// so that IAM + audit trails are granular.
type CredType string

const (
	// CredTypeAppleP8 is the Apple App Store Connect API private key
	// (PKCS#8-encoded ECDSA P-256, PEM-wrapped — the ".p8" file from
	// appstoreconnect.apple.com).
	CredTypeAppleP8 CredType = "apple-asc-api-key"

	// CredTypeAppleIssuerID is the ASC API issuer UUID (per team).
	CredTypeAppleIssuerID CredType = "apple-asc-issuer-id"

	// CredTypeAppleKeyID is the ASC API key ID shown in appstoreconnect.
	CredTypeAppleKeyID CredType = "apple-asc-key-id"

	// CredTypeGooglePlayJSON is the Google Play Android Publisher
	// service-account JSON (not a user OAuth credential).
	CredTypeGooglePlayJSON CredType = "google-play-service-account"

	// CredTypeGooglePackageName is the merchant's Android applicationId
	// (e.g. "com.example.app").
	//
	// NOT A SECRET, and stored here anyway — following
	// CredTypeAppleIssuerID, which is likewise a plain identifier. The
	// reason is the choke-point in this package's doc comment: every read
	// and write emits an audit event and a Prometheus counter, and the
	// secret name embeds the tenant id, so tenant isolation is structural
	// rather than a WHERE clause someone can forget. A new column on some
	// other table would have neither property.
	//
	// It exists because Play, unlike App Store Connect, has no discovery
	// endpoint: the Android Publisher API is edit-scoped, so every call
	// needs the package name you are asking about and nothing can
	// enumerate them (tesserix/mark8ly#872).
	CredTypeGooglePackageName CredType = "google-play-package-name"
)

// AllCredTypes returns every CredType in a stable order. This is the
// iteration order used at §13.5 day-90 PurgeAll — adding a new cred type
// MUST extend this list.
func AllCredTypes() []CredType {
	return []CredType{
		CredTypeAppleP8,
		CredTypeAppleIssuerID,
		CredTypeAppleKeyID,
		CredTypeGooglePlayJSON,
		CredTypeGooglePackageName,
	}
}

// Path returns the fully-qualified Secret Manager secret name for the given
// tenant + cred type.
//
// Secret Manager disallows '/' in secret names, so the logical §18.9 path
//
//	/projects/{project}/secrets/merchant/{tenant_id}/{cred_type}
//
// is flattened to the physical secret name
//
//	projects/{project}/secrets/merchant_{tenant_id}_{cred_type}
//
// Tenant isolation is structural: tenantID is embedded in the secret name,
// so a request from tenant A can never accidentally address tenant B's
// secret — the two names are different strings.
func Path(projectID, tenantID string, t CredType) string {
	return fmt.Sprintf("projects/%s/secrets/merchant_%s_%s",
		projectID, tenantID, string(t))
}
