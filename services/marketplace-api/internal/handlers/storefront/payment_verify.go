// Package storefront — payment_verify.go: client-side payment verification.
//
// POST /api/v1/storefront/stores/:storeSlug/orders/:id/verify-payment
//
// After the Razorpay Checkout widget completes on the storefront, the
// `handler` callback returns a triplet
// {razorpay_order_id, razorpay_payment_id, razorpay_signature}. The
// signature is HMAC-SHA256(order_id + "|" + payment_id, key_secret),
// which proves the payment came from a genuine Razorpay flow keyed to
// the merchant's secret. We re-derive it here, and on match, mark the
// order paid by re-using the same handlePaymentSucceeded path the
// webhook handler uses. This avoids waiting on async webhooks (which
// may not reach the cluster in test environments) without loosening
// any security guarantees.
package storefront

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

type verifyPaymentRequest struct {
	RazorpayOrderID   string `json:"razorpay_order_id" binding:"required"`
	RazorpayPaymentID string `json:"razorpay_payment_id" binding:"required"`
	RazorpaySignature string `json:"razorpay_signature" binding:"required"`
}

// HandleVerifyPayment processes the post-checkout callback from a
// client-side Razorpay widget. It is mounted under the storefront
// group so the URL carries the store slug; the order id comes from
// the path. The handler is intentionally a thin wrapper around the
// existing webhook business logic.
func (h *WebhookHandler) HandleVerifyPayment(c *gin.Context) {
	provider := "razorpay" // only razorpay supports this client callback today

	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing order id"})
		return
	}

	var body verifyPaymentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "details": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Resolve the store from the StoreContext middleware so the gateway
	// config query is scoped by store_id. Without this, the query would
	// match the first active razorpay config across all tenants — the
	// cross-tenant leak that the 2026-04-10 prod-readiness spec §2.3
	// flagged.
	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		h.logError("verify-payment: store context missing", "order_id", orderID)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store context missing"})
		return
	}
	storeID, perr := uuid.Parse(store.ID)
	if perr != nil {
		h.logError("verify-payment: invalid store id", "raw_id", store.ID, "err", perr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway not configured"})
		return
	}

	// Load the active razorpay gateway config scoped to this store.
	var cfg webhookGatewayConfigRow
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true", storeID, provider).
		First(&cfg).Error; err != nil {
		h.logError("verify-payment: no active gateway config",
			"store_id", storeID.String(), "provider", provider, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway not configured"})
		return
	}

	// Razorpay's checkout signature: HMAC-SHA256(order|payment, secret).
	// Prefer the dedicated webhook secret over the API secret (spec §2.6);
	// both are envelope-encrypted at rest — unwrap before HMAC.
	signingCipher := cfg.WebhookSecretEncrypted
	if signingCipher == "" {
		signingCipher = cfg.SecretKeyEncrypted
	}
	secretKey, err := h.resolveWebhookSecret(signingCipher)
	if err != nil {
		h.logError("verify-payment: decrypt signing secret failed", "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gateway not configured"})
		return
	}
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(body.RazorpayOrderID + "|" + body.RazorpayPaymentID))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(body.RazorpaySignature), []byte(expected)) {
		h.logError("verify-payment: signature mismatch",
			"order_id", orderID,
			"razorpay_order_id", body.RazorpayOrderID,
			"razorpay_payment_id", body.RazorpayPaymentID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "signature mismatch"})
		return
	}

	// Reuse the same handler the webhook flow uses.
	evt := &payment.WebhookEvent{
		ProviderEventID:   "rzp-client-" + body.RazorpayPaymentID,
		EventType:         "payment.succeeded",
		OrderID:           orderID,
		ProviderPaymentID: body.RazorpayPaymentID,
		Amount:            decimal.Zero,
		CurrencyCode:      "INR",
		PaymentMethod:     "razorpay",
		RawPayload:        []byte("{}"),
	}
	h.handlePaymentSucceeded(ctx, provider, evt)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
