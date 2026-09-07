// Package idperr holds the provider-neutral sentinel errors platform-api
// classifies identity-provider failures into.
//
// These sentinels used to live in internal/gipadmin, which made every
// consumer — internal/zitadeladmin's error classifier and internal/auth's
// HTTP status mapping — import a GIP package to describe a Zitadel
// failure. They were relocated here when the GIP admin client was retired
// (#791) so the classification vocabulary outlives any single provider.
//
// This package is deliberately a leaf: it imports nothing beyond errors,
// so any provider client or handler can depend on it without creating a
// cycle. Add a sentinel here only when at least one provider client can
// produce it AND a caller changes behaviour on it — otherwise wrap with
// fmt.Errorf and leave the detail in the message.
package idperr

import "errors"

// Sentinel errors surfaced to callers. Providers map their own upstream
// error shapes onto these with %w so callers can use errors.Is without
// knowing which identity provider is configured.
var (
	// ErrUserNotFound means the account does not exist in the provider.
	// Callers that must avoid account enumeration treat this as success.
	ErrUserNotFound = errors.New("idp: user not found")
	// ErrInvalidOobCode means a password-reset code is unknown, already
	// redeemed, or expired.
	ErrInvalidOobCode = errors.New("idp: invalid or expired reset code")
	// ErrWeakPassword means the submitted password failed the provider's
	// complexity policy.
	ErrWeakPassword = errors.New("idp: password does not meet complexity rules")
	// ErrUnauthenticated means platform-api's own credentials for the
	// provider are missing or rejected — an operator problem, not a user one.
	ErrUnauthenticated = errors.New("idp: admin credentials unavailable")
	// ErrTooManyAttempts means the provider rate-limited the request.
	ErrTooManyAttempts = errors.New("idp: too many attempts, try again later")
	// ErrUnavailable means the provider was unreachable or answered
	// unintelligibly. Distinct from ErrUnauthenticated: retrying may work.
	ErrUnavailable = errors.New("idp: upstream unavailable")
)
