package zitadellogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/geoip"
)

// maxRequestBodyBytes bounds request decoding, matching the defensive limits
// already used for Zitadel's own responses in client.go.
const maxRequestBodyBytes = 8 * 1024

// LoginContext is everything the shared post-identity gauntlet needs beyond
// the identity itself. It exists because deviceguard fingerprints the user
// agent and the email-OTP limiter keys on the client IP: passing these empty
// silently disables both, since Fingerprint("") is a constant every user
// would share — the first Zitadel login would look like a new device, and
// every one after it, from anywhere, would look like the same familiar one.
type LoginContext struct {
	UID      string
	Email    string
	TenantID string
	// UserAgent, IPAddress, Device and Country are best-effort client
	// metadata, populated the same way autologin's own handler populates
	// them for a GIP login (see Handler.loginContext below) so the two
	// providers cannot silently diverge in what deviceguard, the email-OTP
	// limiter, the session registry and audit events see.
	UserAgent string
	IPAddress string
	Device    string
	Country   string
}

// CompleteResult is what CompleteFunc reports back about the shared
// post-identity gauntlet, beyond a plain error. Without this, CompleteFunc
// had no way to say "a step-up is still outstanding" — the handler would
// answer 200 with a callback_url while the browser still owed an MFA code or
// an email-OTP, telling it the login was finished when it was not.
type CompleteResult struct {
	// MFARequired is true when the gauntlet minted only a pending cookie
	// pending a TOTP code via POST /auth/mfa-challenge.
	MFARequired bool
	// EmailOTPRequired is true when the gauntlet minted only a pending
	// cookie pending a mailed code via POST /auth/otp/verify.
	EmailOTPRequired bool
}

// CompleteFunc runs the shared post-identity gauntlet — FGA membership, the
// MFA gate, deviceguard, the email-OTP step-up, session minting. It is
// injected rather than imported so this package stays a Zitadel client and
// knows nothing about autologin.
type CompleteFunc func(ctx context.Context, w http.ResponseWriter, lc LoginContext) (CompleteResult, error)

// Handler is the HTTP layer over Client: it owns request/response shapes and
// the outcome switch, and defers every credential and sufficiency decision to
// the client and sufficiency packages respectively.
type Handler struct {
	c        *Client
	complete CompleteFunc

	// hostedLoginBaseURL is the Zitadel instance's own login UI (the
	// Aurora-branded hosted login), used ONLY as the OutcomeHandoff target
	// for factors this page cannot collect (passkeys, U2F, SMS OTP,
	// recovery codes). Optional: set via WithHostedLoginBaseURL. When
	// unset, a handoff still answers 200 with an empty handoff_url and the
	// auth_request_id, and logs a warning — the auth request itself is not
	// lost, only the convenience redirect.
	hostedLoginBaseURL string

	// internalAuthSecret is the expected X-Internal-Auth value. See
	// internal_auth.go: empty means unchecked, and the boot guard in
	// config.ValidateZitadel is what stops that reaching production.
	internalAuthSecret string

	// returnURLs is the allowlist idp/start validates every caller-supplied
	// return_url against before handing it to Zitadel as successUrl/
	// failureUrl. Zitadel does not validate that URL at all (see
	// returnurl.go's file comment) — this is the only thing standing
	// between idp/start and an open redirect. The zero value rejects every
	// candidate (nil hosts/suffixes), so an unconfigured Handler fails
	// closed rather than silently allowing every host.
	returnURLs ReturnURLAllowlist

	// googleIDPID is the id of the Google IDP on the Zitadel org that
	// idp/start opens an intent against. Configured, not hardcoded: it is
	// environment-specific (a staging or replacement instance has a
	// different id), and a constant would mean a code change and redeploy
	// to repoint it. Empty means idp/start is not usable — see
	// WithGoogleIDPID.
	googleIDPID string
}

// NewHandler constructs a Handler. complete may be nil in tests that never
// exercise OutcomeComplete.
func NewHandler(c *Client, complete CompleteFunc) *Handler {
	return &Handler{c: c, complete: complete}
}

