package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/authbffclient"
)

// MobileIDPBackend is the slice of authbffclient.MobileLoginClient the
// federated sign-in routes need.
type MobileIDPBackend interface {
	IDPStart(ctx context.Context, provider, returnURL string) (string, error)
	IDPFinish(ctx context.Context, provider, intentID, intentToken string) (authbffclient.IDPFinishResult, error)
	IDPComplete(ctx context.Context, loginName, sessionID, sessionToken, workspaceTenant string) (authbffclient.LoginResult, error)
}

// MobileIDPHandler is the app's public front door for "Continue with
// Google" (#686 item 1).
//
// # Why it lives here and not on auth-bff
//
// Identical to MobileLoginHandler's reasoning: auth-bff's Zitadel routes
// are gated on the X-Internal-Auth secret that only trusted server-side
// callers hold, and a device cannot hold one. marketplace-api already is
// that trusted caller and already IP-rate-limits its public surface.
//
// # Why the return URL is built here, from configuration
//
// Zitadel does not validate an IDP intent's successUrl at all — auth-bff's
// allowlist is the entire control against handing a completed admin
// sign-in to somebody else's origin. Accepting a return URL from the
// device would make that allowlist the sole reviewer of attacker-supplied
// input; building it server-side means a device can only ever ask "start
// Google", never "start Google and send the result over there".
//
// # Why the return URL is an https page and not the app's own scheme
//
// auth-bff's ValidateReturnURL requires https and an allowlisted host, so
// a custom scheme cannot be the Zitadel return URL at all. The configured
// page is a bridge on the admin web app that 302s to
// mark8ly-admin://auth/idp with the query preserved, which is what the
// app's authentication session intercepts.
type MobileIDPHandler struct {
	lister    TenantLister
	backend   MobileIDPBackend
	returnURL string
	log       *slog.Logger
}

func NewMobileIDPHandler(lister TenantLister, backend MobileIDPBackend, returnURL string, log *slog.Logger) *MobileIDPHandler {
	return &MobileIDPHandler{lister: lister, backend: backend, returnURL: returnURL, log: log}
}

type mobileIDPStartRequest struct {
	Provider string `json:"provider"`
}

type mobileIDPFinishRequest struct {
	Provider    string `json:"provider"`
	IntentID    string `json:"intent_id"`
	IntentToken string `json:"intent_token"`
}

// supportedIDPProviders is the allowlist both routes check. An unknown
// value is refused here AND again in auth-bff — adding a provider must be a
// deliberate change in both, never something a request opts into.
var supportedIDPProviders = map[string]string{
	authbffclient.ProviderGoogle: "Google",
	authbffclient.ProviderApple:  "Apple",
}

// normalizeIDPProvider canonicalises what a request named and reports
// whether this surface trusts it.
func normalizeIDPProvider(raw string) (string, bool) {
	provider := strings.ToLower(strings.TrimSpace(raw))
	_, ok := supportedIDPProviders[provider]
	return provider, ok
}

// providerLabel is the merchant-facing name for a provider. It falls back
// to Google because an absent provider means Google everywhere here.
func providerLabel(provider string) string {
	if label, ok := supportedIDPProviders[provider]; ok {
		return label
	}
	return supportedIDPProviders[authbffclient.ProviderGoogle]
}

// Start returns the URL the app opens in its authentication session.
func (h *MobileIDPHandler) Start(c *gin.Context) {
	var req mobileIDPStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "provider is required"})
		return
	}
	// Start has ALWAYS required an explicit provider and every shipped
	// client sends one, so an empty value stays a 400 here. Finish is
	// deliberately different — see below.
	provider, ok := normalizeIDPProvider(req.Provider)
	if !ok {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "unsupported_provider", "message": "That sign-in method isn't available.",
		})
		return
	}
	if h.returnURL == "" {
		// Fail loudly: an empty return URL would be rejected by auth-bff's
		// allowlist anyway, and reporting it as a bad request would send a
		// merchant chasing their own account for a deploy problem.
		h.logError("mobile idp start: no return url configured", errors.New("empty return url"))
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "auth_unavailable", "message": "Sign-in is temporarily unavailable. Try again shortly.",
		})
		return
	}

	authURL, err := h.backend.IDPStart(c.Request.Context(), provider, h.returnURL)
	if err != nil {
		h.respondIDPError(c, provider, "mobile idp start: auth-bff call failed", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"auth_url": authURL}})
}

