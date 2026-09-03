// This file implements the STOREFRONT CUSTOMER login path. It looks
// unfinished compared to Handler (handler.go): it mints no session, sets no
// cookie, and calls nothing in internal/autologin. That is deliberate — see
// spec D11 (docs/superpowers/specs/2026-09-03-zitadel-migration-design.md).
//
// D11 supersedes an earlier framing (D10) that said the customer path would
// "skip the membership check". Reading the code made a simpler and safer
// shape available: rather than reuse the merchant gauntlet minus a check,
// this endpoint verifies the credential against Zitadel and returns
// {uid, email}. Full stop.
//
// Why not "finish" it by minting a session here too:
//
//   - Storefront customers are deliberately not OpenFGA members. Running them
//     through the merchant gauntlet would mean either adding a bypass flag to
//     that gauntlet (a second, weaker path baked into the trusted one) or
//     giving customers FGA tuples they have no reason to hold.
//   - The storefront mints its own `mp_customer_session` cookie, in its own
//     HMAC format, scoped to the exact request host — a customer signed in on
//     one store's subdomain must never be handed a session usable on another
//     store. Minting a cookie here would either introduce a THIRD session
//     format (alongside `m8_session` and `mp_customer_session`) or require
//     this package to know the request host of a storefront it has no
//     business knowing about. Either destroys the per-store isolation the
//     storefront's own minting code already gives for free.
//
// So: this handler verifies and returns. The storefront's existing sign-in
// action (apps/storefront/app/sign-in/actions.ts) keeps doing everything
// else it does today — resolving the host, resolving the store, minting the
// cookie, driving profile/loyalty side effects. Only the "verify the
// credential" step moves to Zitadel.
//
// It follows from this that the customer path must never finalize: an OIDC
// authorization code is not an identity, and obtaining a genuine one needs
// an auth_request_id from an OIDC authorize round trip the storefront does
// not have. So login/totp below call sufficiency.go's decision-only
// DecideSufficiency / DecideAfterFactor, never CompleteIfSufficient /
// CompleteAfterFactor, and take no auth_request_id at all.
package zitadellogin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CustomerHandler is the HTTP layer over Client for storefront customers. It
// shares Client and the sufficiency decision with Handler, but never touches
// CompleteFunc, session cookies, or internal/autologin — see the file
// comment above.
type CustomerHandler struct {
	c *Client

	// hostedLoginBaseURL mirrors Handler.hostedLoginBaseURL: the Zitadel
	// instance's own login UI, used ONLY as the OutcomeHandoff target for
	// factors this endpoint cannot collect. Optional: set via
	// WithHostedLoginBaseURL.
	hostedLoginBaseURL string

	// internalAuthSecret is the expected X-Internal-Auth value. See
	// internal_auth.go: empty means unchecked, and the boot guard in
	// config.ValidateZitadel is what stops that reaching production.
	internalAuthSecret string

	// returnURLs is the allowlist idp/start validates every caller-supplied
	// return_url against before handing it to Zitadel as successUrl/
	// failureUrl. This MUST be the STOREFRONT allowlist
	// (config.Config.ZitadelReturnURLAllowedHosts/SuffixesStorefront), never
	// the admin one: tenant subdomains under it are merchant-self-
	// provisioned, and the two lists are kept separate precisely so a
	// merchant-controlled storefront origin can never be a valid successUrl
	// for an ADMIN sign-in (see returnurl.go's file doc). The zero value
	// rejects every candidate, so an unconfigured CustomerHandler fails
	// closed. Set via WithReturnURLAllowlist.
	returnURLs ReturnURLAllowlist

	// googleIDPID is the id of the Google IDP on the Zitadel org that
	// idp/start opens an intent against, and idp/finish pins every
	// retrieved identity to (see idpFinish). Same instance-wide IDP the
	// merchant path uses — set via WithGoogleIDPID.
	googleIDPID string

	// orgID scopes idp/finish's FindUserByVerifiedEmail lookup. Same
	// Zitadel org as the merchant path (one instance, two projects) — set
	// via WithOrgID.
	orgID string
}

