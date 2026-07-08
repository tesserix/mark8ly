package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/order"
)

// -----------------------------------------------------------------------------
// Orders — list query
// -----------------------------------------------------------------------------

// ListOrdersQuery is the parsed query string for GET /admin/stores/:storeId/orders.
type ListOrdersQuery struct {
	Status        string `form:"status"`         // optional filter; matches orders.status
	PaymentStatus string `form:"payment_status"` // optional filter
	// Search is a free-text query across order_number + customer_name +
	// customer_email. ILIKE match inside the repo layer so partial typed
	// input (first chars of an order number, a customer domain, etc.)
	// produces useful matches without pressing Enter.
	Search   string `form:"search"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// Defaults sets sensible page / page_size defaults.
func (q *ListOrdersQuery) Defaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 50
	}
}

// -----------------------------------------------------------------------------
// Orders — request bodies
// -----------------------------------------------------------------------------

// CreateOrderItemRequest is one line item on a CreateOrderRequest. Used by
// admin order creation (rare; storefront checkout uses a separate flow in M5).
type CreateOrderItemRequest struct {
	ProductID     *string         `json:"product_id"`
	VariantID     *string         `json:"variant_id"`
	TitleSnapshot string          `json:"title_snapshot" binding:"required"`
	SKUSnapshot   string          `json:"sku_snapshot"   binding:"required"`
	OptionSummary *string         `json:"option_summary"`
	UnitPrice     decimal.Decimal `json:"unit_price"     binding:"required"`
	Quantity      int             `json:"quantity"       binding:"required,min=1"`
	LineTotal     decimal.Decimal `json:"line_total"     binding:"required"`
	CurrencyCode  string          `json:"currency_code"  binding:"required,len=3"`
	ImageURL      *string         `json:"image_url"`
}

// AddressRequest is a single shipping/billing address line.
type AddressRequest struct {
	Name        string  `json:"name"         binding:"required"`
	Line1       string  `json:"line1"        binding:"required"`
	Line2       *string `json:"line2"`
	City        string  `json:"city"         binding:"required"`
	Region      *string `json:"region"`
	PostalCode  *string `json:"postal_code"`
	CountryCode string  `json:"country_code" binding:"required,len=2"`
	Phone       *string `json:"phone"`
}

// CreateOrderRequest is the wire body for POST /admin/stores/:storeId/orders.
type CreateOrderRequest struct {
	IdempotencyKey string                   `json:"idempotency_key" binding:"required"`
	CustomerEmail  string                   `json:"customer_email"  binding:"required,email"`
	CustomerName   *string                  `json:"customer_name"`
	CustomerID     *string                  `json:"customer_id"`
	Items          []CreateOrderItemRequest `json:"items"           binding:"required,min=1"`
	Shipping       AddressRequest           `json:"shipping"        binding:"required"`
	Billing        AddressRequest           `json:"billing"         binding:"required"`
	Subtotal       decimal.Decimal          `json:"subtotal"        binding:"required"`
	ShippingTotal  decimal.Decimal          `json:"shipping_total"`
	TaxTotal       decimal.Decimal          `json:"tax_total"`
	DiscountTotal  decimal.Decimal          `json:"discount_total"`
	GrandTotal     decimal.Decimal          `json:"grand_total"     binding:"required"`
	CurrencyCode   string                   `json:"currency_code"   binding:"required,len=3"`
	Notes          *string                  `json:"notes"`
}

// ConfirmOrderRequest carries optional payment status change.
type ConfirmOrderRequest struct {
	PaymentStatus *string `json:"payment_status"` // optional; one of order.PaymentStatus values
	Reason        string  `json:"reason"`
}

// CancelOrderRequest captures the cancellation reason for the audit log.
type CancelOrderRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// RefundOrderRequest is the wire body for POST /admin/stores/:storeId/orders/:id/refund.
// Money movement now goes through orderrefund.Coordinator, which derives
// the resulting payment_status from the amount rather than trusting the
// caller — see OrdersHandler.Refund.
type RefundOrderRequest struct {
	Amount          *decimal.Decimal `json:"amount"`                      // omit ⇒ full remaining balance
	RefundRequestID string           `json:"refund_request_id,omitempty"` // idempotency scope; server generates one if empty
	Reason          string           `json:"reason"`
	// PaymentStatus is deprecated: the coordinator derives partial vs full
	// from the amount. Retained on the wire so older clients don't fail
	// binding; the value is ignored.
	PaymentStatus string `json:"payment_status,omitempty"`
}

// -----------------------------------------------------------------------------
// Orders — response shapes
// -----------------------------------------------------------------------------

// AdminOrderItemResponse is a single line item in an order response.
type AdminOrderItemResponse struct {
	ID            string          `json:"id"`
	ProductID     *string         `json:"product_id,omitempty"`
	VariantID     *string         `json:"variant_id,omitempty"`
	TitleSnapshot string          `json:"title_snapshot"`
	SKUSnapshot   string          `json:"sku_snapshot"`
	OptionSummary *string         `json:"option_summary,omitempty"`
	UnitPrice     decimal.Decimal `json:"unit_price"`
	Quantity      int             `json:"quantity"`
	LineTotal     decimal.Decimal `json:"line_total"`
	CurrencyCode  string          `json:"currency_code"`
}

// AdminAddressResponse renders an OrderAddress.
type AdminAddressResponse struct {
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	Line1       string  `json:"line1"`
	Line2       *string `json:"line2,omitempty"`
	City        string  `json:"city"`
	Region      *string `json:"region,omitempty"`
	PostalCode  *string `json:"postal_code,omitempty"`
	CountryCode string  `json:"country_code"`
	Phone       *string `json:"phone,omitempty"`
}

// AdminOrderResponse is the canonical wire shape for a single order.
type AdminOrderResponse struct {
	ID                string          `json:"id"`
	TenantID          string          `json:"tenant_id"`
	StoreID           string          `json:"store_id"`
	OrderNumber       string          `json:"order_number"`
	IdempotencyKey    string          `json:"idempotency_key"`
	CustomerEmail     string          `json:"customer_email"`
	CustomerName      *string         `json:"customer_name,omitempty"`
	Status            string          `json:"status"`
	PaymentStatus     string          `json:"payment_status"`
	FulfillmentStatus string          `json:"fulfillment_status"`
	Subtotal          decimal.Decimal `json:"subtotal"`
	ShippingTotal     decimal.Decimal `json:"shipping_total"`
	TaxTotal          decimal.Decimal `json:"tax_total"`
	// TaxLines is the per-jurisdiction breakdown (CGST/SGST/IGST for
	// India, VAT for flat-rate countries, state+county+city for TaxJar).
	// Populated by single-order Get handler; List leaves it nil to keep
	// the page payload tight.
	TaxLines       []AdminOrderTaxLineResponse `json:"tax_lines,omitempty"`
	DiscountTotal  decimal.Decimal             `json:"discount_total"`
	GrandTotal     decimal.Decimal             `json:"grand_total"`
	RefundedAmount decimal.Decimal             `json:"refunded_amount"`
	CurrencyCode   string                      `json:"currency_code"`
	// ShippingService / ShippingCarrier are the customer's checkout choice
	// — captured so the admin "Approve & generate label" panel can default
	// to what the buyer paid for. Empty for orders pre-dating migration 82.
	ShippingService *string                  `json:"shipping_service,omitempty"`
	ShippingCarrier *string                  `json:"shipping_carrier,omitempty"`
	Items           []AdminOrderItemResponse `json:"items"`
	Addresses       []AdminAddressResponse   `json:"addresses"`
	PlacedAt        time.Time                `json:"placed_at"`
	CancelledAt     *time.Time               `json:"cancelled_at,omitempty"`
	FulfilledAt     *time.Time               `json:"fulfilled_at,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
}

