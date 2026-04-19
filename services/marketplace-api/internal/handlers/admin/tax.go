// Package admin — tax.go: handlers for the §19.3 tax-ID submit flow and
// the §19.3.1 US/CA business-entity attestation. The orchestrator (tax.Service)
// owns the validator dispatch and DB writes; this file only translates HTTP
// payloads, dispatches, and maps sentinel errors to HTTP statuses.
package admin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// SubmitService is the narrow contract this handler depends on. *tax.Service
// satisfies it; tests pass a fake.
type SubmitService interface {
	Submit(ctx context.Context, in tax.SubmitInput) error
}

// TaxHandler exposes POST /tax/submit and POST /tax/attestation under the
// store route. Mounted under /admin/stores/:storeId/tax/* per the P3
// allowlist (so merchants in read-only states can still remediate).
type TaxHandler struct {
	db        *gorm.DB
	svc       SubmitService
	ipHashKey []byte
}

// NewTaxHandler constructs a TaxHandler. ipHashKey is the HMAC key used to
// hash the merchant's submitting IP into the attestation row (privacy
// preservation per §18.8); rotate via Secret Manager.
func NewTaxHandler(db *gorm.DB, svc SubmitService, ipHashKey []byte) *TaxHandler {
	return &TaxHandler{db: db, svc: svc, ipHashKey: ipHashKey}
}

type submitTaxBody struct {
	Country        string `json:"country"         binding:"required,len=2"`
	TaxID          string `json:"tax_id"          binding:"required"`
	BusinessName   string `json:"business_name"   binding:"required"`
	BillingAddress string `json:"billing_address"`
}

// Submit handles POST /admin/stores/:storeId/tax/submit. Status mapping:
//
//	200 OK        — validated, row flipped.
//	202 Accepted  — manual review queued OR registry temporarily unavailable.
//	400           — malformed request body or invalid tax-ID format.
//	422           — tax ID not found in registry.
//	503           — validator disabled (NZ awaiting counsel sign-off).
//	500           — unexpected internal error.
func (h *TaxHandler) Submit(c *gin.Context) {
	var body submitTaxBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	tenantID, ok := parseUUID(c, "tenant_id", "")
	if !ok {
		return
	}
	storeID, ok := parseUUID(c, "", "storeId")
	if !ok {
		return
	}

	err := h.svc.Submit(c.Request.Context(), tax.SubmitInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		Country:        body.Country,
		TaxID:          body.TaxID,
		BusinessName:   body.BusinessName,
		BillingAddress: body.BillingAddress,
		Source:         "signup",
	})
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{
			"data":  gin.H{"status": "validated"},
			"error": nil,
		})
	case errors.Is(err, tax.ErrValidatorDisabled):
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"error":   "validator_disabled",
			"message": "Tax-ID validation for this country is temporarily unavailable awaiting legal sign-off. Please contact support.",
		})
	case errors.Is(err, tax.ErrManualReviewRequired):
		c.JSON(http.StatusAccepted, gin.H{
			"data": gin.H{
				"status":            "manual_review_queued",
				"sla_business_days": 5,
			},
			"error": nil,
		})
	case errors.Is(err, tax.ErrRegistryUnavailable):
		c.JSON(http.StatusAccepted, gin.H{
			"data": gin.H{
				"status":  "registry_unavailable",
				"message": "Please retry; the validation window is paused while the registry is unreachable.",
			},
			"error": nil,
		})
	case errors.Is(err, tax.ErrInvalidFormat):
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_format"})
	case errors.Is(err, tax.ErrNotFound):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": "tax_id_not_found"})
	default:
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "tax_validation_failed"})
	}
}

type attestationBody struct {
	Country         string `json:"country"          binding:"required,len=2,oneof=US CA"`
	CheckboxText    string `json:"checkbox_text"    binding:"required"`
	CheckboxVersion string `json:"checkbox_version" binding:"required"`
}

// SignAttestation handles POST /admin/stores/:storeId/tax/attestation. Writes
// an append-only row into business_entity_attestations (migration 045: the
// table has an UPDATE-blocking trigger and revoked DELETE permission).
func (h *TaxHandler) SignAttestation(c *gin.Context) {
	var body attestationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	tenantID, ok := parseUUID(c, "tenant_id", "")
	if !ok {
		return
	}
	storeID, ok := parseUUID(c, "", "storeId")
	if !ok {
		return
	}

	ipHash := hashIP(h.ipHashKey, c.ClientIP())

	err := h.db.WithContext(c.Request.Context()).Exec(`
		INSERT INTO business_entity_attestations
			(store_id, tenant_id, country, checkbox_text, checkbox_version, user_agent, ip_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, storeID, tenantID, body.Country, body.CheckboxText, body.CheckboxVersion, c.Request.UserAgent(), ipHash).Error
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "attestation_write_failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"data":  gin.H{"status": "signed"},
		"error": nil,
	})
}

// parseUUID reads a UUID from either the gin string-context (e.g. "tenant_id")
// or a path param (e.g. "storeId"). Aborts with 400 and returns ok=false on
// failure so callers can early-return.
func parseUUID(c *gin.Context, ctxKey, paramKey string) (uuid.UUID, bool) {
	var raw string
	if ctxKey != "" {
		raw = c.GetString(ctxKey)
	} else {
		raw = c.Param(paramKey)
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "could not parse " + ctxKey + paramKey,
		})
		return uuid.Nil, false
	}
	return id, true
}

func hashIP(key []byte, ip string) string {
	if len(key) == 0 || ip == "" {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}

// RegisterTax mounts the tax routes onto a /admin/stores/:storeId sub-router.
// SubscriptionEditRole gates both endpoints — only the store owner can
// submit a tax ID or sign the attestation.
func RegisterTax(storeRoute gin.IRouter, h *TaxHandler, fgaMw *authz.Middleware) {
	taxGroup := storeRoute.Group("/tax")
	{
		taxGroup.POST("/submit",
			fgaMw.RequireTenantRelation(authz.SubscriptionEditRole),
			h.Submit)
		taxGroup.POST("/attestation",
			fgaMw.RequireTenantRelation(authz.SubscriptionEditRole),
			h.SignAttestation)
	}
}