// WithHostedLoginBaseURL sets the Zitadel instance base URL used to build the
// OutcomeHandoff redirect target. Mirrors the optional-config builder idiom
// used elsewhere in this service (e.g. session.Handler.WithRegistry).
func (h *Handler) WithHostedLoginBaseURL(baseURL string) *Handler {
	h.hostedLoginBaseURL = strings.TrimSuffix(baseURL, "/")
	return h
}

// WithInternalAuth requires every request to /auth/zitadel/{login,totp} to
// present secret in the X-Internal-Auth header. Both callers are
// server-side, so this costs them one header. See internal_auth.go.
func (h *Handler) WithInternalAuth(secret string) *Handler {
	h.internalAuthSecret = secret
	return h
}

// WithReturnURLAllowlist sets the allowlist idp/start validates every
// caller-supplied return_url against. Without this, the Handler's zero-value
// allowlist rejects every return_url (see the returnURLs field doc).
func (h *Handler) WithReturnURLAllowlist(a ReturnURLAllowlist) *Handler {
	h.returnURLs = a
	return h
}

// WithGoogleIDPID sets the Zitadel org's Google IDP id that idp/start opens
// an intent against. See the googleIDPID field doc for why this is
// configuration rather than a constant.
func (h *Handler) WithGoogleIDPID(id string) *Handler {
	h.googleIDPID = id
	return h
}

// Register mounts the Zitadel login routes onto the given gin.RouterGroup.
// The handlers themselves are plain net/http funcs so this package's tests
// stay httptest-only; the two routing closures below are the ONLY place gin
// is touched, and exist solely to carry gin.Context.ClientIP() — the same
// trusted-proxy-aware IP resolution autologin's handler uses via c.ClientIP()
// — into the request context, since a plain http.Request has no equivalent.
func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/zitadel/login", func(c *gin.Context) {
		h.login(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})
	r.POST("/zitadel/totp", func(c *gin.Context) {
		h.totp(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})

	// Google sign-in through Zitadel's IDP-intent flow (#524 phase 3c-2).
	// Grouped separately from the two routes above so a later Apple pair
	// (task 4's note) has an obvious place to land alongside these, not
	// interleaved with the password/TOTP routes.
	r.POST("/zitadel/idp/start", func(c *gin.Context) {
		h.idpStart(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})
	r.POST("/zitadel/idp/finish", func(c *gin.Context) {
		h.idpFinish(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})
}

type contextKey int

const clientIPContextKey contextKey = iota

// withClientIP attaches a pre-resolved client IP to the request context. Kept
// as context plumbing (rather than widening login/totp's signature) so those
// handlers stay plain (http.ResponseWriter, *http.Request) funcs that tests
// can call directly, exactly like the existing handler tests do.
func withClientIP(r *http.Request, ip string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), clientIPContextKey, ip))
}

// clientIPFromContext reads back the IP withClientIP attached. Returns "" for
// a request built directly by a test rather than routed through Register —
// same behaviour as an untested field, never a panic.
func clientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPContextKey).(string)
	return ip
}

// deviceFromUA produces the same short device label autologin's handler
// derives from a User-Agent (see internal/autologin/handler.go). Duplicated
// rather than imported: this package must not depend on autologin, and the
// label is a small, self-contained, presentation-only heuristic.
func deviceFromUA(ua string) string {
	ua = strings.ToLower(ua)
	switch {
	case ua == "":
		return "Unknown device"
	case strings.Contains(ua, "iphone"):
		return "iPhone"
	case strings.Contains(ua, "ipad"):
		return "iPad"
	case strings.Contains(ua, "android"):
		return "Android"
	case strings.Contains(ua, "mac os x"), strings.Contains(ua, "macintosh"):
		return "Mac"
	case strings.Contains(ua, "windows"):
		return "Windows"
	case strings.Contains(ua, "linux"):
		return "Linux"
	default:
		return "Browser"
	}
}

