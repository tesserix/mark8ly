// Package admin — subscription_trial.go: HTTP handler for the deferred-charge
// card-add flow (§5.3). Kept in a separate file from subscription.go (P3) so
// P5 work is isolated and the existing handler is untouched.
package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// TrialSubscriber is the interface the handler needs. *trial.Subscriber satisfies
// this. The indirection keeps handler tests fast — no DB or Stripe required.
type TrialSubscriber interface {
	Subscribe(ctx context.Context, in trial.SubscribeInput) (*trial.SubscribeResult, error)
}

// TrialBillingHandler exposes the deferred-charge trial subscription endpoint.
type TrialBillingHandler struct {
	subscriber TrialSubscriber
	logger     *slog.Logger
}

// NewTrialBillingHandler constructs a TrialBillingHandler.
func NewTrialBillingHandler(s TrialSubscriber, logger *slog.Logger) *TrialBillingHandler {
	return &TrialBillingHandler{subscriber: s, logger: logger}
}

type trialSubscribeRequest struct {
	Plan     string `json:"plan"     binding:"required,oneof=starter studio pro"`
	Period   string `json:"period"   binding:"required,oneof=monthly annual"`
	Currency string `json:"currency" binding:"required,len=3"`
}

// Subscribe handles POST /admin/stores/:storeId/billing/subscription.
//
// Deferred-charge card-add flow: creates a Stripe subscription with trial_end
// set to signup_date + 90d so Stripe charges at day 90, not immediately.
// The subscription.status is NOT mutated here — the webhook owns that transition
// via statemachine.Transition.
func (h *TrialBillingHandler) Subscribe(c *gin.Context) {
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

	var req trialSubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	res, err := h.subscriber.Subscribe(c.Request.Context(), trial.SubscribeInput{
		TenantID: tenantID,
		StoreID:  storeID,
		Plan:     subscription.SubscriptionPlan(req.Plan),
		Period:   subscription.SubscriptionPeriod(req.Period),
		Currency: strings.ToLower(req.Currency),
	})
	switch {
	case errors.Is(err, trial.ErrSubscriptionAlreadyActive):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "already_active"})
		return
	case errors.Is(err, trial.ErrMissingStripeCustomer):
		c.AbortWithStatusJSON(http.StatusPreconditionFailed, gin.H{"error": "missing_stripe_customer"})
		return
	case err != nil:
		if h.logger != nil {
			h.logger.Error("trial subscribe failed", "err", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "subscribe_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"stripe_subscription_id": res.StripeSubscriptionID,
		"trial_end_unix":         res.TrialEndUnix,
	})
}
