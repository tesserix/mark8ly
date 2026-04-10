// Package storefront — order_detail.go: GET /storefront/stores/:storeSlug/orders/:id.
// Returns a customer-facing order view (no admin-only fields like cost_price).
package storefront

import (
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
	PlacedAt        string                       `json:"placed_at"`
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

	resp := mapOrderToStorefrontResponse(o, items, addrs)
	c.JSON(http.StatusOK, gin.H{"data": resp})
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