// AdminOrderTaxLineResponse is one row from order_tax_lines in the
// admin wire shape.
type AdminOrderTaxLineResponse struct {
	Description  string          `json:"description"`
	Rate         decimal.Decimal `json:"rate"`
	Amount       decimal.Decimal `json:"amount"`
	Jurisdiction string          `json:"jurisdiction,omitempty"`
}

// RefundOrderResponse is the wire shape for a successful refund. It
// anonymously embeds AdminOrderResponse so the response still serializes
// every existing order field inline (callers unmarshalling into a plain
// AdminOrderResponse keep working) while also surfacing the gateway's
// provider_refund_id for reconciliation.
type RefundOrderResponse struct {
	AdminOrderResponse
	ProviderRefundID string `json:"provider_refund_id"`
	AlreadyDone      bool   `json:"already_refunded,omitempty"`
}

// ToAdminOrderResponse renders the persistence types into the wire shape.
func ToAdminOrderResponse(o *order.Order, items []order.OrderItem, addrs []order.OrderAddress) AdminOrderResponse {
	out := AdminOrderResponse{
		ID:                o.ID.String(),
		TenantID:          o.TenantID.String(),
		StoreID:           o.StoreID.String(),
		OrderNumber:       o.OrderNumber,
		IdempotencyKey:    o.IdempotencyKey,
		CustomerEmail:     o.CustomerEmail,
		CustomerName:      o.CustomerName,
		Status:            o.Status,
		PaymentStatus:     o.PaymentStatus,
		FulfillmentStatus: o.FulfillmentStatus,
		Subtotal:          o.Subtotal,
		ShippingTotal:     o.ShippingTotal,
		TaxTotal:          o.TaxTotal,
		DiscountTotal:     o.DiscountTotal,
		GrandTotal:        o.GrandTotal,
		RefundedAmount:    o.RefundedAmount,
		CurrencyCode:      o.CurrencyCode,
		ShippingService:   o.ShippingService,
		ShippingCarrier:   o.ShippingCarrier,
		Items:             make([]AdminOrderItemResponse, 0, len(items)),
		Addresses:         make([]AdminAddressResponse, 0, len(addrs)),
		PlacedAt:          o.PlacedAt,
		CancelledAt:       o.CancelledAt,
		FulfilledAt:       o.FulfilledAt,
		CreatedAt:         o.CreatedAt,
		UpdatedAt:         o.UpdatedAt,
	}
	for _, it := range items {
		var pid, vid *string
		if it.ProductID != nil {
			s := it.ProductID.String()
			pid = &s
		}
		if it.VariantID != nil {
			s := it.VariantID.String()
			vid = &s
		}
		out.Items = append(out.Items, AdminOrderItemResponse{
			ID:            it.ID.String(),
			ProductID:     pid,
			VariantID:     vid,
			TitleSnapshot: it.TitleSnapshot,
			SKUSnapshot:   it.SKUSnapshot,
			OptionSummary: it.OptionSummary,
			UnitPrice:     it.UnitPrice,
			Quantity:      it.Quantity,
			LineTotal:     it.LineTotal,
			CurrencyCode:  it.CurrencyCode,
		})
	}
	for _, a := range addrs {
		out.Addresses = append(out.Addresses, AdminAddressResponse{
			Kind:        a.Kind,
			Name:        a.Name,
			Line1:       a.Line1,
			Line2:       a.Line2,
			City:        a.City,
			Region:      a.Region,
			PostalCode:  a.PostalCode,
			CountryCode: a.CountryCode,
			Phone:       a.Phone,
		})
	}
	return out
}