// NewCustomerHandler constructs a CustomerHandler.
func NewCustomerHandler(c *Client) *CustomerHandler {
	return &CustomerHandler{c: c}
}

// WithHostedLoginBaseURL sets the Zitadel instance base URL used to build the
// OutcomeHandoff redirect target. Mirrors Handler.WithHostedLoginBaseURL.
func (h *CustomerHandler) WithHostedLoginBaseURL(baseURL string) *CustomerHandler {
	h.hostedLoginBaseURL = strings.TrimSuffix(baseURL, "/")
	return h
}

// WithInternalAuth requires every request to /auth/customer/{login,totp} to
// present secret in the X-Internal-Auth header. The storefront calls these
// from a server action only, so this costs it one header. See
// internal_auth.go.
func (h *CustomerHandler) WithInternalAuth(secret string) *CustomerHandler {
	h.internalAuthSecret = secret
	return h
}

// WithReturnURLAllowlist sets the allowlist idp/start validates every
// caller-supplied return_url against. MUST be built from the storefront
// hosts/suffixes — see the returnURLs field doc.
func (h *CustomerHandler) WithReturnURLAllowlist(a ReturnURLAllowlist) *CustomerHandler {
	h.returnURLs = a
	return h
}

// WithGoogleIDPID sets the Zitadel org's Google IDP id. See the googleIDPID
// field doc.
func (h *CustomerHandler) WithGoogleIDPID(id string) *CustomerHandler {
	h.googleIDPID = id
	return h
}

// WithOrgID sets the org idp/finish's account lookup is scoped to. See the
// orgID field doc.
func (h *CustomerHandler) WithOrgID(id string) *CustomerHandler {
	h.orgID = id
	return h
}

// Register mounts the customer login routes onto the given gin.RouterGroup.
// Like Handler.Register, the handlers are plain net/http funcs; gin is only
// used to route, matching this package's existing style.
func (h *CustomerHandler) Register(r *gin.RouterGroup) {
	r.POST("/customer/login", func(c *gin.Context) {
		h.login(c.Writer, c.Request)
	})
	r.POST("/customer/totp", func(c *gin.Context) {
		h.totp(c.Writer, c.Request)
	})
	r.POST("/customer/idp/start", func(c *gin.Context) {
		h.idpStart(c.Writer, c.Request)
	})
	r.POST("/customer/idp/finish", func(c *gin.Context) {
		h.idpFinish(c.Writer, c.Request)
	})
}

// Neither request shape carries an auth_request_id: the customer path makes
// a sufficiency decision and returns an identity (see DecideSufficiency /
// DecideAfterFactor in sufficiency.go and spec D11) — it never finalizes, so
// it has no use for an OIDC authorization-request id. A caller that sends
// one anyway (e.g. a client still carrying the auth_request_id plumbing
// removed from the storefront in this same change) has it silently ignored:
// decodeJSON does not reject unknown fields.
type customerLoginRequest struct {
	LoginName string `json:"login_name"`
	Password  string `json:"password"`
}

type customerTOTPRequest struct {
	SessionID    string `json:"session_id"`
	SessionToken string `json:"session_token"`
	Code         string `json:"code"`
}

// customerIDPStartRequest mirrors idpStartRequest (handler.go).
type customerIDPStartRequest struct {
	ReturnURL string `json:"return_url"`
}

