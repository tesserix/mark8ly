package autologin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/emailotp"
	"github.com/mark8ly/auth-bff/internal/geoip"
)

// deviceFromUA produces a short, human-readable device label from a
// User-Agent string. Intentionally crude — users just need to tell
// their laptop from their phone in the "Active sessions" list.
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

// Handler is the HTTP layer for autologin.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the autologin route onto the given gin.RouterGroup.
//
// The route is INTENTIONALLY public — the user comes in unauthenticated
// (they just finished onboarding, no session yet), presents an ID token,
// and we mint a session if everything checks out.
func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/auto-login", h.autoLogin)
}

type autoLoginRequest struct {
	IDToken          string `json:"id_token" binding:"required"`
	ExpectedTenantID string `json:"expected_tenant_id" binding:"required"`
	WorkspaceTenant  string `json:"workspace_tenant" binding:"required"`
}

func (h *Handler) autoLogin(c *gin.Context) {
	var req autoLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	res, err := h.svc.AutoLogin(c.Request.Context(), c.Writer, Request{
		IDToken:          req.IDToken,
		ExpectedTenantID: req.ExpectedTenantID,
		WorkspaceTenant:  req.WorkspaceTenant,
		Device:           deviceFromUA(c.Request.UserAgent()),
		IPAddress:        c.ClientIP(),
		UserAgent:        c.Request.UserAgent(),
		Country:          geoip.CountryFromHeaders(c.Request.Header),
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"uid":                res.UID,
			"email":              res.Email,
			"tenant_id":          res.TenantID,
			"mfa_required":       res.MFARequired,
			"email_otp_required": res.EmailOTPRequired,
		},
	})
}

// respondError maps autologin errors to HTTP responses.
//
// 401: token invalid or tenant pool mismatch — the client is wrong
// 403: not a member of the tenant — the membership tuple is not (yet) visible;
//
//	after the retry budget this is a real "not authorized" answer
//
// 429: the email-OTP rate limit was tripped — waiting is the only way out
// 503: openfga is unreachable, or the sign-in code could not otherwise be
//
//	sent — the system is broken, retry the call
//
// 500: anything else (session mint failure, internal bugs)
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrTokenInvalid), errors.Is(err, ErrTenantMismatch):
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": err.Error(),
		})
	case errors.Is(err, ErrNotMember):
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "not_a_member",
			"message": "you are not a member of this tenant",
		})
	case errors.Is(err, ErrFGAUnreachable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "openfga_unreachable",
			"message": "authorization service is temporarily unavailable; please retry",
		})
	// Checked ahead of ErrChallengeSendFail: a rate limit is wrapped by
	// ErrChallengeSendFail, and errors.Is matches either, so the more
	// specific case must win.
	case errors.Is(err, emailotp.ErrRateLimited):
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":   "rate_limited",
			"message": "too many sign-in codes requested; please wait before trying again",
		})
	case errors.Is(err, ErrChallengeSendFail):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "challenge_send_failed",
			"message": "could not send your sign-in code; please retry",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "an unexpected error occurred",
		})
	}
}
