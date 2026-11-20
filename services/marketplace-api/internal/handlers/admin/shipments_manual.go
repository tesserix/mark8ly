// Package admin — shipments_manual.go: own-courier shipments. A seller on
// own delivery marks the order shipped by picking a saved courier and
// entering a tracking number; no carrier is called.
package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mark8ly/marketplace-api/internal/delivery"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

const ownCarrier = "own"

// WithDelivery attaches the own-delivery service used to resolve couriers.
func (h *ShipmentsHandler) WithDelivery(svc *delivery.Service) *ShipmentsHandler {
	h.delivery = svc
	return h
}

// ManualShipmentRequest is the body for POST .../orders/:id/shipments/manual.
type ManualShipmentRequest struct {
	CourierID      string `json:"courier_id" binding:"required"`
	TrackingNumber string `json:"tracking_number"`
}

// TrackingUpdateRequest is the body for PATCH .../shipments/:shipmentId/tracking.
type TrackingUpdateRequest struct {
	CourierID      string `json:"courier_id"`
	TrackingNumber string `json:"tracking_number" binding:"required"`
}

var (
	errNotOwnDelivery = errors.New("not_own_delivery")
	errAlreadyShipped = errors.New("already_shipped")
	errNotShippable   = errors.New("order_not_shippable")
)

func respondConflict(c *gin.Context, err error) {
	msg := "This order already has an active shipment. Edit its tracking instead."
	if errors.Is(err, errNotShippable) {
		msg = "A cancelled or refunded order cannot be marked as shipped."
	}
	if errors.Is(err, errNotOwnDelivery) {
		msg = "This order was paid with a carrier rate; create a shipping label instead."
	}
	c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": err.Error(), "message": msg})
}

// resolveTracking validates the number and builds the courier's link, if any.
func resolveTracking(courier *delivery.Courier, raw string) (number string, url *string, err error) {
	number = strings.TrimSpace(raw)
	if len(number) > 100 {
		return "", nil, apperrors.ValidationFailed("tracking_number", "must be at most 100 characters")
	}
	if courier.TrackingURLTemplate != nil {
		if u, ok := delivery.BuildTrackingURL(*courier.TrackingURLTemplate, number); ok {
			url = &u
		}
	}
	return number, url, nil
}

