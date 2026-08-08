package loginotp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/emailotp"
	"github.com/mark8ly/auth-bff/internal/session"
)

// CreateParams is the session-registry row written once a device has
// proved itself. Fingerprint is the field that matters — it is what makes
// deviceguard recognise this browser next time.
type CreateParams struct {
	UserID      string
	TenantID    string
	Device      string
	IPAddress   string
	UserAgent   string
	Fingerprint string
}

// SessionRegistry records active sessions. Optional.
type SessionRegistry interface {
	CreateSession(ctx context.Context, p CreateParams) error
}

// Config holds Handler dependencies.
type Config struct {
	Gate     *Gate
	Sessions *session.Manager
	Registry SessionRegistry
	Logger   *slog.Logger
}

// Handler serves the challenge-completion endpoints.
type Handler struct {
	gate     *Gate
	sessions *session.Manager
	registry SessionRegistry
	logger   *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(cfg Config) *Handler {
	return &Handler{
		gate:     cfg.Gate,
		sessions: cfg.Sessions,
		registry: cfg.Registry,
		logger:   cfg.Logger,
	}
}

// Register mounts the routes under the given group (typically /auth).
func (h *Handler) Register(g *gin.RouterGroup) {
	otp := g.Group("/otp")
	otp.POST("/verify", h.verify)
	otp.POST("/resend", h.resend)
}

type verifyBody struct {
	Code string `json:"code"`
}

func (h *Handler) verify(c *gin.Context) {
	pending := h.requirePending(c)
	if pending == nil {
		return
	}

	var body verifyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if body.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_code", "message": "code is required"})
		return
	}

	// Verified against the pending cookie's address, never one supplied
	// in the body — that binding is what stops a code minted for one
	// account completing another's login.
	if err := h.gate.Verify(c.Request.Context(), pending.Email, body.Code); err != nil {
		status, code := classify(err)
		c.JSON(status, gin.H{"error": code, "message": "the code could not be verified"})
		return
	}

	if err := h.sessions.Mint(c.Writer, session.Session{
		UID:      pending.UID,
		Email:    pending.Email,
		TenantID: pending.TenantID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session_mint_failed"})
		return
	}
	h.sessions.ClearPending(c.Writer)

	// Best-effort: the cookie is already the source of truth, so a failed
	// registry write must not lock out a user who just verified. It does
	// mean they get challenged again next time.
	if h.registry != nil {
		if err := h.registry.CreateSession(c.Request.Context(), CreateParams{
			UserID:      pending.UID,
			TenantID:    pending.TenantID,
			Device:      pending.Device,
			IPAddress:   pending.IPAddress,
			UserAgent:   c.Request.UserAgent(),
			Fingerprint: pending.Fingerprint,
		}); err != nil && h.logger != nil {
			h.logger.Warn("loginotp: session registry write failed",
				"err", err, "user_id", pending.UID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"uid":           pending.UID,
		"tenant_id":     pending.TenantID,
	})
}

func (h *Handler) resend(c *gin.Context) {
	pending := h.requirePending(c)
	if pending == nil {
		return
	}
	if err := h.gate.IssueChallenge(c.Request.Context(), pending.Email, c.ClientIP()); err != nil {
		status, code := classify(err)
		c.JSON(status, gin.H{"error": code, "message": "could not send a new code"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true})
}

// requirePending resolves the in-flight login, writing the error response
// and returning nil when there isn't one.
func (h *Handler) requirePending(c *gin.Context) *session.Pending {
	p, err := h.sessions.ReadPending(c.Request)
	if err != nil || p == nil || p.Email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "no_pending_login",
			"message": "start a sign-in before submitting a code",
		})
		return nil
	}
	return p
}

// classify maps emailotp errors onto HTTP without telling the caller
// which specific check failed — distinguishing "wrong code" from "no
// such challenge" would confirm whether an address is mid-login.
func classify(err error) (int, string) {
	switch {
	case errors.Is(err, emailotp.ErrRateLimited):
		return http.StatusTooManyRequests, "rate_limited"
	case errors.Is(err, emailotp.ErrTooManyAttempts):
		return http.StatusTooManyRequests, "too_many_attempts"
	case errors.Is(err, emailotp.ErrInvalidCode),
		errors.Is(err, emailotp.ErrExpired),
		errors.Is(err, emailotp.ErrAlreadyUsed),
		errors.Is(err, emailotp.ErrNoChallenge):
		return http.StatusUnauthorized, "invalid_code"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