// customerIDPFinishRequest mirrors idpFinishRequest's shape, minus
// AuthRequestID and WorkspaceTenant: this endpoint decides and returns an
// identity (see the file comment and customerLoginRequest's doc above), it
// never finalizes an OIDC auth request and never mints a per-tenant session,
// so it has no use for either field.
type customerIDPFinishRequest struct {
	IntentID    string `json:"intent_id"`
	IntentToken string `json:"intent_token"`

	// User is NEVER READ past decoding — see idpFinishRequest.User's doc in
	// handler.go for why: it is attacker-controlled, riding in a URL the
	// browser followed, and the authoritative identity comes only from
	// RetrieveIDPIntent(IntentID, IntentToken).
	User string `json:"user"`
}

// login reads {login_name, password}, creates a Zitadel password session,
// and asks sufficiency.go whether that session is sufficient. It never
// mints a session, sets a cookie, or finalizes — see the file comment.
func (h *CustomerHandler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// First, before the body is even read: an unauthenticated caller must
	// never reach CreatePasswordSession, or this endpoint tells them
	// whether a credential is valid.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req customerLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.LoginName == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// A wrong username and a wrong password take the same code path in
	// CreatePasswordSession and must take the same one here: a different
	// answer for "no such user" is an account-enumeration oracle on a public
	// storefront that anyone can probe.
	sess, err := h.c.CreatePasswordSession(ctx, req.LoginName, req.Password)
	if err != nil {
		h.respondSessionCreateError(ctx, w, err)
		return
	}

	// Password login through this endpoint is never a federated
	// (Google/Apple) identity — those never present a password to us at all.
	res, err := h.c.DecideSufficiency(ctx, sess, false)
	h.respondOutcome(ctx, w, res, err, sess)
}

// totp reads {session_id, session_token, code}, submits the TOTP code
// against the session opened by login, and re-asks sufficiency.go whether
// the session is now sufficient.
func (h *CustomerHandler) totp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// See login: reject before any Zitadel call.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req customerTOTPRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.SessionID == "" || req.SessionToken == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	sess, err := h.c.VerifyTOTP(ctx, Session{ID: req.SessionID, Token: req.SessionToken}, req.Code)
	if err != nil {
		h.respondTOTPVerifyError(ctx, w, err)
		return
	}

	res, err := h.c.DecideAfterFactor(ctx, sess)
	h.respondOutcome(ctx, w, res, err, sess)
}

// idpStart validates the caller-supplied return URL against the STOREFRONT
// allowlist and starts a Zitadel IDP intent for Google. Mirrors
// Handler.idpStart (handler.go) exactly except for which allowlist it
// checks against — see the returnURLs field doc for why that split matters.
func (h *CustomerHandler) idpStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// First, before the body is even read — same discipline as login/totp.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req customerIDPStartRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.ReturnURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	returnURL, err := h.returnURLs.ValidateReturnURL(req.ReturnURL)
	if err != nil {
		slog.WarnContext(ctx, "zitadellogin(customer): idp start rejected: return url not allowed")
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_return_url"})
		return
	}

	if h.googleIDPID == "" {
		slog.ErrorContext(ctx, "zitadellogin(customer): idp start: no google idp id configured (see WithGoogleIDPID)")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	authURL, err := h.c.StartIDPIntent(ctx, h.googleIDPID, returnURL, returnURL)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin(customer): start idp intent failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auth_url": authURL})
}