// Finish exchanges the intent the app carried back for tokens.
//
// It is two auth-bff calls, not one, and cannot be collapsed into one:
// which tenant a Google-authenticated merchant belongs to is unknowable
// until the identity has been resolved, and the tenant is required to
// complete. So finish resolves the identity, this handler looks the tenant
// up BY THE VERIFIED EMAIL auth-bff returned — the same ListMyTenants path
// password login uses — and complete then finishes the sign-in.
func (h *MobileIDPHandler) Finish(c *gin.Context) {
	var req mobileIDPFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.IntentID == "" || req.IntentToken == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "intent_id and intent_token are required",
		})
		return
	}

	// The provider is load-bearing on finish: auth-bff pins the intent
	// against the IDP this names, so forwarding the wrong one refuses a
	// valid sign-in. An ABSENT provider must keep meaning Google — app
	// builds already in the wild send none, and requiring it would break
	// every existing Google merchant on the next deploy.
	provider := authbffclient.ProviderGoogle
	if strings.TrimSpace(req.Provider) != "" {
		named, ok := normalizeIDPProvider(req.Provider)
		if !ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "unsupported_provider", "message": "That sign-in method isn't available.",
			})
			return
		}
		provider = named
	}

	res, err := h.backend.IDPFinish(c.Request.Context(), provider, req.IntentID, req.IntentToken)
	if err != nil {
		h.respondIDPError(c, provider, "mobile idp finish: auth-bff call failed", err)
		return
	}
	// marketplace-api never sends a workspace_tenant on finish, so
	// auth-bff answers tenant_required for every mobile finish. Anything
	// else means the contract changed underneath us, and guessing at it
	// would mint a session for a tenant nobody chose.
	if !res.TenantRequired || res.LoginName == "" || res.SessionID == "" || res.SessionToken == "" {
		h.logError("mobile idp finish: auth-bff did not ask for a tenant", errors.New("unexpected finish shape"))
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "auth_unavailable", "message": "Sign-in is temporarily unavailable. Try again shortly.",
		})
		return
	}

	// The email is auth-bff's, resolved from Zitadel — never anything the
	// device sent. That is what makes this lookup safe to key on.
	tenants, err := h.lister.ListMyTenants(c.Request.Context(), res.LoginName)
	if err != nil {
		h.logError("mobile idp finish: tenant lookup failed", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "platform_unavailable", "message": "could not look up your stores"})
		return
	}
	if len(tenants) == 0 {
		// The person proved they own this Google account and there is
		// still no store for it. Same answer, same copy, as the password
		// path — and no session is minted.
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error":   "no_store",
			"message": "We couldn't find a store for this account. Did you finish onboarding?",
		})
		return
	}
	primary := tenants[0].TenantID

	done, err := h.backend.IDPComplete(c.Request.Context(), res.LoginName, res.SessionID, res.SessionToken, primary)
	if err != nil {
		h.respondIDPError(c, provider, "mobile idp complete: auth-bff call failed", err)
		return
	}

	// The SAME body shape /mobile/admin/auth/login answers with, so the
	// app's existing handling — tokens, or a step-up routed to the OTP
	// screen — works unchanged.
	c.JSON(http.StatusOK, gin.H{"data": mobileLoginResponse(done, tenants)})
}

// idpErrorCopy maps auth-bff's stable error codes to a status and merchant-
// facing copy.
//
// Unlike the password path, these are NOT collapsed: Google already
// authenticated the person, so there is no enumeration oracle to protect,
// and the reason a sign-in was refused is exactly what the merchant needs
// in order to do something about it. A refusal reported as "temporarily
// unavailable" is the dead end that produced #493.
// messageFmt, where set, takes the merchant-facing provider name: naming
// Google in an answer to an Apple sign-in is simply wrong. Only the message
// varies — the status and the stable `error` code are identical either way.
var idpErrorCopy = map[string]struct {
	status     int
	code       string
	message    string
	messageFmt string
}{
	"no_admin_account": {status: http.StatusNotFound, code: "no_store",
		message: "We couldn't find a store for this account. Did you finish onboarding?"},
	"email_not_verified": {status: http.StatusUnauthorized, code: "email_not_verified",
		messageFmt: "%s hasn't verified that email address, so we can't sign you in with it."},
	"email_ambiguous": {status: http.StatusConflict, code: "email_ambiguous",
		message: "More than one account uses that email. Contact support to sort it out."},
	"unexpected_idp": {status: http.StatusUnauthorized, code: "invalid_credentials",
		messageFmt: "Couldn't sign you in with %s. Try again."},
	"invalid_intent": {status: http.StatusUnauthorized, code: "invalid_credentials",
		message: "That sign-in attempt expired. Try again."},
	"unsupported_provider": {status: http.StatusBadRequest, code: "unsupported_provider",
		message: "That sign-in method isn't available."},
	"invalid_return_url": {status: http.StatusBadGateway, code: "auth_unavailable",
		message: "Sign-in is temporarily unavailable. Try again shortly."},
}

// respondIDPError answers with mapped copy where auth-bff gave a code we
// understand, and with an explicit "unavailable" otherwise — never with a
// credential error for an upstream failure.
func (h *MobileIDPHandler) respondIDPError(c *gin.Context, provider, logMsg string, err error) {
	var idpErr *authbffclient.IDPError
	if errors.As(err, &idpErr) {
		if mapped, ok := idpErrorCopy[idpErr.Code]; ok {
			// Logged even when mapped: a spike in email_ambiguous or
			// unexpected_idp is an operational signal, not just copy.
			h.logError(logMsg, err)
			message := mapped.message
			if mapped.messageFmt != "" {
				message = fmt.Sprintf(mapped.messageFmt, providerLabel(provider))
			}
			c.AbortWithStatusJSON(mapped.status, gin.H{"error": mapped.code, "message": message})
			return
		}
	}
	h.logError(logMsg, err)
	c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
		"error": "auth_unavailable", "message": "Sign-in is temporarily unavailable. Try again shortly.",
	})
}

func (h *MobileIDPHandler) logError(msg string, err error) {
	if h.log != nil {
		h.log.Error(msg, "error", err)
	}
}
