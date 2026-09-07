// signature.go, nonce.go and middleware.go moved to the standalone
// github.com/mark8ly/platformauth module (#720) so platform-api can share
// mark8ly's request-signing scheme instead of reimplementing it. audit.go
// stays here — it imports internal/audit, which platformauth must not
// depend on.
//
// The names below are thin aliases onto the new module rather than a
// rewrite of every call site: this package alone had ~90 references to
// them across handlers, routes wiring and tests, none of which needed to
// change behaviour, only which package a name resolves to. A type alias
// (`type X = platformauth.X`) is the same type as platformauth.X, so a
// caller passing platformadmin.SignatureInput to platformauth code (or
// vice versa) needs no conversion.
package platformadmin

import "github.com/mark8ly/platformauth"

// Signature scheme: header names, canonical form, HMAC.
type SignatureInput = platformauth.SignatureInput

var (
	CanonicalQuery  = platformauth.CanonicalQuery
	CanonicalString = platformauth.CanonicalString
	Sign            = platformauth.Sign
	Verify          = platformauth.Verify
)

const (
	HeaderOperator   = platformauth.HeaderOperator
	HeaderCapability = platformauth.HeaderCapability
	HeaderTimestamp  = platformauth.HeaderTimestamp
	HeaderNonce      = platformauth.HeaderNonce
	HeaderSignature  = platformauth.HeaderSignature
)

// Replay defence.
type (
	Nonce      = platformauth.Nonce
	NonceStore = platformauth.NonceStore
)

var NewNonceStore = platformauth.NewNonceStore

// Auth middleware, context keys, and the capability enforcement matrix.
// audit.buildEntry and every handler in this package read CtxOperatorID /
// CtxCapability unqualified — see this package's doc comment on audit.go.
type AuthConfig = platformauth.AuthConfig

var RequirePlatformAuth = platformauth.RequirePlatformAuth

const (
	CtxOperatorID = platformauth.CtxOperatorID
	CtxCapability = platformauth.CtxCapability

	MountPrefix = platformauth.MountPrefix

	CapabilityValueChecked = platformauth.CapabilityValueChecked
)

var CapabilityKey = platformauth.CapabilityKey

// RequiredWriteCapabilities and RequiredReadCapabilities are maps —
// assigning them here binds this name to the SAME underlying map
// platformauth holds, not a copy, so there remains exactly one production
// matrix regardless of which package name a route declaration is read
// through.
var (
	RequiredWriteCapabilities = platformauth.RequiredWriteCapabilities
	RequiredReadCapabilities  = platformauth.RequiredReadCapabilities
)
