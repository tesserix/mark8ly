// Package storefront — order_detail.go: GET /storefront/stores/:storeSlug/orders/:id
// and POST /storefront/stores/:storeSlug/orders/:id/cancel.
// Returns a customer-facing order view (no admin-only fields like cost_price).
package storefront

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// OrderDetailHandler serves the public order view for storefront
// customers and exposes self-service actions (cancel).
type OrderDetailHandler struct {
	db        *gorm.DB
	orderRepo order.Repository
	orderSvc  *order.Service     // optional — when nil, /cancel returns 503
	docMailer *orderdoc.Service  // optional — when nil, the cancel email is skipped
	logger    *slog.Logger
}

// NewOrderDetailHandler constructs an OrderDetailHandler. orderSvc and
// docMailer are optional dependencies — passing nil disables the
// self-service cancel route and the cancellation email respectively.
func NewOrderDetailHandler(db *gorm.DB, orderRepo order.Repository, orderSvc *order.Service, docMailer *orderdoc.Service, logger *slog.Logger) *OrderDetailHandler {
	return &OrderDetailHandler{db: db, orderRepo: orderRepo, orderSvc: orderSvc, docMailer: docMailer, logger: logger}
}

// storefrontOrderResponse is the customer-facing DTO.
type storefrontOrderResponse struct {
	ID              string                       `json:"id"`
	OrderNumber     string                       `json:"order_number"`
	Status          string                       `json:"status"`
	PaymentStatus   string                       `json:"payment_status"`
	Subtotal        string                       `json:"subtotal"`
	ShippingTotal   string                       `json:"shipping_total"`
	TaxTotal        string                       `json:"tax_total"`
	GrandTotal      string                       `json:"grand_total"`
	// RefundedAmount surfaces partial + full refunds on the customer
	// account page. Always present (zero-value "0.00" when no refunds).
	RefundedAmount  string                       `json:"refunded_amount"`
	CurrencyCode    string                       `json:"currency_code"`
	Items           []storefrontOrderItemResponse `json:"items"`
	ShippingAddress *storefrontAddressResponse    `json:"shipping_address"`
	Shipment        *storefrontShipmentResponse   `json:"shipment,omitempty"`
	Timeline        []storefrontTimelineEntry     `json:"timeline"`
	PlacedAt        string                       `json:"placed_at"`
}

// storefrontShipmentResponse is the public view of a shipment. The
// label_url and carrier-internal ids are kept admin-only; customers
// get the tracking number + status only.
type storefrontShipmentResponse struct {
	Carrier           string `json:"carrier"`
	Service           string `json:"service,omitempty"`
	TrackingNumber    string `json:"tracking_number,omitempty"`
	Status            string `json:"status"`
	EstimatedDelivery string `json:"estimated_delivery,omitempty"`
}

// storefrontTimelineEntry is one row in the customer-facing order
// timeline. Kind maps 1:1 to order_events.kind; description is a
// human-readable one-liner pulled from the payload when present.
type storefrontTimelineEntry struct {
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	Status         string `json:"status,omitempty"`
	Carrier        string `json:"carrier,omitempty"`
	TrackingNumber string `json:"tracking_number,omitempty"`
	OccurredAt     string `json:"occurred_at"`
}

type storefrontOrderItemResponse struct {
	TitleSnapshot string `json:"title_snapshot"`
	SKUSnapshot   string `json:"sku_snapshot"`
	OptionSummary string `json:"option_summary,omitempty"`
	UnitPrice     string `json:"unit_price"`
	Quantity      int    `json:"quantity"`
	LineTotal     string `json:"line_total"`
	CurrencyCode  string `json:"currency_code"`
	ImageURL      string `json:"image_url,omitempty"`
}