// loginContext assembles the client metadata half of LoginContext from the
// inbound request, the same way autologin's handler assembles autologin.
// Request's Device/IPAddress/UserAgent/Country fields for a GIP login.
func (h *Handler) loginContext(r *http.Request, uid, email, tenantID string) LoginContext {
	ua := r.UserAgent()
	return LoginContext{
		UID:       uid,
		Email:     email,
		TenantID:  tenantID,
		UserAgent: ua,
		IPAddress: clientIPFromContext(r.Context()),
		Device:    deviceFromUA(ua),
		Country:   geoip.CountryFromHeaders(r.Header),
	}
}

type loginRequest struct {
	AuthRequestID   string `json:"auth_request_id"`
	LoginName       string `json:"login_name"`
	Password        string `json:"password"`
	WorkspaceTenant string `json:"workspace_tenant"`
}

type totpRequest struct {
	AuthRequestID   string `json:"auth_request_id"`
	LoginName       string `json:"login_name"`
	SessionID       string `json:"session_id"`
	SessionToken    string `json:"session_token"`
	Code            string `json:"code"`
	WorkspaceTenant string `json:"workspace_tenant"`
}

// login reads {auth_request_id, login_name, password, workspace_tenant},
// creates a Zitadel password session, and asks sufficiency.go whether that
// session may finalize.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// First, before the body is even read: an unauthenticated caller must
	// never reach CreatePasswordSession, or this endpoint tells them
	// whether a credential is valid.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.AuthRequestID == "" || req.LoginName == "" || req.Password == "" || req.WorkspaceTenant == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// A wrong username and a wrong password take the same code path in
	// CreatePasswordSession and must take the same one here: a different
	// answer for "no such user" is an account-enumeration oracle.
	sess, err := h.c.CreatePasswordSession(ctx, req.LoginName, req.Password)
	if err != nil {
		h.respondSessionCreateError(ctx, w, err)
		return
	}

	// Password login through this page is never a federated (Google/Apple)
	// identity — those never present a password to us at all.
	res, err := h.c.CompleteIfSufficient(ctx, req.AuthRequestID, sess, false)
	h.respondOutcome(w, r, res, err, req.AuthRequestID, sess, req.WorkspaceTenant)
}

// totp reads {auth_request_id, login_name, session_id, session_token, code,
// workspace_tenant}, submits the TOTP code against the session opened by
// login, and re-asks sufficiency.go whether the session may now finalize.
func (h *Handler) totp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// See login: reject before any Zitadel call.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req totpRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.AuthRequestID == "" || req.LoginName == "" || req.SessionID == "" || req.SessionToken == "" ||
		req.Code == "" || req.WorkspaceTenant == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	sess, err := h.c.VerifyTOTP(ctx, Session{ID: req.SessionID, Token: req.SessionToken}, req.Code)
	if err != nil {
		h.respondTOTPVerifyError(ctx, w, err)
		return
	}

	// req.LoginName is NOT used from here on. On this step it is an
	// unverified claim from the request body — the password check that
	// verified login_name happened on the earlier /zitadel/login call, on a
	// session this caller may not even own. finishComplete resolves the
	// real subject's email from Zitadel via SessionFactors + UserEmail
	// instead, so a caller cannot walk away with a session cookie, an audit
	// event, or a mailed sign-in code addressed to an email of their choice.
	res, err := h.c.CompleteAfterFactor(ctx, req.AuthRequestID, sess)
	h.respondOutcome(w, r, res, err, req.AuthRequestID, sess, req.WorkspaceTenant)
}

type idpStartRequest struct {
	ReturnURL string `json:"return_url"`
}