// idpFinish exchanges the intent id/token for the federated identity and
// decides: link it to an existing verified account, register a brand-new
// account for it, or resolve it directly when already linked. It then
// RETURNS THAT IDENTITY AND STOPS — no Zitadel session, no finalize. See the
// file comment for why, and Handler.idpFinish (handler.go) for the merchant
// mirror of the IDP-pinning and email-verified checks below, which are
// IDENTICAL here.
func (h *CustomerHandler) idpFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// First, before the body is even read: an unauthenticated caller must
	// never reach RetrieveIDPIntent or any account lookup/link/create call.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req customerIDPFinishRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.IntentID == "" || req.IntentToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// The ONLY source of identity for this endpoint. req.User (see its doc
	// comment above) is never consulted, here or anywhere below.
	identity, err := h.c.RetrieveIDPIntent(ctx, req.IntentID, req.IntentToken)
	if err != nil {
		h.respondIDPIntentError(ctx, w, err)
		return
	}

	// Pin the IDP exactly like the merchant path: this endpoint is Google
	// sign-in specifically, and the instance can carry more than one IDP
	// (e.g. Apple). Checked immediately after retrieve, before the email
	// gate, find, link, or create — see Handler.idpFinish's doc for the
	// full rationale, which applies here unchanged.
	if identity.IDPID == "" || identity.IDPID != h.googleIDPID {
		slog.WarnContext(ctx, "zitadellogin(customer): idp finish rejected: intent did not come from the configured google idp")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unexpected_idp"})
		return
	}

	userID := identity.ZitadelUserID
	if userID == "" {
		// A first-time Google sign-in. Unlike the merchant path, self-
		// registration IS the desired behaviour here: shoppers are not FGA
		// members of anything, so a freshly created customer account carries
		// no authorization gap the way a freshly created merchant one would.
		// The security rule is identical to the merchant path though, and
		// just as absolute: an unlinked identity may be linked OR used to
		// register a new account ONLY when the provider asserts the email is
		// verified. identity.EmailVerified defaults to false when the claim
		// is absent (see IDPIdentity's doc) — refuse exactly like an
		// explicit false, never like "probably fine".
		if identity.Email == "" || !identity.EmailVerified {
			slog.WarnContext(ctx, "zitadellogin(customer): idp finish rejected: unlinked identity with no verified email")
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "email_not_verified"})
			return
		}

		existingUserID, err := h.c.FindUserByVerifiedEmail(ctx, h.orgID, identity.Email)
		if err != nil {
			if errors.Is(err, ErrAmbiguousEmailMatch) {
				slog.WarnContext(ctx, "zitadellogin(customer): idp finish rejected: more than one existing account matched the verified email")
				writeJSON(w, http.StatusConflict, map[string]any{"error": "email_ambiguous"})
				return
			}
			slog.ErrorContext(ctx, "zitadellogin(customer): idp finish: could not check for an existing account by email", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
			return
		}

		if existingUserID != "" {
			// A Zitadel user already holds this exact, verified email:
			// attach this Google identity to THAT account rather than
			// registering a second, disconnected one. LinkIDPToUser
			// re-checks the same email-verified rule independently.
			if err := h.c.LinkIDPToUser(ctx, existingUserID, identity); err != nil {
				slog.ErrorContext(ctx, "zitadellogin(customer): idp finish: could not link identity to the existing account", "err", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
				return
			}
			slog.InfoContext(ctx, "zitadellogin(customer): idp finish linked a first-time Google identity to an existing account", "user_id", existingUserID)
			userID = existingUserID
		} else {
			// No account exists for this identity at all: unlike the
			// merchant path, this is the normal case worth handling, not a
			// refusal — shoppers self-register. CreateHumanUserWithIDPLink
			// independently re-checks the same email-verified rule.
			newUserID, err := h.c.CreateHumanUserWithIDPLink(ctx, identity)
			if err != nil {
				if errors.Is(err, ErrEmailAlreadyExists) {
					// Deliberately a DIFFERENT outcome than email_ambiguous
					// above, not a reuse of it — the two are NOT the same
					// situation and must stay separable in logs and in
					// customer-facing copy:
					//
					//   - email_ambiguous (above): FindUserByVerifiedEmail
					//     itself found more than one VERIFIED match. A
					//     genuine race between two requests resolves on a
					//     fresh Google click — the loser here can simply
					//     retry.
					//   - email_taken (here): FindUserByVerifiedEmail found
					//     NO verified match (that is precisely why this
					//     create was attempted), yet Zitadel's create still
					//     400s because some UNVERIFIED account already holds
					//     this exact email — an abandoned signup, an
					//     unverified invite, or an attacker who typed the
					//     victim's address and set their own password. This
					//     is a genuine (if rare) race only in the narrow
					//     window against a concurrent request; far more
					//     often it is a PERMANENT lockout: retrying changes
					//     nothing, ever, for as long as that unverified
					//     account exists.
					//
					// Refusing is still correct either way — Google proving
					// the person owns the address does NOT make it safe to
					// link them to an account someone else may control (see
					// LinkIDPToUser's doc: linking an unverified provider
					// email to an existing account is account takeover, and
					// the same reasoning applies to an unverified account
					// that already holds this email — it may not belong to
					// the person signing in now). What must not happen is
					// telling the customer this looks like a transient race
					// when it usually is not: distinct code, so the
					// storefront can render something other than "try
					// again" and a support path can find these in logs.
					slog.WarnContext(ctx, "zitadellogin(customer): idp finish rejected: email already claimed by another account (verified-match search found none — likely an unverified account, not necessarily a race)")
					writeJSON(w, http.StatusConflict, map[string]any{"error": "email_taken"})
					return
				}
				slog.ErrorContext(ctx, "zitadellogin(customer): idp finish: could not create a new account for this identity", "err", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
				return
			}
			slog.InfoContext(ctx, "zitadellogin(customer): idp finish registered a new customer account for a first-time Google identity", "user_id", newUserID)
			userID = newUserID
		}
	}

	// The email in the response MUST come from Zitadel's own record of
	// userID, never from the request or trusted verbatim off the raw
	// provider claim above — same defensive resolution finishComplete uses
	// for the password path.
	email, err := h.c.UserEmail(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin(customer): could not resolve the email for the resolved identity", "err", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// No session, no cookie, no finalize — see the file comment. This
	// endpoint decides and stops.
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"uid":   userID,
			"email": email,
		},
	})
}

