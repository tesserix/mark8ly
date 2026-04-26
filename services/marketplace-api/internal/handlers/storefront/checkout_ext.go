// Package storefront — checkout_ext.go: extended checkout endpoint that wraps
// order creation with payment intent, shipping, and tax integration.
// POST /storefront/stores/:storeSlug/checkout (replaces the simple checkout).
//
// This file does NOT modify checkout.go. CheckoutExtHandler is registered
// alongside (or instead of) the original CheckoutHandler in routes.go.
package storefront

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/discount"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tax"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CheckoutExtHandler is the extended storefront checkout that integrates
// payment intent creation, tax calculation, and shipping rate selection.
type CheckoutExtHandler struct {
	db          *gorm.DB
	orderSvc    *order.Service
	couponSvc   *coupon.Service
	giftCardSvc *giftcard.Service  // nil-safe: no-ops when nil
	loyaltySvc  *loyalty.Service   // nil-safe: no-ops when nil
	audit       *audit.Emitter     // optional — nil-safe
	encryptor   crypto.Encryptor   // decrypts API keys for payment/tax/shipping
	// secretStore, when non-nil, resolves gsm:// references as well as
	// legacy inline ciphertext. Tracks the same pattern as shipments /
	// shipping_rates: the store is preferred, the encryptor remains the
	// fallback so unit tests and inline-mode deployments keep working.
	secretStore carriersecrets.Store
	logger      *slog.Logger
}

// NewCheckoutExtHandler constructs a CheckoutExtHandler.
func NewCheckoutExtHandler(db *gorm.DB, orderSvc *order.Service, couponSvc *coupon.Service, giftCardSvc *giftcard.Service, enc crypto.Encryptor, logger *slog.Logger) *CheckoutExtHandler {
	return &CheckoutExtHandler{db: db, orderSvc: orderSvc, couponSvc: couponSvc, giftCardSvc: giftCardSvc, encryptor: enc, logger: logger}
}

// WithAudit attaches an audit emitter so extended storefront checkouts
// emit order.created as system events. Nil-safe.
func (h *CheckoutExtHandler) WithAudit(e *audit.Emitter) *CheckoutExtHandler {
	h.audit = e
	return h
}

// WithSecretStore wires a carriersecrets.Store so the extended checkout
// resolves gsm:// references and lazily rewrites legacy noop:/aes: rows
// to gsm://. Chainable.
func (h *CheckoutExtHandler) WithSecretStore(s carriersecrets.Store) *CheckoutExtHandler {
	h.secretStore = s
	return h
}

// resolveCred routes a stored credential reference to plaintext. Uses
// the Store when wired (handles both gsm:// and legacy inline refs),
// falling back to the Encryptor path so inline-mode deployments and
// tests that only supply an Encryptor continue to work.
func (h *CheckoutExtHandler) resolveCred(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	if h.secretStore != nil {
		return h.secretStore.Get(ctx, ref)
	}
	return h.encryptor.Decrypt(ref)
}

// maybeRewrapPaymentRow lazily migrates a legacy inline reference on a
// payment_gateway_configs row to gsm://. Best-effort — persist failures
// only log; the read already succeeded.
func (h *CheckoutExtHandler) maybeRewrapPaymentRow(ctx context.Context, rowID, tenantID, provider string, cfg paymentGatewayConfigRow, apiKey, secretKey string) {
	if h.secretStore == nil || rowID == "" || tenantID == "" {
		return
	}
	rw, ok := h.secretStore.(carriersecrets.Rewrapper)
	if !ok {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.APIKey, carriersecrets.Scope{
		TenantID: tenantID,
		Domain:   "payment",
		Provider: provider,
		Field:    "api_key",
	}, apiKey); changed {
		if err := h.db.WithContext(ctx).
			Table("payment_gateway_configs").
			Where("id = ?", rowID).
			Update("api_key_encrypted", newRef).Error; err != nil {
			h.logWarn("checkout_ext: rewrap payment api_key persist failed", "id", rowID, "err", err)
		}
	}
	if cfg.SecretKey == "" {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.SecretKey, carriersecrets.Scope{
		TenantID: tenantID,
		Domain:   "payment",
		Provider: provider,
		Field:    "secret_key",
	}, secretKey); changed {
		if err := h.db.WithContext(ctx).
			Table("payment_gateway_configs").
			Where("id = ?", rowID).
			Update("secret_key_encrypted", newRef).Error; err != nil {
			h.logWarn("checkout_ext: rewrap payment secret_key persist failed", "id", rowID, "err", err)
		}
	}
}