// idpFinishRequest is what the frontend's own finish route (the target of
// idpStart's successUrl) posts back here after Zitadel redirects the
// browser to it.
type idpFinishRequest struct {
	AuthRequestID   string `json:"auth_request_id"`
	IntentID        string `json:"intent_id"`
	IntentToken     string `json:"intent_token"`
	WorkspaceTenant string `json:"workspace_tenant"`

	// User carries whatever value the caller received in Zitadel's `user`
	// redirect query parameter, if it forwards one at all.
	//
	// It is decoded here and NEVER READ again below. That param is
	// attacker-controlled — it arrives in a URL the browser followed, and
	// is present at all only when Zitadel believes the identity is already
	// linked. The authoritative identity for this endpoint comes ONLY from
	// RetrieveIDPIntent(IntentID, IntentToken), which resolves it from the
	// intent id/token pair rather than from anything the caller asserts.
	// Trusting this field instead would let any caller who can guess or
	// observe a valid intent id/token log in as a user of their choosing by
	// simply changing this one field.
	User string `json:"user"`
}

// idpStart validates the caller-supplied return URL against the configured
// allowlist, starts a Zitadel IDP intent for Google, and returns the authUrl
// the browser must be sent to. See returnurl.go: Zitadel does not validate
// successUrl/failureUrl at all, so this allowlist check is the only thing
// standing between this endpoint and an open redirect — it MUST run before
// StartIDPIntent, never after.
func (h *Handler) idpStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// First, before the body is even read — same discipline as login/totp.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req idpStartRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.ReturnURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// The allowlist check happens on the way IN, before any Zitadel call:
	// successUrl/failureUrl must never be constructed from unvalidated
	// caller input.
	returnURL, err := h.returnURLs.ValidateReturnURL(req.ReturnURL)
	if err != nil {
		slog.WarnContext(ctx, "zitadellogin: idp start rejected: return url not allowed")
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_return_url"})
		return
	}

	if h.googleIDPID == "" {
		slog.ErrorContext(ctx, "zitadellogin: idp start: no google idp id configured (see WithGoogleIDPID)")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// Same validated URL for both outcomes: Zitadel distinguishes success
	// from failure by which query params it appends (id+token vs
	// id+error+error_description — see idpintent.go), so the frontend's one
	// finish route can handle both without needing two allowlisted targets.
	authURL, err := h.c.StartIDPIntent(ctx, h.googleIDPID, returnURL, returnURL)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin: start idp intent failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auth_url": authURL})
}

// idpFinish exchanges the intent id/token the browser carried back from
// Zitadel for the federated identity, creates a Zitadel session from that
// intent, and runs it through the exact same sufficiency + gauntlet +
// m8_session path login/totp use.
func (h *Handler) idpFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// First, before the body is even read: an unauthenticated caller must
	// never reach RetrieveIDPIntent or CreateIDPIntentSession.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req idpFinishRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.AuthRequestID == "" || req.IntentID == "" || req.IntentToken == "" || req.WorkspaceTenant == "" {
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

	// A first-time Google sign-in: this intent has never been linked to any
	// Zitadel user. Register one, pre-linked to this identity, so the very
	// next sign-in resolves ZitadelUserID immediately.
	//
	// The security rule that governs everything in this block: an unlinked
	// federated identity may be attached to an account by email ONLY when
	// the provider asserts that email is verified. Anyone able to register
	// a victim's address at any federated provider would otherwise inherit
	// that victim's existing account — this is an account-takeover
	// primitive, not a convenience feature, and the two checks below (both
	// required, in this order) are what stop it:
	//
	//  1. identity.EmailVerified must be true and identity.Email non-empty.
	//     EmailVerified is read SOFT from the provider's raw claims (see
	//     IDPIdentity's doc) and defaults to false when the claim is
	//     absent — so an absent claim refuses here exactly like an
	//     explicit false, never like "probably fine". This re-derives the
	//     trust decision the brief for this endpoint always required;
	//     registration is what makes it load-bearing rather than moot.
	//  2. No EXISTING Zitadel user already holds that verified email
	//     (FindUserByVerifiedEmail). If one does, this handler refuses
	//     rather than silently creating a second, disconnected account for
	//     the same person — see CreateHumanUserWithIDPLink's doc for why
	//     skipping this check is unsafe. Linking the new identity onto
	//     that EXISTING account (rather than merely refusing) is the
	//     better outcome for that merchant, but is not implemented here:
	//     see this handler's package README for why.
	if identity.ZitadelUserID == "" {
		if identity.Email == "" || !identity.EmailVerified {
			slog.WarnContext(ctx, "zitadellogin: idp finish rejected: unlinked identity with no verified email")
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "email_not_verified"})
			return
		}

		existingUserID, err := h.c.FindUserByVerifiedEmail(ctx, identity.Email)
		if err != nil {
			slog.ErrorContext(ctx, "zitadellogin: idp finish: could not check for an existing account by email", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
			return
		}
		if existingUserID != "" {
			slog.WarnContext(ctx, "zitadellogin: idp finish refused: a verified email already belongs to an existing, unlinked account")
			writeJSON(w, http.StatusConflict, map[string]any{"error": "account_exists_link_required"})
			return
		}

		newUserID, err := h.c.CreateHumanUserWithIDPLink(ctx, identity)
		if err != nil {
			slog.ErrorContext(ctx, "zitadellogin: idp finish: could not register a new user for this identity", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
			return
		}
		slog.InfoContext(ctx, "zitadellogin: idp finish registered a new user for a first-time Google sign-in", "user_id", newUserID)
	}

	sess, err := h.c.CreateIDPIntentSession(ctx, req.IntentID, req.IntentToken)
	if err != nil {
		h.respondSessionCreateError(ctx, w, err)
		return
	}

	// Google sign-in through this endpoint is always federated:
	// forceMfaLocalOnly (which applies only to local/password credentials —
	// see LoginPolicy's doc) must not apply to it, unlike login()'s
	// password path immediately below in this file.
	res, err := h.c.CompleteIfSufficient(ctx, req.AuthRequestID, sess, true)
	h.respondOutcome(w, r, res, err, req.AuthRequestID, sess, req.WorkspaceTenant)
}

