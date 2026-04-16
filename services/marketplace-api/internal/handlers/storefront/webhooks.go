// Package storefront — webhooks.go: POST /api/v1/webhooks/:provider.
// Receives payment provider webhook callbacks, verifies signatures,
// deduplicates via the webhook_events table, and updates payment/order
// state. Webhook routes sit at the API root, NOT under
// /storefront/stores/:storeSlug, because providers send callbacks to a
// fixed URL.
package storefront

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/payment"
)

// webhookGatewayConfigRow is a read-only projection of payment_gateway_configs
// used to look up the webhook secret for signature verification.
type webhookGatewayConfigRow struct {
	Provider           string `gorm:"column:provider"`
	APIKey             string `gorm:"column:api_key_encrypted"`
	SecretKeyEncrypted string `gorm:"column:secret_key_encrypted"`
	Mode               string `gorm:"column:mode"`
	IsActive           bool   `gorm:"column:is_active"`
}

func (webhookGatewayConfigRow) TableName() string { return "payment_gateway_configs" }

// WebhookHandler processes provider webhook callbacks.
type WebhookHandler struct {
	db          *gorm.DB
	orderSvc    *order.Service
	giftCardSvc *giftcard.Service     // optional — when set, gift card checkout events activate cards
	loyaltySvc  *loyalty.Service      // optional — when set, awards points after payment success
	notify      *notification.Service // optional — nil-safe; emits payment_received on success
	logger      *slog.Logger
}

// NewWebhookHandler constructs a WebhookHandler.
func NewWebhookHandler(db *gorm.DB, orderSvc *order.Service, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{db: db, orderSvc: orderSvc, logger: logger}
}

// WithNotifier attaches the notification service so payment-succeeded
// events fire in-app notifications. Nil-safe.
func (h *WebhookHandler) WithNotifier(svc *notification.Service) *WebhookHandler {
	h.notify = svc
	return h
}

// WithGiftCardService attaches the gift card service so webhook events
// carrying `gift_card_id` metadata can activate pending cards.
func (h *WebhookHandler) WithGiftCardService(svc *giftcard.Service) *WebhookHandler {
	h.giftCardSvc = svc
	return h
}

// WithLoyaltyService attaches the loyalty service so payment-succeeded
// events award points to enrolled customers. Idempotent — retries are safe.
func (h *WebhookHandler) WithLoyaltyService(svc *loyalty.Service) *WebhookHandler {
	h.loyaltySvc = svc
	return h
}