// maybeRewrapShippingRow migrates legacy inline refs on a shipping_carrier_configs
// row to gsm://. Mirrors maybeRewrapPaymentRow for the shipping domain.
func (h *CheckoutExtHandler) maybeRewrapShippingRow(ctx context.Context, cfg carrierConfigRow, apiKey, secretKey string) {
	if h.secretStore == nil || cfg.ID == "" || cfg.TenantID == "" {
		return
	}
	rw, ok := h.secretStore.(carriersecrets.Rewrapper)
	if !ok {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.APIKey, carriersecrets.Scope{
		TenantID: cfg.TenantID,
		Domain:   "shipping",
		Provider: cfg.Provider,
		Field:    "api_key",
	}, apiKey); changed {
		if err := h.db.WithContext(ctx).
			Table("shipping_carrier_configs").
			Where("id = ?", cfg.ID).
			Update("api_key_encrypted", newRef).Error; err != nil {
			h.logWarn("checkout_ext: rewrap shipping api_key persist failed", "id", cfg.ID, "err", err)
		}
	}
	if cfg.SecretKey == "" {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.SecretKey, carriersecrets.Scope{
		TenantID: cfg.TenantID,
		Domain:   "shipping",
		Provider: cfg.Provider,
		Field:    "secret_key",
	}, secretKey); changed {
		if err := h.db.WithContext(ctx).
			Table("shipping_carrier_configs").
			Where("id = ?", cfg.ID).
			Update("secret_key_encrypted", newRef).Error; err != nil {
			h.logWarn("checkout_ext: rewrap shipping secret_key persist failed", "id", cfg.ID, "err", err)
		}
	}
}

// maybeRewrapTaxRow migrates the tax_provider_configs api_key to gsm://.
// Tax configs don't carry a secret_key, so only the api_key field is checked.
func (h *CheckoutExtHandler) maybeRewrapTaxRow(ctx context.Context, cfg *tax.TaxProviderConfig, apiKey string) {
	if h.secretStore == nil || cfg == nil || cfg.ID == uuid.Nil {
		return
	}
	rw, ok := h.secretStore.(carriersecrets.Rewrapper)
	if !ok {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.APIKeyEncrypted, carriersecrets.Scope{
		TenantID: cfg.TenantID.String(),
		Domain:   "tax",
		Provider: cfg.Provider,
		Field:    "api_key",
	}, apiKey); changed {
		if err := h.db.WithContext(ctx).
			Table("tax_provider_configs").
			Where("id = ?", cfg.ID).
			Update("api_key_encrypted", newRef).Error; err != nil {
			h.logWarn("checkout_ext: rewrap tax api_key persist failed", "id", cfg.ID, "err", err)
		}
	}
}

// SetLoyaltyService wires the loyalty service for post-checkout point awards.
func (h *CheckoutExtHandler) SetLoyaltyService(svc *loyalty.Service) {
	h.loyaltySvc = svc
}

// CheckoutExtRequest is the wire body for the extended checkout endpoint.
type CheckoutExtRequest struct {
	IdempotencyKey  string                 `json:"idempotency_key"  binding:"required"`
	CartSessionID   *string                `json:"cart_session_id"`
	CustomerEmail   string                 `json:"customer_email"   binding:"required,email"`
	CustomerName    *string                `json:"customer_name"`
	Items           []CheckoutItemRequest  `json:"items"            binding:"required,min=1"`
	ShippingAddress CheckoutAddressRequest `json:"shipping_address" binding:"required"`
	BillingAddress  *CheckoutAddressRequest `json:"billing_address"`
	ShippingService string                 `json:"shipping_service" binding:"required"`
	PaymentProvider string                 `json:"payment_provider" binding:"required"`
	Subtotal        decimal.Decimal        `json:"subtotal"         binding:"required"`
	DiscountTotal   decimal.Decimal        `json:"discount_total"`
	CouponCode      *string                `json:"coupon_code"`
	GiftCardCode    *string                `json:"gift_card_code"`
	RedeemPoints    *int                   `json:"redeem_points"`
}

// CheckoutExtResponse is the extended checkout response including payment
// token and computed totals.
type CheckoutExtResponse struct {
	OrderID         string          `json:"order_id"`
	OrderNumber     string          `json:"order_number"`
	PaymentToken    string          `json:"payment_token"`
	Provider        string          `json:"provider"`
	TaxTotal        decimal.Decimal `json:"tax_total"`
	ShippingTotal   decimal.Decimal `json:"shipping_total"`
	DiscountTotal   decimal.Decimal `json:"discount_total"`
	Total           decimal.Decimal `json:"total"`
	GiftCardApplied decimal.Decimal `json:"gift_card_applied"`
	CouponCode      *string         `json:"coupon_code,omitempty"`
	LoyaltyDiscount decimal.Decimal `json:"loyalty_discount,omitempty"`
	PointsRedeemed  int             `json:"points_redeemed,omitempty"`
}

