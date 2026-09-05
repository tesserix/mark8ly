package zitadeladmin

import (
	"errors"
	"fmt"
)

// This file classifies Zitadel's password-complexity rejections down to
// the individual RULE that was broken, so the merchant who typed the
// password is told what to change instead of "we couldn't finish setting
// up your account".
//
// The incident: an invitee chose an 11-character password. Zitadel's
// policy requires 12. EnsureHumanUser returned an opaque error, the
// accept handler answered 500 provisioning_failed, and nothing was
// logged — so the invitee retried the same password and the cause had to
// be found by replaying the Zitadel call by hand.
//
// # The live policy
//
// GET /management/v1/policies/password/complexity for org
// 386377229942128837, PROBED 2026-09-05:
//
//	minLength: 12, hasUppercase: true, hasLowercase: true,
//	hasNumber: true, hasSymbol: true
//
// That instance policy is the AUTHORITY; the constants below are a copy
// kept for message text and for the browser-side pre-check in
// apps/admin/lib/auth/password-policy.ts. Deliberately NOT fetched at
// request time: an extra Zitadel round trip on the accept path buys
// nothing — Zitadel still enforces the real policy, and this code's job
// is only to explain the rejection it already received. If the org
// policy is ever changed, update these two files together.
//
// # Ids are matched WHOLE, never by suffix
//
// COMMA-HuJf6 (too short, from POST /v2/users/human) and DOMAIN-HuJf6
// (too short, already in zitadelErrorIDSentinels for the reset path)
// share a suffix and are different errors from different Zitadel
// commands. Everything here keys off the FULL id, the same discipline
// this package's doc comment records for COMMAND-2M9fs vs COMMAND-G8dh3.
const (
	// PasswordMinLength mirrors the live policy's minLength.
	PasswordMinLength = 12
)

// The five password-complexity sentinels, one per rule the live policy
// enforces. A caller branches on these with errors.Is; each one is also
// reachable as gipadmin.ErrWeakPassword (see policyError.Unwrap) so the
// existing coarse checks in internal/auth keep working unchanged.
var (
	ErrPasswordTooShort    = errors.New("zitadeladmin: password is shorter than the policy minimum")
	ErrPasswordNoUppercase = errors.New("zitadeladmin: password has no uppercase letter")
	ErrPasswordNoLowercase = errors.New("zitadeladmin: password has no lowercase letter")
	ErrPasswordNoNumber    = errors.New("zitadeladmin: password has no number")
	ErrPasswordNoSymbol    = errors.New("zitadeladmin: password has no symbol")
)

// PasswordPolicyRuleCode is the machine-readable name of a broken rule.
// It travels into logs so a support question ("why did this invite
// fail?") is answerable from one line.
type PasswordPolicyRuleCode = string

const (
	RuleTooShort    PasswordPolicyRuleCode = "too_short"
	RuleNoUppercase PasswordPolicyRuleCode = "no_uppercase"
	RuleNoLowercase PasswordPolicyRuleCode = "no_lowercase"
	RuleNoNumber    PasswordPolicyRuleCode = "no_number"
	RuleNoSymbol    PasswordPolicyRuleCode = "no_symbol"
)

// passwordPolicyRule is one row of the id table.
type passwordPolicyRule struct {
	rule     PasswordPolicyRuleCode
	sentinel error
	// message is shown to the person who typed the password. It names
	// the rule that was actually broken AND restates the full policy,
	// because fixing one rule commonly reveals the next.
	message string
}

// passwordRequirementsSuffix is appended to every rule message so the
// invitee can satisfy the whole policy in one more attempt rather than
// discovering it a rule at a time.
const passwordRequirementsSuffix = " Passwords need at least 12 characters and must include an uppercase letter, a lowercase letter, a number, and a symbol."

// zitadelPasswordPolicyIDs maps Zitadel's stable details[0].id from
// POST /v2/users/human to the rule it means. Every id here was PROBED
// live against the TESSERIX instance on 2026-09-05 by submitting a
// password that broke exactly one rule.
//
// Keys are FULL ids. Do not shorten them to suffixes and do not add a
// prefix-insensitive lookup: DOMAIN-HuJf6 is a different error that ends
// in the same five characters as COMMA-HuJf6.
var zitadelPasswordPolicyIDs = map[string]passwordPolicyRule{
	"COMMA-HuJf6": {RuleTooShort, ErrPasswordTooShort,
		fmt.Sprintf("That password is too short — it needs at least %d characters.", PasswordMinLength) + passwordRequirementsSuffix},
	"COMMA-VoaRj": {RuleNoUppercase, ErrPasswordNoUppercase,
		"That password needs an uppercase letter." + passwordRequirementsSuffix},
	"COMMA-co3Xw": {RuleNoLowercase, ErrPasswordNoLowercase,
		"That password needs a lowercase letter." + passwordRequirementsSuffix},
	"COMMA-ZBv4H": {RuleNoNumber, ErrPasswordNoNumber,
		"That password needs a number." + passwordRequirementsSuffix},
	"COMMA-ZDLwA": {RuleNoSymbol, ErrPasswordNoSymbol,
		"That password needs a symbol, for example ! ? @ or #." + passwordRequirementsSuffix},
}

// policyError is the error EnsureHumanUser returns when Zitadel rejected
// the supplied password for breaking one named complexity rule.
//
// It carries NO password material — only the Zitadel error id, the rule
// name, and static message text — so it is safe to log whole. That is
// deliberate: the reason this classification exists is that the original
// failure logged nothing, and the fix must not swap one hazard (silence)
// for another (a credential in Cloud Logging).
//
// Unwrap returns BOTH the transport error (whose own chain ends at
// gipadmin.ErrWeakPassword, see zitadelErrorIDSentinels) and the
// rule-specific sentinel, so errors.Is matches either granularity.
type policyError struct {
	errorID string
	rule    passwordPolicyRule
	cause   error
}

func (e *policyError) Error() string {
	return fmt.Sprintf("zitadeladmin: password policy violation %s (%s): %v", e.rule.rule, e.errorID, e.cause)
}

func (e *policyError) Unwrap() []error { return []error{e.cause, e.rule.sentinel} }

// PasswordPolicyRule reports the broken rule and the message to show the
// person who typed the password.
//
// The method — rather than an exported struct — is what lets callers
// outside this package detect the condition WITHOUT importing
// zitadeladmin: internal/invitation declares a one-method interface of
// this shape and errors.As against it, keeping the invitation package
// free of any identity-provider import (it only ever holds a
// StaffProvisioner interface today, and that stays true).
func (e *policyError) PasswordPolicyRule() (PasswordPolicyRuleCode, string) {
	return e.rule.rule, e.rule.message
}

// asPasswordPolicyError wraps err with its rule classification when err
// is a Zitadel password-complexity rejection, and returns err untouched
// otherwise. Keyed strictly on the whole error id.
func asPasswordPolicyError(err error) error {
	if err == nil {
		return nil
	}
	id := errorID(err)
	if id == "" {
		return err
	}
	rule, ok := zitadelPasswordPolicyIDs[id]
	if !ok {
		return err
	}
	return &policyError{errorID: id, rule: rule, cause: err}
}
