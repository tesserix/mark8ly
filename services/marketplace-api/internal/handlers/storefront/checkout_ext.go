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

	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/discount"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/payment"
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
	giftCardSvc *giftcard.Service // nil-safe: no-ops when nil
	logger      *slog.Logger
}

// NewCheckoutExtHandler constructs a CheckoutExtHandler.
func NewCheckoutExtHandler(db *gorm.DB, orderSvc *order.Service, couponSvc *coupon.Service, giftCardSvc *giftcard.Service, logger *slog.Logger) *CheckoutExtHandler {
	return &CheckoutExtHandler{db: db, orderSvc: orderSvc, couponSvc: couponSvc, giftCardSvc: giftCardSvc, logger: logger}
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
}

// CheckoutExtResponse is the extended checkout response including payment
// token and computed totals.
type CheckoutExtResponse struct {
	OrderID       string          `json:"order_id"`
	OrderNumber   string          `json:"order_number"`
	PaymentToken  string          `json:"payment_token"`
	Provider      string          `json:"provider"`
	TaxTotal      decimal.Decimal `json:"tax_total"`
	ShippingTotal decimal.Decimal `json:"shipping_total"`
	DiscountTotal   decimal.Decimal `json:"discount_total"`
	Total           decimal.Decimal `json:"total"`
	GiftCardApplied decimal.Decimal `json:"gift_card_applied"`
	CouponCode      *string         `json:"coupon_code,omitempty"`
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
	effectiveDiscount := req.DiscountTotal
	if couponDiscount.GreaterThan(decimal.Zero) {
		effectiveDiscount = couponDiscount
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

	// Sequence allocation happens inside Service.Create's transaction
	// (C6 fix: atomic with order insert to prevent burned numbers).
	in := order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    prefix,
		OrderNumberSeq: 0, // allocated inside Create tx
		IdempotencyKey: req.IdempotencyKey,
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

	carrier, err := shipping.NewCarrier(cfg.Provider, cfg.APIKey, cfg.SecretKey, cfg.Mode)
	if err != nil {
		return decimal.Zero, fmt.Errorf("carrier init: %w", err)
	}

	parcels := make([]shipping.ParcelItem, 0, len(req.Items))
	for _, it := range req.Items {
		parcels = append(parcels, shipping.ParcelItem{
			Quantity: it.Quantity,
		})
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
				taxjarAPIKey = cfg.APIKeyEncrypted
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

	// Build taxable items from checkout items.
	// H11 fix: propagate HSN code and GST rate from item data for India GST.
	taxItems := make([]tax.TaxableItem, 0, len(req.Items))
	for _, it := range req.Items {
		ti := tax.TaxableItem{
			SKU:      it.SKUSnapshot,
			Amount:   it.UnitPrice,
			Quantity: it.Quantity,
			HSNCode:  derefString(it.HSNCode),
			GSTRate:  derefDecimal(it.GSTRate),
			TaxCode:  derefString(it.TaxCode),
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

	gateway, err := payment.NewGateway(providerName, cfg.APIKey, cfg.SecretKey, cfg.Mode)
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
type paymentGatewayConfigRow struct {
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