// respondIDPIntentError maps RetrieveIDPIntent's errors. Mirrors
// respondSessionCreateError's shape: log the real reason, answer the caller
// with a code that carries no Zitadel error detail.
func (h *Handler) respondIDPIntentError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrIDPIntentInvalid):
		slog.WarnContext(ctx, "zitadellogin: idp finish rejected: intent invalid")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_intent"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin: zitadel unavailable retrieving idp intent", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin: unexpected error retrieving idp intent", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// respondOutcome is shared by login and totp: both end at CompleteIfSufficient
// / CompleteAfterFactor and switch on the same three outcomes.
func (h *Handler) respondOutcome(
	w http.ResponseWriter,
	r *http.Request,
	res Result,
	resErr error,
	authRequestID string,
	sess Session,
	workspaceTenant string,
) {
	ctx := r.Context()
	switch res.Outcome {
	case OutcomeComplete:
		if resErr != nil {
			// Not reachable per CompleteIfSufficient/CompleteAfterFactor's
			// contract — OutcomeComplete is only ever returned with a nil
			// error — but refuse to complete a login on an outcome/error
			// mismatch rather than trust an incoherent result.
			slog.ErrorContext(ctx, "zitadellogin: OutcomeComplete carried a non-nil error, refusing to complete", "err", resErr)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		h.finishComplete(w, r, res, sess, workspaceTenant)

	case OutcomeFactorRequired:
		// No session minted here — this IS the MFA gate. Minting now would
		// defeat the entire reason this package exists.
		writeJSON(w, http.StatusOK, map[string]any{
			"totp_required": true,
			"session_id":    sess.ID,
			"session_token": sess.Token,
		})

	default: // OutcomeHandoff, including the zero value.
		if resErr != nil {
			// finalize() ran, Zitadel returned a positive decision, and the
			// exchange itself failed — a real error, not routine
			// uncertainty. Distinguished from the nil-error case below so
			// an operator can tell "policy was unreadable" apart from
			// "we decided to finalize and it broke".
			slog.ErrorContext(ctx, "zitadellogin: handoff after a failed finalize (positive decision, exchange failed)",
				"err", resErr, "auth_request_id", authRequestID)
		} else {
			slog.InfoContext(ctx, "zitadellogin: handoff (uncollectible factor or unreadable policy/session)",
				"auth_request_id", authRequestID)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"handoff_url":     h.handoffURL(authRequestID),
			"auth_request_id": authRequestID,
		})
	}
}

