// Package admin — shipments.go: admin handler for creating and retrieving
// shipments on orders. Mounted under /api/v1/admin/stores/:storeId/orders/:id/shipments.
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ShipmentsHandler bundles dependencies for the admin shipment endpoints.
type ShipmentsHandler struct {
	db        *gorm.DB
	svc       *shipping.ShippingService
	repo      shipping.Repository
	docMailer *orderdoc.Service     // optional — nil disables receipt-on-delivery email
	notify    *notification.Service // optional — nil-safe
	logger    *slog.Logger
}

// NewShipmentsHandler constructs a ShipmentsHandler. docMailer is
// optional; when nil, the receipt email on delivery is skipped.
func NewShipmentsHandler(
	db *gorm.DB,
	svc *shipping.ShippingService,
	repo shipping.Repository,
	docMailer *orderdoc.Service,
	logger *slog.Logger,
) *ShipmentsHandler {
	return &ShipmentsHandler{db: db, svc: svc, repo: repo, docMailer: docMailer, logger: logger}
}

// WithNotifier attaches the notification service so delivery events fire
// in-app notifications. Nil-safe.
func (h *ShipmentsHandler) WithNotifier(n *notification.Service) *ShipmentsHandler {
	h.notify = n
	return h
}

// dispatchReceiptEmail fires the receipt email on a detached background
// context. Caller has already verified the shipment transitioned to
// "delivered".
func (h *ShipmentsHandler) dispatchReceiptEmail(orderID uuid.UUID) {
	if h.docMailer == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.docMailer.SendReceipt(ctx, orderID); err != nil {
			h.logger.Warn("orderdoc: receipt email dispatch failed",
				"order_id", orderID, "err", err)
		}
	}()
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
		if h.logger != nil {
			h.logger.Error("shipments: carrier config not found",
				"store_id", storeID, "provider", provider, "err", err)
		}
		RespondErr(c, apperrors.ValidationFailed("provider",
			fmt.Sprintf("no active carrier config for %q. Configure it under Settings → Shipping.", provider)), h.logger)
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
		// Temporary verbose surface — bubble the upstream carrier error
		// so the admin UI shows something actionable instead of "internal
		// server error". Keep for now; trim after delivery is stable.
		if h.logger != nil {
			h.logger.Error("shipments: carrier create failed",
				"order_id", orderID.String(),
				"provider", provider,
				"service", req.Service,
				"err", err.Error())
		}
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error":   "carrier_create_failed",
			"message": err.Error(),
		})
		return
	}

	// Persist the shipment record.
	storeUUID, _ := uuid.Parse(storeID)
	tenantUUID, _ := uuid.Parse(tenantID)

	// ship_from and ship_to are NOT NULL JSONB on the migration-defined
	// shipments table. Build them from the warehouse + customer address
	// we already loaded.
	fromJSON, _ := json.Marshal(map[string]any{
		"name":         carrierCfg.WarehouseName,
		"line1":        carrierCfg.WarehouseLine1,
		"line2":        carrierCfg.WarehouseLine2,
		"city":         carrierCfg.WarehouseCity,
		"region":       carrierCfg.WarehouseRegion,
		"postal_code":  carrierCfg.WarehousePostal,
		"country_code": carrierCfg.WarehouseCountry,
		"phone":        carrierCfg.WarehousePhone,
	})
	toJSON, _ := json.Marshal(map[string]any{
		"name":         shippingAddr.Name,
		"line1":        shippingAddr.Line1,
		"line2":        derefStringPtr(shippingAddr.Line2),
		"city":         shippingAddr.City,
		"region":       derefStringPtr(shippingAddr.Region),
		"postal_code":  derefStringPtr(shippingAddr.PostalCode),
		"country_code": shippingAddr.CountryCode,
		"phone":        derefStringPtr(shippingAddr.Phone),
	})

	rec := &shipping.ShipmentRecord{
		TenantID:           tenantUUID,
		StoreID:            storeUUID,
		OrderID:            orderID,
		Carrier:            provider,
		TrackingNumber:     shipment.TrackingNumber,
		LabelURL:           shipment.LabelURL,
		Status:             "pending",
		ShipFrom:           datatypes.JSON(fromJSON),
		ShipTo:             datatypes.JSON(toJSON),
		CurrencyCode:       o.CurrencyCode,
		EstimatedDelivery:  shipment.EstimatedDelivery,
		// Response-only fields — not persisted, but carried so the wire
		// DTO can surface them.
		Provider:           provider,
		ProviderShipmentID: shipment.ProviderShipmentID,
		Service:            shipment.Service,
	}

	if err := h.repo.CreateShipment(ctx, rec); err != nil {
		RespondErr(c, fmt.Errorf("shipments: persist record: %w", err), h.logger)
		return
	}

	// Timeline event — customer order-detail page reads order_events to
	// render the tracking feed. Failures here only log; the shipment
	// still persisted successfully.
	h.appendShipmentEvent(ctx, orderID, rec, order.EventKindShipmentCreated,
		"Shipping label created — package will be picked up shortly.")

	c.JSON(http.StatusCreated, toShipmentResponse(rec))
}