type storefrontAddressResponse struct {
	Name        string `json:"name"`
	Line1       string `json:"line1"`
	Line2       string `json:"line2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
	CountryCode string `json:"country_code"`
	Phone       string `json:"phone,omitempty"`
}

// GetOrder handles GET /storefront/stores/:storeSlug/orders/:id.
func (h *OrderDetailHandler) GetOrder(c *gin.Context) {
	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "store_not_found", "message": "store not found",
		})
		return
	}

	idStr := c.Param("id")
	orderID, err := uuid.Parse(idStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_id", "message": "invalid order ID",
		})
		return
	}

	o, items, addrs, err := h.orderRepo.GetByID(c.Request.Context(), h.db, orderID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "order not found",
		})
		return
	}

	// Verify order belongs to this store.
	if o.StoreID.String() != store.ID {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "order not found",
		})
		return
	}

	shipment := h.loadShipment(c.Request.Context(), orderID)
	timeline := h.loadTimeline(c.Request.Context(), orderID)

	resp := mapOrderToStorefrontResponse(o, items, addrs)
	resp.Shipment = shipment
	resp.Timeline = timeline
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

type shipmentRow struct {
	// Real DB columns: shipments has `carrier` (not `provider`) and no
	// `service` column at all — service level is kept in-memory on the
	// canonical shipping.Shipment model (gorm:"-"). Earlier this struct
	// queried non-existent columns and silently returned nil, which made
	// every customer-side receipt download return 409 not_delivered.
	Carrier           string  `gorm:"column:carrier"`
	TrackingNumber    string  `gorm:"column:tracking_number"`
	Status            string  `gorm:"column:status"`
	EstimatedDelivery *string `gorm:"column:estimated_delivery"`
}

func (r shipmentRow) TableName() string { return "shipments" }

func (h *OrderDetailHandler) loadShipment(ctx context.Context, orderID uuid.UUID) *storefrontShipmentResponse {
	var row shipmentRow
	err := h.db.WithContext(ctx).
		Table("shipments").
		Select("carrier", "tracking_number", "status", "estimated_delivery").
		Where("order_id = ?", orderID).
		Order("created_at DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil || row.Carrier == "" {
		return nil
	}
	resp := &storefrontShipmentResponse{
		Carrier:        row.Carrier,
		// Service level isn't persisted, so we leave it empty on the
		// public DTO — the customer card already reads "Standard delivery"
		// from the order's chosen shipping_service.
		TrackingNumber: row.TrackingNumber,
		Status:         row.Status,
	}
	if row.EstimatedDelivery != nil {
		resp.EstimatedDelivery = *row.EstimatedDelivery
	}
	return resp
}

type timelineRow struct {
	Kind      string `gorm:"column:kind"`
	Payload   []byte `gorm:"column:payload"`
	CreatedAt string `gorm:"column:created_at"`
}

func (timelineRow) TableName() string { return "order_events" }

func (h *OrderDetailHandler) loadTimeline(ctx context.Context, orderID uuid.UUID) []storefrontTimelineEntry {
	var rows []timelineRow
	if err := h.db.WithContext(ctx).
		Table("order_events").
		Select("kind", "payload", "to_char(created_at, 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') AS created_at").
		Where("order_id = ?", orderID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return []storefrontTimelineEntry{}
	}

	out := make([]storefrontTimelineEntry, 0, len(rows))
	for _, r := range rows {
		entry := storefrontTimelineEntry{
			Kind:       r.Kind,
			OccurredAt: r.CreatedAt,
		}
		// Best-effort enrichment — shipment events carry extra fields,
		// others have simpler payloads. We only surface what the UI
		// actually renders; unknown payloads are left as bare entries.
		var raw map[string]any
		_ = json.Unmarshal(r.Payload, &raw)
		if raw != nil {
			if s, ok := raw["description"].(string); ok {
				entry.Description = s
			}
			if s, ok := raw["status"].(string); ok {
				entry.Status = s
			}
			if s, ok := raw["carrier"].(string); ok {
				entry.Carrier = s
			}
			if s, ok := raw["tracking_number"].(string); ok {
				entry.TrackingNumber = s
			}
		}
		if entry.Description == "" {
			entry.Description = defaultTimelineDescription(r.Kind)
		}
		out = append(out, entry)
	}
	return out
}

// defaultTimelineDescription is a fallback label for events whose payload
// doesn't carry a pre-formatted description (older rows, returns, etc.).
func defaultTimelineDescription(kind string) string {
	switch kind {
	case "created":
		return "Order placed."
	case "status_changed":
		return "Order status updated."
	case "payment_recorded":
		return "Payment received."
	case "fulfilled":
		return "Order fulfilled."
	case "cancelled":
		return "Order cancelled."
	case "refunded":
		return "Order refunded."
	case "shipment_created":
		return "Shipping label created."
	case "shipment_in_transit":
		return "Package is on its way."
	case "shipment_out_for_delivery":
		return "Out for delivery."
	case "shipment_delivered":
		return "Package delivered."
	case "shipment_exception":
		return "Delivery exception."
	default:
		return kind
	}
}

func mapOrderToStorefrontResponse(o *order.Order, items []order.OrderItem, addrs []order.OrderAddress) storefrontOrderResponse {
	respItems := make([]storefrontOrderItemResponse, 0, len(items))
	for _, it := range items {
		ri := storefrontOrderItemResponse{
			TitleSnapshot: it.TitleSnapshot,
			SKUSnapshot:   it.SKUSnapshot,
			UnitPrice:     it.UnitPrice.StringFixed(2),
			Quantity:      it.Quantity,
			LineTotal:     it.LineTotal.StringFixed(2),
			CurrencyCode:  it.CurrencyCode,
		}
		if it.OptionSummary != nil {
			ri.OptionSummary = *it.OptionSummary
		}
		if it.ImageURL != nil {
			ri.ImageURL = *it.ImageURL
		}
		respItems = append(respItems, ri)
	}

	var shippingAddr *storefrontAddressResponse
	for _, a := range addrs {
		if a.Kind == "shipping" {
			shippingAddr = &storefrontAddressResponse{
				Name:        a.Name,
				Line1:       a.Line1,
				Line2:       derefStr(a.Line2),
				City:        a.City,
				Region:      derefStr(a.Region),
				PostalCode:  derefStr(a.PostalCode),
				CountryCode: a.CountryCode,
				Phone:       derefStr(a.Phone),
			}
			break
		}
	}

	return storefrontOrderResponse{
		ID:              o.ID.String(),
		OrderNumber:     o.OrderNumber,
		Status:          string(o.Status),
		PaymentStatus:   string(o.PaymentStatus),
		Subtotal:        o.Subtotal.StringFixed(2),
		ShippingTotal:   decimalStr(o.ShippingTotal),
		TaxTotal:        decimalStr(o.TaxTotal),
		GrandTotal:      o.GrandTotal.StringFixed(2),
		RefundedAmount:  o.RefundedAmount.StringFixed(2),
		CurrencyCode:    o.CurrencyCode,
		Items:           respItems,
		ShippingAddress: shippingAddr,
		PlacedAt:        o.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func decimalStr(d decimal.Decimal) string {
	if d.IsZero() {
		return "0.00"
	}
	return d.StringFixed(2)
}

// ─────────────────────────────────────────────────────────────────────────
// Self-service cancel
// ─────────────────────────────────────────────────────────────────────────

// CancelRequest is the wire body for POST /storefront/.../orders/:id/cancel.
// Reason is optional — when empty we substitute a generic copy so the
// merchant always sees something in the order_events row.
type CancelRequest struct {
	Reason string `json:"reason"`
}

// Cancel handles POST /storefront/stores/:storeSlug/orders/:id/cancel.
//
// Authn: requires the customer session cookie (OptionalCustomerAuth must
// have populated CustomerProfileKey on the Gin context).
// Authz: the order must belong to the same customer profile + same store.
// Lifecycle guards: same as the admin path (status machine in
// order.Service rejects fulfilled, plus we bounce shipment-in-flight here).
func (h *OrderDetailHandler) Cancel(c *gin.Context) {
	if h.orderSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "service_unavailable",
			"message": "Self-service cancel is not configured for this deployment.",
		})
		return
	}

	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "store_not_found", "message": "store not found"})
		return
	}

	profileVal, exists := c.Get(CustomerProfileKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Sign in to cancel an order."})
		return
	}
	profile, ok := profileVal.(*customer.CustomerProfile)
	if !ok || profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Sign in to cancel an order."})
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id", "message": "invalid order id"})
		return
	}

	// Load + scope the order to the signed-in customer + this store. We
	// deliberately return the same not_found shape for "wrong customer"
	// and "wrong store" so the endpoint doesn't double as an order-id
	// existence oracle.
	o, _, _, err := h.orderRepo.GetByID(c.Request.Context(), h.db, orderID)
	if err != nil || o == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "order not found"})
		return
	}
	if o.StoreID.String() != store.ID || o.CustomerID == nil || o.CustomerID.String() != profile.ID.String() {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "order not found"})
		return
	}

	// Belt-and-braces shipment guard — once a shipment is in flight the
	// customer can't self-cancel; they need to contact support for a
	// return-to-sender + refund.
	var shipStatus string
	row := h.db.WithContext(c.Request.Context()).
		Table("shipments").
		Select("status").
		Where("order_id = ?", orderID).
		Order("created_at DESC").
		Limit(1).
		Row()
	if scanErr := row.Scan(&shipStatus); scanErr == nil {
		switch shipStatus {
		case "in_transit", "out_for_delivery", "delivered":
			c.JSON(http.StatusConflict, gin.H{
				"error":   "shipment_in_flight",
				"message": "Your order has already shipped — please contact support to arrange a return and refund.",
			})
			return
		}
	}

	var req CancelRequest
	_ = c.ShouldBindJSON(&req)
	if req.Reason == "" {
		req.Reason = "Cancelled by customer"
	}

	if err := h.orderSvc.Cancel(c.Request.Context(), nil, orderID, req.Reason); err != nil {
		// Status-machine guard inside service.Cancel returns
		// apperrors.InvalidTransition for non-cancellable states; map to 409.
		if errors.Is(err, apperrors.ErrInvalidTransition) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "not_cancellable",
				"message": "This order is not in a state where it can be cancelled.",
			})
			return
		}
		h.logger.Error("storefront cancel: service error", "err", err, "order_id", orderID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "Could not cancel the order. Please try again."})
		return
	}

	// Fire the cancellation email on a detached context — same fire-and-
	// forget contract as the admin path. byCustomer=true switches the
	// email copy to the self-service tone.
	if h.docMailer != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := h.docMailer.SendCancellation(ctx, orderID, req.Reason, true); err != nil {
				h.logger.Warn("orderdoc: customer cancel email dispatch failed",
					"order_id", orderID, "err", err)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"cancelled": true, "order_id": orderID.String()})
}
