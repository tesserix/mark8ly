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