// HandleWebhook handles POST /api/v1/webhooks/:provider.
//
// The handler always returns 200 to the provider, even when internal
// processing fails. Returning a non-2xx causes providers to retry, which
// is worse than logging and investigating. Errors are logged.
func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	provider := strings.ToLower(c.Param("provider"))
	if provider == "" {
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	// Read raw body for signature verification. Must happen before any
	// JSON binding consumes the reader.
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logError("webhook: failed to read body", "provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}
	defer c.Request.Body.Close()

	ctx := c.Request.Context()

	// Look up the gateway config by provider name. We need the first active
	// config to get the webhook/secret key for verification. For webhooks
	// we pick the first active config since the provider sends to one URL.
	var cfg webhookGatewayConfigRow
	if err := h.db.WithContext(ctx).
		Where("provider = ? AND is_active = true", provider).
		First(&cfg).Error; err != nil {
		h.logError("webhook: no active gateway config for provider",
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	// Instantiate the gateway for verification.
	gateway, err := payment.NewGateway(provider, cfg.APIKey, cfg.SecretKeyEncrypted, cfg.Mode)
	if err != nil {
		h.logError("webhook: gateway instantiation failed",
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}

	// Extract signature from provider-specific headers.
	signature := extractWebhookSignature(c, provider)

	// Verify webhook signature and parse the event.
	evt, err := gateway.VerifyWebhook(ctx, body, signature)
	if err != nil {
		h.logError("webhook: signature verification failed",
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "rejected"})
		return
	}

	// Idempotency check: INSERT ON CONFLICT DO NOTHING via unique index on
	// (provider, provider_event_id).
	insertResult := h.db.WithContext(ctx).Exec(
		`INSERT INTO webhook_events (id, provider, provider_event_id, event_type, payload, status, created_at)
		 VALUES (gen_random_uuid(), ?, ?, ?, ?::jsonb, 'received', now())
		 ON CONFLICT (provider, provider_event_id) DO NOTHING`,
		provider, evt.ProviderEventID, evt.EventType, string(evt.RawPayload),
	)
	if insertResult.Error != nil {
		h.logError("webhook: failed to insert event record",
			"provider", provider,
			"event_id", evt.ProviderEventID,
			"err", insertResult.Error)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}
	if insertResult.RowsAffected == 0 {
		// Already processed — idempotent skip.
		c.JSON(http.StatusOK, gin.H{"status": "duplicate"})
		return
	}

	// Process the event based on type.
	h.processEvent(ctx, provider, evt)

	// Mark event as processed.
	_ = h.db.WithContext(ctx).Exec(
		`UPDATE webhook_events SET status = 'processed', processed_at = now()
		 WHERE provider = ? AND provider_event_id = ?`,
		provider, evt.ProviderEventID,
	)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// processEvent handles the business logic for each webhook event type.
// Errors are logged but never surfaced to the provider.
//
// Gift card events are routed based on the `gift_card_id` metadata key —
// both checkout.session.* and payment_intent.* can be the trigger,
// depending on whether the buyer used Stripe Checkout (hosted) or
// Elements (embedded).
func (h *WebhookHandler) processEvent(ctx context.Context, provider string, evt *payment.WebhookEvent) {
	// Gift card dispatch takes priority when the metadata is present —
	// we don't want to fall through to order.Confirm for a card that has
	// no matching order row.
	if gcID := evt.Metadata["gift_card_id"]; gcID != "" {
		h.handleGiftCardEvent(ctx, evt, gcID)
		return
	}

	switch evt.EventType {
	case "checkout.completed":
		// For product checkouts, if the session carried an order_id, treat
		// it the same as payment.succeeded. (Gift cards were already
		// short-circuited above.)
		if evt.OrderID != "" {
			h.handlePaymentSucceeded(ctx, provider, evt)
		}
	case "payment.succeeded":
		h.handlePaymentSucceeded(ctx, provider, evt)
	case "payment.failed", "checkout.failed", "checkout.expired":
		h.handlePaymentFailed(ctx, provider, evt)
	case "refund.succeeded":
		h.handleRefundSucceeded(ctx, provider, evt)
	default:
		h.logError("webhook: unhandled event type",
			"provider", provider,
			"event_type", evt.EventType,
			"event_id", evt.ProviderEventID)
	}
}

// handleGiftCardEvent routes a gift-card-tagged webhook event to the
// giftcard service. We dispatch by checkout_session_id when available
// (checkout.* events) and fall back to the payment_intent id otherwise.
func (h *WebhookHandler) handleGiftCardEvent(ctx context.Context, evt *payment.WebhookEvent, gcID string) {
	if h.giftCardSvc == nil {
		h.logError("webhook: gift card service not wired",
			"event_type", evt.EventType, "gift_card_id", gcID)
		return
	}

	switch evt.EventType {
	case "checkout.completed", "payment.succeeded":
		var (
			card       *giftcard.GiftCard
			flipped    bool
			err        error
		)
		if evt.SessionID != "" {
			card, flipped, err = h.giftCardSvc.ActivateByCheckoutSession(ctx, evt.SessionID, evt.ProviderPaymentID)
		} else if evt.ProviderPaymentID != "" {
			card, flipped, err = h.giftCardSvc.ActivateByPaymentIntent(ctx, evt.ProviderPaymentID)
		} else {
			h.logError("webhook: gift card event missing correlation id",
				"gift_card_id", gcID)
			return
		}
		if err != nil {
			h.logError("webhook: gift card activation failed",
				"gift_card_id", gcID, "session_id", evt.SessionID, "err", err)
			return
		}
		if h.logger != nil && card != nil {
			h.logger.Info("webhook: gift card activated",
				"gift_card_id", card.ID, "flipped", flipped)
		}

	case "payment.failed", "checkout.failed", "checkout.expired":
		ref := evt.SessionID
		if ref == "" {
			ref = evt.ProviderPaymentID
		}
		if err := h.giftCardSvc.MarkPurchaseFailed(ctx, ref); err != nil {
			h.logError("webhook: gift card mark failed error",
				"gift_card_id", gcID, "err", err)
		}

	default:
		h.logError("webhook: unhandled gift card event type",
			"event_type", evt.EventType, "gift_card_id", gcID)
	}
}

// handlePaymentSucceeded updates the payment transaction and confirms the order.
func (h *WebhookHandler) handlePaymentSucceeded(ctx context.Context, provider string, evt *payment.WebhookEvent) {
	// Update payment_transaction status to "captured".
	if err := h.db.WithContext(ctx).Exec(
		`UPDATE payment_transactions SET status = 'captured', payment_method = ?, updated_at = now()
		 WHERE order_id = ?::uuid AND provider = ?`,
		evt.PaymentMethod, evt.OrderID, provider,
	).Error; err != nil {
		h.logError("webhook: failed to update payment transaction",
			"order_id", evt.OrderID,
			"err", err)
		return
	}

	// Confirm the order via order.Service.
	if h.orderSvc != nil && evt.OrderID != "" {
		orderID, err := uuid.Parse(evt.OrderID)
		if err != nil {
			h.logError("webhook: invalid order_id in event",
				"order_id", evt.OrderID,
				"err", err)
			return
		}
		paidStatus := order.PaymentStatusPaid
		if err := h.orderSvc.Confirm(ctx, nil, orderID, &paidStatus, "payment webhook: "+provider); err != nil {
			h.logError("webhook: order confirm failed",
				"order_id", evt.OrderID,
				"err", err)
			return
		}

		// Award loyalty points (idempotent — no-op if already credited or
		// customer not enrolled). Non-fatal: an earn failure must not block
		// the webhook ack.
		if h.loyaltySvc != nil {
			h.awardLoyaltyPoints(ctx, orderID)
		}

		// Emit payment_received notification. Look up tenant/store/order
		// number off the order so the notification is properly scoped and
		// the merchant sees which order was paid. Failures are logged and
		// swallowed — the webhook must not be rejected over notification
		// issues.
		h.emitPaymentReceived(ctx, orderID)
	}
}

// emitPaymentReceived fires the payment_received notification for an
// order that just transitioned to paid. Nil-safe when the notifier is
// not wired. All errors are swallowed after logging.
func (h *WebhookHandler) emitPaymentReceived(ctx context.Context, orderID uuid.UUID) {
	if h.notify == nil {
		return
	}
	var row struct {
		TenantID    uuid.UUID `gorm:"column:tenant_id"`
		StoreID     uuid.UUID `gorm:"column:store_id"`
		OrderNumber string    `gorm:"column:order_number"`
	}
	if err := h.db.WithContext(ctx).
		Table("orders").
		Select("tenant_id, store_id, order_number").
		Where("id = ?", orderID).
		Take(&row).Error; err != nil {
		h.logError("webhook: order lookup for notification failed",
			"order_id", orderID.String(), "err", err)
		return
	}
	msg := "Payment received for order " + row.OrderNumber + "."
	resourceType := "order"
	notification.Emit(ctx, h.notify, h.logger, notification.Notification{
		TenantID:     row.TenantID,
		StoreID:      row.StoreID,
		Type:         notification.TypePaymentReceived,
		Title:        "Payment received",
		Message:      &msg,
		ResourceType: &resourceType,
		ResourceID:   &orderID,
	})
}

// awardLoyaltyPoints looks up the order and credits points on the
// post-discount product value (subtotal − discount_total). Shipping and
// tax are excluded, matching industry convention.
func (h *WebhookHandler) awardLoyaltyPoints(ctx context.Context, orderID uuid.UUID) {
	var row struct {
		TenantID      uuid.UUID       `gorm:"column:tenant_id"`
		StoreID       uuid.UUID       `gorm:"column:store_id"`
		CustomerEmail string          `gorm:"column:customer_email"`
		Subtotal      decimal.Decimal `gorm:"column:subtotal"`
		DiscountTotal decimal.Decimal `gorm:"column:discount_total"`
	}
	err := h.db.WithContext(ctx).
		Table("orders").
		Select("tenant_id, store_id, customer_email, subtotal, discount_total").
		Where("id = ?", orderID).
		Take(&row).Error
	if err != nil {
		h.logError("webhook: order lookup for loyalty failed",
			"order_id", orderID.String(), "err", err)
		return
	}

	earnBase := row.Subtotal.Sub(row.DiscountTotal)
	if earnBase.LessThanOrEqual(decimal.Zero) {
		return
	}

	if err := h.loyaltySvc.AwardPoints(ctx, row.TenantID, row.StoreID, row.CustomerEmail, earnBase, orderID); err != nil {
		h.logError("webhook: loyalty award failed",
			"order_id", orderID.String(), "err", err)
	}
}

// handlePaymentFailed updates the payment transaction status.
func (h *WebhookHandler) handlePaymentFailed(ctx context.Context, provider string, evt *payment.WebhookEvent) {
	if err := h.db.WithContext(ctx).Exec(
		`UPDATE payment_transactions SET status = 'failed', updated_at = now()
		 WHERE order_id = ?::uuid AND provider = ?`,
		evt.OrderID, provider,
	).Error; err != nil {
		h.logError("webhook: failed to update payment transaction on failure",
			"order_id", evt.OrderID,
			"err", err)
	}
}

// handleRefundSucceeded updates the refund transaction status.
func (h *WebhookHandler) handleRefundSucceeded(ctx context.Context, _ string, evt *payment.WebhookEvent) {
	if err := h.db.WithContext(ctx).Exec(
		`UPDATE refund_transactions SET status = 'succeeded', updated_at = now()
		 WHERE order_id = ?::uuid AND provider_refund_id IS NOT NULL`,
		evt.OrderID,
	).Error; err != nil {
		h.logError("webhook: failed to update refund transaction",
			"order_id", evt.OrderID,
			"err", err)
	}
}

// extractWebhookSignature reads the provider-specific signature header.
func extractWebhookSignature(c *gin.Context, provider string) string {
	switch provider {
	case "stripe":
		return c.GetHeader("Stripe-Signature")
	case "razorpay":
		return c.GetHeader("X-Razorpay-Signature")
	case "paypal":
		return c.GetHeader("PAYPAL-TRANSMISSION-SIG")
	default:
		return c.GetHeader("X-Webhook-Signature")
	}
}

// logError emits a structured error log entry. Silently no-ops when logger is nil.
func (h *WebhookHandler) logError(msg string, args ...any) {
	if h.logger != nil {
		h.logger.Error(msg, args...)
	}
}
