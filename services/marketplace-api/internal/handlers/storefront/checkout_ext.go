// Package storefront — checkout_ext.go: extended checkout endpoint that wraps
// order creation with payment intent, shipping, and tax integration.
// POST /storefront/stores/:storeSlug/checkout (replaces the simple checkout).
//
// This file does NOT modify checkout.go. CheckoutExtHandler is registered
// alongside (or instead of) the original CheckoutHandler in routes.go.
package storefront

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/country"
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
	db       *gorm.DB
	orderSvc *order.Service
	logger   *slog.Logger
}

// NewCheckoutExtHandler constructs a CheckoutExtHandler.
func NewCheckoutExtHandler(db *gorm.DB, orderSvc *order.Service, logger *slog.Logger) *CheckoutExtHandler {
	return &CheckoutExtHandler{db: db, orderSvc: orderSvc, logger: logger}
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
	Total         decimal.Decimal `json:"total"`
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

	// ── Step 2: Calculate shipping ──────────────────────────────────────
	shippingTotal, err := h.calculateShipping(ctx, store, &sc, req)
	if err != nil {
		h.logWarn("checkout_ext: shipping calculation failed", "err", err)
		// Fall back to zero shipping rather than blocking checkout.
		shippingTotal = decimal.Zero
	}

	// ── Step 3: Calculate tax ───────────────────────────────────────────
	taxBreakdown, err := h.calculateTax(ctx, store, &sc, req, shippingTotal)
	if err != nil {
		h.logWarn("checkout_ext: tax calculation failed", "err", err)
		taxBreakdown = &tax.TaxBreakdown{TaxTotal: decimal.Zero}
	}

	// ── Step 4: Create order ────────────────────────────────────────────
	grandTotal := req.Subtotal.Add(shippingTotal).Add(taxBreakdown.TaxTotal).Sub(req.DiscountTotal)

	// Use billing address if provided, otherwise mirror shipping.
	billing := req.ShippingAddress
	if req.BillingAddress != nil {
		billing = *req.BillingAddress
	}

	prefix := storePrefixFromSlug(store.Slug)

	var seq int64
	if err := h.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
		n, e := order.NextDocumentNumber(ctx, tx, storeID, "order")
		if e != nil {
			return e
		}
		seq = n
		return nil
	}); err != nil {
		h.respondErr(c, err)
		return
	}

	in := order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    prefix,
		OrderNumberSeq: seq,
		IdempotencyKey: req.IdempotencyKey,
		CustomerEmail:  req.CustomerEmail,
		CustomerName:   req.CustomerName,
		Items:          checkoutToServiceItems(req.Items),
		Shipping:       checkoutToServiceAddress(req.ShippingAddress),
		Billing:        checkoutToServiceAddress(billing),
		Subtotal:       req.Subtotal,
		ShippingTotal:  shippingTotal,
		TaxTotal:       taxBreakdown.TaxTotal,
		DiscountTotal:  req.DiscountTotal,
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
			OrderID:       result.Order.ID.String(),
			OrderNumber:   result.Order.OrderNumber,
			PaymentToken:  "", // caller should use the original token
			Provider:      req.PaymentProvider,
			TaxTotal:      result.Order.TaxTotal,
			ShippingTotal: result.Order.ShippingTotal,
			Total:         result.Order.GrandTotal,
		})
		return
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
			OrderID:       result.Order.ID.String(),
			OrderNumber:   result.Order.OrderNumber,
			PaymentToken:  "",
			Provider:      req.PaymentProvider,
			TaxTotal:      taxBreakdown.TaxTotal,
			ShippingTotal: shippingTotal,
			Total:         grandTotal,
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
		OrderID:       result.Order.ID.String(),
		OrderNumber:   result.Order.OrderNumber,
		PaymentToken:  paymentToken,
		Provider:      req.PaymentProvider,
		TaxTotal:      taxBreakdown.TaxTotal,
		ShippingTotal: shippingTotal,
		Total:         grandTotal,
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
// computes the tax breakdown for the order items.
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
	}

	calc, err := tax.NewCalculator(sc.TaxStrategy, sc.TaxRate, taxjarAPIKey, taxjarMode)
	if err != nil {
		return nil, fmt.Errorf("tax calculator init: %w", err)
	}

	// Build taxable items from checkout items.
	taxItems := make([]tax.TaxableItem, 0, len(req.Items))
	for _, it := range req.Items {
		ti := tax.TaxableItem{
			SKU:      it.SKUSnapshot,
			Amount:   it.LineTotal,
			Quantity: it.Quantity,
		}
		if it.ProductID != nil {
			ti.ProductID = *it.ProductID
		}
		taxItems = append(taxItems, ti)
	}

	taxReq := tax.TaxRequest{
		StoreCountryCode: store.CountryCode,
		SellerAddress: tax.Address{
			CountryCode: store.CountryCode,
		},
		BuyerAddress: tax.Address{
			Line1:       req.ShippingAddress.Line1,
			City:        req.ShippingAddress.City,
			Region:      derefString(req.ShippingAddress.Region),
			PostalCode:  derefString(req.ShippingAddress.PostalCode),
			CountryCode: req.ShippingAddress.CountryCode,
		},
		Items:          taxItems,
		ShippingAmount: shippingTotal,
		CurrencyCode:   store.CurrencyCode,
	}

	breakdown, err := calc.Calculate(ctx, taxReq)
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
	var ae *apperrors.Error
	if asErr, ok := err.(*apperrors.Error); ok {
		ae = asErr
	}
	if ae != nil {
		switch ae.Code {
		case apperrors.CodeValidationFailed,
			apperrors.CodeCurrencyMismatch,
			apperrors.CodeInvalidTransition,
			apperrors.CodeIdempotencyConflict:
			c.AbortWithStatusJSON(http.StatusBadRequest, map[string]any{
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
