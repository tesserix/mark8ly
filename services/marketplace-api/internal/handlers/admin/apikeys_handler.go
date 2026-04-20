// Package admin — apikeys_handler.go: §18.4 enterprise API-key admin
// endpoints. Mounted under /admin/stores/:storeId/api-keys. Reads (GET)
// are gated by plangate.RequireFeature(FeatureReadAPI) (Studio+); writes
// (POST create/rotate, DELETE revoke) are gated by FeatureFullAPI (Pro).
// The service layer additionally rejects write-scoped key creation when
// FeatureFullAPI is not allowed for the resolved plan.
package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// APIKeysHandler exposes CRUD for enterprise API keys.
type APIKeysHandler struct {
	svc      *apikeys.Service
	resolver *plangate.PlanResolver
	logger   *slog.Logger
}

// NewAPIKeysHandler constructs an APIKeysHandler. resolver is used to look
// up the current plan inside Create (the route-level RequireFeature only
// gates Studio+ for the read API; we re-resolve for Plan-aware service input).
func NewAPIKeysHandler(svc *apikeys.Service, resolver *plangate.PlanResolver, logger *slog.Logger) *APIKeysHandler {
	return &APIKeysHandler{svc: svc, resolver: resolver, logger: logger}
}

type createAPIKeyBody struct {
	Label           string   `json:"label"              binding:"required,max=100"`
	Scopes          []string `json:"scopes"             binding:"required,min=1"`
	RateLimitPerMin int      `json:"rate_limit_per_min"`
}

