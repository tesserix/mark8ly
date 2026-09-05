package cancel

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
	// cancel_scheduled → active, with a best-effort prospective discount.
	AcceptSaveOffer bool `json:"accept_save_offer"`
}

// Cancel handles POST /admin/stores/:storeId/subscription/cancel.
func (h *Handler) Cancel(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		respondValidationErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"))
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		respondValidationErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"))
		return
	}

	var req cancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondValidationErr(c, apperrors.ValidationFailed("body", err.Error()))
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

// respondValidationErr writes a 422 JSON error for input validation failures.
// Mirrors the pattern in internal/handlers/admin/errors.go without importing
// that package (would create an import cycle via admin/routes.go → cancel).
func respondValidationErr(c *gin.Context, err error) {
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
			"error":   string(ae.Code),
			"message": ae.Message,
		})
		return
	}
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": "validation_failed"})
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
