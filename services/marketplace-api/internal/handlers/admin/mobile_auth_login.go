package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/authbffclient"
	"github.com/mark8ly/marketplace-api/internal/teamproxy"
)

// MobileLoginBackend is the slice of authbffclient.MobileLoginClient this
// handler needs.
type MobileLoginBackend interface {
	Login(ctx context.Context, email, password, workspaceTenant string) (authbffclient.LoginResult, error)
	VerifyOTP(ctx context.Context, pendingToken, code string) (authbffclient.LoginResult, error)
	VerifyTOTP(ctx context.Context, pendingToken, code string) (authbffclient.LoginResult, error)
}

// MobileLoginHandler is the app's public front door for signing in.
//
// # Why the front door is here and not on auth-bff
//
// The app keeps mark8ly's own login form rather than being sent to
// Zitadel's hosted login, so credentials have to reach a public endpoint.
// auth-bff is internet-reachable at auth.mark8ly.com and the ONLY thing
// protecting its login route from credential stuffing is the
// X-Internal-Auth secret its trusted server-side callers hold — which a
// device cannot hold. Adding an unauthenticated route there would remove
// that protection for every surface at once.
//
// marketplace-api is already the app's public backend, already holds the
// identical secret, and already has IP rate limiting. So the public
// surface lives here and auth-bff keeps its gate.
//
// # Why it resolves the tenant first
//
// auth-bff requires a workspace_tenant, and the client cannot know one: a
// Zitadel token carries no tenant claim, and tenant discovery itself needs
// a token. So the tenant is resolved from the EMAIL before authenticating
// — exactly what the web does in resolveWorkspaceTenant, and why
// platform-api's lookup accepts an email as well as a uid.
type MobileLoginHandler struct {
	lister  TenantLister
	backend MobileLoginBackend
	log     *slog.Logger
}

func NewMobileLoginHandler(lister TenantLister, backend MobileLoginBackend, log *slog.Logger) *MobileLoginHandler {
	return &MobileLoginHandler{lister: lister, backend: backend, log: log}
}

type mobileLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login authenticates and, on success, returns bearer tokens.
//
// A step-up (email OTP or TOTP) is a SUCCESSFUL outcome reported in the
// body, not an error. Treating it as failure is what produced the silent
// redirect loop in #493, where every layer logged 200 and the user was
// bounced back to the login screen with nothing shown.
func (h *MobileLoginHandler) Login(c *gin.Context) {
	var req mobileLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "email and password are required"})
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || req.Password == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "email and password are required"})
		return
	}

	tenants, err := h.lister.ListMyTenants(c.Request.Context(), email)
	if err != nil {
		h.logError("mobile login: tenant lookup failed", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "platform_unavailable", "message": "could not look up your stores"})
		return
	}
	if len(tenants) == 0 {
		// Refused WITHOUT sending the password anywhere: there is no
		// workspace_tenant to authenticate against, so probing the
		// credential would be exposure for no result.
		//
		// This does reveal that an address has no membership. It is the
		// same signal the web sign-in already gives, and the alternative
		// (authenticate, then refuse) would check credentials for
		// arbitrary addresses on an unauthenticated endpoint.
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error":   "no_store",
			"message": "We couldn't find a store for this account. Did you finish onboarding?",
		})
		return
	}

	// Authenticate against the first membership. The full list is returned
	// so a merchant on more than one tenant can switch, rather than being
	// silently pinned to whichever one happened to sort first.
	primary := tenants[0].TenantID

	res, err := h.backend.Login(c.Request.Context(), email, req.Password, primary)
	switch {
	case errors.Is(err, authbffclient.ErrInvalidCredentials):
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials", "message": "Couldn't sign you in. Check your details and try again."})
		return
	case err != nil:
		// Deliberately NOT 401: a merchant typing a correct password
		// during an auth-bff outage would otherwise retry forever.
		h.logError("mobile login: auth-bff call failed", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "auth_unavailable", "message": "Sign-in is temporarily unavailable. Try again shortly."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mobileLoginResponse(res, tenants)})
}

