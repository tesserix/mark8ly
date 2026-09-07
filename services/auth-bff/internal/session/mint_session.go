package session

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/internalauth"
)

// allowedAuthContexts is the explicit allow-list for the auth_context
// field on POST /internal/mint-session. This is the safety control the
// endpoint exists to enforce: an unrecognized or absent value must be
// rejected, never silently defaulted to "staff" — a typo minting a full
// staff session is exactly the failure this guards against. See
// docs/superpowers/plans/2026-09-07-break-glass-activation-v2.md, D1 and
// Task 2.
var allowedAuthContexts = map[string]bool{
	"staff":       true,
	"customer":    true,
	"break_glass": true,
}

// MintSessionRequest is the payload marketplace-api sends to mint a
// session cookie for a principal it has already authenticated (e.g. a
// break-glass login, and later an SSO callback). auth-bff never trusts
// the caller's authentication decision — it only turns an already-made
// decision into a signed cookie, and enforces the auth_context
// allow-list regardless of what the caller asserts.
type MintSessionRequest struct {
	TenantID    string `json:"tenant_id" binding:"required"`
	TenantSlug  string `json:"tenant_slug"`
	UserID      string `json:"user_id" binding:"required"`
	Email       string `json:"email"`
	AuthContext string `json:"auth_context"`
	TTLSeconds  int    `json:"ttl_seconds"`
}

// MintSessionResponse carries the full Set-Cookie header VALUE (not the
// header name). marketplace-api forwards it verbatim via
// c.Writer.Header().Add("Set-Cookie", setCookie) — see
// marketplace-api/internal/handlers/admin/break_glass_login.go.
type MintSessionResponse struct {
	SetCookie string `json:"set_cookie"`
}

// MintSessionHandler implements POST /internal/mint-session. Cookie
// cryptography stays in auth-bff: marketplace-api must never sign a
// session cookie itself, it only asks auth-bff to mint one for a
// principal it has already authenticated.
type MintSessionHandler struct {
	sessions *Manager
	secret   string
	logger   *slog.Logger
}

// NewMintSessionHandler constructs the handler. secret is the expected
// value of the X-Internal-Auth header; an empty secret disables the
// endpoint entirely (every request returns 503) so a misconfigured
// deploy fails closed instead of serving an unauthenticated route.
func NewMintSessionHandler(sessions *Manager, secret string, logger *slog.Logger) *MintSessionHandler {
	return &MintSessionHandler{
		sessions: sessions,
		secret:   secret,
		logger:   logger,
	}
}

// Register mounts POST /internal/mint-session onto the given router group.
func (h *MintSessionHandler) Register(r *gin.RouterGroup) {
	r.POST("/mint-session", h.mintSession)
}

func (h *MintSessionHandler) mintSession(c *gin.Context) {
	if h.secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "not_configured",
			"message": "internal auth secret is not configured",
		})
		return
	}
	if !internalauth.Equal(c.GetHeader(internalauth.Header), h.secret) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing or invalid internal auth",
		})
		return
	}

	var req MintSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "request body is invalid",
		})
		return
	}

	if !allowedAuthContexts[req.AuthContext] {
		if h.logger != nil {
			h.logger.Warn("mint-session: rejected auth_context",
				"tenant_id", req.TenantID,
			)
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_auth_context",
			"message": "auth_context must be one of: staff, customer, break_glass",
		})
		return
	}

	s := Session{
		UID:         req.UserID,
		Email:       req.Email,
		TenantID:    req.TenantID,
		AuthContext: req.AuthContext,
	}
	if req.TTLSeconds > 0 {
		now := time.Now()
		s.IssuedAt = now
		s.ExpiresAt = now.Add(time.Duration(req.TTLSeconds) * time.Second)
	}

	encoded, err := h.sessions.EncodeSession(s)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("mint-session: encode", "err", err, "tenant_id", req.TenantID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "failed to mint session",
		})
		return
	}

	setCookie := h.sessions.BuildSetCookieHeader(encoded, "")

	if h.logger != nil {
		h.logger.Info("mint-session: minted",
			"tenant_id", req.TenantID,
			"auth_context", req.AuthContext,
		)
	}

	c.JSON(http.StatusOK, MintSessionResponse{SetCookie: setCookie})
}
