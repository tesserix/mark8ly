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
	"github.com/google/uuid"

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
	// tokens/tokenRedirectURI power the mobile routes only. Nil leaves
	// them refusing rather than degrading — see WithTokenIssuer.
	tokens           *TokenExchanger
	tokenRedirectURI string
	tokenProjectID   string
	// Mobile email-OTP step-up (mark8ly#686). See WithStepUp.
	codes   CodeVerifier
	pending PendingStore

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

	// orgID scopes FindUserByVerifiedEmail (idp/finish's link path) to the
	// merchant org. Required for that call to run at all — see
	// Client.FindUserByVerifiedEmail's doc for why an unscoped, instance-
	// wide search is unsafe on a shared instance. Set via WithOrgID.
	orgID string
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

// WithOrgID sets the merchant org id that idp/finish's link path scopes
// Client.FindUserByVerifiedEmail to. See the orgID field doc for why an
// unset value refuses that lookup rather than searching instance-wide.
func (h *Handler) WithOrgID(id string) *Handler {
	h.orgID = id
	return h
}

// WithTokenIssuer enables the mobile routes, which answer a completed
// login with OAuth tokens instead of a callback_url.
//
// The mobile app keeps mark8ly's own login form rather than being sent to
// Zitadel's hosted login, so it posts credentials here — but it then has
// to call marketplace-api, which verifies a BEARER JWT and cannot use a
// session cookie or a callback URL. redirectURI must byte-match the one
// the auth request was created with.
//
// Unset, the mobile routes fail closed with 500 rather than falling back
// to callback_url: that fallback would look like a successful login right
// up until the first API call 401s, which is the hardest possible failure
// to diagnose from a device.
func (h *Handler) WithTokenIssuer(ex *TokenExchanger, redirectURI, adminProjectID string) *Handler {
	h.tokens = ex
	h.tokenRedirectURI = redirectURI
	h.tokenProjectID = adminProjectID
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

	// Mobile surface (#686). Same handlers, same internal-auth gate, same
	// gauntlet — they differ ONLY in answering a completed login with
	// tokens, because marketplace-api verifies a bearer JWT and a native
	// client can use neither a session cookie nor a callback_url. Mounted
	// as separate routes rather than a request flag so a web caller can
	// never opt itself into token issuance.
	r.POST("/zitadel/mobile/login", func(c *gin.Context) {
		h.mobileLogin(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})
	r.POST("/zitadel/mobile/totp", func(c *gin.Context) {
		h.mobileTOTP(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})
	r.POST("/zitadel/mobile/otp/verify", func(c *gin.Context) {
		h.mobileOTPVerify(c.Writer, withClientIP(c.Request, c.ClientIP()))
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
	r.POST("/zitadel/idp/complete", func(c *gin.Context) {
		h.idpComplete(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})

	// The same three steps for the native app (#686 item 1). idp/start is
	// literally the same handler — a start has no completion tail to
	// differ in — while finish/complete run in token-issuing mode. They
	// share the merchant Handler, and therefore the ADMIN return-URL
	// allowlist; wiring them anywhere else would hand a completed admin
	// sign-in to a merchant-controlled storefront host (see
	// cmd/server/main.go's newZitadelHandlers and its swap test).
	r.POST("/zitadel/mobile/idp/start", func(c *gin.Context) {
		h.idpStart(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})
	r.POST("/zitadel/mobile/idp/finish", func(c *gin.Context) {
		h.mobileIDPFinish(c.Writer, withClientIP(c.Request, c.ClientIP()))
	})
	r.POST("/zitadel/mobile/idp/complete", func(c *gin.Context) {
		h.mobileIDPComplete(c.Writer, withClientIP(c.Request, c.ClientIP()))
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
	h.loginMode(w, r, false)
}

// mobileLogin is login with one difference: a completed login answers with
// tokens. It shares the whole path deliberately — the credential
// enumeration collapse, resolving the subject from Zitadel rather than the
// request body, the gauntlet, and the step-up gate are all security
// properties that must not be reimplemented per surface.
func (h *Handler) mobileLogin(w http.ResponseWriter, r *http.Request) {
	h.loginMode(w, r, true)
}

func (h *Handler) loginMode(w http.ResponseWriter, r *http.Request, issueTokens bool) {
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
	// A mobile caller has no browser to be redirected through
	// /oauth/v2/authorize, so it cannot obtain an auth_request_id and the
	// server mints one for it. Web callers still must supply theirs: the
	// browser already has one, and creating a second would orphan the
	// flow the user is actually in.
	if issueTokens && req.AuthRequestID == "" {
		id, err := h.newAuthRequest(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "zitadellogin: could not create an auth request for mobile login", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		req.AuthRequestID = id
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
	h.respondOutcome(w, r, res, err, req.AuthRequestID, sess, req.WorkspaceTenant, issueTokens)
}

// totp reads {auth_request_id, login_name, session_id, session_token, code,
// workspace_tenant}, submits the TOTP code against the session opened by
// login, and re-asks sufficiency.go whether the session may now finalize.
func (h *Handler) totp(w http.ResponseWriter, r *http.Request) {
	h.totpMode(w, r, false)
}

// mobileTOTP is totp for the mobile surface: same verification, tokens
// instead of a callback_url on completion.
func (h *Handler) mobileTOTP(w http.ResponseWriter, r *http.Request) {
	h.totpMode(w, r, true)
}

func (h *Handler) totpMode(w http.ResponseWriter, r *http.Request, issueTokens bool) {
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
	h.respondOutcome(w, r, res, err, req.AuthRequestID, sess, req.WorkspaceTenant, issueTokens)
}

type idpStartRequest struct {
	ReturnURL string `json:"return_url"`
	// Provider names WHICH federated IDP to open an intent against.
	// Empty means Google, so the web callers — which predate this field —
	// keep working byte-for-byte unchanged. See idpIDForProvider.
	Provider string `json:"provider"`
}

// providerGoogle is the only federated provider this handler accepts.
// Named rather than inlined so a later provider (Apple is provisioned on
// the same Zitadel org but never exercised — see README.md) is added by
// extending idpIDForProvider's switch and nothing else.
const providerGoogle = "google"

var (
	// errUnsupportedIDPProvider means the caller named a provider this
	// handler will not open an intent against. A CLIENT error: the caller
	// asked for something that does not exist here.
	errUnsupportedIDPProvider = errors.New("zitadellogin: unsupported idp provider")
	// errIDPProviderNotConfigured means the provider is supported but its
	// Zitadel IDP id was never configured. A SERVER error: the deployment
	// is incomplete, not the request.
	errIDPProviderNotConfigured = errors.New("zitadellogin: no idp id configured for provider")
)

// idpIDForProvider resolves the provider a caller named to the single
// Zitadel IDP id an intent is allowed to have come from.
//
// The pin is provider-SELECTED, never "any IDP that happens to be
// configured on this handler". Those look the same while exactly one IDP
// exists and diverge dangerously the moment a second one does: accepting
// whichever provider an intent carries would let an attacker start an
// intent against the WEAKER provider, register victim@merchant.com there,
// and have idpFinish trust that provider's email_verified claim exactly
// like Google's — which is an account-takeover primitive, not a
// convenience. An Apple IDP already exists on the same org (README.md), so
// this is not hypothetical. Adding a provider here is therefore a
// deliberate act of trusting it, one switch case at a time.
func (h *Handler) idpIDForProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", providerGoogle:
		if h.googleIDPID == "" {
			return "", errIDPProviderNotConfigured
		}
		return h.googleIDPID, nil
	default:
		return "", errUnsupportedIDPProvider
	}
}

// respondIDPProviderError maps idpIDForProvider's two errors onto the
// client/server split they describe. Shared by idpStart and idpFinish so
// the two cannot drift into answering the same condition differently.
func (h *Handler) respondIDPProviderError(ctx context.Context, w http.ResponseWriter, err error) {
	if errors.Is(err, errUnsupportedIDPProvider) {
		slog.WarnContext(ctx, "zitadellogin: idp request rejected: unsupported provider")
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported_provider"})
		return
	}
	slog.ErrorContext(ctx, "zitadellogin: idp request: no idp id configured for the requested provider (see WithGoogleIDPID)", "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
}

// idpFinishRequest is what the frontend's own finish route (the target of
// idpStart's successUrl) posts back here after Zitadel redirects the
// browser to it.
type idpFinishRequest struct {
	AuthRequestID   string `json:"auth_request_id"`
	IntentID        string `json:"intent_id"`
	IntentToken     string `json:"intent_token"`
	WorkspaceTenant string `json:"workspace_tenant"`

	// Provider must name the SAME provider idp/start was called with.
	// Empty means Google, matching idpStartRequest.Provider. It selects
	// which IDP id the retrieved intent is pinned against — it never
	// widens what is accepted, because an unknown value is refused rather
	// than falling back. See idpIDForProvider.
	Provider string `json:"provider"`

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

	idpID, err := h.idpIDForProvider(req.Provider)
	if err != nil {
		h.respondIDPProviderError(ctx, w, err)
		return
	}

	// Same validated URL for both outcomes: Zitadel distinguishes success
	// from failure by which query params it appends (id+token vs
	// id+error+error_description — see idpintent.go), so the frontend's one
	// finish route can handle both without needing two allowlisted targets.
	authURL, err := h.c.StartIDPIntent(ctx, idpID, returnURL, returnURL)
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
	h.idpFinishMode(w, r, false)
}

// mobileIDPFinish is idpFinish for the mobile surface. It differs in
// EXACTLY the two ways loginMode already differs for a native client: an
// auth_request_id is minted here because there is no browser round trip
// through /oauth/v2/authorize to obtain one, and a completed sign-in
// answers with tokens instead of a callback_url. Every trust decision —
// the IDP pin, the verified-email rule, link-only, the gauntlet — is the
// same code, deliberately: a second implementation of this path is a
// second place for an account-takeover bug to live.
func (h *Handler) mobileIDPFinish(w http.ResponseWriter, r *http.Request) {
	h.idpFinishMode(w, r, true)
}

func (h *Handler) idpFinishMode(w http.ResponseWriter, r *http.Request, issueTokens bool) {
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
	// workspace_tenant is deliberately NOT required here (unlike login/totp):
	// with Google, the merchant's identity is unknown until after the
	// redirect back from Zitadel, so the admin app cannot know which tenant
	// to send until this call has told it who signed in. See the
	// tenant_required branch below.
	// See loginMode: a native client has no browser redirect through
	// /oauth/v2/authorize, so it cannot obtain an auth_request_id and the
	// server mints one for it. Web callers still must supply theirs — the
	// browser already has one, and creating a second would orphan the
	// flow the user is actually in.
	if issueTokens && req.AuthRequestID == "" {
		id, err := h.newAuthRequest(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "zitadellogin: could not create an auth request for mobile idp finish", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		req.AuthRequestID = id
	}

	if req.AuthRequestID == "" || req.IntentID == "" || req.IntentToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// Resolved BEFORE RetrieveIDPIntent: an unsupported provider is a
	// refusal this endpoint can make without touching Zitadel at all, and
	// the pin below must compare against a value chosen by the request,
	// never against "whatever the intent brought".
	expectedIDPID, err := h.idpIDForProvider(req.Provider)
	if err != nil {
		h.respondIDPProviderError(ctx, w, err)
		return
	}

	// The ONLY source of identity for this endpoint. req.User (see its doc
	// comment above) is never consulted, here or anywhere below.
	identity, err := h.c.RetrieveIDPIntent(ctx, req.IntentID, req.IntentToken)
	if err != nil {
		h.respondIDPIntentError(ctx, w, err)
		return
	}

	// This endpoint is Google sign-in specifically, not "any federated
	// identity Zitadel happens to have" — the instance can (and, as of
	// 2026-09-04, does) carry more than one IDP. Without this check, an
	// intent from a WEAKER or attacker-influenced provider would be
	// accepted here and its email_verified claim trusted exactly like
	// Google's: start an intent against that other provider, register
	// victim@merchant.com there, and this endpoint would link it straight
	// onto the victim's Zitadel account. Every trust decision below this
	// line assumes "Google asserted this" — this is what actually makes
	// that assumption true, rather than merely convenient to write.
	if identity.IDPID == "" || identity.IDPID != expectedIDPID {
		slog.WarnContext(ctx, "zitadellogin: idp finish rejected: intent did not come from the idp the caller named")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unexpected_idp"})
		return
	}

	// A first-time Google sign-in: this intent has never been linked to any
	// Zitadel user. The merchant path is LINK-ONLY — it never registers a
	// new user. Two independent reasons, not one:
	//
	//   - Merchant authorization is FGA tenant membership, keyed by user
	//     id. A user that did not exist a moment ago cannot be a member of
	//     anything, so a freshly created merchant user is GUARANTEED to
	//     fail the post-identity gauntlet a few lines below. Creating one
	//     here is pure garbage generation: unbounded user-table growth by
	//     any unauthenticated visitor with a Google account, and every row
	//     becomes a future verified-email match target — see
	//     FindUserByVerifiedEmail's doc on ambiguous matches.
	//   - Merchants get an account through onboarding, not through the
	//     login page. If no admin account exists for this identity, the
	//     correct answer is a clean refusal, not a bootstrap.
	//
	// (CreateHumanUserWithIDPLink still exists as a primitive for a future
	// customer path, where self-registration IS the desired behaviour —
	// see the storefront customer sign-in design — it is simply never
	// called from here.)
	//
	// The security rule that governs the link decision: an unlinked
	// federated identity may be attached to an account by email ONLY when
	// the provider asserts that email is verified. Anyone able to register
	// a victim's address at any federated provider would otherwise inherit
	// that victim's existing account — this is an account-takeover
	// primitive, not a convenience feature, and the check below is what
	// stops it. identity.EmailVerified is read SOFT from the provider's
	// raw claims (see IDPIdentity's doc) and defaults to false when the
	// claim is absent — so an absent claim refuses here exactly like an
	// explicit false, never like "probably fine".
	if identity.ZitadelUserID == "" {
		if identity.Email == "" || !identity.EmailVerified {
			slog.WarnContext(ctx, "zitadellogin: idp finish rejected: unlinked identity with no verified email")
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "email_not_verified"})
			return
		}

		existingUserID, err := h.c.FindUserByVerifiedEmail(ctx, h.orgID, identity.Email)
		if err != nil {
			if errors.Is(err, ErrAmbiguousEmailMatch) {
				// More than one user in the org matched — a refusal, not
				// an error to retry: see FindUserByVerifiedEmail's doc for
				// why picking one would be unsafe.
				slog.WarnContext(ctx, "zitadellogin: idp finish rejected: more than one existing account matched the verified email")
				writeJSON(w, http.StatusConflict, map[string]any{"error": "email_ambiguous"})
				return
			}
			slog.ErrorContext(ctx, "zitadellogin: idp finish: could not check for an existing account by email", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
			return
		}
		if existingUserID == "" {
			// No admin account exists for this identity. Merchants are
			// provisioned through onboarding, not created here — see the
			// link-only rationale above.
			slog.WarnContext(ctx, "zitadellogin: idp finish rejected: no admin account exists for this identity")
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "no_admin_account"})
			return
		}

		// A Zitadel user already holds this exact, verified email: attach
		// this Google identity to THAT account. LinkIDPToUser re-checks
		// the same email-verified rule independently rather than trusting
		// this call site alone — this is the whole security boundary:
		// linking an unverified provider email to an existing account is
		// account takeover.
		if err := h.c.LinkIDPToUser(ctx, existingUserID, identity); err != nil {
			slog.ErrorContext(ctx, "zitadellogin: idp finish: could not link identity to the existing account", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
			return
		}
		slog.InfoContext(ctx, "zitadellogin: idp finish linked a first-time Google identity to an existing account", "user_id", existingUserID)
	}

	sess, err := h.c.CreateIDPIntentSession(ctx, req.IntentID, req.IntentToken)
	if err != nil {
		h.respondIDPSessionCreateError(ctx, w, err)
		return
	}

	// The admin app cannot supply workspace_tenant on this call: with
	// Google, which tenant the merchant belongs to is unknowable until AFTER
	// this retrieve/pin/verify/link/session-create sequence has told it who
	// signed in. Everything above this line — including creating the
	// Zitadel session — still ran; only CompleteIfSufficient/finalize (which
	// would mint an m8_session for a tenant not yet chosen) is deferred.
	// Mirrors OutcomeFactorRequired's totp_required shape below: a session
	// exists, something else (here, tenant selection) is still needed
	// before completion. login_name is identity.Email — the email from the
	// retrieved identity — never anything the caller supplied.
	if req.WorkspaceTenant == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_required": true,
			"session_id":      sess.ID,
			"session_token":   sess.Token,
			"login_name":      identity.Email,
		})
		return
	}

	// Google sign-in through this endpoint is always federated:
	// forceMfaLocalOnly (which applies only to local/password credentials —
	// see LoginPolicy's doc) must not apply to it, unlike login()'s
	// password path immediately below in this file.
	// issueTokens is the mode parameter this comment used to promise: the
	// mobile surface answers a completed Google sign-in with tokens, the
	// web surface with a callback_url, and NOTHING else differs.
	res, err := h.c.CompleteIfSufficient(ctx, req.AuthRequestID, sess, true)
	h.respondOutcome(w, r, res, err, req.AuthRequestID, sess, req.WorkspaceTenant, issueTokens)
}

// idpCompleteRequest is what the admin app posts once it has resolved which
// tenant a Google-authenticated merchant should land in — the counterpart to
// idpFinish's tenant_required response, which hands back exactly
// session_id/session_token/login_name for this purpose. Same shape as
// totpRequest minus Code: there is no second factor to check here, only a
// tenant to complete with.
type idpCompleteRequest struct {
	AuthRequestID   string `json:"auth_request_id"`
	LoginName       string `json:"login_name"`
	SessionID       string `json:"session_id"`
	SessionToken    string `json:"session_token"`
	WorkspaceTenant string `json:"workspace_tenant"`
}

// idpComplete reads {auth_request_id, login_name, session_id, session_token,
// workspace_tenant} and completes a Google sign-in from a session idp/finish
// already created — the admin app calls this once it knows which tenant the
// merchant should land in. It is totp's completion path minus the code
// check: there is no second factor to verify here, so it goes straight to
// the same sufficiency decision idpFinish's own tail call makes (federated
// = true, since every session reaching this endpoint was created by
// CreateIDPIntentSession, never CreatePasswordSession) and the shared
// respondOutcome/finishComplete path login, totp and idpFinish all end at.
// Deliberately NOT a parallel completion implementation — see the package
// doc on CompleteFunc for why that would be a mistake.
//
// req.LoginName is read only for the same required-field discipline totp
// applies to it and is never used past validation: see finishComplete's doc
// on why the subject's email is always re-resolved from Zitadel rather than
// trusted from the request body.
//
// This endpoint accepts a session_id/session_token pair from its caller and
// completes a login from it without any further binding to the original
// caller. That is safe because: (1) it sits behind the same internalauth
// guard as login/totp/idpFinish, so only the admin app's own backend can
// reach it at all; (2) session_token is a high-entropy secret Zitadel mints
// and hands back only in idpFinish's tenant_required response — it is not
// guessable, and possessing it is already Zitadel's own definition of
// controlling that session; and (3) totp already accepts this exact pair
// with no additional binding, for the same reason. Adding a second
// binding (e.g. requiring the caller to also prove it saw idpFinish's
// response) would duplicate a guarantee Zitadel's session token already
// provides and diverge from totp's precedent for no additional safety.
func (h *Handler) idpComplete(w http.ResponseWriter, r *http.Request) {
	h.idpCompleteMode(w, r, false)
}

// mobileIDPComplete is idpComplete for the mobile surface: same session,
// same sufficiency decision, same gauntlet — tokens instead of a
// callback_url, and a minted auth_request_id, for the reasons
// mobileIDPFinish states.
func (h *Handler) mobileIDPComplete(w http.ResponseWriter, r *http.Request) {
	h.idpCompleteMode(w, r, true)
}

func (h *Handler) idpCompleteMode(w http.ResponseWriter, r *http.Request, issueTokens bool) {
	ctx := r.Context()

	// See login/totp/idpFinish: reject before any Zitadel call.
	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}

	var req idpCompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	// See loginMode/idpFinishMode: minted for a native client, required
	// from a browser caller.
	if issueTokens && req.AuthRequestID == "" {
		id, err := h.newAuthRequest(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "zitadellogin: could not create an auth request for mobile idp complete", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		req.AuthRequestID = id
	}

	if req.AuthRequestID == "" || req.LoginName == "" || req.SessionID == "" || req.SessionToken == "" ||
		req.WorkspaceTenant == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	sess := Session{ID: req.SessionID, Token: req.SessionToken}

	// federated=true: mirrors idpFinish's own tail call (see that function's
	// comment on forceMfaLocalOnly) — a session reaching this endpoint is
	// always the product of a Google IDP intent, never a password login.
	res, err := h.c.CompleteIfSufficient(ctx, req.AuthRequestID, sess, true)
	h.respondOutcome(w, r, res, err, req.AuthRequestID, sess, req.WorkspaceTenant, issueTokens)
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

// respondIDPSessionCreateError maps CreateIDPIntentSession's errors.
//
// This exists because respondSessionCreateError — which this call site used
// to share — maps CreatePasswordSession's errors, and do() turns ANY Zitadel
// 400 into ErrBadCredentials (see client.go's requestOptions default). On the
// Google path that produced two lies at once: the operator saw
// "login rejected: bad credentials" for a flow in which no credential was
// ever presented, and the merchant got `invalid_credentials`, which
// google-sign-in-admin.ts states as a hard rule must never happen here —
// "no outcome may imply the Google credential itself was wrong (it never is:
// every failure here is either a platform-side account/authorization
// decision or an availability problem, never a bad password)". Observed in
// production 2026-09-06: a merchant with a correctly linked Google identity
// was told to check their details.
//
// Unlike the password path, every branch logs `err`. The wrapped error
// carries Zitadel's own error id (do() formats it in, e.g. "COMMAND-3M0fs")
// and nothing else — no credential, no token. On the password path a bare
// "bad credentials" is a complete explanation; here it is the only clue
// there is, and discarding it is what made this cost an afternoon to find.
func (h *Handler) respondIDPSessionCreateError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials), errors.Is(err, ErrUserNotFound):
		// Zitadel refused the intent -> session exchange: consumed,
		// expired, or not linked to a user. Same answer as a bad intent
		// on retrieve, because to the caller it is the same situation.
		slog.WarnContext(ctx, "zitadellogin: idp finish rejected: could not create a session from the intent", "err", err)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_intent"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin: zitadel unavailable creating a session from the idp intent", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin: unexpected error creating a session from the idp intent", "err", err)
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
	issueTokens bool,
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
		h.finishComplete(w, r, res, sess, workspaceTenant, issueTokens)

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
func (h *Handler) finishComplete(w http.ResponseWriter, r *http.Request, res Result, sess Session, workspaceTenant string, issueTokens bool) {
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
		out := map[string]any{
			"uid":                factors.UserID,
			"email":              email,
			"tenant_id":          workspaceTenant,
			"mfa_required":       cr.MFARequired,
			"email_otp_required": cr.EmailOTPRequired,
		}
		// A native client cannot resume from the pending COOKIE the
		// gauntlet just set, so it gets the same state sealed into a value
		// it hands back to /mobile/otp/verify. The Zitadel session travels
		// with it, because the challenge has to re-finalize this session to
		// mint a token — the login's own authorization code is discarded on
		// this branch.
		//
		// Refuse rather than answer with a challenge the client could never
		// complete: a step-up the app cannot finish is a dead end that
		// looks like a working login.
		if issueTokens {
			token, err := h.mintPendingLogin(factors.UserID, email, workspaceTenant, sess)
			if err != nil {
				slog.ErrorContext(ctx, "zitadellogin: could not mint the mobile step-up token", "err", err, "user_id", factors.UserID)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
				return
			}
			out["pending_token"] = token
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
		return
	}
	if issueTokens {
		h.respondTokens(w, r, res, factors.UserID, email, workspaceTenant)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"callback_url": res.CallbackURL})
}

// newAuthRequest starts an OIDC flow server-side for a mobile login.
//
// The state parameter is not used for CSRF here: there is no browser
// round-trip to protect — the same process creates the request, drives the
// login and exchanges the code — so it carries a random value purely to
// satisfy the parameter.
func (h *Handler) newAuthRequest(ctx context.Context) (string, error) {
	if h.tokens == nil {
		return "", fmt.Errorf("zitadellogin: no token issuer configured")
	}
	return h.tokens.CreateAuthRequest(ctx, h.tokenRedirectURI, h.tokenProjectID, uuid.NewString())
}

// respondTokens exchanges the completed login's authorization code for
// OAuth tokens and returns them to the mobile client.
//
// Reached only after the full gauntlet has passed with no outstanding
// step-up, so by here the login IS complete — the exchange is the last
// step, not another gate.
func (h *Handler) respondTokens(w http.ResponseWriter, r *http.Request, res Result, uid, email, workspaceTenant string) {
	ctx := r.Context()
	if h.tokens == nil {
		slog.ErrorContext(ctx, "zitadellogin: mobile login completed but no token issuer is configured")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	code, err := CodeFromCallbackURL(res.CallbackURL)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin: completed login produced no usable code", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	tok, err := h.tokens.ExchangeCodeForTokens(ctx, code, h.tokenRedirectURI)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin: token exchange failed after a completed login", "err", err, "user_id", uid)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// The callback_url is deliberately NOT included: a mobile client has
	// no use for it, and it carries a live authorization code.
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"uid":           uid,
			"email":         email,
			"tenant_id":     workspaceTenant,
			"access_token":  tok.AccessToken,
			"refresh_token": tok.RefreshToken,
			"token_type":    tok.TokenType,
			"expires_in":    tok.ExpiresIn,
		},
	})
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
