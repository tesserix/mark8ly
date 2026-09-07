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

// StripeCustomerEnsurer attaches a Stripe customer to a subscription row that
// has none. *subscription.Service satisfies it.
//
// Needed since #827: a row created at signup deliberately carries no Stripe
// customer, and trial.Subscribe refuses without one. Without this the card-add
// step would answer 412 for every merchant who signed up after that change —
// which is exactly the shape of bug #827 was opened for, moved one step later.
type StripeCustomerEnsurer interface {
	EnsureStripeCustomer(ctx context.Context, tenantID, storeID uuid.UUID,
		email, name string) (*subscription.StoreSubscription, error)
}

// TrialBillingHandler exposes the deferred-charge trial subscription endpoint.
type TrialBillingHandler struct {
	subscriber TrialSubscriber
	customers  StripeCustomerEnsurer
	logger     *slog.Logger
}

// WithCustomerEnsurer wires the Stripe-customer step that must run before a
// card-backed subscription can be created. Nil-safe: without it the handler
// behaves exactly as it did before #827, answering 412 missing_stripe_customer
// for a row that has none.
func (h *TrialBillingHandler) WithCustomerEnsurer(e StripeCustomerEnsurer) *TrialBillingHandler {
	h.customers = e
	return h
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

	// A row created at signup carries no Stripe customer by design (#827);
	// mint one now that the merchant is actually adding a card.
	//
	// A failure here is logged and NOT returned: Subscribe below produces the
	// merchant-facing answer for a row with no customer — 412
	// missing_stripe_customer — and that is the honest thing to tell someone
	// whose card we cannot take yet. The log line carries why.
	if h.customers != nil {
		if _, err := h.customers.EnsureStripeCustomer(c.Request.Context(),
			tenantID, storeID, c.GetString("user_email"), ""); err != nil && h.logger != nil {
			h.logger.Error("trial subscribe: could not attach a stripe customer",
				"store_id", storeID, "tenant_id", tenantID, "err", err)
		}
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
