// Package admin — shipments.go: admin handler for creating and retrieving
// shipments on orders. Mounted under /api/v1/admin/stores/:storeId/orders/:id/shipments.
package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ShipmentsHandler bundles dependencies for the admin shipment endpoints.
type ShipmentsHandler struct {
	db      *gorm.DB
	svc     *shipping.ShippingService
	repo    shipping.Repository
	logger  *slog.Logger
}

// NewShipmentsHandler constructs a ShipmentsHandler.
func NewShipmentsHandler(
	db *gorm.DB,
	svc *shipping.ShippingService,
	repo shipping.Repository,
	logger *slog.Logger,
) *ShipmentsHandler {
	return &ShipmentsHandler{db: db, svc: svc, repo: repo, logger: logger}
}

// ─────────────────────────────────────────────────────────────────────────
// DTOs
// ─────────────────────────────────────────────────────────────────────────

// CreateShipmentRequest is the wire body for POST .../orders/:id/shipments.
type CreateShipmentRequest struct {
	Provider string `json:"provider" binding:"required"`
	Service  string `json:"service"  binding:"required"`
}

// ShipmentResponse is the wire shape returned by shipment endpoints.
type ShipmentResponse struct {
	ID                 string     `json:"id"`
	OrderID            string     `json:"order_id"`
	Provider           string     `json:"provider"`
	ProviderShipmentID string     `json:"provider_shipment_id"`
	TrackingNumber     string     `json:"tracking_number"`
	LabelURL           string     `json:"label_url"`
	Service            string     `json:"service"`
	Status             string     `json:"status"`
	CurrencyCode       string     `json:"currency_code"`
	EstimatedDelivery  *time.Time `json:"estimated_delivery,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

func toShipmentResponse(rec *shipping.ShipmentRecord) ShipmentResponse {
	return ShipmentResponse{
		ID:                 rec.ID.String(),
		OrderID:            rec.OrderID.String(),
		Provider:           rec.Provider,
		ProviderShipmentID: rec.ProviderShipmentID,
		TrackingNumber:     rec.TrackingNumber,
		LabelURL:           rec.LabelURL,
		Service:            rec.Service,
		Status:             rec.Status,
		CurrencyCode:       rec.CurrencyCode,
		EstimatedDelivery:  rec.EstimatedDelivery,
		CreatedAt:          rec.CreatedAt,
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────

// Create handles POST /admin/stores/:storeId/orders/:id/shipments.
func (h *ShipmentsHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}

	var req CreateShipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	// Load the order to get destination address + items + currency.
	var o order.Order
	if err := h.db.WithContext(ctx).
		Where("id = ? AND store_id = ? AND tenant_id = ? AND deleted_at IS NULL", orderID, storeID, tenantID).
		First(&o).Error; err != nil {
		RespondErr(c, apperrors.NotFound("order"), h.logger)
		return
	}

	// Load shipping address.
	var shippingAddr order.OrderAddress
	if err := h.db.WithContext(ctx).
		Where("order_id = ? AND kind = ?", orderID, "shipping").
		First(&shippingAddr).Error; err != nil {
		RespondErr(c, apperrors.ValidationFailed("order", "order has no shipping address"), h.logger)
		return
	}

	// Load order items for parcel info.
	var items []order.OrderItem
	if err := h.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Find(&items).Error; err != nil {
		RespondErr(c, fmt.Errorf("shipments: load order items: %w", err), h.logger)
		return
	}

	// Load carrier config for credentials + warehouse address.
	provider := strings.ToLower(req.Provider)
	carrierCfg, err := h.repo.GetCarrierConfig(ctx, storeID, provider)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("provider",
			fmt.Sprintf("no carrier config found for provider %q", provider)), h.logger)
		return
	}

	// Validate warehouse address is configured.
	if carrierCfg.WarehouseLine1 == "" || carrierCfg.WarehouseCity == "" || carrierCfg.WarehouseCountry == "" {
		RespondErr(c, apperrors.ValidationFailed("provider",
			"warehouse address is not configured for this carrier"), h.logger)
		return
	}

	// Create the carrier instance.
	carrier, err := shipping.NewCarrier(provider, carrierCfg.APIKey, carrierCfg.SecretKey, carrierCfg.Mode)
	if err != nil {
		RespondErr(c, fmt.Errorf("shipments: create carrier: %w", err), h.logger)
		return
	}

	// Build origin address from warehouse config.
	fromAddress := shipping.Address{
		Name:        carrierCfg.WarehouseName,
		Line1:       carrierCfg.WarehouseLine1,
		Line2:       carrierCfg.WarehouseLine2,
		City:        carrierCfg.WarehouseCity,
		Region:      carrierCfg.WarehouseRegion,
		PostalCode:  carrierCfg.WarehousePostal,
		CountryCode: carrierCfg.WarehouseCountry,
		Phone:       carrierCfg.WarehousePhone,
	}

	// Build destination address from order.
	toAddress := shipping.Address{
		Name:        shippingAddr.Name,
		Line1:       shippingAddr.Line1,
		City:        shippingAddr.City,
		CountryCode: shippingAddr.CountryCode,
	}
	if shippingAddr.Line2 != nil {
		toAddress.Line2 = *shippingAddr.Line2
	}
	if shippingAddr.Region != nil {
		toAddress.Region = *shippingAddr.Region
	}
	if shippingAddr.PostalCode != nil {
		toAddress.PostalCode = *shippingAddr.PostalCode
	}
	if shippingAddr.Phone != nil {
		toAddress.Phone = *shippingAddr.Phone
	}

	// Build parcel items from order items.
	parcelItems := make([]shipping.ParcelItem, 0, len(items))
	for _, it := range items {
		parcelItems = append(parcelItems, shipping.ParcelItem{
			Title:    it.TitleSnapshot,
			SKU:      it.SKUSnapshot,
			Quantity: it.Quantity,
		})
	}

	// Call the carrier to create shipment.
	shipment, err := h.svc.CreateShipment(
		ctx,
		orderID.String(),
		fromAddress,
		toAddress,
		parcelItems,
		req.Service,
		o.CurrencyCode,
		carrier,
	)
	if err != nil {
		RespondErr(c, fmt.Errorf("shipments: carrier create: %w", err), h.logger)
		return
	}

	// Persist the shipment record.
	storeUUID, _ := uuid.Parse(storeID)
	tenantUUID, _ := uuid.Parse(tenantID)

	rec := &shipping.ShipmentRecord{
		TenantID:           tenantUUID,
		StoreID:            storeUUID,
		OrderID:            orderID,
		Provider:           provider,
		ProviderShipmentID: shipment.ProviderShipmentID,
		TrackingNumber:     shipment.TrackingNumber,
		LabelURL:           shipment.LabelURL,
		Service:            shipment.Service,
		Status:             "created",
		CurrencyCode:       o.CurrencyCode,
		EstimatedDelivery:  shipment.EstimatedDelivery,
	}

	if err := h.repo.CreateShipment(ctx, rec); err != nil {
		RespondErr(c, fmt.Errorf("shipments: persist record: %w", err), h.logger)
		return
	}

	c.JSON(http.StatusCreated, toShipmentResponse(rec))
}

// GetByOrder handles GET /admin/stores/:storeId/orders/:id/shipments.
func (h *ShipmentsHandler) GetByOrder(c *gin.Context) {
	ctx := c.Request.Context()

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}

	rec, err := h.repo.GetShipmentByOrderID(ctx, orderID)
	if err != nil {
		// No shipment found — return null.
		c.JSON(http.StatusOK, nil)
		return
	}

	// Verify the shipment belongs to this store.
	if rec.StoreID.String() != c.Param("storeId") {
		c.JSON(http.StatusOK, nil)
		return
	}

	c.JSON(http.StatusOK, toShipmentResponse(rec))
}