// respondOutcome is shared by login and totp: both end at DecideSufficiency
// / DecideAfterFactor and switch on the same three outcomes as
// Handler.respondOutcome, but OutcomeComplete here resolves and returns an
// identity instead of finalizing — the decision functions never call
// finalize, so there is no callback URL to hand back and no
// "handoff after a failed finalize" case to report.
func (h *CustomerHandler) respondOutcome(
	ctx context.Context,
	w http.ResponseWriter,
	res Result,
	resErr error,
	sess Session,
) {
	switch res.Outcome {
	case OutcomeComplete:
		if resErr != nil {
			// Not reachable per DecideSufficiency/DecideAfterFactor's
			// contract, but refuse to report success on an outcome/error
			// mismatch rather than trust an incoherent result.
			slog.ErrorContext(ctx, "zitadellogin(customer): OutcomeComplete carried a non-nil error, refusing to complete", "err", resErr)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		h.finishComplete(ctx, w, sess)

	case OutcomeFactorRequired:
		// No session minted here — this IS the MFA gate.
		writeJSON(w, http.StatusOK, map[string]any{
			"totp_required": true,
			"session_id":    sess.ID,
			"session_token": sess.Token,
		})

	default: // OutcomeHandoff, including the zero value.
		if resErr != nil {
			slog.ErrorContext(ctx, "zitadellogin(customer): handoff after a decision error", "err", resErr)
		} else {
			slog.InfoContext(ctx, "zitadellogin(customer): handoff (uncollectible factor or unreadable policy/session)")
		}
		// A customer who reaches this branch has a real, uncollectible
		// factor (a passkey, U2F, SMS OTP, recovery code, ...) — see
		// classifyEnrolledMethods — and handoff is the only door out. If no
		// hosted login base URL is configured, handoffURL() returns "", and
		// silently returning that string would be a 200 with nowhere to go:
		// the customer would be stuck with no way to finish signing in. Fail
		// loudly instead of leaving that empty, so the storefront can render
		// "sign-in is unavailable" rather than a dead handoff link.
		handoffURL := h.handoffURL()
		if handoffURL == "" {
			slog.ErrorContext(ctx, "zitadellogin(customer): handoff has no hosted login base URL configured; customer has no way to complete sign-in")
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "signin_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"handoff_url": handoffURL,
		})
	}
}