// derefStringPtr returns "" when the input is nil — used while building
// ship_to / ship_from JSONB from the OrderAddress's nullable columns.
func derefStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// appendShipmentEvent writes a row into order_events describing a
// shipment lifecycle transition. Separate from the main response path
// so failures don't block the user-visible action.
func (h *ShipmentsHandler) appendShipmentEvent(
	ctx context.Context,
	orderID uuid.UUID,
	rec *shipping.ShipmentRecord,
	kind order.EventKind,
	description string,
) {
	evt := &order.OrderEvent{
		OrderID: orderID,
		Kind:    string(kind),
		Payload: order.EncodeShipment(order.ShipmentEventPayload{
			ShipmentID:     rec.ID.String(),
			Carrier:        rec.Provider,
			TrackingNumber: rec.TrackingNumber,
			Status:         rec.Status,
			Description:    description,
		}),
	}
	if err := h.db.WithContext(ctx).Create(evt).Error; err != nil && h.logger != nil {
		h.logger.Error("shipments: append event failed",
			"order_id", orderID, "kind", kind, "err", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Status advance — PATCH .../shipments/:shipmentId/status
// ─────────────────────────────────────────────────────────────────────────

// UpdateStatusRequest is the wire body for advancing a shipment through
// its tracking lifecycle. Accepted values: in_transit,
// out_for_delivery, delivered, exception.
type UpdateStatusRequest struct {
	Status      string `json:"status" binding:"required"`
	Description string `json:"description,omitempty"`
}

var shipmentStatusKinds = map[string]order.EventKind{
	"in_transit":       order.EventKindShipmentInTransit,
	"out_for_delivery": order.EventKindShipmentOutForDelivery,
	"delivered":        order.EventKindShipmentDelivered,
	"exception":        order.EventKindShipmentException,
}

var shipmentStatusDefaultCopy = map[string]string{
	"in_transit":       "Package is on its way.",
	"out_for_delivery": "Out for delivery — it will arrive today.",
	"delivered":        "Package delivered.",
	"exception":        "Delivery exception — we're looking into it.",
}

// UpdateStatus handles PATCH /admin/stores/:storeId/orders/:id/shipments/:shipmentId/status.
func (h *ShipmentsHandler) UpdateStatus(c *gin.Context) {
	ctx := c.Request.Context()

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	shipmentID, err := uuid.Parse(c.Param("shipmentId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("shipmentId", "must be a uuid"), h.logger)
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	kind, ok := shipmentStatusKinds[status]
	if !ok {
		RespondErr(c, apperrors.ValidationFailed("status",
			"must be one of in_transit, out_for_delivery, delivered, exception"), h.logger)
		return
	}

	rec, err := h.repo.GetShipmentByOrderID(ctx, orderID)
	if err != nil {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}
	if rec.ID != shipmentID || rec.StoreID.String() != c.Param("storeId") {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}

	// Update the shipment row + stamp the relevant timestamp. The
	// shipped_at/delivered_at columns exist in the schema but aren't on
	// the GORM model; write them with a raw field update.
	now := time.Now().UTC()
	fields := map[string]any{"status": status, "updated_at": now}
	if status == "in_transit" {
		fields["shipped_at"] = now
	}
	if status == "delivered" {
		fields["delivered_at"] = now
	}
	if err := h.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", shipmentID).
		Updates(fields).Error; err != nil {
		RespondErr(c, fmt.Errorf("shipments: update status: %w", err), h.logger)
		return
	}

	rec.Status = status

	desc := req.Description
	if desc == "" {
		desc = shipmentStatusDefaultCopy[status]
	}
	h.appendShipmentEvent(ctx, orderID, rec, kind, desc)

	// Customer notification: shipment just moved into "delivered" — fire
	// the receipt email. Detached from the request context so the merchant
	// gets a fast 200 even if SendGrid is slow.
	if status == "delivered" {
		h.dispatchReceiptEmail(orderID)
		fulfilledMsg := "An order was marked delivered."
		fulfilledResource := "order"
		notification.Emit(ctx, h.notify, h.logger, notification.Notification{
			TenantID:     rec.TenantID,
			StoreID:      rec.StoreID,
			Type:         notification.TypeOrderFulfilled,
			Title:        "Order fulfilled",
			Message:      &fulfilledMsg,
			ResourceType: &fulfilledResource,
			ResourceID:   &orderID,
		})
	}

	c.JSON(http.StatusOK, toShipmentResponse(rec))
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