// -----------------------------------------------------------------------------
// Returns — request bodies
// -----------------------------------------------------------------------------

// ReturnItemRequest is one line of a return.
type ReturnItemRequest struct {
	OrderItemID string  `json:"order_item_id" binding:"required,uuid"`
	Quantity    int     `json:"quantity"      binding:"required,min=1"`
	Reason      *string `json:"reason"`
}

// CreateReturnRequest is the wire body for POST /admin/stores/:storeId/orders/:orderId/returns.
type CreateReturnRequest struct {
	// Type is "return" (refund only, default) or "replace" (exchange).
	Type         string              `json:"type"`
	Reason       *string             `json:"reason"`
	Notes        *string             `json:"notes"`
	Items        []ReturnItemRequest `json:"items"         binding:"required,min=1"`
	CurrencyCode string              `json:"currency_code" binding:"required,len=3"`
}

// ApproveReturnRequest carries the pickup/logistics block the agent
// promises the customer after approval. Optional — POST with an empty
// body simply moves the status to approved without any pickup note.
type ApproveReturnRequest struct {
	PickupDetails string `json:"pickup_details"`
}

// RejectReturnRequest captures the rejection reason.
type RejectReturnRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// MarkRefundedRequest is the wire body for the cross-module refund step.
type MarkRefundedRequest struct {
	Amount        decimal.Decimal `json:"amount"         binding:"required"`
	PaymentStatus string          `json:"payment_status" binding:"required"`
	Reason        string          `json:"reason"`
}

// -----------------------------------------------------------------------------
// Returns — response shape
// -----------------------------------------------------------------------------

// AdminReturnItemResponse is a single line of a return wire response.
type AdminReturnItemResponse struct {
	ID          string  `json:"id"`
	OrderItemID string  `json:"order_item_id"`
	Quantity    int     `json:"quantity"`
	Reason      *string `json:"reason,omitempty"`
}

