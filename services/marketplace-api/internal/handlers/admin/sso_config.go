package admin

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/sso"
)

// SSOConfigHandler answers the four SSO configuration endpoints:
//
//	POST   /admin/tenants/:tenantId/sso/config   — Upsert
//	GET    /admin/tenants/:tenantId/sso/config   — Get
//	DELETE /admin/tenants/:tenantId/sso/config   — Delete
//	POST   /admin/tenants/:tenantId/sso/test     — Test (dry-run validate)
//
// All methods enforce:
//  1. The path :tenantId must equal the tenant_id set by the auth middleware
//     (cross-tenant requests get 403).
//  2. The route group must be wrapped in plangate.RequireFeatureByTenant so
//     only Pro+ tenants reach these handlers.
type SSOConfigHandler struct {
	Repo   *sso.Repository
	Audit  *audit.Emitter
	Logger *slog.Logger
}

// NewSSOConfigHandler constructs the handler. Audit may be nil (tests).
func NewSSOConfigHandler(repo *sso.Repository, auditEmitter *audit.Emitter, logger *slog.Logger) *SSOConfigHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SSOConfigHandler{Repo: repo, Audit: auditEmitter, Logger: logger}
}

// ssoUpsertRequest is the JSON body accepted by Upsert.
type ssoUpsertRequest struct {
	Provider    sso.Provider           `json:"provider" binding:"required"`
	Metadata    map[string]interface{} `json:"metadata" binding:"required"`
	AttrMapping map[string]string      `json:"attr_mapping"`
	Enabled     bool                   `json:"enabled"`
}

// Upsert creates or replaces the SSO config for the tenant.
//
// POST /admin/tenants/:tenantId/sso/config
func (h *SSOConfigHandler) Upsert(c *gin.Context) {
	tenantID, ok := h.requirePathTenantMatch(c)
	if !ok {
		return
	}

	var req ssoUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": err.Error()})
		return
	}

	attrMap := sso.AttrMapping(req.AttrMapping)
	if len(attrMap) > 0 {
		if err := attrMap.Validate(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_attr_mapping", "message": err.Error()})
			return
		}
	}

	attrMapJSON := datatypes.JSONMap{}
	for k, v := range attrMap {
		attrMapJSON[k] = v
	}

	cfg := &sso.Config{
		TenantID:    tenantID,
		Provider:    req.Provider,
		Metadata:    datatypes.JSONMap(req.Metadata),
		AttrMapping: attrMapJSON,
		Enabled:     req.Enabled,
		UpdatedAt:   time.Now().UTC(),
	}

	if err := sso.Validate(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_config", "message": err.Error()})
		return
	}

	if err := h.Repo.Upsert(c.Request.Context(), cfg); err != nil {
		h.Logger.Error("sso_config: upsert failed", "tenant_id", tenantID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to save SSO config"})
		return
	}

	if h.Audit != nil {
		h.Audit.Emit(c, audit.Event{
			Action:       "sso.config.upserted",
			ResourceType: "sso_config",
			ResourceID:   tenantID.String(),
			Metadata:     map[string]any{"provider": string(cfg.Provider), "enabled": cfg.Enabled},
			TenantID:     tenantID,
		})
	}

	c.JSON(http.StatusCreated, gin.H{
		"tenant_id": tenantID.String(),
		"provider":  string(cfg.Provider),
		"enabled":   cfg.Enabled,
	})
}

// Get returns the SSO config for the tenant, with secrets redacted.
//
// GET /admin/tenants/:tenantId/sso/config
func (h *SSOConfigHandler) Get(c *gin.Context) {
	tenantID, ok := h.requirePathTenantMatch(c)
	if !ok {
		return
	}

	cfg, err := h.Repo.GetByTenant(c.Request.Context(), tenantID)
	if err != nil {
		if err == sso.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "no SSO config for this tenant"})
			return
		}
		h.Logger.Error("sso_config: get failed", "tenant_id", tenantID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to load SSO config"})
		return
	}

	c.JSON(http.StatusOK, redactConfig(cfg))
}

