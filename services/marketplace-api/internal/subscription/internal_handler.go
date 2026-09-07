// Package subscription — internal_handler.go
//
// POST /internal/stores/:storeID/ensure-subscription is the platform-api
// callback that gives a brand-new store its subscription row, and with it its
// trial clock.
//
// It exists because that clock had no other start (#827). Bootstrap was
// reachable from exactly one place — a CTA on the admin Billing page — so the
// 90-day trial began when a merchant happened to open Billing, and a merchant
// who never opened it had no row at all: no trial, no expiry, and invisible to
// every billing cron.
//
// Called from onboarding.Complete AFTER EnsureSelfStore, because
// store_subscriptions.store_id is a foreign key onto stores(id) — the
// projection has to land first. Idempotent, so a retry of onboarding is safe.
package subscription

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/auth"
)

// Bootstrapper is the one method this handler needs. *Service satisfies it;
// the interface keeps the handler testable without a database.
type Bootstrapper interface {
	Bootstrap(ctx context.Context, in BootstrapInput) (*StoreSubscription, error)
}

// InternalHandler serves the platform-api subscription callback.
type InternalHandler struct {
	svc Bootstrapper
}

// NewInternalHandler constructs an InternalHandler.
func NewInternalHandler(svc Bootstrapper) *InternalHandler {
	return &InternalHandler{svc: svc}
}

// RegisterRoutes mounts the callback behind the shared internal secret.
//
// InternalSecretAuth, not HeaderTrustAuth: this call is made during
// onboarding, when the merchant has not signed in anywhere, so there is no
// X-User-Id or X-Tenant-Id to present and HeaderTrustAuth would refuse every
// legitimate call. platform-api's VendorClient already sends X-Internal-Auth,
// so no new configuration is needed on the calling side.
func (h *InternalHandler) RegisterRoutes(g *gin.RouterGroup, internalSecret string) {
	g.POST("/stores/:storeID/ensure-subscription",
		auth.InternalSecretAuth(internalSecret), h.ensureSubscription)
}

type ensureSubscriptionRequest struct {
	TenantID string `json:"tenant_id" binding:"required"`
	// Email and Name are used only if a Stripe customer is minted later; they
	// are carried now so the caller does not have to be asked again.
	Email string `json:"email"`
	Name  string `json:"name"`
	// Currency is the store's ISO 4217 billing currency. Known at signup and
	// supplied by nothing else afterwards.
	Currency string `json:"currency"`
}

func (h *InternalHandler) ensureSubscription(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeID"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"})
		return
	}

	var req ensureSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "detail": err.Error(),
		})
		return
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_tenant_id"})
		return
	}

	sub, err := h.svc.Bootstrap(c.Request.Context(), BootstrapInput{
		TenantID:        tenantID,
		StoreID:         storeID,
		Email:           req.Email,
		Name:            req.Name,
		BillingCurrency: req.Currency,
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "ensure_subscription_failed", "detail": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"id":         sub.ID.String(),
		"plan":       string(sub.Plan),
		"status":     string(sub.Status),
		"created_at": sub.CreatedAt,
	}})
}
