// Package storefront — payment_confirm.go: server-side payment confirmation.
//
// POST /api/v1/storefront/stores/:storeSlug/orders/:id/confirm-payment
//
// This is the Cashfree-shaped sibling of payment_verify.go. Razorpay Checkout
// hands the browser a signed {order_id, payment_id, signature} triplet the
// server can re-derive; the Cashfree SDK returns nothing signed at all, so
// there is no client value to verify. The authority is the gateway itself:
// after checkout closes, the storefront calls this endpoint and the server asks
// Cashfree what actually happened, then reuses the same handlePaymentSucceeded
// path the webhook flow uses.
//
// That makes this a STRONGER gate than the verify path, not a weaker one — no
// client-supplied value is trusted at any point:
//
//   - The provider and the provider-side order id come from THIS order's
//     payment_transactions row, never from the request body. A caller cannot
//     point the confirm at someone else's paid Cashfree order.
//   - The gateway credentials come from the store's own active gateway config,
//     so the poll can only ever see payments on that merchant's account.
//   - The captured amount must cover what we recorded at intent creation,
//     so a ₹1 payment cannot settle a ₹5,000 order.
//
// Like the verify path, it exists so a paid order confirms immediately rather
// than waiting on an async webhook that may never reach the cluster in a test
// environment.
package storefront

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// confirmPaymentTxRow is the pending payment_transactions projection the
// confirm path needs: which gateway to ask, which provider-side order to ask
// about, and how much we expect to have been captured.
type confirmPaymentTxRow struct {
	Provider         string          `gorm:"column:provider"`
	ProviderIntentID string          `gorm:"column:provider_intent_id"`
	Amount           decimal.Decimal `gorm:"column:amount"`
	Status           string          `gorm:"column:status"`
}

// HandleConfirmPayment polls the payment provider for an order's settled
// payment and, when the gateway confirms a capture, marks the order paid.
//
// Responses always carry an explicit "status" so the storefront can branch:
//
//	{"status":"ok"}       — the gateway confirms a capture; the order is paid
//	{"status":"pending"}  — no captured payment yet; the buyer can retry
//
// A pending result is a 200, not an error: "the customer has not paid yet" is
// a normal outcome of this poll, and returning 4xx would make the storefront
// render a failure for a buyer who simply closed the sheet.
func (h *WebhookHandler) HandleConfirmPayment(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing order id"})
		return
	}
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	ctx := c.Request.Context()

	// Store scoping, same as the verify path: without it the gateway-config
	// query would match the first active config across all tenants.
	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		h.logError("confirm-payment: store context missing", "order_id", orderID)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store context missing"})
		return
	}
	storeID, perr := uuid.Parse(store.ID)
	if perr != nil {
		h.logError("confirm-payment: invalid store id", "raw_id", store.ID, "err", perr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway not configured"})
		return
	}

	// The provider and the provider-side order id are read from OUR ledger,
	// not the request body — this is what binds the poll to this order.
	var tx confirmPaymentTxRow
	if err := h.db.WithContext(ctx).
		Table("payment_transactions").
		Select("provider", "provider_intent_id", "amount", "status").
		Where("order_id = ? AND store_id = ?", orderUUID, storeID).
		Order("created_at DESC").
		First(&tx).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no payment on this order"})
			return
		}
		h.logError("confirm-payment: payment transaction lookup failed",
			"order_id", orderID, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not read payment"})
		return
	}

	// Already captured — a second confirm (buyer refreshed, two tabs) is an
	// idempotent success, not a re-settlement.
	if tx.Status == "captured" {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	if tx.ProviderIntentID == "" {
		h.logError("confirm-payment: transaction has no provider intent id",
			"order_id", orderID, "provider", tx.Provider)
		c.JSON(http.StatusConflict, gin.H{"error": "payment was never started with the provider"})
		return
	}

	var cfg webhookGatewayConfigRow
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true", storeID, tx.Provider).
		First(&cfg).Error; err != nil {
		h.logError("confirm-payment: no active gateway config",
			"store_id", storeID.String(), "provider", tx.Provider, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway not configured"})
		return
	}

	// Resolve via decryptAPIKey, not the Encryptor directly: a rewrapped row
	// holds a "gsm://" reference that Decrypt cannot read. The Store handles
	// both forms.
	apiKey, err := h.decryptAPIKey(ctx, cfg.APIKey)
	if err != nil {
		h.logError("confirm-payment: decrypt api key failed", "provider", tx.Provider, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway not configured"})
		return
	}
	secretKey, err := h.decryptAPIKey(ctx, cfg.SecretKeyEncrypted)
	if err != nil {
		h.logError("confirm-payment: decrypt secret key failed", "provider", tx.Provider, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway not configured"})
		return
	}

	gateway, err := payment.NewGateway(tx.Provider, apiKey, secretKey, cfg.Mode)
	if err != nil {
		h.logError("confirm-payment: gateway init failed", "provider", tx.Provider, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway not configured"})
		return
	}

	// Providers with a client-side signature (Razorpay) verify through
	// /verify-payment instead; there is nothing to poll for them.
	poller, ok := gateway.(payment.OrderStatusGateway)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider " + tx.Provider + " does not support server-side payment confirmation",
		})
		return
	}

	settled, err := poller.FetchOrderPayment(ctx, tx.ProviderIntentID)
	if err != nil {
		h.logError("confirm-payment: gateway status poll failed",
			"order_id", orderID, "provider", tx.Provider, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not reach the payment provider"})
		return
	}
	// Belt-and-braces on the capability contract: FetchOrderPayment is
	// specified to return only captured payments, so anything else here means
	// the adapter is not honouring it. Treat it as unpaid rather than trusting
	// the nil-check alone to have filtered a failed attempt.
	if settled == nil || settled.Status != "payment.succeeded" {
		c.JSON(http.StatusOK, gin.H{"status": "pending"})
		return
	}

	// Amount binding. The provider-side order id already ties this payment to
	// this order, so this is defence in depth against a gateway-side amount
	// mismatch (a partially-paid order, or an order whose total was recomputed
	// after the intent was created) settling the order in full.
	if settled.Amount.IsPositive() && settled.Amount.LessThan(tx.Amount) {
		h.logError("confirm-payment: captured amount is short of the recorded intent",
			"order_id", orderID,
			"provider", tx.Provider,
			"captured", settled.Amount.String(),
			"expected", tx.Amount.String())
		c.JSON(http.StatusConflict, gin.H{"error": "captured amount does not cover the order"})
		return
	}

	// Reuse the same handler the webhook flow uses, so notifications, loyalty
	// points, invoice mail and order confirmation all fire exactly as they
	// would have on a webhook delivery.
	h.handlePaymentSucceeded(ctx, tx.Provider, &payment.WebhookEvent{
		ProviderEventID:   "confirm-" + tx.Provider + "-" + settled.ProviderPaymentID,
		EventType:         "payment.succeeded",
		OrderID:           orderID,
		ProviderPaymentID: settled.ProviderPaymentID,
		Amount:            settled.Amount,
		CurrencyCode:      settled.CurrencyCode,
		PaymentMethod:     settled.PaymentMethod,
		RawPayload:        []byte("{}"),
	})

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
