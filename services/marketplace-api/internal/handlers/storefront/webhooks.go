// Package storefront — webhooks.go: payment-provider webhook receiver.
//
// Two routes mount here:
//
//	POST /api/v1/webhooks/:storeSlug/:provider   — scoped (preferred)
//	POST /api/v1/webhooks/:provider              — legacy, fail-closed on ambiguity
//
// The scoped form resolves the store from :storeSlug and looks up the
// payment gateway config by (store_id, provider). The legacy form
// queries by provider alone; it returns 200 "ambiguous" and drops the
// event when more than one tenant has an active config for the same
// provider — this closes the cross-tenant webhook-spoofing gap flagged
// in the 2026-04-10 prod-readiness spec §2.3. New merchant onboarding
// must use the scoped URL.
//
// Webhook routes sit at the API root (not under
// /storefront/stores/:storeSlug/...) because providers send callbacks
// to a URL baked into their dashboard config.
package storefront

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// webhookGatewayConfigRow is a read-only projection of payment_gateway_configs
// used to look up the webhook secret for signature verification.
// ID + TenantID are loaded so MaybeRewrap can scope the lazy migration
// and we know which row to UPDATE when rewriting a legacy inline reference.
// WebhookSecretEncrypted is preferred over SecretKeyEncrypted when
// non-empty (spec §2.6).
type webhookGatewayConfigRow struct {
	ID                     string    `gorm:"column:id"`
	TenantID               string    `gorm:"column:tenant_id"`
	StoreID                uuid.UUID `gorm:"column:store_id"`
	Provider               string    `gorm:"column:provider"`
	APIKey                 string    `gorm:"column:api_key_encrypted"`
	SecretKeyEncrypted     string    `gorm:"column:secret_key_encrypted"`
	WebhookSecretEncrypted string    `gorm:"column:webhook_secret_encrypted"`
	Mode                   string    `gorm:"column:mode"`
	IsActive               bool      `gorm:"column:is_active"`
}

func (webhookGatewayConfigRow) TableName() string { return "payment_gateway_configs" }

// WebhookHandler processes provider webhook callbacks.
type WebhookHandler struct {
	db          *gorm.DB
	orderSvc    *order.Service
	storeCache  *stores.SlugCache     // optional — enables the /webhooks/:storeSlug/:provider scoped route
	giftCardSvc *giftcard.Service     // optional — when set, gift card checkout events activate cards
	loyaltySvc  *loyalty.Service      // optional — when set, awards points after payment success
	notify      *notification.Service // optional — nil-safe; emits payment_received on success
	docMailer   *orderdoc.Service     // optional — when set, fires the invoice email after a payment succeeds (the "order placed" customer-facing receipt for the buyer)
	encryptor   crypto.Encryptor      // optional — decrypts payment gateway secret_key_encrypted
	// secretStore, when non-nil, resolves gsm:// references as well as
	// legacy inline ciphertext. The Encryptor path stays as a fallback
	// so inline-mode deployments and tests keep working.
	secretStore carriersecrets.Store
	logger      *slog.Logger
}

// NewWebhookHandler constructs a WebhookHandler.
func NewWebhookHandler(db *gorm.DB, orderSvc *order.Service, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{db: db, orderSvc: orderSvc, logger: logger}
}

// WithStoreCache wires the slug→store lookup used by the scoped
// /webhooks/:storeSlug/:provider route. Without it, only the legacy
// unscoped route is served (fail-closed on ambiguity).
func (h *WebhookHandler) WithStoreCache(c *stores.SlugCache) *WebhookHandler {
	h.storeCache = c
	return h
}

// WithEncryptor attaches the envelope-encryption decoder used to unwrap
// payment-gateway secret keys before HMAC signature verification. The column
// `payment_gateway_configs.secret_key_encrypted` is written via
// crypto.Encryptor.Encrypt in admin/settings, so webhook verification must
// decrypt before feeding the secret into hmac.New. Without this, signatures
// are computed against the CIPHERTEXT and every real provider webhook 401s.
func (h *WebhookHandler) WithEncryptor(enc crypto.Encryptor) *WebhookHandler {
	h.encryptor = enc
	return h
}