// Delete removes the SSO config and all JIT user mappings for the tenant.
//
// DELETE /admin/tenants/:tenantId/sso/config
func (h *SSOConfigHandler) Delete(c *gin.Context) {
	tenantID, ok := h.requirePathTenantMatch(c)
	if !ok {
		return
	}

	cfg, err := h.Repo.GetByTenant(c.Request.Context(), tenantID)
	if err != nil {
		if err == sso.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "no SSO config for this tenant"})
			return
		}
		h.Logger.Error("sso_config: delete load failed", "tenant_id", tenantID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to load SSO config"})
		return
	}
	provider := cfg.Provider

	// Delete config + all JIT mappings in one transaction.
	if err := h.Repo.Delete(c.Request.Context(), tenantID); err != nil {
		if err == sso.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "no SSO config for this tenant"})
			return
		}
		h.Logger.Error("sso_config: delete failed", "tenant_id", tenantID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to delete SSO config"})
		return
	}

	if h.Audit != nil {
		h.Audit.Emit(c, audit.Event{
			Action:       "sso.config.deleted",
			ResourceType: "sso_config",
			ResourceID:   tenantID.String(),
			Severity:     audit.SeverityWarning,
			Metadata:     map[string]any{"provider": string(provider)},
			TenantID:     tenantID,
		})
	}

	c.Status(http.StatusNoContent)
}

// Test is a dry-run endpoint: loads the config and validates it, returning
// the provider kind and any validation errors. Full IdP connectivity checks
// are deferred to a future release.
//
// POST /admin/tenants/:tenantId/sso/test
func (h *SSOConfigHandler) Test(c *gin.Context) {
	tenantID, ok := h.requirePathTenantMatch(c)
	if !ok {
		return
	}

	cfg, err := h.Repo.GetByTenant(c.Request.Context(), tenantID)
	if err != nil {
		if err == sso.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "no SSO config for this tenant"})
			return
		}
		h.Logger.Error("sso_config: test load failed", "tenant_id", tenantID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "failed to load SSO config"})
		return
	}

	if err := sso.Validate(cfg); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"ok":       false,
			"provider": string(cfg.Provider),
			"error":    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"provider": string(cfg.Provider),
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// requirePathTenantMatch parses :tenantId, verifies it equals the authed
// tenant_id from the gin context, and returns (tenantID, true) on success.
// On failure it writes the appropriate error response and returns (Nil, false).
func (h *SSOConfigHandler) requirePathTenantMatch(c *gin.Context) (uuid.UUID, bool) {
	pathTenantStr := c.Param("tenantId")
	pathTenant, err := uuid.Parse(pathTenantStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid tenantId in path"})
		return uuid.Nil, false
	}

	authedTenant, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil || authedTenant != pathTenant {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden", "message": "tenant mismatch"})
		return uuid.Nil, false
	}

	return pathTenant, true
}

// redactConfig returns a copy of the config with sensitive fields removed so
// it can be safely serialised into a response body.
func redactConfig(cfg *sso.Config) map[string]any {
	meta := make(map[string]any, len(cfg.Metadata))
	for k, v := range cfg.Metadata {
		// Redact secrets.
		if k == sso.OIDCKeyClientSecretRef || k == "client_secret" {
			meta[k] = "[redacted]"
			continue
		}
		meta[k] = v
	}

	attrMap := make(map[string]any, len(cfg.AttrMapping))
	for k, v := range cfg.AttrMapping {
		attrMap[k] = v
	}

	return map[string]any{
		"tenant_id":    cfg.TenantID.String(),
		"provider":     string(cfg.Provider),
		"metadata":     meta,
		"attr_mapping": attrMap,
		"enabled":      cfg.Enabled,
		"created_at":   cfg.CreatedAt,
		"updated_at":   cfg.UpdatedAt,
	}
}

// stringMetaVal extracts a string from a datatypes.JSONMap without panicking.
func stringMetaVal(m datatypes.JSONMap, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
