// Package stores — internal_handler.go
//
// POST /internal/stores/upsert is the platform-api callback that mirrors
// a tenant's authoritative store row from platform_api.stores into
// marketplace_api.stores so all downstream lookups (slug → store, custom-
// domain join, products etc.) can hit local data instead of cross-service
// HTTP on every admin/storefront request.
//
// Idempotent: keyed on store id. The portal secret is generated server-
// side on the first INSERT and preserved on subsequent UPDATEs (Upsert's
// DoUpdates list deliberately omits storefront_customer_portal_secret).
package stores

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type InternalHandler struct {
	repo Repository
}

func NewInternalHandler(repo Repository) *InternalHandler {
	return &InternalHandler{repo: repo}
}

// RegisterRoutes mounts POST /stores/upsert on the supplied group.
// Caller is expected to mount this on the /internal group of the admin
// engine, alongside the existing vendor.Handler routes.
func (h *InternalHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/stores/upsert", h.upsert)
}

type upsertStoreRequest struct {
	ID           string `json:"id"            binding:"required"`
	TenantID     string `json:"tenant_id"     binding:"required"`
	Slug         string `json:"slug"          binding:"required"`
	Name         string `json:"name"          binding:"required"`
	CountryCode  string `json:"country_code"  binding:"required"`
	CurrencyCode string `json:"currency_code" binding:"required"`
	Timezone     string `json:"timezone"      binding:"required"`
	Status       string `json:"status"`
}

func (h *InternalHandler) upsert(c *gin.Context) {
	var req upsertStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": err.Error(),
		})
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}

	secret, err := generatePortalSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "secret_gen_failed",
			"message": err.Error(),
		})
		return
	}

	s := &Store{
		ID:                             req.ID,
		TenantID:                       req.TenantID,
		Slug:                           req.Slug,
		Name:                           req.Name,
		CountryCode:                    req.CountryCode,
		CurrencyCode:                   req.CurrencyCode,
		Timezone:                       req.Timezone,
		Status:                         status,
		SyncedAt:                       time.Now().UTC(),
		StorefrontCustomerPortalSecret: secret,
	}
	if err := h.repo.Upsert(c.Request.Context(), s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "upsert_failed",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": s})
}

// generatePortalSecret returns 64 hex chars (32 random bytes) suitable
// for the storefront_customer_portal_secret CHAR(64) column.
func generatePortalSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