func mobileLoginResponse(res authbffclient.LoginResult, tenants []teamproxy.TenantMembership) gin.H {
	// nil on the OTP path: the tenant was already resolved and returned by
	// the login call, and re-deriving it would mean a second lookup for a
	// value the client already holds.
	if tenants == nil {
		tenants = []teamproxy.TenantMembership{}
	}
	out := gin.H{
		"uid":                res.UID,
		"email":              res.Email,
		"tenant_id":          res.TenantID,
		"tenants":            tenants,
		"email_otp_required": res.EmailOTPRequired,
		"mfa_required":       res.MFARequired,
		"totp_required":      res.TOTPRequired,
	}
	// Tokens are present only when no step-up is outstanding; auth-bff
	// guarantees that, and this mirrors it rather than re-deciding.
	if res.AccessToken != "" {
		out["access_token"] = res.AccessToken
		out["refresh_token"] = res.RefreshToken
		out["token_type"] = res.TokenType
		out["expires_in"] = res.ExpiresIn
	}
	// Opaque to us; the client hands it straight back to /otp/verify.
	if res.PendingToken != "" {
		out["pending_token"] = res.PendingToken
	}
	// Only when auth-bff actually sent them. Since #686 item 2 the mobile
	// TOTP gate answers with a sealed pending_token INSTEAD of these
	// handles — the app resumes at /auth/totp/verify — so emitting two
	// empty strings here would advertise a resumption route that does not
	// work.
	if res.SessionID != "" {
		out["session_id"] = res.SessionID
		out["session_token"] = res.SessionToken
	}
	return out
}

type mobileOTPRequest struct {
	PendingToken string `json:"pending_token"`
	Code         string `json:"code"`
}

// VerifyOTP completes a sign-in that stopped at the email-OTP gate.
//
// This is the COMMON first-login path on mobile: a fresh install is always
// an unrecognised device, so the very first sign-in on a new phone lands
// here rather than getting tokens straight away.
//
// No tenant lookup and no password: the sealed pending_token already
// carries the identity and tenant the login resolved, and auth-bff derives
// them from it rather than from anything the client sends.
func (h *MobileLoginHandler) VerifyOTP(c *gin.Context) {
	var req mobileOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PendingToken == "" || req.Code == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "pending_token and code are required",
		})
		return
	}

	res, err := h.backend.VerifyOTP(c.Request.Context(), req.PendingToken, req.Code)
	switch {
	case errors.Is(err, authbffclient.ErrInvalidCredentials):
		// Covers both a wrong code and an expired/forged challenge —
		// auth-bff answers those identically on purpose, and flattening
		// them here keeps that property rather than re-deriving it.
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "invalid_code", "message": "That code isn't right, or it has expired. Request a new one.",
		})
		return
	case err != nil:
		// The code may well have been correct. Saying so matters: a user
		// told "wrong code" will retype a correct one forever.
		h.logError("mobile otp verify: auth-bff call failed", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "auth_unavailable", "message": "Sign-in is temporarily unavailable. Try again shortly.",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mobileLoginResponse(res, nil)})
}

// VerifyTOTP completes a sign-in that stopped at the TOTP gate.
//
// Same shape as VerifyOTP, and deliberately so: both step-ups resume from
// the one sealed pending_token, so the app has a single mechanism rather
// than a second one built out of raw Zitadel session handles.
//
// Before this existed a merchant with an authenticator app was locked out
// of the app entirely — the login answered `totp_required` with nothing
// the client could resume from, and the app rendered that as "this app
// version needs an update", which no update could ever fix (#686 item 2).
func (h *MobileLoginHandler) VerifyTOTP(c *gin.Context) {
	var req mobileOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PendingToken == "" || req.Code == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "pending_token and code are required",
		})
		return
	}

	res, err := h.backend.VerifyTOTP(c.Request.Context(), req.PendingToken, req.Code)
	switch {
	case errors.Is(err, authbffclient.ErrInvalidCredentials):
		// `invalid_totp`, NOT `invalid_credentials`: the screen this
		// answers has no password field on it, so password copy there is
		// advice a merchant cannot act on. As on the OTP path it also
		// covers an expired or forged challenge, which auth-bff answers
		// identically on purpose.
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "invalid_totp", "message": "That code isn't right, or it has expired. Try the next one from your authenticator app.",
		})
		return
	case err != nil:
		// The code may well have been correct — a six-digit code rolls
		// every 30 seconds, and telling a merchant it was wrong sends them
		// round a loop that cannot end.
		h.logError("mobile totp verify: auth-bff call failed", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "auth_unavailable", "message": "Sign-in is temporarily unavailable. Try again shortly.",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mobileLoginResponse(res, nil)})
}

func (h *MobileLoginHandler) logError(msg string, err error) {
	if h.log != nil {
		h.log.Error(msg, "error", err)
	}
}
