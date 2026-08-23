package onboarding

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/platform-api/internal/verification"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for onboarding endpoints.
//
// Onboarding has its own verification subroutes that wrap the verification
// service with onboarding-specific behavior (mark the session as verified
// on success, return only the relevant fields).
type Handler struct {
	svc      *Service
	verifSvc *verification.Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, verifSvc *verification.Service) *Handler {
	return &Handler{svc: svc, verifSvc: verifSvc}
}

// Register mounts onboarding routes onto the given gin.RouterGroup.
func (h *Handler) Register(r *gin.RouterGroup) {
	o := r.Group("/onboarding")
	{
		o.POST("/sessions", h.createSession)
		o.GET("/sessions/:id", h.getSession)
		o.PATCH("/sessions/:id/draft", h.saveDraft)
		o.POST("/sessions/:id/verification/send", h.sendVerification)
		// Magic link verification: no session id in path because the magic
		// link only carries the token. The handler resolves the session
		// from the token's session_id.
		o.POST("/verify-token", h.verifyAndMarkSession)
		o.POST("/sessions/:id/complete", h.completeSession)
	}
}

func (h *Handler) createSession(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	sess, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": sess})
}

func (h *Handler) getSession(c *gin.Context) {
	sess, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sess})
}

func (h *Handler) saveDraft(c *gin.Context) {
	// Read raw body so callers can submit any JSON shape.
	var raw json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	if err := h.svc.SaveDraft(c.Request.Context(), c.Param("id"), raw); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"saved": true}})
}

type sendVerificationRequest struct {
	BusinessName string `json:"business_name"`
}

func (h *Handler) sendVerification(c *gin.Context) {
	sessionID := c.Param("id")
	sess, err := h.svc.Get(c.Request.Context(), sessionID)
	if err != nil {
		respondError(c, err)
		return
	}
	var req sendVerificationRequest
	_ = c.ShouldBindJSON(&req) // body is optional

	if err := h.verifSvc.SendMagicLink(c.Request.Context(), sessionID, sess.Email, req.BusinessName); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"sent": true}})
}

type verifyTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

// verifyAndMarkSession consumes a magic link token, marks the session
// verified, and returns the session ID + email so the caller (a server
// action in apps/onboarding) can immediately complete onboarding.
func (h *Handler) verifyAndMarkSession(c *gin.Context) {
	var req verifyTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	res, err := h.verifSvc.VerifyToken(c.Request.Context(), req.Token)
	if err != nil {
		respondError(c, err)
		return
	}
	if err := h.svc.MarkVerified(c.Request.Context(), res.SessionID); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"verified":   true,
		"session_id": res.SessionID,
		"email":      res.Email,
	}})
}

func (h *Handler) completeSession(c *gin.Context) {
	var req CompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.BadRequest("invalid_request", err.Error()))
		return
	}
	req.SessionID = c.Param("id")
	res, err := h.svc.Complete(c.Request.Context(), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": res})
}

// RegisterAnalytics mounts the onboarding funnel and sessions analytics
// routes (#283) onto the supplied group. The CALLER is responsible for
// gating it — main.go wraps it in middleware.RequireInternalAuthStrict,
// the same strict guard the tenant directory (#277) uses: both return
// estate-wide data, so an unconfigured deploy must refuse rather than
// serve the lot.
//
// Deliberately separate from Register: those routes are the public wizard
// routes on /api/v1 and keep the permissive/no auth branch — mirroring how
// tenant.RegisterDirectory is kept separate from tenant.Register.
func (h *Handler) RegisterAnalytics(g *gin.RouterGroup) {
	o := g.Group("/onboarding")
	{
		o.GET("/funnel", h.getFunnel)
		// Static sibling of Register's /sessions/:id. Verified against
		// gin 1.12 with the real two-group router shape main.go builds:
		// no router-build panic, and both routes resolve to their own
		// handlers (the #287 class of bug — see funnel_handler_test.go).
		o.GET("/sessions", h.listFunnelSessions)
	}
}

// getFunnel serves GET /internal/onboarding/funnel.
//
// Response is a single object under "data" — no "pagination" key, since
// the funnel has no page concept. median_completion_seconds serialises as
// JSON null (not 0, not omitted) when no session in the window completed;
// FunnelStats' field has no omitempty for exactly this reason.
func (h *Handler) getFunnel(c *gin.Context) {
	f := parseFunnelFilter(c)
	stats, err := h.svc.GetFunnel(c.Request.Context(), f)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// listFunnelSessions serves GET /internal/onboarding/sessions.
//
// Standard envelope: data + pagination. Empty result is 200 with data: [],
// never null. pagination.limit reports the EFFECTIVE (clamped) limit, so
// total / limit is a correct page count.
func (h *Handler) listFunnelSessions(c *gin.Context) {
	f := parseFunnelFilter(c)
	rows, total, err := h.svc.ListSessions(c.Request.Context(), f)
	if err != nil {
		respondError(c, err)
		return
	}
	if rows == nil {
		rows = []SessionRow{}
	}
	c.JSON(http.StatusOK, gin.H{
		"data": rows,
		"pagination": gin.H{
			"page":  max(f.Page, 1),
			"limit": f.Limit,
			"total": total,
		},
	})
}

// parseFunnelFilter never returns an error. A missing or malformed
// parameter takes the default; an oversized limit clamps here so
// pagination.limit in the response reflects the effective value used.
//
// AsOf is deliberately never parsed from a query parameter — see
// FunnelFilter.AsOf's doc comment. A console caller able to set it could
// time-travel the funnel. It is left at its zero value on every path
// through this function.
func parseFunnelFilter(c *gin.Context) FunnelFilter {
	f := FunnelFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Page:   1,
		Limit:  DefaultFunnelPageSize,
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = min(n, MaxFunnelPageSize)
		}
	}
	if t, ok := parseFunnelRFC3339(c.Query("created_from")); ok {
		f.CreatedFrom = t
	}
	if t, ok := parseFunnelRFC3339(c.Query("created_to")); ok {
		f.CreatedTo = t
	}
	if v := strings.TrimSpace(c.Query("abandoned")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			f.Abandoned = &b
		}
	}
	return f
}

func parseFunnelRFC3339(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func respondError(c *gin.Context, err error) {
	if ae, ok := apperrors.As(err); ok {
		c.JSON(ae.Status, gin.H{"error": ae.Code, "message": ae.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": "an unexpected error occurred",
	})
}
