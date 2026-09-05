package invitation

import (
	"errors"
	"log"
	"strings"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// This file turns a StaffProvisioner failure into (a) an HTTP answer the
// invitee can act on and (b) exactly one log line an operator can
// diagnose from.
//
// Both halves are a fix for the same production incident: a merchant
// accepted a staff invitation with an 11-character password, Zitadel's
// policy requires 12, and the accept endpoint answered 500
// provisioning_failed with the copy "we couldn't finish setting up your
// account — please try the invitation link again". That message is a
// lie in this case (nothing about retrying the LINK helps) and the
// server logged nothing at all, so the real cause had to be found by
// replaying platform-api's own Zitadel call by hand.

// passwordPolicyViolation is the shape a provisioner error implements
// when the identity provider rejected the chosen password for breaking a
// named complexity rule. *zitadeladmin.policyError satisfies it.
//
// It is declared HERE, as an interface, rather than importing
// zitadeladmin: this package deliberately knows nothing about which
// identity provider is wired (see StaffProvisioner's doc — projects,
// role keys and now policy ids are all deployment concerns), and an
// import would make that false. errors.As against a one-method
// interface keeps the dependency pointing the right way.
//
// The returned message is provider-authored, static text naming the
// broken rule; it contains no password material.
type passwordPolicyViolation interface {
	error
	PasswordPolicyRule() (rule string, message string)
}

// passwordPolicyCode is the single machine-readable code every
// password-complexity rejection is returned under.
//
// One code, not five: the browser needs exactly one branch — "put this
// on the password field" — and the rule-specific detail belongs in the
// message, which is the part a human reads. The rule name is not lost,
// it goes to the log line below where an operator can aggregate on it.
const passwordPolicyCode = "password_policy"

// provisioningError maps a ProvisionStaff failure onto an AppError and
// logs the cause.
//
// A password-complexity rejection is 400, not 500: it is the caller's
// input that is wrong, and answering 500 tells both the invitee and
// every dashboard the wrong story about whose fault it is.
//
// password is passed in ONLY so redactPassword can guarantee it is not
// echoed back out of an error string. It is never itself logged; see
// that function's doc.
func provisioningError(err error, invitationID, tenantID, password string) error {
	var violation passwordPolicyViolation
	if errors.As(err, &violation) {
		rule, message := violation.PasswordPolicyRule()
		log.Printf("ERROR invitation.Accept: identity provider rejected the chosen password: "+
			"invitation=%s tenant=%s rule=%s cause=%s",
			invitationID, tenantID, rule, redactPassword(err.Error(), password))
		return apperrors.Wrap(err, 400, passwordPolicyCode, message)
	}

	log.Printf("ERROR invitation.Accept: staff provisioning failed: invitation=%s tenant=%s cause=%s",
		invitationID, tenantID, redactPassword(err.Error(), password))
	return apperrors.Wrap(err, 500, "provisioning_failed",
		"we couldn't finish setting up your account — please try the invitation link again")
}

// redactPassword removes password from s.
//
// Belt and braces. No error this package receives is supposed to embed
// the credential — zitadeladmin's apiError formats only method, path,
// status, error id and sentinel, and never the request body — but the
// log line above is the first place in this flow that prints an upstream
// error verbatim, and "no current code path leaks it" is a property that
// silently stops holding the moment someone adds %v of a request body to
// an error string upstream. The check costs one string scan per failed
// accept.
//
// An empty password redacts nothing: strings.ReplaceAll with an empty
// old value splices the replacement between every character.
func redactPassword(s, password string) string {
	if password == "" {
		return s
	}
	return strings.ReplaceAll(s, password, "[redacted]")
}
