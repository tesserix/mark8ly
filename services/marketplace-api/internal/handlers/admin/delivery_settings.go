package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/delivery"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// DeliverySettingsHandler serves a store's own-delivery settings.
type DeliverySettingsHandler struct {
	svc    *delivery.Service
	logger *slog.Logger
}

func NewDeliverySettingsHandler(svc *delivery.Service, logger *slog.Logger) *DeliverySettingsHandler {
	return &DeliverySettingsHandler{svc: svc, logger: logger}
}

func (h *DeliverySettingsHandler) Get(c *gin.Context) {
	s := storeFromCtx(c)
	if s == nil {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), s.ID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *DeliverySettingsHandler) SetMode(c *gin.Context) {
	s := storeFromCtx(c)
	if s == nil {
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid JSON"), h.logger)
		return
	}
	if err := h.svc.SetMode(c.Request.Context(), s.ID, body.Mode); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": body.Mode})
}

// PutOptions replaces the option list; free_shipping_min is always applied
// (omitted or null clears it).
func (h *DeliverySettingsHandler) PutOptions(c *gin.Context) {
	s := storeFromCtx(c)
	if s == nil {
		return
	}
	var body struct {
		Options         []delivery.OptionInput `json:"options"`
		FreeShippingMin *decimal.Decimal       `json:"free_shipping_min"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid JSON"), h.logger)
		return
	}
	ctx := c.Request.Context()
	if body.FreeShippingMin != nil && (body.FreeShippingMin.IsNegative() || body.FreeShippingMin.Exponent() < -2) {
		RespondErr(c, apperrors.ValidationFailed("free_shipping_min", "must be zero or more with at most 2 decimals"), h.logger)
		return
	}
	if _, err := h.svc.ReplaceOptions(ctx, s.TenantID, s.ID, body.Options); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if err := h.svc.SetOwnFreeShippingMin(ctx, s.ID, body.FreeShippingMin); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.Get(c)
}

func (h *DeliverySettingsHandler) CreateCourier(c *gin.Context) {
	s := storeFromCtx(c)
	if s == nil {
		return
	}
	var in delivery.CourierInput
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid JSON"), h.logger)
		return
	}
	out, err := h.svc.CreateCourier(c.Request.Context(), s.TenantID, s.ID, in)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (h *DeliverySettingsHandler) UpdateCourier(c *gin.Context) {
	s := storeFromCtx(c)
	if s == nil {
		return
	}
	var in delivery.CourierInput
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid JSON"), h.logger)
		return
	}
	out, err := h.svc.UpdateCourier(c.Request.Context(), s.ID, c.Param("courierId"), in)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *DeliverySettingsHandler) DeleteCourier(c *gin.Context) {
	s := storeFromCtx(c)
	if s == nil {
		return
	}
	if err := h.svc.DeleteCourier(c.Request.Context(), s.ID, c.Param("courierId")); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}
