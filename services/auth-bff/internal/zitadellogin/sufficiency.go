package zitadellogin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
)

// Outcome is what to do with a login attempt.
type Outcome int

const (
	// OutcomeHandoff is the zero value ON PURPOSE. Any Result built without a
	// decision lands on "do not complete this login". The opposite default
	// would cost an MFA bypass; that asymmetry decides which value is zero.
	OutcomeHandoff Outcome = iota
	OutcomeComplete
	OutcomeFactorRequired
)

type Result struct {
	Outcome     Outcome
	CallbackURL string
	Factors     []string
}

// sufficient is proof that a session was evaluated by one of this package's
// two classification paths and found adequate to finalize.
//
// It makes "call finalize with no decision at all" a compile error. It does
// NOT stop someone constructing sufficient{} beside a new check-free function
// — Go permits that inside the package — which is why an archtest pins both
// the finalize call site and sufficient{} construction to this file.
type sufficient struct{}

const (
	methodPassword = "AUTHENTICATION_METHOD_TYPE_PASSWORD" // not a second factor
	methodTOTP     = "AUTHENTICATION_METHOD_TYPE_TOTP"     // the one we can collect
)

// finalize exchanges a session for an authorization code. Unexported and
// requiring a sufficient witness, so it is unreachable without a decision.
func (c *Client) finalize(ctx context.Context, authRequestID string, s Session, _ sufficient) (string, error) {
	body := map[string]any{
		"session": map[string]any{"sessionId": s.ID, "sessionToken": s.Token},
	}
	var wire struct {
		CallbackURL string `json:"callbackUrl"`
	}
	err := c.do(ctx, http.MethodPost, "/v2/oidc/auth_requests/"+url.PathEscape(authRequestID), body, &wire, ErrAuthRequestInvalid)
	if err != nil {
		return "", err
	}
	if wire.CallbackURL == "" {
		return "", fmt.Errorf("zitadellogin: finalize returned no callbackUrl: %w", ErrUnavailable)
	}
	return wire.CallbackURL, nil
}

// classifyEnrolledMethods reports whether the user has TOTP enrolled and
// whether they have any factor this login page cannot collect.
//
// Everything that is not PASSWORD or TOTP is uncollectible — an include list,
// so a factor type we have never seen fails closed into a handoff rather than
// being silently skipped.
func (c *Client) classifyEnrolledMethods(ctx context.Context, userID string) (totpEnrolled, uncollectible bool, err error) {
	types, err := c.EnrolledMethodTypes(ctx, userID)
	if err != nil {
		return false, false, err
	}
	for _, t := range types {
		switch t {
		case methodPassword:
		case methodTOTP:
			totpEnrolled = true
		default:
			uncollectible = true
		}
	}
	return totpEnrolled, uncollectible, nil
}

// mfaRequired applies the two policy fields to this user.
//
// forceMfa applies to everyone. forceMfaLocalOnly applies only to users
// authenticating with a local credential — mark8ly has federated Google and
// Apple users, and forcing MFA on them would be wrong. These are kept separate
// rather than OR-ed together for exactly that reason.
func mfaRequired(p LoginPolicy, federated bool) bool {
	if p.ForceMFA {
		return true
	}
	return p.ForceMFALocalOnly && !federated
}

