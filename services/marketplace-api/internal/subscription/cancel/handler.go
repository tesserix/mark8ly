package cancel

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Handler serves POST /admin/stores/:storeId/subscription/cancel.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

type cancelRequest struct {
	// SurveyReason is the optional exit-survey reason from the merchant.
	SurveyReason string `json:"survey_reason"`
	// AcceptSaveOffer true triggers the save-offer reversal branch:
	// cancel_scheduled → active (prospective discount deferred to P10).
	AcceptSaveOffer bool `json:"accept_save_offer"`
}

// Cancel handles POST /admin/stores/:storeId/subscription/cancel.
func (h *Handler) Cancel(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		admin.RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		admin.RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req cancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		admin.RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	actor := "user:" + c.GetString("user_id")

	out, err := h.svc.Cancel(c.Request.Context(), Input{
		TenantID:        tenantID,
		StoreID:         storeID,
		Actor:           actor,
		SurveyReason:    req.SurveyReason,
		AcceptSaveOffer: req.AcceptSaveOffer,
	})
	if err != nil {
		mapCancelErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, out)
}

func mapCancelErr(c *gin.Context, err error, logger *slog.Logger) {
	switch {
	case errors.Is(err, ErrNotCancellable):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "not_cancellable", "message": err.Error()})
	case errors.Is(err, ErrSaveOfferAlreadyAccepted):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "save_offer_already_accepted"})
	default:
		logger.Error("cancel handler: unexpected error", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
	}
}