// WithSecretStore wires a carriersecrets.Store so webhook signature
// verification can resolve gsm:// references and lazily rewrite legacy
// noop:/aes: rows to gsm://. Chainable.
func (h *WebhookHandler) WithSecretStore(s carriersecrets.Store) *WebhookHandler {
	h.secretStore = s
	return h
}

// decryptAPIKey resolves a stored carrier-config reference to plaintext.
// Prefers the carriersecrets.Store when wired (handles gsm:// + legacy
// inline) and falls back to the Encryptor path for inline-mode
// deployments. An empty input is passed through so tests that stub
// plaintext columns keep working.
func (h *WebhookHandler) decryptAPIKey(ctx context.Context, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if h.secretStore != nil {
		return h.secretStore.Get(ctx, stored)
	}
	if h.encryptor == nil {
		return stored, nil
	}
	return h.encryptor.Decrypt(stored)
}

// resolveWebhookSecret is the legacy (non-context) wrapper kept for
// compatibility with storefront-adjacent callers (payment_verify) that
// don't thread context through. New call sites should prefer
// decryptAPIKey, which routes via the Store when wired.
func (h *WebhookHandler) resolveWebhookSecret(stored string) (string, error) {
	if h.encryptor == nil || stored == "" {
		return stored, nil
	}
	return h.encryptor.Decrypt(stored)
}

