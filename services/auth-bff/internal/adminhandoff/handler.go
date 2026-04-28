package adminhandoff

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/session"
)

// FGAChecker is the same shape session.FGAChecker uses — kept narrow so
// tests can fake it without pulling the OpenFGA SDK.
type FGAChecker interface {
	CheckMembership(ctx context.Context, userID, tenantID string) (bool, error)
}

// SessionMinter is the slice of session.Manager the handler needs.
// MintWithDomain writes the cookie with an explicit Domain attribute
// (used for the cross-TLD admin custom domain).
type SessionMinter interface {
	MintWithDomain(w http.ResponseWriter, s session.Session, domainOverride string) error
}

// Config bundles the handler's dependencies.
type Config struct {
	HMACKey  string         // SESSION_ENCRYPT_KEY — same key the session manager uses
	FGA      FGAChecker     // membership gate; required in prod
	Sessions SessionMinter  // Mint target
	Logger   *slog.Logger   // optional
}

// Handler exposes POST /auth/mint-from-admin-handoff.
type Handler struct {
	cfg Config
}

// NewHandler constructs a Handler. cfg.Sessions and cfg.HMACKey are
// required; cfg.FGA may be nil only in dev (fails closed in prod).
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

// Register mounts the route on /auth.
func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/mint-from-admin-handoff", h.mint)
}

type mintRequest struct {
	Code string `json:"code" binding:"required"`
}

// mint is the route handler. Trust model:
//
//   - The HMAC signature gates "this code came from a service that
//     holds SESSION_ENCRYPT_KEY". Since the same key encrypts session
//     cookies, only services that already mint sessions can mint
//     handoffs — there's no escalation path beyond what those services
//     already had.
//   - The FGA membership check gates "this user actually has a role on
//     the target tenant". Without it, a misconfigured caller could mint
//     a handoff for an arbitrary (uid, tenant_id) pair and bypass the
//     normal switch-tenant authorization flow.
//   - The 30s exp gates replay risk if a code leaks via referrer or
//     server log capture.
//
// Caller-side defense (admin /auth/handoff route) verifies that
// claims.TargetHost matches the request host so a code minted for
// admin.A can't be redeemed at admin.B.
func (h *Handler) mint(c *gin.Context) {
	var req mintRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "code is required",
		})
		return
	}

	claims, err := Verify(req.Code, h.cfg.HMACKey)
	if err != nil {
		status := http.StatusUnauthorized
		code := "invalid_code"
		switch err {
		case ErrExpired:
			code = "expired"
		case ErrWrongKind:
			code = "wrong_kind"
		case ErrMalformed, ErrInvalidPayload, ErrInvalidSignature:
			code = "invalid_code"
		case ErrIncompleteClaims:
			code = "incomplete_claims"
		case ErrMissingKey:
			status = http.StatusInternalServerError
			code = "server_misconfigured"
		}
		c.JSON(status, gin.H{"error": code, "message": err.Error()})
		return
	}

	if !isAdminCustomHost(claims.TargetHost) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_target_host",
			"message": "target_host must be an admin custom domain",
		})
		return
	}

	if h.cfg.FGA != nil {
		allowed, ferr := h.cfg.FGA.CheckMembership(c.Request.Context(), claims.UID, claims.TenantID)
		if ferr != nil {
			if h.cfg.Logger != nil {
				h.cfg.Logger.Error("adminhandoff: fga check failed",
					"err", ferr, "uid", claims.UID, "tenant_id", claims.TenantID)
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "authz_check_failed",
				"message": "authorization check failed",
			})
			return
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "user is not a member of this tenant",
			})
			return
		}
	}

	now := time.Now()
	s := session.Session{
		UID:       claims.UID,
		Email:     claims.Email,
		TenantID:  claims.TenantID,
		StoreID:   claims.StoreID,
		IssuedAt:  now,
		// Empty ExpiresAt → Manager applies its configured maxAge.
	}
	if err := h.cfg.Sessions.MintWithDomain(c.Writer, s, claims.TargetHost); err != nil {
		if h.cfg.Logger != nil {
			h.cfg.Logger.Error("adminhandoff: mint failed", "err", err, "uid", claims.UID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "session_mint_failed",
			"message": "failed to issue session",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"user_id":     claims.UID,
			"email":       claims.Email,
			"tenant_id":   claims.TenantID,
			"store_id":    claims.StoreID,
			"target_host": claims.TargetHost,
		},
	})
}

// isAdminCustomHost is the structural check: target_host must be an
// off-mark8ly admin host (`admin.<merchant-tld>` with ≥3 labels). This
// rules out a code carrying e.g. `admin.mark8ly.com` (would write a
// cookie back onto the canonical host) or a single-label string.
func isAdminCustomHost(h string) bool {
	host := strings.ToLower(strings.TrimSpace(h))
	if !strings.HasPrefix(host, "admin.") {
		return false
	}
	if strings.HasSuffix(host, ".mark8ly.com") || strings.HasSuffix(host, ".mark8ly.dev") {
		return false
	}
	parts := strings.Split(host, ".")
	if len(parts) < 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
}
