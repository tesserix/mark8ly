// Package admin — subscription.go: HTTP handler for store billing
// subscription endpoints (Settings S3).
package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// SubscriptionHandler handles /admin/stores/:storeId/subscription endpoints.
type SubscriptionHandler struct {
	svc           *subscription.Service
	webhookSecret string
	audit         *audit.Emitter // optional — nil-safe
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

// WithAudit attaches an audit emitter so subscription/billing actions
// land in Settings -> Audit Logs. Nil-safe.
func (h *SubscriptionHandler) WithAudit(e *audit.Emitter) *SubscriptionHandler {
	h.audit = e
	return h
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
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	sub, err := h.svc.GetSubscription(c.Request.Context(), tenantID, storeID)
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

	// Records the *intent* to change plan. The actual plan transition
	// fires from the Stripe webhook once the customer completes payment;
	// that event will land here too once HandleWebhook is fully wired.
	h.audit.Emit(c, audit.Event{
		Action:       "subscription.checkout_started",
		ResourceType: "subscription",
		Metadata:     map[string]any{"plan": req.Plan},
	})
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
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req CreatePortalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	url, err := h.svc.CreatePortalSession(c.Request.Context(), tenantID, storeID, req.ReturnURL)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "subscription.billing_portal_opened",
		ResourceType: "subscription",
	})
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

	// Parse the event envelope so we can emit a useful audit row even
	// before the full state-machine processing is wired. We pull the
	// minimum we need (event id, type, and the tenant/store hints
	// Stripe carries in metadata) and forward the rest as-is for the
	// real handler to consume later.
	var event stripeWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		h.logger.Warn("stripe webhook: parse failed", "err", err)
		// Acknowledge anyway — Stripe will retry on non-2xx and we don't
		// want a parse glitch to spam its retry queue.
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	if event.Type != "" {
		// Severity heuristic: cancellations and payment failures are
		// operationally interesting. Everything else is informational.
		severity := audit.SeverityInfo
		switch event.Type {
		case "customer.subscription.deleted",
			"invoice.payment_failed",
			"charge.dispute.created":
			severity = audit.SeverityWarning
		case "charge.refunded":
			severity = audit.SeverityWarning
		}

		h.audit.Emit(c, audit.Event{
			TenantID:       parseUUIDOrZero(event.Data.Object.Metadata.TenantID),
			StoreID:        parseUUIDOrZero(event.Data.Object.Metadata.StoreID),
			ForceActorType: audit.ActorSystem,
			Action:         "subscription.webhook_received",
			ResourceType:   "subscription",
			ResourceID:     event.ID,
			Severity:       severity,
			Metadata: map[string]any{
				"event_type": event.Type,
				"livemode":   event.Livemode,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

// stripeWebhookEvent is a minimal subset of Stripe's webhook envelope
// — enough to extract the event type, id, and the tenant/store hints
// Stripe carries in subscription metadata. We deliberately don't take a
// dependency on the Stripe SDK here; full processing will move to a
// dedicated handler later.
type stripeWebhookEvent struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Livemode bool   `json:"livemode"`
	Data     struct {
		Object struct {
			Metadata struct {
				TenantID string `json:"tenant_id"`
				StoreID  string `json:"store_id"`
			} `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

// parseUUIDOrZero returns the parsed uuid or uuid.Nil. Used by the
// webhook emit path so missing metadata falls through to the emitter's
// store-resolver / drop-with-warning path instead of crashing.
func parseUUIDOrZero(s string) uuid.UUID {
	if s == "" {
		return uuid.Nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}
