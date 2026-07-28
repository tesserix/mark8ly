package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// GiftCardHandler handles admin gift card endpoints.
type GiftCardHandler struct {
	svc    *giftcard.Service
	audit  *audit.Emitter // optional — nil-safe
	logger *slog.Logger
}

// NewGiftCardHandler constructs a GiftCardHandler.
func NewGiftCardHandler(svc *giftcard.Service, logger *slog.Logger) *GiftCardHandler {
	return &GiftCardHandler{svc: svc, logger: logger}
}

// WithAudit attaches an audit emitter so gift card lifecycle events show
// up in Settings -> Audit Logs. Nil-safe.
func (h *GiftCardHandler) WithAudit(e *audit.Emitter) *GiftCardHandler {
	h.audit = e
	return h
}

// List handles GET /admin/stores/:storeId/gift-cards.
func (h *GiftCardHandler) List(c *gin.Context) {
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

	var status *giftcard.GiftCardStatus
	if s := c.Query("status"); s != "" {
		st := giftcard.GiftCardStatus(s)
		status = &st
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	cards, total, err := h.svc.ListByStore(c.Request.Context(), storeID, tenantID, status, page, pageSize)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]AdminGiftCardResponse, 0, len(cards))
	for i := range cards {
		out = append(out, toAdminGiftCardResponse(&cards[i]))
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// Issue handles POST /admin/stores/:storeId/gift-cards.
func (h *GiftCardHandler) Issue(c *gin.Context) {
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

	var req IssueGiftCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	gc, err := h.svc.Issue(c.Request.Context(), giftcard.IssueInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		InitialBalance: req.InitialBalance,
		CurrencyCode:   req.CurrencyCode,
		SenderName:     req.SenderName,
		SenderEmail:    req.SenderEmail,
		RecipientName:  req.RecipientName,
		RecipientEmail: req.RecipientEmail,
		Message:        req.Message,
		ExpiresAt:      req.ExpiresAt,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "gift_card.issued",
		ResourceType: "gift_card",
		ResourceID:   gc.ID.String(),
		Metadata: map[string]any{
			"initial_balance": req.InitialBalance,
			"currency":        req.CurrencyCode,
			"recipient_email": req.RecipientEmail,
		},
	})
	c.JSON(http.StatusCreated, gin.H{"data": toAdminGiftCardResponse(gc)})
}

// Get handles GET /admin/stores/:storeId/gift-cards/:id.
func (h *GiftCardHandler) Get(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	gc, txns, err := h.svc.GetByID(c.Request.Context(), storeID, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	txnResponses := make([]AdminGiftCardTxnResponse, 0, len(txns))
	for i := range txns {
		txnResponses = append(txnResponses, toAdminGiftCardTxnResponse(&txns[i]))
	}

	resp := AdminGiftCardDetailResponse{
		AdminGiftCardResponse: toAdminGiftCardResponse(gc),
		Transactions:          txnResponses,
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// setStatus is the shared preamble + write path for Disable/Enable — keeps
// each public method tiny, matching the existing Issue/Get preamble style.
func (h *GiftCardHandler) setStatus(c *gin.Context, to giftcard.GiftCardStatus, action string) {
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
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	gc, err := h.svc.SetStatus(c.Request.Context(), storeID, tenantID, id, to)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "gift_card." + action,
		ResourceType: "gift_card",
		ResourceID:   id.String(),
		Metadata: map[string]any{
			"to_status": string(to),
		},
	})
	c.JSON(http.StatusOK, gin.H{"data": toAdminGiftCardResponse(gc)})
}

// Disable handles POST /admin/stores/:storeId/gift-cards/:id/disable.
// Freezes the card's balance — it is not refunded or destroyed. Enable
// restores the exact same balance.
func (h *GiftCardHandler) Disable(c *gin.Context) {
	h.setStatus(c, giftcard.StatusDisabled, "disabled")
}

// Enable handles POST /admin/stores/:storeId/gift-cards/:id/enable.
// Restores full spendability of a previously disabled card's existing
// balance — no value was ever removed while it was disabled.
func (h *GiftCardHandler) Enable(c *gin.Context) {
	h.setStatus(c, giftcard.StatusActive, "enabled")
}