// Checkout handles POST /storefront/stores/:storeSlug/checkout (extended).
//
// Flow:
//  1. Resolve store, validate request
//  2. Calculate shipping rate for the selected service
//  3. Calculate tax via the store's country tax strategy
//  4. Create order via order.Service.Create with computed totals
//  5. Save tax lines
//  6. Create payment intent via the store's payment gateway
//  7. Save payment transaction record
//  8. Return order + payment token to storefront
func (h *CheckoutExtHandler) Checkout(c *gin.Context) {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		h.respondErr(c, apperrors.ValidationFailed("store.id", "invalid uuid"))
		return
	}
	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		h.respondErr(c, apperrors.ValidationFailed("store.tenant_id", "invalid uuid"))
		return
	}

	var req CheckoutExtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondErr(c, apperrors.ValidationFailed("body", err.Error()))
		return
	}

	// Currency must match the store.
	for _, it := range req.Items {
		if !strings.EqualFold(it.CurrencyCode, store.CurrencyCode) {
			h.respondErr(c, apperrors.CurrencyMismatch(it.CurrencyCode, store.CurrencyCode))
			return
		}
	}

	// Delhivery (and every India domestic carrier) rejects shipment
	// creation when the destination has no phone, with a generic
	// serviceable:false remark that hides the real cause. Fail the
	// checkout here with a specific error so the storefront can
	// surface it to the customer before the order is ever persisted.
	if strings.EqualFold(req.ShippingAddress.CountryCode, "IN") {
		phone := ""
		if req.ShippingAddress.Phone != nil {
			phone = strings.TrimSpace(*req.ShippingAddress.Phone)
		}
		if phone == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, map[string]any{
				"error":   "validation_failed",
				"message": "Phone number is required for shipping to India",
			})
			return
		}
	}

	ctx := c.Request.Context()

	// ── Step 1: Resolve country config ──────────────────────────────────
	var sc country.SupportedCountry
	if err := h.db.WithContext(ctx).
		Where("country_code = ? AND is_active = true", store.CountryCode).
		First(&sc).Error; err != nil {
		h.logWarn("checkout_ext: country lookup failed",
			"country_code", store.CountryCode, "err", err)
		h.respondErr(c, apperrors.ValidationFailed("country", "store country not supported"))
		return
	}

	// ── C2 fix: Recompute subtotal server-side from unit_price * quantity ──
	computedSubtotal := decimal.Zero
	for _, it := range req.Items {
		computedSubtotal = computedSubtotal.Add(it.UnitPrice.Mul(decimal.NewFromInt(int64(it.Quantity))))
	}
	if !computedSubtotal.Equal(req.Subtotal) {
		h.logWarn("checkout_ext: client subtotal mismatch",
			"client", req.Subtotal.String(), "computed", computedSubtotal.String())
		req.Subtotal = computedSubtotal
	}

	// ── H3 fix: Cap discount_total to subtotal ─────────────────────────
	if req.DiscountTotal.GreaterThan(req.Subtotal) {
		h.respondErr(c, apperrors.ValidationFailed("discount_total",
			"discount cannot exceed subtotal"))
		return
	}

	// ── Step 1.5: Coupon preview (for shipping/tax calc) ───────────────
	// We do a lightweight validation here to compute the discount amount
	// for the grand total calculation. The authoritative validate+apply
	// happens atomically inside the order transaction (amendment FIX 1+2).
	var couponDiscount decimal.Decimal
	var appliedCouponCode *string
	var freeShippingCoupon bool
	if req.CouponCode != nil && *req.CouponCode != "" && h.couponSvc != nil {
		validateResult, err := h.couponSvc.Validate(ctx, coupon.ValidateInput{
			TenantID:      tenantID,
			StoreID:       storeID,
			Code:          *req.CouponCode,
			CustomerEmail: req.CustomerEmail,
			Subtotal:      req.Subtotal,
		})
		if err != nil {
			h.respondErr(c, err)
			return
		}
		couponDiscount = validateResult.DiscountAmount
		appliedCouponCode = &validateResult.Code
		freeShippingCoupon = validateResult.FreeShipping
	}

	// ── Step 1.6: Loyalty redemption preview ───────────────────────────
	// Validate the customer can redeem the requested points and compute
	// the currency value (points × points_value, in store currency). The
	// authoritative debit runs inside the order transaction below.
	var loyaltyDiscount decimal.Decimal
	var loyaltyPoints int
	if req.RedeemPoints != nil && *req.RedeemPoints > 0 && h.loyaltySvc != nil {
		program, err := h.loyaltySvc.GetProgram(ctx, storeID)
		if err != nil {
			h.respondErr(c, err)
			return
		}
		if program == nil || !program.IsActive {
			h.respondErr(c, apperrors.ValidationFailed("redeem_points", "loyalty program is not active"))
			return
		}
		if *req.RedeemPoints < program.MinRedeemPoints {
			h.respondErr(c, apperrors.ValidationFailed("redeem_points",
				fmt.Sprintf("minimum redemption is %d points", program.MinRedeemPoints)))
			return
		}
		customer, err := h.loyaltySvc.GetCustomer(ctx, storeID, req.CustomerEmail)
		if err != nil {
			h.respondErr(c, err)
			return
		}
		if customer == nil {
			h.respondErr(c, apperrors.ValidationFailed("redeem_points", "not enrolled in loyalty program"))
			return
		}
		if customer.PointsBalance < *req.RedeemPoints {
			h.respondErr(c, apperrors.ValidationFailed("redeem_points", "insufficient points balance"))
			return
		}
		loyaltyPoints = *req.RedeemPoints
		loyaltyDiscount = decimal.NewFromInt(int64(loyaltyPoints)).Mul(program.PointsValue).Round(2)
	}

	// ── Step 2: Calculate shipping ──────────────────────────────────────
	shippingTotal, err := h.calculateShipping(ctx, store, &sc, req)
	if err != nil {
		// M8 fix: return error instead of silently falling back to zero.
		h.respondErr(c, fmt.Errorf("shipping calculation failed: %w", err))
		return
	}

	// Free shipping coupon override.
	if freeShippingCoupon {
		shippingTotal = decimal.Zero
	}

	// ── Step 3: Calculate tax ───────────────────────────────────────────
	taxBreakdown, err := h.calculateTax(ctx, store, &sc, req, shippingTotal)
	if err != nil {
		// M8 fix: return error instead of silently falling back to zero.
		h.respondErr(c, fmt.Errorf("tax calculation failed: %w", err))
		return
	}

	// ── Step 4: Create order ────────────────────────────────────────────
	// Use coupon discount if a coupon was applied, otherwise use client-supplied discount.
	// Loyalty redemption stacks on top as an additional discount so post-purchase
	// earn (subtotal − discount_total) excludes the points-paid portion.
	effectiveDiscount := req.DiscountTotal
	if couponDiscount.GreaterThan(decimal.Zero) {
		effectiveDiscount = couponDiscount
	}
	if loyaltyDiscount.GreaterThan(decimal.Zero) {
		effectiveDiscount = effectiveDiscount.Add(loyaltyDiscount)
	}
	grandTotal := req.Subtotal.Add(shippingTotal).Add(taxBreakdown.TaxTotal).Sub(effectiveDiscount)

	// ── Step 3.5: Gift card lookup (before tx) ─────────────────────────
	var giftCardApplied decimal.Decimal
	var giftCardID *uuid.UUID
	if req.GiftCardCode != nil && *req.GiftCardCode != "" && h.giftCardSvc != nil {
		gcResult, err := h.giftCardSvc.CheckBalance(ctx, storeID, *req.GiftCardCode)
		if err != nil {
			h.respondErr(c, err)
			return
		}
		// Amendment HIGH FIX 5: debit min(balance, grandTotal).
		debitAmount := grandTotal
		if gcResult.CurrentBalance.LessThan(grandTotal) {
			debitAmount = gcResult.CurrentBalance
		}
		if debitAmount.GreaterThan(decimal.Zero) {
			gcCard, err := h.giftCardSvc.GetByCode(ctx, storeID, *req.GiftCardCode)
			if err != nil {
				h.respondErr(c, err)
				return
			}
			giftCardID = &gcCard.ID
			giftCardApplied = debitAmount
			grandTotal = grandTotal.Sub(debitAmount)
		}
	}

	// Use billing address if provided, otherwise mirror shipping.
	billing := req.ShippingAddress
	if req.BillingAddress != nil {
		billing = *req.BillingAddress
	}

	prefix := storePrefixFromSlug(store.Slug)

	// If the shopper is signed in, OptionalCustomerAuth has attached the
	// customer profile to the gin context. Threading its ID onto the order
	// is what makes /account/orders list the purchase on the customer's
	// side; without it the order is created "anonymous" and only shows up
	// in the admin view.
	var customerID *uuid.UUID
	if profileVal, exists := c.Get(CustomerProfileKey); exists {
		if profile, ok := profileVal.(*customer.CustomerProfile); ok && profile != nil {
			id := profile.ID
			customerID = &id
		}
	}

	// Sequence allocation happens inside Service.Create's transaction
	// (C6 fix: atomic with order insert to prevent burned numbers).
	in := order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    prefix,
		OrderNumberSeq: 0, // allocated inside Create tx
		IdempotencyKey: req.IdempotencyKey,
		CustomerID:     customerID,
		CustomerEmail:  req.CustomerEmail,
		CustomerName:   req.CustomerName,
		Items:          checkoutToServiceItems(req.Items),
		Shipping:       checkoutToServiceAddress(req.ShippingAddress),
		Billing:        checkoutToServiceAddress(billing),
		Subtotal:       req.Subtotal,
		ShippingTotal:  shippingTotal,
		TaxTotal:       taxBreakdown.TaxTotal,
		DiscountTotal:  effectiveDiscount,
		GrandTotal:     grandTotal,
		CurrencyCode:   store.CurrencyCode,
	}

	result, err := h.orderSvc.Create(ctx, in)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	if !result.Reused {
		h.audit.Emit(c, audit.Event{
			TenantID:       tenantID,
			StoreID:        storeID,
			ForceActorType: audit.ActorSystem,
			Action:         "order.created",
			ResourceType:   "order",
			ResourceID:     result.Order.ID.String(),
			Metadata: map[string]any{
				"order_number":   result.Order.OrderNumber,
				"grand_total":    result.Order.GrandTotal,
				"currency":       result.Order.CurrencyCode,
				"source":         "storefront",
				"customer_email": req.CustomerEmail,
			},
		})
	}

	// Persist the per-jurisdiction tax breakdown (CGST/SGST/IGST for
	// India, VAT line for flat-rate countries, state+county+city splits
	// for TaxJar) so the customer's invoice + receipt PDFs can render
	// the same lines the merchant sees on the audit trail. Best-effort:
	// failures get logged but never fail the checkout — the aggregate
	// tax_total is already on the order.
	if !result.Reused && taxBreakdown != nil && len(taxBreakdown.Lines) > 0 {
		taxRepo := tax.NewRepository()
		if err := taxRepo.SaveTaxLines(ctx, h.db, result.Order.ID, taxBreakdown.Lines); err != nil {
			h.logger.Warn("checkout: failed to persist tax lines",
				"order_id", result.Order.ID, "err", err)
		}
	}

	// If idempotent replay, return the existing order without re-creating
	// payment intents or tax lines.
	if result.Reused {
		c.JSON(http.StatusOK, CheckoutExtResponse{
			OrderID:         result.Order.ID.String(),
			OrderNumber:     result.Order.OrderNumber,
			PaymentToken:    "", // caller should use the original token
			Provider:        req.PaymentProvider,
			TaxTotal:        result.Order.TaxTotal,
			ShippingTotal:   result.Order.ShippingTotal,
			DiscountTotal:   result.Order.DiscountTotal,
			Total:           result.Order.GrandTotal,
			GiftCardApplied: giftCardApplied,
			CouponCode:      appliedCouponCode,
			LoyaltyDiscount: loyaltyDiscount,
			PointsRedeemed:  loyaltyPoints,
		})
		return
	}

	// ── Step 4.5: Atomic coupon apply ──────────────────────────────────
	// Amendment CRITICAL FIX 1+2: validate + apply + usage increment
	// inside a single transaction. If this fails, the order exists but
	// the coupon was NOT consumed — we return an error.
	if appliedCouponCode != nil && h.couponSvc != nil {
		applier := coupon.NewCouponApplier(h.couponSvc, *appliedCouponCode, req.CustomerEmail)
		if err := h.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
			_, applyErr := applier.Apply(ctx, tx, discount.ApplyInput{
				TenantID:      tenantID,
				StoreID:       storeID,
				OrderID:       result.Order.ID,
				CustomerEmail: req.CustomerEmail,
				Subtotal:      req.Subtotal,
				CurrencyCode:  store.CurrencyCode,
			})
			return applyErr
		}); err != nil {
			h.respondErr(c, err)
			return
		}
	}

	// ── Step 4.6: Debit gift card (in tx) ──────────────────────────────
	// Amendment CRITICAL FIX 1: debit runs inside a transaction so it
	// rolls back if anything downstream fails.
	if giftCardID != nil && giftCardApplied.GreaterThan(decimal.Zero) && h.giftCardSvc != nil {
		if err := h.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
			_, err := h.giftCardSvc.Debit(tx, *giftCardID, giftCardApplied, result.Order.ID, tenantID)
			return err
		}); err != nil {
			h.logWarn("checkout_ext: gift card debit failed",
				"order_id", result.Order.ID.String(), "err", err)
			h.respondErr(c, err)
			return
		}
	}

	// ── Step 4.7: Debit loyalty points (in tx) ─────────────────────────
	// Mirrors the gift card pattern: the debit + ledger entry run inside
	// a transaction. If a concurrent redemption drained the balance since
	// the preview, DebitPoints returns InsufficientLoyaltyPoints and the
	// whole call is rolled back.
	if loyaltyPoints > 0 && h.loyaltySvc != nil {
		if err := h.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
			orderID := result.Order.ID
			_, err := h.loyaltySvc.RedeemPointsTx(ctx, tx, tenantID, storeID, req.CustomerEmail, loyaltyPoints, &orderID)
			return err
		}); err != nil {
			h.logWarn("checkout_ext: loyalty redeem failed",
				"order_id", result.Order.ID.String(), "err", err)
			h.respondErr(c, err)
			return
		}
	}

	// ── Step 5: Save tax lines ──────────────────────────────────────────
	if len(taxBreakdown.Lines) > 0 {
		taxRepo := tax.NewRepository()
		if err := taxRepo.SaveTaxLines(ctx, h.db, result.Order.ID, taxBreakdown.Lines); err != nil {
			h.logWarn("checkout_ext: failed to save tax lines",
				"order_id", result.Order.ID.String(), "err", err)
			// Non-fatal: the order is already created. Tax lines can be
			// recalculated from the order data if needed.
		}
	}

	// ── Step 6: Create payment intent ───────────────────────────────────
	paymentToken, err := h.createPaymentIntent(ctx, store, req.PaymentProvider, result.Order, grandTotal)
	if err != nil {
		h.logWarn("checkout_ext: payment intent creation failed",
			"order_id", result.Order.ID.String(), "err", err)
		// Return the order anyway — the storefront can retry payment.
		c.JSON(http.StatusCreated, CheckoutExtResponse{
			OrderID:         result.Order.ID.String(),
			OrderNumber:     result.Order.OrderNumber,
			PaymentToken:    "",
			Provider:        req.PaymentProvider,
			TaxTotal:        taxBreakdown.TaxTotal,
			ShippingTotal:   shippingTotal,
			DiscountTotal:   effectiveDiscount,
			Total:           grandTotal,
			GiftCardApplied: giftCardApplied,
			CouponCode:      appliedCouponCode,
			LoyaltyDiscount: loyaltyDiscount,
			PointsRedeemed:  loyaltyPoints,
		})
		return
	}

	// Link abandoned cart if applicable (best-effort, same as checkout.go).
	if req.CartSessionID != nil && *req.CartSessionID != "" {
		if err := h.orderSvc.LinkAbandonedCart(ctx, nil, storeID, *req.CartSessionID, result.Order.ID); err != nil && h.logger != nil {
			h.logger.Warn("checkout_ext: link abandoned cart failed",
				"cart_session_id", *req.CartSessionID,
				"order_id", result.Order.ID.String(),
				"err", err)
		}
	}

	c.JSON(http.StatusCreated, CheckoutExtResponse{
		OrderID:         result.Order.ID.String(),
		OrderNumber:     result.Order.OrderNumber,
		PaymentToken:    paymentToken,
		Provider:        req.PaymentProvider,
		TaxTotal:        taxBreakdown.TaxTotal,
		ShippingTotal:   shippingTotal,
		DiscountTotal:   effectiveDiscount,
		Total:           grandTotal,
		GiftCardApplied: giftCardApplied,
		CouponCode:      appliedCouponCode,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// calculateShipping resolves the carrier config and gets the rate for the
// selected service. Returns the final price including handling fee.
func (h *CheckoutExtHandler) calculateShipping(
	ctx context.Context,
	store *stores.Store,
	sc *country.SupportedCountry,
	req CheckoutExtRequest,
) (decimal.Decimal, error) {
	if len(sc.ShippingCarriers) == 0 {
		return decimal.Zero, nil
	}

	var cfg carrierConfigRow
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND is_active = true AND provider IN ?", store.ID, []string(sc.ShippingCarriers)).
		First(&cfg).Error; err != nil {
		return decimal.Zero, fmt.Errorf("no active carrier config: %w", err)
	}

	// Resolve API keys before passing to carrier. Routes through the
	// carriersecrets.Store when wired (handles gsm:// + legacy inline),
	// otherwise falls back to the Encryptor-only path.
	apiKey, err := h.resolveCred(ctx, cfg.APIKey)
	if err != nil {
		return decimal.Zero, fmt.Errorf("resolve carrier api_key: %w", err)
	}
	var secretKey string
	if cfg.SecretKey != "" {
		secretKey, err = h.resolveCred(ctx, cfg.SecretKey)
		if err != nil {
			return decimal.Zero, fmt.Errorf("resolve carrier secret_key: %w", err)
		}
	}

	// Lazy migration: if the stored references are legacy inline and
	// we're on a gcpsm-configured store, rewrite them as gsm://
	// references in the DB.
	h.maybeRewrapShippingRow(ctx, cfg, apiKey, secretKey)

	carrier, err := shipping.NewCarrier(cfg.Provider, apiKey, secretKey, cfg.Mode)
	if err != nil {
		return decimal.Zero, fmt.Errorf("carrier init: %w", err)
	}

	// Fetch per-variant weight + dims so the carrier rate request gets
	// real numbers. Without this every shipment looks like a 0-gram
	// envelope to ShipEngine, which Australia Post rejects with
	// "weight must be greater than or equal to 0.01 kg".
	variantIDs := make([]string, 0, len(req.Items))
	for _, it := range req.Items {
		if it.VariantID != nil && *it.VariantID != "" {
			variantIDs = append(variantIDs, *it.VariantID)
		}
	}
	type variantShipRow struct {
		ID          string
		WeightGrams *int
		LengthCM    *decimal.Decimal
		WidthCM     *decimal.Decimal
		HeightCM    *decimal.Decimal
	}
	shipByVariant := map[string]variantShipRow{}
	if len(variantIDs) > 0 {
		var rows []variantShipRow
		if err := h.db.WithContext(ctx).
			Table("product_variants").
			Select("id", "weight_grams", "length_cm", "width_cm", "height_cm").
			Where("id IN ?", variantIDs).
			Find(&rows).Error; err == nil {
			for _, r := range rows {
				shipByVariant[r.ID] = r
			}
		}
	}

	parcels := make([]shipping.ParcelItem, 0, len(req.Items))
	for _, it := range req.Items {
		p := shipping.ParcelItem{Quantity: it.Quantity}
		if it.VariantID != nil {
			if vs, ok := shipByVariant[*it.VariantID]; ok {
				if vs.WeightGrams != nil {
					p.WeightGrams = *vs.WeightGrams
				}
				if vs.LengthCM != nil {
					f, _ := vs.LengthCM.Float64()
					p.LengthCM = f
				}
				if vs.WidthCM != nil {
					f, _ := vs.WidthCM.Float64()
					p.WidthCM = f
				}
				if vs.HeightCM != nil {
					f, _ := vs.HeightCM.Float64()
					p.HeightCM = f
				}
			}
		}
		parcels = append(parcels, p)
	}

	fromAddr := warehouseAddress(cfg)
	rateReq := shipping.RateRequest{
		FromAddress: fromAddr,
		ToAddress: shipping.Address{
			Line1:       req.ShippingAddress.Line1,
			City:        req.ShippingAddress.City,
			Region:      derefString(req.ShippingAddress.Region),
			PostalCode:  derefString(req.ShippingAddress.PostalCode),
			CountryCode: req.ShippingAddress.CountryCode,
		},
		Items:        parcels,
		CurrencyCode: store.CurrencyCode,
	}

	rates, err := carrier.GetRates(ctx, rateReq)
	if err != nil {
		return decimal.Zero, fmt.Errorf("carrier.GetRates: %w", err)
	}

	// Find the rate matching the requested service.
	for _, r := range rates {
		if strings.EqualFold(r.Service, req.ShippingService) {
			price := r.Price.Add(cfg.HandlingFee)

			// Apply free shipping threshold.
			if cfg.FreeShippingMin != nil && !cfg.FreeShippingMin.IsZero() &&
				req.Subtotal.GreaterThanOrEqual(*cfg.FreeShippingMin) {
				return decimal.Zero, nil
			}

			return price, nil
		}
	}

	// If the requested service is not found, use the first rate.
	if len(rates) > 0 {
		price := rates[0].Price.Add(cfg.HandlingFee)
		if cfg.FreeShippingMin != nil && !cfg.FreeShippingMin.IsZero() &&
			req.Subtotal.GreaterThanOrEqual(*cfg.FreeShippingMin) {
			return decimal.Zero, nil
		}
		return price, nil
	}

	return decimal.Zero, nil
}

// calculateTax resolves the tax strategy for the store's country and
// computes the tax breakdown for the order items. Routes through
// tax.Service.CalculateOrderTax so the validateBreakdown check runs (H10 fix).
func (h *CheckoutExtHandler) calculateTax(
	ctx context.Context,
	store *stores.Store,
	sc *country.SupportedCountry,
	req CheckoutExtRequest,
	shippingTotal decimal.Decimal,
) (*tax.TaxBreakdown, error) {
	// Try to get a tax provider config for TaxJar (US stores).
	var taxjarAPIKey, taxjarMode string
	if sc.TaxStrategy == "taxjar" {
		storeUUID, err := uuid.Parse(store.ID)
		if err == nil {
			taxRepo := tax.NewRepository()
			if cfg, err := taxRepo.GetProviderConfig(ctx, h.db, storeUUID); err == nil {
				// Resolve the API key before use. Routes through the
				// carriersecrets.Store when wired and lazily rewrites
				// legacy inline refs to gsm://.
				resolvedKey, resErr := h.resolveCred(ctx, cfg.APIKeyEncrypted)
				if resErr != nil {
					return nil, fmt.Errorf("resolve taxjar api_key: %w", resErr)
				}
				h.maybeRewrapTaxRow(ctx, cfg, resolvedKey)
				taxjarAPIKey = resolvedKey
				taxjarMode = cfg.Mode
			}
		}
		// H9 fix: if TaxJar is the configured strategy but no API key is found,
		// return an error rather than silently computing zero tax.
		if taxjarAPIKey == "" {
			return nil, fmt.Errorf("TaxJar is configured for country %s but no API key found for store %s",
				store.CountryCode, store.ID)
		}
	}

	calc, err := tax.NewCalculator(sc.TaxStrategy, sc.TaxRate, taxjarAPIKey, taxjarMode)
	if err != nil {
		return nil, fmt.Errorf("tax calculator init: %w", err)
	}

	// Build taxable items from checkout lines. Per-item fields win when
	// set; otherwise fall back to the country default from
	// supported_countries.tax_rate.
	//
	// Both sources store the rate as a PERCENTAGE (e.g. 18.00 = 18%) —
	// the admin Tax tab writes percentages, and supported_countries
	// seeds them that way. The calculator expects a FRACTION (0.18), so
	// divide by 100 on the way in.
	hundred := decimal.NewFromInt(100)
	defaultRate := decimal.Zero
	if sc.TaxRate != nil && *sc.TaxRate > 0 {
		defaultRate = decimal.NewFromFloat(*sc.TaxRate).Div(hundred)
	}

	taxItems := make([]tax.TaxableItem, 0, len(req.Items))
	for _, it := range req.Items {
		rate := decimal.Zero
		if it.TaxRateOverride != nil && it.TaxRateOverride.GreaterThan(decimal.Zero) {
			rate = it.TaxRateOverride.Div(hundred)
		} else if !defaultRate.IsZero() {
			rate = defaultRate
		}
		ti := tax.TaxableItem{
			SKU:         it.SKUSnapshot,
			Amount:      it.UnitPrice,
			Quantity:    it.Quantity,
			TaxCode:     derefString(it.TaxCode),
			TaxRate:     rate,
			TaxCategory: derefString(it.TaxCategory),
		}
		if it.ProductID != nil {
			ti.ProductID = *it.ProductID
		}
		taxItems = append(taxItems, ti)
	}

	// C5 fix: populate SellerAddress.Region from the store's warehouse config
	// so India GST can determine intra-state vs inter-state correctly.
	sellerRegion := ""
	var cfg carrierConfigRow
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND is_active = true", store.ID).
		First(&cfg).Error; err == nil && cfg.WarehouseRegion != nil {
		sellerRegion = *cfg.WarehouseRegion
	}

	// H10 fix: route through tax.Service.CalculateOrderTax instead of
	// calling calc.Calculate directly, so validateBreakdown runs.
	taxSvc := tax.NewService()
	breakdown, err := taxSvc.CalculateOrderTax(
		ctx,
		store.CountryCode,
		tax.Address{
			CountryCode: store.CountryCode,
			Region:      sellerRegion,
		},
		tax.Address{
			Line1:       req.ShippingAddress.Line1,
			City:        req.ShippingAddress.City,
			Region:      derefString(req.ShippingAddress.Region),
			PostalCode:  derefString(req.ShippingAddress.PostalCode),
			CountryCode: req.ShippingAddress.CountryCode,
		},
		taxItems,
		shippingTotal,
		store.CurrencyCode,
		calc,
	)
	if err != nil {
		return nil, fmt.Errorf("tax calculation: %w", err)
	}

	return breakdown, nil
}

// createPaymentIntent creates a payment intent and persists the transaction.
// Returns the client token for the storefront SDK.
func (h *CheckoutExtHandler) createPaymentIntent(
	ctx context.Context,
	store *stores.Store,
	providerName string,
	ord *order.Order,
	amount decimal.Decimal,
) (string, error) {
	// Look up the payment gateway config for this provider + store.
	var cfg paymentGatewayConfigRow
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true", store.ID, providerName).
		First(&cfg).Error; err != nil {
		return "", fmt.Errorf("no active payment config for %s: %w", providerName, err)
	}

	// Resolve API keys before passing to gateway. Same Store-first /
	// Encryptor-fallback pattern as shipping.
	apiKey, err := h.resolveCred(ctx, cfg.APIKey)
	if err != nil {
		return "", fmt.Errorf("resolve payment api_key: %w", err)
	}
	var secretKey string
	if cfg.SecretKey != "" {
		secretKey, err = h.resolveCred(ctx, cfg.SecretKey)
		if err != nil {
			return "", fmt.Errorf("resolve payment secret_key: %w", err)
		}
	}

	// Lazy migration: rewrite legacy inline refs to gsm://.
	h.maybeRewrapPaymentRow(ctx, cfg.ID, cfg.TenantID, providerName, cfg, apiKey, secretKey)

	gateway, err := payment.NewGateway(providerName, apiKey, secretKey, cfg.Mode)
	if err != nil {
		return "", fmt.Errorf("gateway init: %w", err)
	}

	intent, err := gateway.CreateIntent(ctx, payment.CreateIntentInput{
		OrderID:       ord.ID.String(),
		Amount:        amount,
		CurrencyCode:  ord.CurrencyCode,
		CustomerEmail: ord.CustomerEmail,
		Description:   fmt.Sprintf("Order %s", ord.OrderNumber),
		Metadata: map[string]string{
			"order_id":     ord.ID.String(),
			"order_number": ord.OrderNumber,
			"store_id":     store.ID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("create intent: %w", err)
	}

	// Persist payment transaction record.
	txRecord := payment.PaymentTransaction{
		TenantID:         store.TenantID,
		StoreID:          store.ID,
		OrderID:          ord.ID.String(),
		Provider:         providerName,
		ProviderIntentID: intent.ProviderIntentID,
		Amount:           amount,
		CurrencyCode:     ord.CurrencyCode,
		Status:           intent.Status,
	}
	if err := h.db.WithContext(ctx).Create(&txRecord).Error; err != nil {
		return "", fmt.Errorf("persist payment transaction: %w", err)
	}

	return intent.ClientToken, nil
}

// paymentGatewayConfigRow is a read-only projection of payment_gateway_configs
// for the checkout ext handler. Includes the secret key for gateway init.
// ID + TenantID are loaded so MaybeRewrap can scope the lazy migration
// and we know which row to UPDATE when rewriting a legacy inline reference.
type paymentGatewayConfigRow struct {
	ID        string `gorm:"column:id"`
	TenantID  string `gorm:"column:tenant_id"`
	Provider  string `gorm:"column:provider"`
	APIKey    string `gorm:"column:api_key_encrypted"`
	SecretKey string `gorm:"column:secret_key_encrypted"`
	Mode      string `gorm:"column:mode"`
	IsActive  bool   `gorm:"column:is_active"`
}

func (paymentGatewayConfigRow) TableName() string { return "payment_gateway_configs" }

// respondErr mirrors the checkout.go error response pattern.
func (h *CheckoutExtHandler) respondErr(c *gin.Context, err error) {
	// Amendment LOW FIX 9: use errors.As instead of manual type assertion.
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case apperrors.CodeValidationFailed,
			apperrors.CodeCurrencyMismatch,
			apperrors.CodeInvalidTransition,
			apperrors.CodeIdempotencyConflict,
			apperrors.CodeCouponInvalid,
			apperrors.CodeCouponMinPurchaseNotMet:
			c.AbortWithStatusJSON(http.StatusBadRequest, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		case apperrors.CodeCouponNotFound,
			apperrors.CodeGiftCardNotFound:
			respondNotFound(c)
			return
		case apperrors.CodeGiftCardExpired:
			c.AbortWithStatusJSON(http.StatusGone, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		case apperrors.CodeInsufficientGiftCardBalance:
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		case apperrors.CodeCouponExpired,
			apperrors.CodeCouponUsageLimitReached:
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		case apperrors.CodeNotFound:
			respondNotFound(c)
			return
		}
	}
	if h.logger != nil {
		h.logger.Error("checkout_ext: unhandled error", "err", err.Error())
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
		"error":   "internal",
		"message": "internal server error",
	})
}

// logWarn emits a warning log. No-ops when logger is nil.
func (h *CheckoutExtHandler) logWarn(msg string, args ...any) {
	if h.logger != nil {
		h.logger.Warn(msg, args...)
	}
}

// derefString safely dereferences a *string, returning "" for nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefDecimal safely dereferences a *decimal.Decimal, returning zero for nil.
func derefDecimal(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}