// finishComplete resolves the subject of the now-sufficient session and runs
// the shared post-identity gauntlet via the injected CompleteFunc.
func (h *Handler) finishComplete(w http.ResponseWriter, r *http.Request, res Result, sess Session, workspaceTenant string) {
	ctx := r.Context()
	// Result carries no subject — re-read it.
	factors, err := h.c.SessionFactors(ctx, sess.ID)
	if err != nil || factors.UserID == "" {
		slog.ErrorContext(ctx, "zitadellogin: could not resolve session subject after a sufficient decision", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// The email MUST come from Zitadel's own record of this session's
	// subject, never from a request body. login_name on /zitadel/login is
	// verified because it is checked against the password in the same call;
	// login_name on /zitadel/totp is NOT verified against anything — a
	// caller who owns valid credentials of their own could submit any
	// address there and, without this resolution, walk away with a session
	// cookie, an audit event, and a new-device sign-in email addressed to a
	// victim of their choosing.
	email, err := h.c.UserEmail(ctx, factors.UserID)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin: could not resolve the verified email for session subject", "err", err, "user_id", factors.UserID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	if h.complete == nil {
		slog.ErrorContext(ctx, "zitadellogin: no CompleteFunc configured")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}
	lc := h.loginContext(r, factors.UserID, email, workspaceTenant)
	cr, err := h.complete(ctx, w, lc)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin: post-identity gauntlet failed", "err", err, "user_id", factors.UserID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// A step-up (MFA or email-OTP) is still outstanding: the gauntlet minted
	// only a pending cookie, not a real session. Answering with callback_url
	// here would tell the browser the login finished when it did not — the
	// exact defect this branch exists to prevent. Mirror the shape
	// autologin's own GIP handler uses for the same two cases so the two
	// providers answer identically.
	if cr.MFARequired || cr.EmailOTPRequired {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"uid":                factors.UserID,
				"email":              email,
				"tenant_id":          workspaceTenant,
				"mfa_required":       cr.MFARequired,
				"email_otp_required": cr.EmailOTPRequired,
			},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"callback_url": res.CallbackURL})
}

// respondSessionCreateError maps CreatePasswordSession's errors.
//
// ErrBadCredentials and ErrUserNotFound MUST produce the identical response —
// collapsing them is the entire point of this function. Which one actually
// happened is logged for operators, never returned to the caller.
func (h *Handler) respondSessionCreateError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials):
		slog.WarnContext(ctx, "zitadellogin: login rejected: bad credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
	case errors.Is(err, ErrUserNotFound):
		slog.WarnContext(ctx, "zitadellogin: login rejected: user not found")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin: zitadel unavailable creating session", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin: unexpected error creating session", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// respondTOTPVerifyError maps VerifyTOTP's errors. Unlike the login step,
// there is no enumeration concern here — the account is already established
// — but the wrong-code case still must never echo Zitadel's error body.
func (h *Handler) respondTOTPVerifyError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials):
		slog.WarnContext(ctx, "zitadellogin: totp rejected: bad code")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_totp"})
	case errors.Is(err, ErrUserNotFound):
		// The session itself vanished/expired between steps.
		slog.WarnContext(ctx, "zitadellogin: totp rejected: session not found")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_totp"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin: zitadel unavailable verifying totp", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin: unexpected error verifying totp", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// handoffURL builds the Aurora-branded hosted login's continuation URL for an
// auth request this page decided it cannot (or should not) finish itself.
// Returns "" when no hosted login base URL was configured; the caller still
// gets auth_request_id back and is not stranded.
func (h *Handler) handoffURL(authRequestID string) string {
	if h.hostedLoginBaseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/ui/v2/login/login?authRequestID=%s", h.hostedLoginBaseURL, url.QueryEscape(authRequestID))
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes)).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
