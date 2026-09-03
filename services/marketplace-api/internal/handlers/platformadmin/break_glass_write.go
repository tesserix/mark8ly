package platformadmin

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
)

// BreakGlassRotator is the subset of *breakglass.Rotator this handler needs
// for POST /admin/break-glass/:tenantId/rotate. Declared locally, same
// reason as BreakGlassLister (break_glass.go), so the handler is stubbable.
type BreakGlassRotator interface {
	RotateOne(ctx context.Context, tenantID uuid.UUID) error
}

// BreakGlassWriter is the subset of *breakglass.Repository this handler
// needs for disable/enable/clear-lockout (#404).
type BreakGlassWriter interface {
	Disable(ctx context.Context, tenantID uuid.UUID, reason string) error
	Enable(ctx context.Context, tenantID uuid.UUID) error
	ClearIPLock(ctx context.Context, ipHash []byte) (int64, error)
}

// BreakGlassRateLimiter is the subset of *breakglass.LoginRateLimiter this
// handler needs to reset the in-process login limiter alongside the
// durable DB lockout on clear-lockout. Declared as an interface (not the
// concrete pointer type) so a nil Deps field is a genuine nil interface,
// never a non-nil interface wrapping a nil pointer — see the nil check in
// clearLockout.
type BreakGlassRateLimiter interface {
	Reset(key string)
}

// breakGlassAuditFunc records a platform-operator action against a tenant.
// Alias of lifecycleAuditFunc (tenant_lifecycle.go) for the same reason
// trialExtendAuditFunc is: Go does not implicitly convert between two
// distinct named function types even when their underlying signatures
// match, and this package already has exactly one such adapter
// (NewOperatorActionAuditFunc) it should keep using.
type breakGlassAuditFunc = lifecycleAuditFunc

// BreakGlassWriteHandler serves the break-glass write half (#404): rotate,
// disable, enable, clear-lockout. Split from BreakGlassHandler (the read
// side, #333) because these routes need write-scoped dependencies
// (Rotator, Writer, RateLimiter, an IP HMAC key, an audit emitter) that
// the read route has no use for — see Register (routes.go) for the hard
// DB+Emitter gate that keeps these four routes unmounted rather than
// mounted-but-unaudited, the same guarantee TenantLifecycle and
// TrialExtender already enforce on this surface.
type BreakGlassWriteHandler struct {
	db          *gorm.DB
	writer      BreakGlassWriter
	rotator     BreakGlassRotator
	rateLimiter BreakGlassRateLimiter
	ipHMACKey   breakglass.HMACKey
	emit        breakGlassAuditFunc
	logger      *slog.Logger
}

// NewBreakGlassWriteHandler constructs the handler. rateLimiter may be
// nil — clearLockout degrades to clearing only the durable DB lock and
// logs loudly, rather than panicking on a nil interface. emit may be nil,
// defaulting to a no-op for direct low-level callers; the real audit
// guarantee lives in Register's mount gate, not here (see
// NewTenantLifecycleHandler's doc for why that split exists). logger may
// be nil, substituted with slog.Default().
func NewBreakGlassWriteHandler(
	db *gorm.DB,
	writer BreakGlassWriter,
	rotator BreakGlassRotator,
	rateLimiter BreakGlassRateLimiter,
	ipHMACKey breakglass.HMACKey,
	emit breakGlassAuditFunc,
	logger *slog.Logger,
) *BreakGlassWriteHandler {
	if emit == nil {
		emit = func(*gin.Context, uuid.UUID, audit.Event) error { return nil }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BreakGlassWriteHandler{
		db:          db,
		writer:      writer,
		rotator:     rotator,
		rateLimiter: rateLimiter,
		ipHMACKey:   ipHMACKey,
		emit:        emit,
		logger:      logger,
	}
}

// Register mounts the four write routes on the supplied group, beside
// List (break_glass.go).
func (h *BreakGlassWriteHandler) Register(g *gin.RouterGroup) {
	g.POST("/admin/break-glass/:tenantId/rotate", h.rotate)
	g.POST("/admin/break-glass/:tenantId/disable", h.disable)
	g.POST("/admin/break-glass/:tenantId/enable", h.enable)
	g.POST("/admin/break-glass/clear-lockout", h.clearLockout)
}

// breakGlassRLKey MUST stay byte-identical to rlKey in
// internal/handlers/admin/break_glass_login.go: both shape the same
// LoginRateLimiter bucket key from an ip_hash, and the login path resets
// that bucket on a successful login. A key that shapes the hash
// differently here would mean clear-lockout resets a bucket the login
// path never reads, leaving the in-memory limiter stuck even though the
// durable DB lock was cleared.
func breakGlassRLKey(ipHash []byte) string {
	n := len(ipHash)
	if n > 16 {
		n = 16
	}
	return string(ipHash[:n])
}

func (h *BreakGlassWriteHandler) parseTenantID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("tenantId")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_tenant_id", "message": "tenantId must be a uuid", "field": "tenantId",
		})
		return uuid.Nil, false
	}
	return id, true
}

