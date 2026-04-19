// Package admin — refund.go: HTTP handler for the 14-day cooling-off refund (§8).
// POST /admin/stores/:storeId/subscription/refund
package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/refund"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// RefundHandler exposes the §8 cooling-off refund endpoint.
type RefundHandler struct {
	db      *gorm.DB
	svc     *refund.Service
	emitter *audit.Emitter
	logger  *slog.Logger
}

// NewRefundHandler constructs a RefundHandler.
func NewRefundHandler(db *gorm.DB, svc *refund.Service, logger *slog.Logger) *RefundHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RefundHandler{db: db, svc: svc, logger: logger}
}

// WithAudit wires the audit emitter into the handler.
func (h *RefundHandler) WithAudit(e *audit.Emitter) *RefundHandler {
	h.emitter = e
	return h
}

type issueRefundRequest struct {
	// StripeChargeID is the charge to validate the 14-day gate against.
	StripeChargeID string `json:"stripe_charge_id" binding:"required"`
	// StripeSubscriptionID is the Stripe subscription that will be cancelled
	// after a successful refund. Optional — passed through to audit.
	StripeSubscriptionID string `json:"stripe_subscription_id"`
	// Reason is a free-text merchant explanation for the refund.
	Reason string `json:"reason"`
}

type issueRefundResponse struct {
	StripeRefundID  string `json:"stripe_refund_id"`
	AmountMinor     int64  `json:"amount_minor"`
	Currency        string `json:"currency"`
	CardFingerprint string `json:"card_fingerprint,omitempty"`
}

// IssueRefund handles POST /admin/stores/:storeId/subscription/refund.
//
// Authorization: SubscriptionEditRole (owner-tier) — financial operation.
//
// HTTP responses:
//   - 200 OK          — refund issued successfully.
//   - 422 Unprocessable Entity — outside 14-day window, card fingerprint already
//     refunded, or Pro+App store (non-refundable). Error codes:
//     "refund_outside_cooling_off", "refund_card_fingerprint_exists",
//     "refund_pro_app_not_refundable".
func (h *RefundHandler) IssueRefund(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req issueRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	actor := "user:" + c.GetString("user_id")
	deviceFP := c.GetHeader("X-Device-Fingerprint")

	out, issueErr := h.svc.IssueRefund(c.Request.Context(), refund.IssueInput{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeChargeID:       req.StripeChargeID,
		StripeSubscriptionID: req.StripeSubscriptionID,
		Reason:               req.Reason,
		DeviceFingerprint:    deviceFP,
		Actor:                actor,
	})

	// Emit audit regardless of outcome (§23.1).
	if h.emitter != nil {
		accepted := issueErr == nil
		h.emitter.EmitRefundIssued(c, audit.RefundIssued{
			TenantID:       tenantID,
			StoreID:        storeID,
			StripeChargeID: req.StripeChargeID,
			Actor:          actor,
			Accepted:       accepted,
		})
	}

	if issueErr != nil {
		switch {
		case errors.Is(issueErr, refund.ErrOutsideCoolingOff):
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "refund_outside_cooling_off",
				"message": "the 14-day cooling-off window has closed",
			})
		case errors.Is(issueErr, refund.ErrCardFingerprintExists):
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "refund_unavailable",
				"message": "refund is not available for this payment method",
			})
		case errors.Is(issueErr, refund.ErrProAppNotRefundable):
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "refund_pro_app_not_refundable",
				"message": "Pro+App setup fee is not refundable",
			})
		default:
			RespondErr(c, issueErr, h.logger)
		}
		return
	}

	c.JSON(http.StatusOK, issueRefundResponse{
		StripeRefundID:  out.StripeRefundID,
		AmountMinor:     out.AmountMinor,
		Currency:        out.Currency,
		CardFingerprint: out.CardFingerprint,
	})
}
