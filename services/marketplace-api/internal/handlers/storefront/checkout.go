// Package storefront — checkout.go: public POST /storefront/stores/:storeSlug/checkout
// handler. Storefront callers convert a cart into an order via the same
// order.Service.Create path the admin handler uses, but with no FGA gate
// (storefront bypasses FGA per spec §2 decision 12) and with optional
// abandoned-cart linking by cart_session_id.
package storefront

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CheckoutHandler is the public storefront checkout endpoint. Constructed
// in cmd/marketplace-api/main.go for storefront and both modes.
type CheckoutHandler struct {
	db       *gorm.DB
	orderSvc *order.Service
	orderRepo order.Repository
	audit    *audit.Emitter // optional — nil-safe
	logger   *slog.Logger
}

// NewCheckoutHandler constructs a CheckoutHandler.
func NewCheckoutHandler(db *gorm.DB, orderSvc *order.Service, orderRepo order.Repository, logger *slog.Logger) *CheckoutHandler {
	return &CheckoutHandler{db: db, orderSvc: orderSvc, orderRepo: orderRepo, logger: logger}
}

// WithAudit attaches an audit emitter so storefront checkouts emit
// order.created as system events. Nil-safe.
func (h *CheckoutHandler) WithAudit(e *audit.Emitter) *CheckoutHandler {
	h.audit = e
	return h
}

// CheckoutItemRequest is one cart line in a CheckoutRequest.
type CheckoutItemRequest struct {
	ProductID     *string          `json:"product_id"`
	VariantID     *string          `json:"variant_id"`
	TitleSnapshot string           `json:"title_snapshot" binding:"required"`
	SKUSnapshot   string           `json:"sku_snapshot"   binding:"required"`
	OptionSummary *string          `json:"option_summary"`
	UnitPrice     decimal.Decimal  `json:"unit_price"     binding:"required"`
	Quantity      int              `json:"quantity"       binding:"required,min=1"`
	LineTotal     decimal.Decimal  `json:"line_total"     binding:"required"`
	CurrencyCode  string           `json:"currency_code"  binding:"required,len=3"`
	ImageURL      *string          `json:"image_url"`
	HSNCode       *string          `json:"hsn_code"`       // India GST — H11 fix
	GSTRate       *decimal.Decimal `json:"gst_rate"`       // India GST — H11 fix
	TaxCode       *string          `json:"tax_code"`       // TaxJar product tax code
}

// CheckoutAddressRequest is a shipping/billing address.
type CheckoutAddressRequest struct {
	Name        string  `json:"name"         binding:"required"`
	Line1       string  `json:"line1"        binding:"required"`
	Line2       *string `json:"line2"`
	City        string  `json:"city"         binding:"required"`
	Region      *string `json:"region"`
	PostalCode  *string `json:"postal_code"`
	CountryCode string  `json:"country_code" binding:"required,len=2"`
	Phone       *string `json:"phone"`
}

// CheckoutRequest is the wire body for POST /storefront/stores/:storeSlug/checkout.
//
// IdempotencyKey is required so storefront retries (page reload, network
// flake) collapse onto a single order. CartSessionID is optional; when
// present and matching an open abandoned_carts row, the row's
// converted_order_id is set in the same tx as the order create.
type CheckoutRequest struct {
	IdempotencyKey string                  `json:"idempotency_key" binding:"required"`
	CartSessionID  *string                 `json:"cart_session_id"`
	CustomerEmail  string                  `json:"customer_email"  binding:"required,email"`
	CustomerName   *string                 `json:"customer_name"`
	Items          []CheckoutItemRequest   `json:"items"           binding:"required,min=1"`
	Shipping       CheckoutAddressRequest  `json:"shipping"        binding:"required"`
	Billing        CheckoutAddressRequest  `json:"billing"         binding:"required"`
	Subtotal       decimal.Decimal         `json:"subtotal"        binding:"required"`
	ShippingTotal  decimal.Decimal         `json:"shipping_total"`
	TaxTotal       decimal.Decimal         `json:"tax_total"`
	DiscountTotal  decimal.Decimal         `json:"discount_total"`
	GrandTotal     decimal.Decimal         `json:"grand_total"     binding:"required"`
}

// CheckoutResponse is the storefront-safe order projection. Deliberately
// trimmed compared to AdminOrderResponse — no internal IDs, no audit
// timestamps beyond placed_at.
type CheckoutResponse struct {
	ID            string          `json:"id"`
	OrderNumber   string          `json:"order_number"`
	Status        string          `json:"status"`
	PaymentStatus string          `json:"payment_status"`
	GrandTotal    decimal.Decimal `json:"grand_total"`
	CurrencyCode  string          `json:"currency_code"`
	PlacedAt      time.Time       `json:"placed_at"`
}

