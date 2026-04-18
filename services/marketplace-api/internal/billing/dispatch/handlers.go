package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// handleCheckoutSessionCompleted binds billing_currency (first-write wins via
// COALESCE), stores stripe_subscription_id, and advances status from signup →
// trialing.
//
// TODO(P3): emit audit event on successful bind.
func handleCheckoutSessionCompleted(ctx context.Context, tx *gorm.DB, raw []byte) error {
	var e struct {
		Data struct {
			Object struct {
				Subscription string `json:"subscription"`
				Customer     string `json:"customer"`
				Currency     string `json:"currency"`
				Metadata     struct {
					Plan   string `json:"plan"`
					Period string `json:"period"`
				} `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("dispatch: unmarshal checkout: %w", err)
	}
	obj := e.Data.Object
	if obj.Customer == "" || obj.Currency == "" {
		return errors.New("dispatch: checkout.session.completed missing customer/currency")
	}
	currency := strings.ToUpper(obj.Currency)
	res := tx.WithContext(ctx).Exec(
		`UPDATE store_subscriptions
         SET stripe_subscription_id = ?,
             billing_currency       = COALESCE(billing_currency, ?),
             status                 = CASE status WHEN 'signup' THEN 'trialing' ELSE status END,
             updated_at             = ?
         WHERE stripe_customer_id = ?`,
		obj.Subscription, currency, time.Now(), obj.Customer,
	)
	if res.Error != nil {
		return fmt.Errorf("dispatch: checkout update: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return errors.New("dispatch: no subscription for customer")
	}
	return nil
}

// HandleCheckoutSessionCompletedForTesting exposes the checkout handler
// directly so Task 12 tests can exercise the COALESCE lock-in path in
// isolation, without needing a full Dispatcher and StripeWebhookEvent.
func HandleCheckoutSessionCompletedForTesting(ctx context.Context, tx *gorm.DB, raw []byte) error {
	return handleCheckoutSessionCompleted(ctx, tx, raw)
}

// handleSubscriptionUpdated refreshes period boundaries and cancel_at_period_end.
//
// TODO(P3): emit audit event on period transition.
func handleSubscriptionUpdated(ctx context.Context, tx *gorm.DB, raw []byte) error {
	var e struct {
		Data struct {
			Object struct {
				ID                 string `json:"id"`
				Customer           string `json:"customer"`
				CurrentPeriodStart int64  `json:"current_period_start"`
				CurrentPeriodEnd   int64  `json:"current_period_end"`
				CancelAtPeriodEnd  bool   `json:"cancel_at_period_end"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("dispatch: unmarshal subscription.updated: %w", err)
	}
	obj := e.Data.Object
	if obj.Customer == "" {
		return errors.New("dispatch: subscription.updated missing customer")
	}
	var periodStart, periodEnd *time.Time
	if obj.CurrentPeriodStart > 0 {
		ts := time.Unix(obj.CurrentPeriodStart, 0).UTC()
		periodStart = &ts
	}
	if obj.CurrentPeriodEnd > 0 {
		ts := time.Unix(obj.CurrentPeriodEnd, 0).UTC()
		periodEnd = &ts
	}
	res := tx.WithContext(ctx).Exec(
		`UPDATE store_subscriptions
         SET current_period_start = ?,
             current_period_end   = ?,
             cancel_at_period_end = ?,
             updated_at           = ?
         WHERE stripe_customer_id = ?`,
		periodStart, periodEnd, obj.CancelAtPeriodEnd, time.Now(), obj.Customer,
	)
	if res.Error != nil {
		return fmt.Errorf("dispatch: subscription update: %w", res.Error)
	}
	return nil
}

// handleSubscriptionDeleted marks status=expired.
// TODO(P3): richer state transitions (e.g. store_closed grace period).
func handleSubscriptionDeleted(ctx context.Context, tx *gorm.DB, raw []byte) error {
	customer, err := extractCustomerID(raw)
	if err != nil {
		return err
	}
	return simpleStatusUpdate(ctx, tx, customer, "expired")
}

// handleInvoicePaid is audit-only in P2; P3 handles trial → active transition.
//
// TODO(P3): advance status trialing → active on first paid invoice.
func handleInvoicePaid(ctx context.Context, tx *gorm.DB, raw []byte) error {
	return nil
}

// handleInvoicePaymentFailed marks the subscription as past_due.
//
// TODO(P3): emit notification event for merchant dunning flow.
func handleInvoicePaymentFailed(ctx context.Context, tx *gorm.DB, raw []byte) error {
	customer, err := extractCustomerID(raw)
	if err != nil {
		return err
	}
	return simpleStatusUpdate(ctx, tx, customer, "past_due")
}

// handleInvoicePaymentActionRequired marks the subscription as
// payment_action_required so the merchant is prompted to complete 3DS.
//
// TODO(P3): emit notification event for merchant action prompt.
func handleInvoicePaymentActionRequired(ctx context.Context, tx *gorm.DB, raw []byte) error {
	customer, err := extractCustomerID(raw)
	if err != nil {
		return err
	}
	return simpleStatusUpdate(ctx, tx, customer, "payment_action_required")
}

// handleCustomerUpdated is audit-only in P2.
// TODO(P3): sync billing email / name changes.
func handleCustomerUpdated(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// handleChargeRefunded is audit-only in P2.
// TODO(P3): create refund record in billing ledger.
func handleChargeRefunded(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// handlePaymentMethodAttached is audit-only in P2.
// TODO(P3): store default payment method fingerprint.
func handlePaymentMethodAttached(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// handlePaymentMethodDetached is audit-only in P2.
// TODO(P3): clear stored payment method if it was the default.
func handlePaymentMethodDetached(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// handleFraudWarning is audit-only in P2.
// TODO(P3): flag account for manual review via arbitrage_flag column.
func handleFraudWarning(ctx context.Context, tx *gorm.DB, raw []byte) error { return nil }

// extractCustomerID parses the customer ID from a standard Stripe event
// payload that wraps the object under data.object.customer.
func extractCustomerID(raw []byte) (string, error) {
	var e struct {
		Data struct {
			Object struct {
				Customer string `json:"customer"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return "", fmt.Errorf("dispatch: unmarshal customer: %w", err)
	}
	if e.Data.Object.Customer == "" {
		return "", errors.New("dispatch: missing customer")
	}
	return e.Data.Object.Customer, nil
}

// simpleStatusUpdate sets the status column on the row matching the given
// Stripe customer ID. A zero RowsAffected is not treated as an error here
// because some events may arrive for stores that have already been cleaned up.
func simpleStatusUpdate(ctx context.Context, tx *gorm.DB, customer, newStatus string) error {
	res := tx.WithContext(ctx).Exec(
		`UPDATE store_subscriptions
         SET status = ?, updated_at = ?
         WHERE stripe_customer_id = ?`,
		newStatus, time.Now(), customer,
	)
	if res.Error != nil {
		return fmt.Errorf("dispatch: status update: %w", res.Error)
	}
	return nil
}
