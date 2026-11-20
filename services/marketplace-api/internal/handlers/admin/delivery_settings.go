package admin

import (
	"encoding/json"
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

// PutOptions replaces the option list and free-shipping minimum atomically.
// Both keys are required so a client cannot wipe either by omission: options
// may be an empty array, free_shipping_min may be an explicit null (clears it).
func (h *DeliverySettingsHandler) PutOptions(c *gin.Context) {
	s := storeFromCtx(c)
	if s == nil {
		return
	}
	var body struct {
		Options         []delivery.OptionInput `json:"options"`
		FreeShippingMin json.RawMessage        `json:"free_shipping_min"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid JSON"), h.logger)
		return
	}
	if body.Options == nil {
		RespondErr(c, apperrors.ValidationFailed("options", "required (use [] for none)"), h.logger)
		return
	}
	if body.FreeShippingMin == nil {
		RespondErr(c, apperrors.ValidationFailed("free_shipping_min", "required (use null for none)"), h.logger)
		return
	}
	var freeMin *decimal.Decimal
	if string(body.FreeShippingMin) != "null" {
		var d decimal.Decimal
		if err := d.UnmarshalJSON(body.FreeShippingMin); err != nil {
			RespondErr(c, apperrors.ValidationFailed("free_shipping_min", "must be a number"), h.logger)
			return
		}
		freeMin = &d
	}
	if _, err := h.svc.ReplaceOptions(c.Request.Context(), s.TenantID, s.ID, body.Options, freeMin); err != nil {
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