// AdminReturnResponse is the canonical wire shape for a single return.
type AdminReturnResponse struct {
	ID            string                    `json:"id"`
	TenantID      string                    `json:"tenant_id"`
	StoreID       string                    `json:"store_id"`
	OrderID       string                    `json:"order_id"`
	ReturnNumber  string                    `json:"return_number"`
	Type          string                    `json:"type"`
	Status        string                    `json:"status"`
	Reason        *string                   `json:"reason,omitempty"`
	Notes         *string                   `json:"notes,omitempty"`
	PickupDetails *string                   `json:"pickup_details,omitempty"`
	RejectReason  *string                   `json:"reject_reason,omitempty"`
	RefundAmount  *decimal.Decimal          `json:"refund_amount,omitempty"`
	CurrencyCode  string                    `json:"currency_code"`
	Items         []AdminReturnItemResponse `json:"items"`
	RequestedAt   time.Time                 `json:"requested_at"`
	ApprovedAt    *time.Time                `json:"approved_at,omitempty"`
	RejectedAt    *time.Time                `json:"rejected_at,omitempty"`
	ReceivedAt    *time.Time                `json:"received_at,omitempty"`
	RefundedAt    *time.Time                `json:"refunded_at,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
}

// ToAdminReturnResponse renders Return + items into the wire shape.
func ToAdminReturnResponse(r *order.Return, items []order.ReturnItem) AdminReturnResponse {
	out := AdminReturnResponse{
		ID:            r.ID.String(),
		TenantID:      r.TenantID.String(),
		StoreID:       r.StoreID.String(),
		OrderID:       r.OrderID.String(),
		ReturnNumber:  r.ReturnNumber,
		Type:          r.Type,
		Status:        r.Status,
		Reason:        r.Reason,
		Notes:         r.Notes,
		PickupDetails: r.PickupDetails,
		RejectReason:  r.RejectReason,
		RefundAmount:  r.RefundAmount,
		CurrencyCode:  r.CurrencyCode,
		Items:         make([]AdminReturnItemResponse, 0, len(items)),
		RequestedAt:   r.RequestedAt,
		ApprovedAt:    r.ApprovedAt,
		RejectedAt:    r.RejectedAt,
		ReceivedAt:    r.ReceivedAt,
		RefundedAt:    r.RefundedAt,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	for _, it := range items {
		out.Items = append(out.Items, AdminReturnItemResponse{
			ID:          it.ID.String(),
			OrderItemID: it.OrderItemID.String(),
			Quantity:    it.Quantity,
			Reason:      it.Reason,
		})
	}
	return out
}

// -----------------------------------------------------------------------------
// Abandoned carts — response shape
// -----------------------------------------------------------------------------

// AdminAbandonedCartResponse is the wire shape for an abandoned cart.
type AdminAbandonedCartResponse struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenant_id"`
	StoreID          string          `json:"store_id"`
	CartSessionID    string          `json:"cart_session_id"`
	CustomerEmail    *string         `json:"customer_email,omitempty"`
	CustomerName     *string         `json:"customer_name,omitempty"`
	ItemCount        int             `json:"item_count"`
	Subtotal         decimal.Decimal `json:"subtotal"`
	CurrencyCode     string          `json:"currency_code"`
	RecoveryURL      *string         `json:"recovery_url,omitempty"`
	LastActiveAt     time.Time       `json:"last_active_at"`
	RecoverySentAt   *time.Time      `json:"recovery_sent_at,omitempty"`
	ConvertedOrderID *string         `json:"converted_order_id,omitempty"`
}

// ToAdminAbandonedCartResponse renders an AbandonedCart row into the wire shape.
func ToAdminAbandonedCartResponse(c *order.AbandonedCart) AdminAbandonedCartResponse {
	var convertedID *string
	if c.ConvertedOrderID != nil {
		s := c.ConvertedOrderID.String()
		convertedID = &s
	}
	return AdminAbandonedCartResponse{
		ID:               c.ID.String(),
		TenantID:         c.TenantID.String(),
		StoreID:          c.StoreID.String(),
		CartSessionID:    c.CartSessionID,
		CustomerEmail:    c.CustomerEmail,
		CustomerName:     c.CustomerName,
		ItemCount:        c.ItemCount,
		Subtotal:         c.Subtotal,
		CurrencyCode:     c.CurrencyCode,
		RecoveryURL:      c.RecoveryURL,
		LastActiveAt:     c.LastActiveAt,
		RecoverySentAt:   c.RecoverySentAt,
		ConvertedOrderID: convertedID,
	}
}
