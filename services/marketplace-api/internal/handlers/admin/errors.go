// Package admin holds the marketplace-api admin HTTP handlers.
package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// codeStatus maps every typed error code to its HTTP status. The set is
// closed; adding a new code requires updating this map.
var codeStatus = map[apperrors.Code]int{
	apperrors.CodeValidationFailed:        http.StatusBadRequest,
	apperrors.CodeVariantMatrixMismatch:   http.StatusBadRequest,
	apperrors.CodeTooManyOptions:          http.StatusBadRequest,
	apperrors.CodeTooManyVariants:         http.StatusBadRequest,
	apperrors.CodeCurrencyMismatch:        http.StatusBadRequest,
	apperrors.CodeTargetStoreInvalid:      http.StatusBadRequest,
	apperrors.CodeUploadNotFound:          http.StatusBadRequest,
	apperrors.CodeForbidden:               http.StatusForbidden,
	apperrors.CodeNotFound:                http.StatusNotFound,
	apperrors.CodeHandleTaken:             http.StatusConflict,
	apperrors.CodeSKUTaken:                http.StatusConflict,
	apperrors.CodeSlugTaken:               http.StatusConflict,
	apperrors.CodeCategoryNotEmpty:        http.StatusConflict,
	apperrors.CodeCategoryHasChildren:     http.StatusConflict,
	apperrors.CodeCurrencyChangeForbidden: http.StatusConflict,
	apperrors.CodePayloadTooLarge:         http.StatusRequestEntityTooLarge,
	apperrors.CodeUnsupportedMediaType:    http.StatusUnsupportedMediaType,
	apperrors.CodeRateLimited:             http.StatusTooManyRequests,

	// Orders slice 1.
	apperrors.CodeInvalidTransition:        http.StatusConflict,
	apperrors.CodeRefundExceedsTotal:       http.StatusUnprocessableEntity,
	apperrors.CodeIdempotencyConflict:      http.StatusConflict,
	apperrors.CodeReturnItemsExceedOrdered: http.StatusUnprocessableEntity,
	apperrors.CodeRecoveryTooRecent:        http.StatusTooManyRequests,

	// Coupons M1.
	apperrors.CodeCouponNotFound:          http.StatusNotFound,
	apperrors.CodeCouponExpired:           http.StatusUnprocessableEntity,
	apperrors.CodeCouponUsageLimitReached: http.StatusUnprocessableEntity,
	apperrors.CodeCouponInvalid:           http.StatusUnprocessableEntity,
	apperrors.CodeCouponMinPurchaseNotMet: http.StatusUnprocessableEntity,

	// Gift cards — Marketing M2.
	apperrors.CodeInsufficientGiftCardBalance: http.StatusUnprocessableEntity,
	apperrors.CodeGiftCardExpired:             http.StatusGone,
	apperrors.CodeGiftCardNotFound:            http.StatusNotFound,
}

// RespondErr writes the standard error envelope for the given error.
// Typed errors (*apperrors.Error) render with their code, message, and
// details. Untyped errors render as a generic 500 with the actual error
// stack logged via slog.
func RespondErr(c *gin.Context, err error, logger *slog.Logger) {
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		status, ok := codeStatus[ae.Code]
		if !ok {
			status = http.StatusInternalServerError
		}
		c.AbortWithStatusJSON(status, envelope(string(ae.Code), ae.Message, ae.Details))
		return
	}
	if logger != nil {
		logger.Error("unhandled handler error", "err", err.Error())
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError,
		envelope("internal", "internal server error", nil))
}

func envelope(code, msg string, details map[string]any) map[string]any {
	out := map[string]any{"error": code, "message": msg}
	if len(details) > 0 {
		out["details"] = details
	}
	return out
}