type apiKeyResponse struct {
	ID              uuid.UUID `json:"id"`
	Label           string    `json:"label"`
	KeyPrefix       string    `json:"key_prefix"`
	Scopes          []string  `json:"scopes"`
	RateLimitPerMin int       `json:"rate_limit_per_min"`
	CreatedAt       time.Time `json:"created_at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

func toAPIKeyResponse(k apikeys.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID:              k.ID,
		Label:           k.Label,
		KeyPrefix:       k.KeyPrefix,
		Scopes:          []string(k.Scopes),
		RateLimitPerMin: k.RateLimitPerMin,
		CreatedAt:       k.CreatedAt,
		RevokedAt:       k.RevokedAt,
		LastUsedAt:      k.LastUsedAt,
	}
}

// Create handles POST /admin/stores/:storeId/api-keys.
func (h *APIKeysHandler) Create(c *gin.Context) {
	tenantID, storeID, ok := h.parseScope(c)
	if !ok {
		return
	}
	createdBy, ok := h.parseUser(c)
	if !ok {
		return
	}

	var body createAPIKeyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	plan := h.resolver.Resolve(c.Request.Context(), tenantID, storeID)
	out, err := h.svc.Create(c.Request.Context(), apikeys.CreateInput{
		TenantID:        tenantID,
		StoreID:         storeID,
		CreatedBy:       createdBy,
		Scopes:          body.Scopes,
		RateLimitPerMin: body.RateLimitPerMin,
		Label:           body.Label,
		Plan:            subscription.SubscriptionPlan(plan),
	})
	switch {
	case err == nil:
		c.JSON(http.StatusCreated, gin.H{
			"data": gin.H{
				"id":              out.ID,
				"plaintext":       out.Plaintext,
				"display":         out.Display,
				"warning":         "Store this key now — it will not be shown again.",
			},
			"error": nil,
		})
	case errors.Is(err, apikeys.ErrPlanDoesNotAllowAPI):
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "plan_does_not_allow_api",
			"message": "Your current plan does not include API key access.",
		})
	case errors.Is(err, apikeys.ErrWriteScopeRequiresPro):
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "write_scope_requires_pro",
			"message": "Write scopes require the Pro plan; downgrade scopes or upgrade.",
		})
	case errors.Is(err, apikeys.ErrRateLimitExceedsPlanCeiling):
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "rate_limit_too_high",
			"message": err.Error(),
		})
	default:
		h.logger.Error("apikeys: create failed", "err", err, "tenant_id", tenantID, "store_id", storeID)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
	}
}

// List handles GET /admin/stores/:storeId/api-keys.
func (h *APIKeysHandler) List(c *gin.Context) {
	tenantID, storeID, ok := h.parseScope(c)
	if !ok {
		return
	}

	rows, err := h.svc.List(c.Request.Context(), tenantID, storeID)
	if err != nil {
		h.logger.Error("apikeys: list failed", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}
	out := make([]apiKeyResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAPIKeyResponse(r))
	}
	c.JSON(http.StatusOK, gin.H{"data": out, "error": nil})
}

type rotateAPIKeyBody struct {
	Reason string `json:"reason"`
}

// Rotate handles POST /admin/stores/:storeId/api-keys/:keyId/rotate.
func (h *APIKeysHandler) Rotate(c *gin.Context) {
	tenantID, _, ok := h.parseScope(c)
	if !ok {
		return
	}
	keyID, err := uuid.Parse(c.Param("keyId"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_key_id"})
		return
	}

	var body rotateAPIKeyBody
	_ = c.ShouldBindJSON(&body) // optional body
	reason := body.Reason
	if reason == "" {
		reason = "scheduled_rotation"
	}

	out, err := h.svc.Rotate(c.Request.Context(), tenantID, keyID, reason)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"new_id":         out.NewID,
				"new_plaintext":  out.NewPlaintext,
				"old_expires_at": out.OldExpiresAt,
				"warning":        "Old key remains valid for 24h to allow zero-downtime swap.",
			},
			"error": nil,
		})
	case errors.Is(err, apikeys.ErrNotFound):
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "key_not_found"})
	case errors.Is(err, apikeys.ErrCannotRotateRevoked):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "key_revoked"})
	default:
		h.logger.Error("apikeys: rotate failed", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "rotate_failed"})
	}
}

type revokeAPIKeyBody struct {
	Reason string `json:"reason"`
}

// Revoke handles DELETE /admin/stores/:storeId/api-keys/:keyId.
func (h *APIKeysHandler) Revoke(c *gin.Context) {
	tenantID, _, ok := h.parseScope(c)
	if !ok {
		return
	}
	keyID, err := uuid.Parse(c.Param("keyId"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_key_id"})
		return
	}
	var body revokeAPIKeyBody
	_ = c.ShouldBindJSON(&body)
	reason := body.Reason
	if reason == "" {
		reason = "merchant_revoked"
	}

	err = h.svc.Revoke(c.Request.Context(), tenantID, keyID, reason)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "revoked"}, "error": nil})
	case errors.Is(err, apikeys.ErrNotFound):
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "key_not_found"})
	default:
		h.logger.Error("apikeys: revoke failed", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "revoke_failed"})
	}
}

func (h *APIKeysHandler) parseScope(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing_tenant"})
		return uuid.Nil, uuid.Nil, false
	}
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"})
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, storeID, true
}

func (h *APIKeysHandler) parseUser(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing_user"})
		return uuid.Nil, false
	}
	return id, true
}

// RegisterAPIKeys mounts the admin endpoints. List is gated by FeatureReadAPI
// (Studio+); create/rotate/revoke are gated by FeatureFullAPI (Pro). Read
// endpoints under FeatureReadAPI let Studio+ merchants list their keys
// without upgrading; write endpoints require an explicit Pro plan.
func RegisterAPIKeys(storeRoute gin.IRouter, h *APIKeysHandler, fgaMw *authz.Middleware, logger *slog.Logger) {
	if h == nil || h.resolver == nil {
		return
	}
	keys := storeRoute.Group("/api-keys")
	{
		keys.GET("",
			plangate.RequireFeature(h.resolver, plangate.FeatureReadAPI, logger),
			fgaMw.RequireTenantRelation(authz.SubscriptionViewRole),
			h.List)
		keys.POST("",
			plangate.RequireFeature(h.resolver, plangate.FeatureFullAPI, logger),
			fgaMw.RequireTenantRelation(authz.SubscriptionEditRole),
			h.Create)
		keys.POST("/:keyId/rotate",
			plangate.RequireFeature(h.resolver, plangate.FeatureFullAPI, logger),
			fgaMw.RequireTenantRelation(authz.SubscriptionEditRole),
			h.Rotate)
		keys.DELETE("/:keyId",
			plangate.RequireFeature(h.resolver, plangate.FeatureFullAPI, logger),
			fgaMw.RequireTenantRelation(authz.SubscriptionEditRole),
			h.Revoke)
	}
}
