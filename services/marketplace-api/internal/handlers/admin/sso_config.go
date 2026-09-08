package admin

import (
	"context"
	"errors"
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
	// RelyingParties is optional; nil makes Test a shape check only.
	RelyingParties SSORelyingParties
	Repo           SSOConfigStore
	Audit          *audit.Emitter
	Logger         *slog.Logger
}

// NewSSOConfigHandler constructs the handler. Audit may be nil (tests).
func NewSSOConfigHandler(repo SSOConfigStore, auditEmitter *audit.Emitter, logger *slog.Logger) *SSOConfigHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SSOConfigHandler{Repo: repo, Audit: auditEmitter, Logger: logger}
}

// WithRelyingParties gives Test something real to test.
//
// Without it Test re-runs sso.Validate — the same check Upsert already ran to
// accept the config — so it answers "ok" for a config that has never been
// tried against the IdP at all. With it, Test builds the relying party: it
// fetches the issuer's discovery document and reads the client secret out of
// OpenBao, which are the two things that actually fail in practice and the two
// a merchant pressing "Test connection" is asking about.
func (h *SSOConfigHandler) WithRelyingParties(rps SSORelyingParties) *SSOConfigHandler {
	h.RelyingParties = rps
	return h
}

// SSOConfigStore is the persistence this handler needs. *sso.Repository
// satisfies it.
//
// An interface rather than the concrete repository, for the reason #833 found
// on the login side: with a *sso.Repository here, no test could reach any
// branch that depends on a LOADED config, and the Test endpoint's entire
// behaviour is one of those branches.
type SSOConfigStore interface {
	GetByTenant(ctx context.Context, tenantID uuid.UUID) (*sso.Config, error)
	Upsert(ctx context.Context, cfg *sso.Config) error
	Delete(ctx context.Context, tenantID uuid.UUID) error
}

// SSORelyingParties builds (and caches) a tenant's OIDC relying party.
// *sso.RelyingPartyCache satisfies it.
type SSORelyingParties interface {
	For(ctx context.Context, cfg *sso.Config) (*sso.OIDCRelyingParty, error)
	Invalidate(tenantID uuid.UUID)
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

	// SAML has no SP, so there is nothing to reach. Say so rather than
	// reporting a pass for a provider that cannot serve a login.
	if cfg.Provider == sso.ProviderSAML {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"ok":       false,
			"provider": string(cfg.Provider),
			"error":    "SAML sign-in is not available; configure this tenant for OIDC",
		})
		return
	}

	if h.RelyingParties == nil {
		// Shape-checked only. Reported honestly rather than as a pass: this
		// build cannot reach the IdP, and a merchant told "ok" would learn
		// otherwise at their first login attempt.
		c.JSON(http.StatusOK, gin.H{
			"ok":       true,
			"provider": string(cfg.Provider),
			"checked":  "configuration",
		})
		return
	}

	// Drop any cached party first, so this tests the config as it is NOW
	// rather than one built before the merchant's last edit — which is the
	// whole reason someone presses Test after changing something.
	h.RelyingParties.Invalidate(tenantID)

	if _, err := h.RelyingParties.For(c.Request.Context(), cfg); err != nil {
		h.Logger.Warn("sso_config: test failed to build the relying party",
			"tenant_id", tenantID, "err", err)
		// The cause is deliberately summarised, not echoed: it can carry the
		// issuer URL and the secret path, and this response is rendered in a
		// merchant's browser.
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"ok":       false,
			"provider": string(cfg.Provider),
			"error":    testFailureReason(err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"provider": string(cfg.Provider),
		"checked":  "idp",
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

// testFailureReason turns a build failure into something a merchant can act on
// without leaking their issuer URL or secret path into a browser.
//
// Two causes, two different fixes: the secret store could not produce a client
// secret at the path the config names, or the IdP itself could not be reached
// or did not answer with a usable discovery document.
func testFailureReason(err error) string {
	if errors.Is(err, sso.ErrNoClientSecret) {
		return "we could not read the client secret at the path this configuration names"
	}
	if errors.Is(err, sso.ErrInvalidMetadata) {
		return "this configuration is missing something the provider needs"
	}
	return "we could not reach the identity provider, or it did not answer with a valid OpenID configuration"
}
