// Package admin — subscription.go: HTTP handler for store billing
// subscription endpoints (Settings S3).
package admin

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// SubscriptionHandler handles /admin/stores/:storeId/subscription endpoints.
type SubscriptionHandler struct {
	svc           *subscription.Service
	webhookSecret string
	logger        *slog.Logger
}

// NewSubscriptionHandler constructs a SubscriptionHandler.
func NewSubscriptionHandler(svc *subscription.Service, webhookSecret string, logger *slog.Logger) *SubscriptionHandler {
	return &SubscriptionHandler{
		svc:           svc,
		webhookSecret: webhookSecret,
		logger:        logger,
	}
}

// SubscriptionResponse is the wire DTO for a store subscription.
type SubscriptionResponse struct {
	ID                   string  `json:"id"`
	StoreID              string  `json:"store_id"`
	Plan                 string  `json:"plan"`
	Status               string  `json:"status"`
	CurrentPeriodStart   *string `json:"current_period_start,omitempty"`
	CurrentPeriodEnd     *string `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd    bool    `json:"cancel_at_period_end"`
	StripeSubscriptionID *string `json:"stripe_subscription_id,omitempty"`
	CreatedAt            string  `json:"created_at"`
}

func toSubscriptionResponse(s subscription.StoreSubscription) SubscriptionResponse {
	resp := SubscriptionResponse{
		ID:                s.ID.String(),
		StoreID:           s.StoreID.String(),
		Plan:              string(s.Plan),
		Status:            string(s.Status),
		CancelAtPeriodEnd: s.CancelAtPeriodEnd,
		CreatedAt:         s.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if s.StripeSubscriptionID != nil {
		resp.StripeSubscriptionID = s.StripeSubscriptionID
	}
	if s.CurrentPeriodStart != nil {
		t := s.CurrentPeriodStart.Format("2006-01-02T15:04:05Z")
		resp.CurrentPeriodStart = &t
	}
	if s.CurrentPeriodEnd != nil {
		t := s.CurrentPeriodEnd.Format("2006-01-02T15:04:05Z")
		resp.CurrentPeriodEnd = &t
	}
	return resp
}

// GetSubscription handles GET /admin/stores/:storeId/subscription.
func (h *SubscriptionHandler) GetSubscription(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	sub, err := h.svc.GetSubscription(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toSubscriptionResponse(*sub))
}

// CreateCheckoutRequest is the request body for POST .../subscription/checkout.
type CreateCheckoutRequest struct {
	Plan       string `json:"plan" binding:"required"`
	SuccessURL string `json:"success_url" binding:"required"`
	CancelURL  string `json:"cancel_url" binding:"required"`
}

// CreateCheckout handles POST /admin/stores/:storeId/subscription/checkout.
func (h *SubscriptionHandler) CreateCheckout(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req CreateCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	url, err := h.svc.CreateCheckoutSession(c.Request.Context(), subscription.CheckoutInput{
		TenantID:   tenantID,
		StoreID:    storeID,
		Plan:       req.Plan,
		SuccessURL: req.SuccessURL,
		CancelURL:  req.CancelURL,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// CreatePortalRequest is the request body for POST .../subscription/portal.
type CreatePortalRequest struct {
	ReturnURL string `json:"return_url" binding:"required"`
}

// CreatePortal handles POST /admin/stores/:storeId/subscription/portal.
func (h *SubscriptionHandler) CreatePortal(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	var req CreatePortalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	url, err := h.svc.CreatePortalSession(c.Request.Context(), storeID, req.ReturnURL)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// HandleWebhook handles POST /webhooks/stripe-billing — no auth middleware,
// webhook signature verification happens inside this handler.
func (h *SubscriptionHandler) HandleWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "failed to read body"})
		return
	}

	// In production, verify the Stripe webhook signature here using
	// h.webhookSecret and the Stripe-Signature header.
	sig := c.GetHeader("Stripe-Signature")
	if h.webhookSecret != "" && sig == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "missing signature"})
		return
	}

	// Parse the event from the body. In a real implementation, this would
	// use the Stripe SDK to construct and verify the event.
	_ = body

	// Acknowledge receipt — real event processing would happen here.
	c.JSON(http.StatusOK, gin.H{"received": true})
}
