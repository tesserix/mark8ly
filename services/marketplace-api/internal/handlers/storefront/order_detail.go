// Package storefront — order_detail.go: GET /storefront/stores/:storeSlug/orders/:id.
// Returns a customer-facing order view (no admin-only fields like cost_price).
package storefront

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// OrderDetailHandler serves the public order view for storefront customers.
type OrderDetailHandler struct {
	db        *gorm.DB
	orderRepo order.Repository
	logger    *slog.Logger
}

// NewOrderDetailHandler constructs an OrderDetailHandler.
func NewOrderDetailHandler(db *gorm.DB, orderRepo order.Repository, logger *slog.Logger) *OrderDetailHandler {
	return &OrderDetailHandler{db: db, orderRepo: orderRepo, logger: logger}
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
	Carrier           string  `gorm:"column:provider"`
	Service           string  `gorm:"column:service"`
	TrackingNumber    string  `gorm:"column:tracking_number"`
	Status            string  `gorm:"column:status"`
	EstimatedDelivery *string `gorm:"column:estimated_delivery"`
}

func (r shipmentRow) TableName() string { return "shipments" }

func (h *OrderDetailHandler) loadShipment(ctx context.Context, orderID uuid.UUID) *storefrontShipmentResponse {
	var row shipmentRow
	err := h.db.WithContext(ctx).
		Table("shipments").
		Select("provider", "service", "tracking_number", "status", "estimated_delivery").
		Where("order_id = ?", orderID).
		Order("created_at DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil || row.Carrier == "" {
		return nil
	}
	resp := &storefrontShipmentResponse{
		Carrier:        row.Carrier,
		Service:        row.Service,
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