// maybeRewrapWebhookRow migrates legacy inline references on a
// payment_gateway_configs row to gsm://. Best-effort — failures only
// log because the read already succeeded.
func (h *WebhookHandler) maybeRewrapWebhookRow(ctx context.Context, cfg webhookGatewayConfigRow, apiKey, secretKey string) {
	if h.secretStore == nil || cfg.ID == "" || cfg.TenantID == "" {
		return
	}
	rw, ok := h.secretStore.(carriersecrets.Rewrapper)
	if !ok {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.APIKey, carriersecrets.Scope{
		TenantID: cfg.TenantID,
		Domain:   "payment",
		Provider: cfg.Provider,
		Field:    "api_key",
	}, apiKey); changed {
		if err := h.db.WithContext(ctx).
			Table("payment_gateway_configs").
			Where("id = ?", cfg.ID).
			Update("api_key_encrypted", newRef).Error; err != nil {
			h.logError("webhook: rewrap api_key persist failed", "id", cfg.ID, "err", err)
		}
	}
	if cfg.SecretKeyEncrypted == "" {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.SecretKeyEncrypted, carriersecrets.Scope{
		TenantID: cfg.TenantID,
		Domain:   "payment",
		Provider: cfg.Provider,
		Field:    "secret_key",
	}, secretKey); changed {
		if err := h.db.WithContext(ctx).
			Table("payment_gateway_configs").
			Where("id = ?", cfg.ID).
			Update("secret_key_encrypted", newRef).Error; err != nil {
			h.logError("webhook: rewrap secret_key persist failed", "id", cfg.ID, "err", err)
		}
	}
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

// WithDocMailer attaches the orderdoc service so the webhook can fire
// the customer-facing invoice email immediately after a payment is
// captured and the order transitions to confirmed. This is the
// "buyer placed an order, here's their invoice" receipt — distinct
// from the post-delivery receipt PDF that fires from the shipment-
// status handler. Nil-safe; without it the dispatch is skipped and
// invoices only go out via the admin "Email to customer" affordance
// or the auto-fire that the OrdersHandler.Confirm path triggers when
// a merchant manually clicks Confirm.
func (h *WebhookHandler) WithDocMailer(svc *orderdoc.Service) *WebhookHandler {
	h.docMailer = svc
	return h
}

// WithLoyaltyService attaches the loyalty service so payment-succeeded
// events award points to enrolled customers. Idempotent — retries are safe.
func (h *WebhookHandler) WithLoyaltyService(svc *loyalty.Service) *WebhookHandler {
	h.loyaltySvc = svc
	return h
}

// HandleWebhook handles POST /api/v1/webhooks/:provider — legacy route.
//
// **Deprecated.** Prefer /api/v1/webhooks/:storeSlug/:provider
// (HandleScopedWebhook). This variant only processes events when
// exactly one active gateway config exists for the given provider
// across the whole database — otherwise it fails closed with a log
// line and drops the event, because picking "first active" would let
// a webhook for tenant A match tenant B's config (cross-tenant data
// leak; see 2026-04-10 prod-readiness spec §2.3).
//
// The handler always returns 200 to the provider, even when internal
// processing fails. Returning a non-2xx causes providers to retry,
// which is worse than logging and investigating.
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

	// Ambiguity check: count active configs for this provider. If more
	// than one tenant serves this provider, we cannot safely guess which
	// store the callback belongs to, so we fail closed. Merchants must
	// migrate to /webhooks/:storeSlug/:provider.
	var activeCount int64
	if err := h.db.WithContext(ctx).
		Table("payment_gateway_configs").
		Where("provider = ? AND is_active = true", provider).
		Count(&activeCount).Error; err != nil {
		h.logError("webhook: count active configs failed",
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}
	if activeCount == 0 {
		h.logError("webhook: no active gateway config", "provider", provider)
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}
	if activeCount > 1 {
		h.logError("webhook: ambiguous gateway config — multiple active tenants; migrate merchant to /webhooks/:storeSlug/:provider",
			"provider", provider, "active_count", activeCount)
		c.JSON(http.StatusOK, gin.H{"status": "ambiguous_store_configs"})
		return
	}

	var cfg webhookGatewayConfigRow
	if err := h.db.WithContext(ctx).
		Where("provider = ? AND is_active = true", provider).
		First(&cfg).Error; err != nil {
		h.logError("webhook: gateway config fetch failed",
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}

	if h.logger != nil {
		h.logger.Warn("webhook: using legacy unscoped route — migrate this merchant to /webhooks/:storeSlug/:provider",
			"provider", provider, "store_id", cfg.StoreID.String())
	}

	h.processWithConfig(c, ctx, provider, body, cfg)
}

// HandleScopedWebhook handles POST /api/v1/webhooks/:storeSlug/:provider
// — the preferred route. Resolves the store from the slug, then looks
// up the gateway config scoped by store_id. Cross-tenant webhook
// spoofing is impossible because the config is hard-pinned to the
// store named in the URL.
func (h *WebhookHandler) HandleScopedWebhook(c *gin.Context) {
	storeSlug := strings.ToLower(c.Param("storeSlug"))
	provider := strings.ToLower(c.Param("provider"))
	if storeSlug == "" || provider == "" {
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}
	if h.storeCache == nil {
		h.logError("webhook: scoped route hit but storeCache not wired",
			"provider", provider, "store_slug", storeSlug)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logError("webhook: failed to read body",
			"provider", provider, "store_slug", storeSlug, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}
	defer c.Request.Body.Close()

	ctx := c.Request.Context()

	store, err := h.storeCache.Get(ctx, storeSlug)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
			return
		}
		h.logError("webhook: store lookup failed",
			"provider", provider, "store_slug", storeSlug, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}
	if store == nil {
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}
	storeID, perr := uuid.Parse(store.ID)
	if perr != nil {
		h.logError("webhook: store row has invalid UUID",
			"provider", provider, "store_slug", storeSlug, "raw_id", store.ID, "err", perr)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}

	var cfg webhookGatewayConfigRow
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true", storeID, provider).
		First(&cfg).Error; err != nil {
		h.logError("webhook: no active gateway config for (store, provider)",
			"provider", provider, "store_slug", storeSlug, "store_id", storeID.String(), "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	h.processWithConfig(c, ctx, provider, body, cfg)
}

// processWithConfig runs the common decrypt, rewrap, verify, idempotency,
// and dispatch flow once a webhook has been matched to a (store,
// provider, config) tuple by one of the route entrypoints. Prefers
// WebhookSecretEncrypted over SecretKeyEncrypted (spec §2.6); legacy
// merchants without a dedicated webhook secret keep working.
func (h *WebhookHandler) processWithConfig(c *gin.Context, ctx context.Context, provider string, body []byte, cfg webhookGatewayConfigRow) {
	apiKey, err := h.decryptAPIKey(ctx, cfg.APIKey)
	if err != nil {
		h.logError("webhook: api_key decrypt failed",
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}

	// Choose signing secret: prefer webhook_secret when configured.
	signingCipher := cfg.WebhookSecretEncrypted
	signingField := "webhook_secret"
	if signingCipher == "" {
		signingCipher = cfg.SecretKeyEncrypted
		signingField = "secret_key"
	}
	signingSecret, err := h.decryptAPIKey(ctx, signingCipher)
	if err != nil {
		h.logError("webhook: signing_secret decrypt failed",
			"provider", provider, "field", signingField, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}
	// Still decrypt the raw secret_key for the rewrap migration path so
	// MaybeRewrap can migrate both fields independently.
	secretKey, err := h.decryptAPIKey(ctx, cfg.SecretKeyEncrypted)
	if err != nil {
		h.logError("webhook: secret_key decrypt failed",
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}

	// Lazy migration: rewrite legacy inline refs to gsm://.
	h.maybeRewrapWebhookRow(ctx, cfg, apiKey, secretKey)

	// Instantiate the gateway for verification.
	gateway, err := payment.NewGateway(provider, apiKey, signingSecret, cfg.Mode)
	if err != nil {
		h.logError("webhook: gateway instantiation failed",
			"provider", provider, "err", err)
		c.JSON(http.StatusOK, gin.H{"status": "error"})
		return
	}

	signature := extractWebhookSignature(c, provider)

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
		c.JSON(http.StatusOK, gin.H{"status": "duplicate"})
		return
	}

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
			card    *giftcard.GiftCard
			flipped bool
			err     error
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

		// Order placed + paid → the buyer's invoice email goes out now.
		// Detached goroutine: an email-provider blip must NOT make the
		// webhook retry, because order.Confirm already committed and a
		// retry would push the order through invalid-transition guards.
		// Same fire-and-forget contract the admin
		// OrdersHandler.dispatchInvoiceEmail uses on the manual-Confirm
		// path. Nil-safe — when the docMailer isn't wired this is a
		// no-op and the order still confirms cleanly.
		if h.docMailer != nil {
			go func() {
				dctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := h.docMailer.SendInvoice(dctx, orderID); err != nil {
					h.logError("webhook: invoice email dispatch failed",
						"order_id", evt.OrderID,
						"err", err)
				}
			}()
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
		TenantID    uuid.UUID  `gorm:"column:tenant_id"`
		StoreID     uuid.UUID  `gorm:"column:store_id"`
		CustomerID  *uuid.UUID `gorm:"column:customer_id"`
		OrderNumber string     `gorm:"column:order_number"`
	}
	if err := h.db.WithContext(ctx).
		Table("orders").
		Select("tenant_id, store_id, customer_id, order_number").
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

	// Also notify the customer's own bell when the order belongs to a
	// logged-in customer profile (recipient_user_id = customer profile id).
	if row.CustomerID != nil {
		custMsg := "Your payment for order " + row.OrderNumber + " was received — your order is confirmed."
		recipient := row.CustomerID.String()
		notification.EmitToCustomer(ctx, h.notify, h.logger, notification.Notification{
			TenantID:        row.TenantID,
			StoreID:         row.StoreID,
			RecipientUserID: &recipient,
			Type:            notification.TypeOrderPlaced,
			Title:           "Order confirmed",
			Message:         &custMsg,
			ResourceType:    &resourceType,
			ResourceID:      &orderID,
		})
	}
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