// finishComplete resolves the subject of the now-sufficient session and
// returns {uid, email}. It does not mint a session, set a cookie, or call
// anything in internal/autologin — see the file comment.
func (h *CustomerHandler) finishComplete(ctx context.Context, w http.ResponseWriter, sess Session) {
	// Result carries no subject — re-read it.
	factors, err := h.c.SessionFactors(ctx, sess.ID)
	if err != nil || factors.UserID == "" {
		slog.ErrorContext(ctx, "zitadellogin(customer): could not resolve session subject after a sufficient decision", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// The email MUST come from Zitadel's own record of this session's
	// subject, never from a request body — the same defect fixed on the
	// merchant path in phase 2. A caller with valid credentials of their own
	// could otherwise submit an arbitrary login_name and walk away with an
	// identity response addressed to a victim's email of their choosing.
	email, err := h.c.UserEmail(ctx, factors.UserID)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin(customer): could not resolve the verified email for session subject", "err", err, "user_id", factors.UserID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"uid":   factors.UserID,
			"email": email,
		},
	})
}

// respondSessionCreateError maps CreatePasswordSession's errors.
//
// ErrBadCredentials and ErrUserNotFound MUST produce the identical response —
// collapsing them is the entire point of this function, and it matters more
// here than on the merchant path: this endpoint is reachable by anyone on a
// public storefront, so a different answer for "no such user" is a live
// account-enumeration oracle. Which one actually happened is logged for
// operators, never returned to the caller.
func (h *CustomerHandler) respondSessionCreateError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials):
		slog.WarnContext(ctx, "zitadellogin(customer): login rejected: bad credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
	case errors.Is(err, ErrUserNotFound):
		slog.WarnContext(ctx, "zitadellogin(customer): login rejected: user not found")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin(customer): zitadel unavailable creating session", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin(customer): unexpected error creating session", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// respondTOTPVerifyError maps VerifyTOTP's errors. Unlike the login step,
// there is no enumeration concern here — the account is already established
// — but the wrong-code case still must never echo Zitadel's error body.
func (h *CustomerHandler) respondTOTPVerifyError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials):
		slog.WarnContext(ctx, "zitadellogin(customer): totp rejected: bad code")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_totp"})
	case errors.Is(err, ErrUserNotFound):
		// The session itself vanished/expired between steps.
		slog.WarnContext(ctx, "zitadellogin(customer): totp rejected: session not found")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_totp"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin(customer): zitadel unavailable verifying totp", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin(customer): unexpected error verifying totp", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// respondIDPIntentError maps RetrieveIDPIntent's errors. Mirrors
// Handler.respondIDPIntentError (handler.go): log the real reason, answer
// the caller with a code that carries no Zitadel error detail.
func (h *CustomerHandler) respondIDPIntentError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrIDPIntentInvalid):
		slog.WarnContext(ctx, "zitadellogin(customer): idp finish rejected: intent invalid")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_intent"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin(customer): zitadel unavailable retrieving idp intent", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin(customer): unexpected error retrieving idp intent", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// handoffURL builds the Aurora-branded hosted login's landing URL for a
// login this endpoint decided it cannot (or should not) finish itself.
// Unlike Handler.handoffURL on the merchant path, there is no
// authRequestID to append — the customer path never has a genuine one (see
// the file comment) — so this is a bare landing URL, not a continuation of
// a specific auth request. Returns "" when no hosted login base URL was
// configured.
func (h *CustomerHandler) handoffURL() string {
	if h.hostedLoginBaseURL == "" {
		return ""
	}
	return h.hostedLoginBaseURL + "/ui/v2/login/login"
}