// CreateManual handles POST /admin/stores/:storeId/orders/:id/shipments/manual.
func (h *ShipmentsHandler) CreateManual(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	var req ManualShipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if h.delivery == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "delivery_unavailable", "message": "Own delivery is not configured for this deployment."})
		return
	}
	courier, err := h.delivery.GetCourier(ctx, storeID, req.CourierID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	number, trackingURL, err := resolveTracking(courier, req.TrackingNumber)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	courierUUID, _ := uuid.Parse(courier.ID)
	storeUUID, _ := uuid.Parse(storeID)
	tenantUUID, _ := uuid.Parse(tenantID)

	var rec *shipping.ShipmentRecord
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Row lock serialises concurrent Mark-as-shipped clicks on one order;
		// the partial unique index is the backstop.
		var o order.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND store_id = ? AND tenant_id = ? AND deleted_at IS NULL", orderID, storeID, tenantID).
			First(&o).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NotFound("order")
			}
			return err
		}
		if o.ShippingCarrier == nil || *o.ShippingCarrier != ownCarrier {
			return errNotOwnDelivery
		}
		if o.Status == string(order.OrderStatusCancelled) || o.PaymentStatus == string(order.PaymentStatusRefunded) {
			return errNotShippable
		}
		var active int64
		if err := tx.Table("shipments").Where("order_id = ? AND status <> 'cancelled'", orderID).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return errAlreadyShipped
		}

		var addr order.OrderAddress
		if err := tx.Where("order_id = ? AND kind = ?", orderID, "shipping").First(&addr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ValidationFailed("order", "order has no shipping address")
			}
			return err
		}
		toJSON, _ := json.Marshal(map[string]any{
			"name":         addr.Name,
			"line1":        addr.Line1,
			"line2":        derefStringPtr(addr.Line2),
			"city":         addr.City,
			"region":       derefStringPtr(addr.Region),
			"postal_code":  derefStringPtr(addr.PostalCode),
			"country_code": addr.CountryCode,
			"phone":        derefStringPtr(addr.Phone),
		})
		fromJSON := []byte(`{}`)
		wh, err := h.warehouseRepo.DefaultForStore(ctx, tx, storeID)
		switch {
		case err == nil:
			fromJSON, _ = json.Marshal(map[string]any{
				"name":         wh.Name,
				"line1":        wh.Line1,
				"line2":        wh.Line2,
				"city":         wh.City,
				"region":       wh.Region,
				"postal_code":  wh.PostalCode,
				"country_code": wh.CountryCode,
				"phone":        wh.Phone,
			})
		case !errors.Is(err, warehouse.ErrNotFound):
			return err
		}

		now := time.Now().UTC()
		courierName := courier.Name
		rec = &shipping.ShipmentRecord{
			ID:             uuid.New(),
			TenantID:       tenantUUID,
			StoreID:        storeUUID,
			OrderID:        orderID,
			Carrier:        ownCarrier,
			CourierID:      &courierUUID,
			CourierName:    &courierName,
			TrackingNumber: number,
			TrackingURL:    trackingURL,
			Status:         "in_transit",
			ShipFrom:       datatypes.JSON(fromJSON),
			ShipTo:         datatypes.JSON(toJSON),
			CurrencyCode:   o.CurrencyCode,
			ShippedAt:      &now,
			Provider:       ownCarrier,
		}
		if err := tx.Create(rec).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "shipments_one_active_own_per_order" {
				return errAlreadyShipped
			}
			return fmt.Errorf("shipments: persist manual shipment: %w", err)
		}
		if err := tx.Table("order_allocations").
			Where("order_id = ? AND shipment_id IS NULL", orderID).
			Update("shipment_id", rec.ID).Error; err != nil {
			return fmt.Errorf("shipments: link allocations: %w", err)
		}
		// Already fulfilled (manual Fulfil, or a re-ship after deleting a
		// mistaken shipment) is the end state we want, not an error.
		if h.orderSvc != nil && o.FulfillmentStatus != string(order.FulfillmentStatusFulfilled) {
			return h.orderSvc.MarkFulfillmentComplete(ctx, tx, orderID)
		}
		return nil
	})
	if errors.Is(err, errNotOwnDelivery) || errors.Is(err, errAlreadyShipped) || errors.Is(err, errNotShippable) {
		respondConflict(c, err)
		return
	}
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.appendShipmentEvent(ctx, orderID, rec, order.EventKindShipmentInTransit, shipmentStatusDefaultCopy["in_transit"])
	h.dispatchShipmentDispatchedEmail(orderID, rec)
	c.JSON(http.StatusCreated, toShipmentResponse(rec))
}

// UpdateTracking handles PATCH /admin/stores/:storeId/orders/:id/shipments/:shipmentId/tracking.
// Fixes the courier or number on an own shipment; never re-sends the email.
func (h *ShipmentsHandler) UpdateTracking(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")
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
	var req TrackingUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if h.delivery == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "delivery_unavailable", "message": "Own delivery is not configured for this deployment."})
		return
	}

	rec, err := h.repo.GetShipmentByID(ctx, shipmentID)
	if err != nil || rec.OrderID != orderID || rec.StoreID.String() != storeID {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}
	if rec.Carrier != ownCarrier {
		respondConflict(c, errNotOwnDelivery)
		return
	}

	courierID := req.CourierID
	if courierID == "" && rec.CourierID != nil {
		courierID = rec.CourierID.String()
	}
	courier, err := h.delivery.GetCourier(ctx, storeID, courierID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if strings.TrimSpace(req.TrackingNumber) == "" {
		RespondErr(c, apperrors.ValidationFailed("tracking_number", "is required"), h.logger)
		return
	}
	number, trackingURL, err := resolveTracking(courier, req.TrackingNumber)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	courierUUID, _ := uuid.Parse(courier.ID)
	courierName := courier.Name
	if err := h.db.WithContext(ctx).Table("shipments").Where("id = ?", shipmentID).Updates(map[string]any{
		"courier_id":      courierUUID,
		"courier_name":    courierName,
		"tracking_number": number,
		"tracking_url":    trackingURL,
		"updated_at":      time.Now().UTC(),
	}).Error; err != nil {
		RespondErr(c, fmt.Errorf("shipments: update tracking: %w", err), h.logger)
		return
	}
	rec.CourierID, rec.CourierName, rec.TrackingNumber, rec.TrackingURL = &courierUUID, &courierName, number, trackingURL
	rec.Provider = ownCarrier
	c.JSON(http.StatusOK, toShipmentResponse(rec))
}