// rotateResponse carries METADATA ONLY. RotateOne (breakglass/rotation.go)
// writes the new password and TOTP secret directly to Secret Manager and
// returns only an error — there is no credential value anywhere in this
// handler for a response to leak, even by mistake.
type rotateResponse struct {
	TenantID  string `json:"tenant_id"`
	RotatedAt string `json:"rotated_at"`
}

func (h *BreakGlassWriteHandler) rotate(c *gin.Context) {
	tenantID, ok := h.parseTenantID(c)
	if !ok {
		return
	}

	// RotateOne does not touch disabled_at (see
	// breakglass.Repository.ReplaceAfterRotation) — a deliberately
	// disabled account stays disabled through an unrelated rotation.
	if err := h.rotator.RotateOne(c.Request.Context(), tenantID); err != nil {
		if errors.Is(err, breakglass.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "not_found", "message": "no break-glass account for that tenant",
			})
			return
		}
		h.logger.Error("break-glass: rotate failed", "tenant_id", tenantID.String(), "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "rotate_failed", "message": "could not rotate break-glass credentials",
		})
		return
	}

	rotatedAt := time.Now().UTC()
	if err := h.emit(c, tenantID, audit.Event{
		Action:       "break_glass.rotated",
		ResourceType: "break_glass_account",
		ResourceID:   tenantID.String(),
		Metadata: map[string]any{
			"rotated_at": rotatedAt.Format(time.RFC3339),
		},
	}); err != nil {
		// The rotation already SUCCEEDED. Log loudly; do not fail the
		// response for an audit-attribution failure alone.
		h.logger.Error("break-glass: rotate succeeded but was not audited", "tenant_id", tenantID.String(), "err", err)
	}

	c.JSON(http.StatusOK, rotateResponse{
		TenantID:  tenantID.String(),
		RotatedAt: rotatedAt.Format(time.RFC3339),
	})
}

type disableRequest struct {
	Reason string `json:"reason"`
}

type disableResponse struct {
	TenantID   string `json:"tenant_id"`
	DisabledAt string `json:"disabled_at"`
	Reason     string `json:"reason"`
}

func (h *BreakGlassWriteHandler) disable(c *gin.Context) {
	tenantID, ok := h.parseTenantID(c)
	if !ok {
		return
	}

	var req disableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// gin's JSON binder returns io.EOF for a completely empty body —
		// an omitted body is rejected HERE, before the empty-reason check
		// below, matching tenant_lifecycle.go's handle().
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "reason_required", "message": "reason is required and must not be empty", "field": "reason",
		})
		return
	}

	if err := h.writer.Disable(c.Request.Context(), tenantID, reason); err != nil {
		h.respondWriterErr(c, "disable", err)
		return
	}

	disabledAt := time.Now().UTC()
	if err := h.emit(c, tenantID, audit.Event{
		Action:       "break_glass.disabled",
		ResourceType: "break_glass_account",
		ResourceID:   tenantID.String(),
		Metadata: map[string]any{
			"reason":      reason,
			"disabled_at": disabledAt.Format(time.RFC3339),
		},
	}); err != nil {
		h.logger.Error("break-glass: disable succeeded but was not audited", "tenant_id", tenantID.String(), "err", err)
	}

	c.JSON(http.StatusOK, disableResponse{
		TenantID:   tenantID.String(),
		DisabledAt: disabledAt.Format(time.RFC3339),
		Reason:     reason,
	})
}

type enableResponse struct {
	TenantID  string `json:"tenant_id"`
	EnabledAt string `json:"enabled_at"`
}

