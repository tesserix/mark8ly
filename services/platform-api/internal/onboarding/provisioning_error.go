package onboarding

import (
	"errors"
	"log"
	"strings"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// This file turns an OwnerProvisioner failure into (a) an HTTP answer the
// merchant can act on and (b) exactly one log line an operator can
// diagnose from. It is the onboarding twin of
// internal/invitation/provisioning_error.go, written after the same
// incident that one records: an 11-character password against a policy
// that requires 12 produced an opaque 500 and no log line at all, so the
// person who typed it retried the same password.
//
// It is a deliberate copy rather than a shared helper. The two paths
// share a classification but not a REMEDY — "open the invitation link
// again" is correct for an invitee and meaningless to a merchant whose
// wizard session is still open in front of them — and the message is the
// whole point of the file.

// passwordPolicyViolation is the shape a provisioner error implements
// when the identity provider rejected the chosen password for breaking a
// named complexity rule. *zitadeladmin.policyError satisfies it.
//
// Declared here as an interface rather than importing zitadeladmin: this
// package knows nothing about which identity provider is wired (projects,
// role keys and policy ids are all deployment concerns), and an import
// would make that false. errors.As against a one-method interface keeps
// the dependency pointing the right way.
//
// The returned message is provider-authored, static text naming the
// broken rule; it contains no password material.
type passwordPolicyViolation interface {
	error
	PasswordPolicyRule() (rule string, message string)
}

// passwordPolicyCode is the single machine-readable code every
// password-complexity rejection is returned under. One code, not five:
// the browser needs exactly one branch — "put this on the password
// field" — and the rule-specific detail belongs in the message, which is
// the part a human reads. It matches the code apps/admin's accept form
// already branches on, so both wizards behave identically.
const passwordPolicyCode = "password_policy"

// provisioningError maps a ProvisionStaff failure onto an AppError and
// logs the cause.
//
// A password-complexity rejection is 400, not 500: it is the caller's
// input that is wrong, and answering 500 tells both the merchant and
// every dashboard the wrong story about whose fault it is.
//
// password is passed in ONLY so redactPassword can guarantee it is not
// echoed back out of an error string. It is never itself logged.
func provisioningError(err error, sessionID, email, password string) error {
	var violation passwordPolicyViolation
	if errors.As(err, &violation) {
		rule, message := violation.PasswordPolicyRule()
		log.Printf("ERROR onboarding.Complete: identity provider rejected the chosen password: "+
			"session=%s email=%s rule=%s cause=%s",
			sessionID, email, rule, redactPassword(err.Error(), password))
		return apperrors.Wrap(err, 400, passwordPolicyCode, message)
	}

	log.Printf("ERROR onboarding.Complete: owner provisioning failed: session=%s email=%s cause=%s",
		sessionID, email, redactPassword(err.Error(), password))
	return apperrors.Wrap(err, 500, "provisioning_failed",
		"we couldn't finish creating your admin account — nothing was charged or created, please try again")
}

// redactPassword removes password from s.
//
// Belt and braces. No error this package receives is supposed to embed
// the credential — zitadeladmin's apiError formats only method, path,
// status, error id and sentinel, and never the request body — but the
// log line above is the first place in this flow that prints an upstream
// error verbatim, and "no current code path leaks it" is a property that
// silently stops holding the moment someone adds %v of a request body to
// an error string upstream.
//
// An empty password redacts nothing: strings.ReplaceAll with an empty
// old value splices the replacement between every character.
func redactPassword(s, password string) string {
	if password == "" {
		return s
	}
	return strings.ReplaceAll(s, password, "[redacted]")
}