// DecideSufficiency runs the same evaluation CompleteIfSufficient does —
// SessionFactors, classifyEnrolledMethods, LoginPolicyForOrg, mfaRequired —
// and stops there: it never calls finalize and never constructs the
// sufficient{} witness, so it cannot obtain an OIDC authorization code.
//
// This is what the storefront customer path calls (see customer_handler.go
// and spec D11): a sufficiency decision and, on OutcomeComplete, an identity
// — never a callback URL. CompleteIfSufficient below is this function plus
// finalize, and is the only thing that changed to make that split: the
// decision logic itself is not duplicated anywhere.
//
// Zitadel does NOT enforce forceMfa for a login client: it issues an
// authorization code for a password-only session and signals nothing. Every
// uncertain input therefore fails closed to OutcomeHandoff, which is a
// legitimate outcome rather than an error.
func (c *Client) DecideSufficiency(ctx context.Context, s Session, federated bool) (Result, error) {
	factors, err := c.SessionFactors(ctx, s.ID)
	if err != nil || factors.UserID == "" || factors.OrgID == "" {
		slog.WarnContext(ctx, "zitadel sufficiency: cannot read session subject, handing off", "err", err)
		return Result{Outcome: OutcomeHandoff}, nil
	}
	totpEnrolled, uncollectible, err := c.classifyEnrolledMethods(ctx, factors.UserID)
	if err != nil {
		slog.WarnContext(ctx, "zitadel sufficiency: cannot read enrolled methods, handing off", "err", err)
		return Result{Outcome: OutcomeHandoff}, nil
	}
	if uncollectible {
		return Result{Outcome: OutcomeHandoff}, nil
	}
	policy, err := c.LoginPolicyForOrg(ctx, factors.OrgID)
	if err != nil {
		slog.WarnContext(ctx, "zitadel sufficiency: cannot read login policy, handing off", "err", err)
		return Result{Outcome: OutcomeHandoff}, nil
	}
	if mfaRequired(policy, federated) && !factors.TOTP {
		if !totpEnrolled {
			return Result{Outcome: OutcomeHandoff}, nil
		}
		return Result{Outcome: OutcomeFactorRequired, Factors: []string{methodTOTP}}, nil
	}
	return Result{Outcome: OutcomeComplete}, nil
}

// CompleteIfSufficient decides whether a freshly created session may finalize,
// and finalizes it when it may. It is DecideSufficiency plus the finalize
// call — the merchant path's behavior is unchanged by this split.
func (c *Client) CompleteIfSufficient(ctx context.Context, authRequestID string, s Session, federated bool) (Result, error) {
	res, err := c.DecideSufficiency(ctx, s, federated)
	if err != nil || res.Outcome != OutcomeComplete {
		return res, err
	}
	cb, err := c.finalize(ctx, authRequestID, s, sufficient{})
	if err != nil {
		return Result{Outcome: OutcomeHandoff}, err
	}
	return Result{Outcome: OutcomeComplete, CallbackURL: cb}, nil
}

// DecideAfterFactor is CompleteAfterFactor's decision half: it re-reads the
// session's factors after a TOTP check and reports whether TOTP is now
// confirmed, without finalizing. Used by the storefront customer path's
// /customer/totp step for the same reason DecideSufficiency exists — see its
// doc comment.
//
// It re-reads the factors from Zitadel rather than trusting that VerifyTOTP
// returned without error, because the caller may be holding a stale session
// value from before the token rotated.
func (c *Client) DecideAfterFactor(ctx context.Context, s Session) (Result, error) {
	factors, err := c.SessionFactors(ctx, s.ID)
	if err != nil {
		slog.WarnContext(ctx, "zitadel sufficiency: cannot re-read factors after TOTP, handing off", "err", err)
		return Result{Outcome: OutcomeHandoff}, nil
	}
	if !factors.TOTP {
		return Result{Outcome: OutcomeHandoff}, nil
	}
	return Result{Outcome: OutcomeComplete}, nil
}

// CompleteAfterFactor finalizes after a TOTP check. It is DecideAfterFactor
// plus the finalize call — the merchant path's behavior is unchanged by this
// split.
func (c *Client) CompleteAfterFactor(ctx context.Context, authRequestID string, s Session) (Result, error) {
	res, err := c.DecideAfterFactor(ctx, s)
	if err != nil || res.Outcome != OutcomeComplete {
		return res, err
	}
	cb, err := c.finalize(ctx, authRequestID, s, sufficient{})
	if err != nil {
		return Result{Outcome: OutcomeHandoff}, err
	}
	return Result{Outcome: OutcomeComplete, CallbackURL: cb}, nil
}