func (h *BreakGlassWriteHandler) enable(c *gin.Context) {
	tenantID, ok := h.parseTenantID(c)
	if !ok {
		return
	}

	if err := h.writer.Enable(c.Request.Context(), tenantID); err != nil {
		h.respondWriterErr(c, "enable", err)
		return
	}

	enabledAt := time.Now().UTC()
	if err := h.emit(c, tenantID, audit.Event{
		Action:       "break_glass.enabled",
		ResourceType: "break_glass_account",
		ResourceID:   tenantID.String(),
		Metadata: map[string]any{
			"enabled_at": enabledAt.Format(time.RFC3339),
		},
	}); err != nil {
		h.logger.Error("break-glass: enable succeeded but was not audited", "tenant_id", tenantID.String(), "err", err)
	}

	c.JSON(http.StatusOK, enableResponse{
		TenantID:  tenantID.String(),
		EnabledAt: enabledAt.Format(time.RFC3339),
	})
}

// clearLockoutRequest carries the plaintext IP — the one place in this
// handler a raw IP is allowed to exist, in the request body of the
// operator who already knows it. It is hashed immediately (see
// clearLockout below) and the plaintext is never stored or logged.
//
// TenantID is OPTIONAL and used only for audit attribution:
// break_glass_lockouts is keyed by ip_hash, not tenant, and its own
// tenant_id column is nullable (an attempt that never resolved a tenant —
// see breakglass.Lockout's doc). When the caller does not supply one,
// EmitOperatorAction (audit.go) cannot attribute the event to a tenant;
// that failure is logged, not surfaced, matching every other
// audit-attribution failure on this surface.
type clearLockoutRequest struct {
	IP       string `json:"ip"`
	TenantID string `json:"tenant_id"`
}

type clearLockoutResponse struct {
	Removed int64 `json:"removed"`
}

func (h *BreakGlassWriteHandler) clearLockout(c *gin.Context) {
	var req clearLockoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}

	ip := strings.TrimSpace(req.IP)
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "ip_required", "message": "ip is required", "field": "ip",
		})
		return
	}

	// The SAME key the login path uses (breakglass.HMACIPHash) — a
	// different key hashes to different bytes, and this endpoint would
	// silently clear nothing while still reporting success.
	ipHash := breakglass.HMACIPHash(h.ipHMACKey, ip)

	removed, err := h.writer.ClearIPLock(c.Request.Context(), ipHash)
	if err != nil {
		h.logger.Error("break-glass: clear-lockout failed", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "clear_lockout_failed", "message": "could not clear the lockout",
		})
		return
	}

	// LoginRateLimiter is an in-memory, per-pod map (breakglass/ratelimit.go)
	// — this Reset only reaches the pod serving THIS request. The admin
	// deployment runs 1 replica today, so the durable DB row cleared above
	// is the one that actually matters; a second replica's own in-memory
	// counter would be untouched by this call. That is current luck, not
	// design (#404).
	if h.rateLimiter != nil {
		h.rateLimiter.Reset(breakGlassRLKey(ipHash))
	} else {
		h.logger.Warn("break-glass: clear-lockout has no rate limiter wired; only the durable DB lock was cleared")
	}

	var tenantID uuid.UUID
	if parsed, err := uuid.Parse(strings.TrimSpace(req.TenantID)); err == nil {
		tenantID = parsed
	}

	// NEVER the raw IP — only the HMAC hash and the row count are
	// recorded, matching the storage rule break_glass_lockouts.ip_hash
	// already applies. The audit trail must not become the one place the
	// plaintext IP survives.
	if err := h.emit(c, tenantID, audit.Event{
		Action:       "break_glass.lockout_cleared",
		ResourceType: "break_glass_lockout",
		ResourceID:   hex.EncodeToString(ipHash),
		Metadata: map[string]any{
			"ip_hash": hex.EncodeToString(ipHash),
			"removed": removed,
		},
	}); err != nil {
		h.logger.Error("break-glass: clear-lockout succeeded but was not audited", "err", err)
	}

	c.JSON(http.StatusOK, clearLockoutResponse{Removed: removed})
}

func (h *BreakGlassWriteHandler) respondWriterErr(c *gin.Context, action string, err error) {
	if errors.Is(err, breakglass.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "no break-glass account for that tenant",
		})
		return
	}
	h.logger.Error("break-glass: "+action+" failed", "err", err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": "internal_error", "message": "could not update the break-glass account",
	})
}