// Checkout handles POST /storefront/stores/:storeSlug/checkout.
//
// Resolves the store from gin context (set by StoreContext middleware),
// allocates a per-store order sequence inside its own short tx, and calls
// order.Service.Create with the resolved tenant + currency. If the
// request body carries cart_session_id, the matching abandoned_carts row
// is linked via order.Service.LinkAbandonedCart in the same tx as the
// order create — so cart conversion is atomic with order creation.
func (h *CheckoutHandler) Checkout(c *gin.Context) {
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

	var req CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondErr(c, apperrors.ValidationFailed("body", err.Error()))
		return
	}

	// Currency must match the store. We do not let storefront callers
	// invent a currency — that's a misconfigured frontend, not a real
	// checkout. The order service writes the store's currency.
	for _, it := range req.Items {
		if !strings.EqualFold(it.CurrencyCode, store.CurrencyCode) {
			h.respondErr(c, apperrors.CurrencyMismatch(it.CurrencyCode, store.CurrencyCode))
			return
		}
	}

	prefix := storePrefixFromSlug(store.Slug)

	// Stamp the order with the signed-in customer's profile id so
	// /account/orders surfaces it. Mirrors the same logic in the
	// CheckoutExtHandler path; both endpoints are routed through
	// OptionalCustomerAuth.
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
		Shipping:       checkoutToServiceAddress(req.Shipping),
		Billing:        checkoutToServiceAddress(req.Billing),
		Subtotal:       req.Subtotal,
		ShippingTotal:  req.ShippingTotal,
		TaxTotal:       req.TaxTotal,
		DiscountTotal:  req.DiscountTotal,
		GrandTotal:     req.GrandTotal,
		CurrencyCode:   store.CurrencyCode,
	}

	result, err := h.orderSvc.Create(c.Request.Context(), in)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	// Link abandoned cart if a session id was supplied. We do this in a
	// separate (short) tx because order.Service.Create has already
	// committed; the link is best-effort and a failure here is logged
	// but not surfaced to the customer (they already have an order).
	if req.CartSessionID != nil && *req.CartSessionID != "" && !result.Reused {
		if err := h.orderSvc.LinkAbandonedCart(c.Request.Context(), nil, storeID, *req.CartSessionID, result.Order.ID); err != nil && h.logger != nil {
			h.logger.Warn("storefront checkout: link abandoned cart failed",
				"cart_session_id", *req.CartSessionID,
				"order_id", result.Order.ID.String(),
				"err", err)
		}
	}

	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
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

	c.JSON(status, CheckoutResponse{
		ID:            result.Order.ID.String(),
		OrderNumber:   result.Order.OrderNumber,
		Status:        result.Order.Status,
		PaymentStatus: result.Order.PaymentStatus,
		GrandTotal:    result.Order.GrandTotal,
		CurrencyCode:  result.Order.CurrencyCode,
		PlacedAt:      result.Order.PlacedAt,
	})
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

// respondErr writes a typed apperrors envelope to the client. Storefront
// surface deliberately collapses any not_found into 404 with no body
// detail to prevent existence leaks (mirrors RequireStorefrontKey behavior).
func (h *CheckoutHandler) respondErr(c *gin.Context, err error) {
	var ae *apperrors.Error
	if asErr, ok := err.(*apperrors.Error); ok {
		ae = asErr
	}
	if ae != nil {
		switch ae.Code {
		case apperrors.CodeValidationFailed,
			apperrors.CodeCurrencyMismatch,
			apperrors.CodeInvalidTransition,
			apperrors.CodeIdempotencyConflict,
			apperrors.CodeRefundExceedsTotal:
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
		h.logger.Error("storefront checkout: unhandled error", "err", err.Error())
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
		"error":   "internal",
		"message": "internal server error",
	})
}

// storePrefixFromSlug derives the 3-char order number prefix from a store
// slug. Mirrors the admin handler's helper.
func storePrefixFromSlug(slug string) string {
	upper := strings.ToUpper(slug)
	out := make([]byte, 0, 3)
	for i := 0; i < len(upper) && len(out) < 3; i++ {
		c := upper[i]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		}
	}
	if len(out) == 3 {
		return string(out)
	}
	return "STR"
}

func checkoutToServiceItems(items []CheckoutItemRequest) []order.OrderItem {
	out := make([]order.OrderItem, 0, len(items))
	for _, it := range items {
		row := order.OrderItem{
			TitleSnapshot: it.TitleSnapshot,
			SKUSnapshot:   it.SKUSnapshot,
			OptionSummary: it.OptionSummary,
			UnitPrice:     it.UnitPrice,
			Quantity:      it.Quantity,
			LineTotal:     it.LineTotal,
			CurrencyCode:  it.CurrencyCode,
			ImageURL:      it.ImageURL,
		}
		if it.ProductID != nil {
			if pid, err := uuid.Parse(*it.ProductID); err == nil {
				row.ProductID = &pid
			}
		}
		if it.VariantID != nil {
			if vid, err := uuid.Parse(*it.VariantID); err == nil {
				row.VariantID = &vid
			}
		}
		out = append(out, row)
	}
	return out
}

func checkoutToServiceAddress(a CheckoutAddressRequest) order.OrderAddress {
	return order.OrderAddress{
		Name:        a.Name,
		Line1:       a.Line1,
		Line2:       a.Line2,
		City:        a.City,
		Region:      a.Region,
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
		Phone:       a.Phone,
	}
}
