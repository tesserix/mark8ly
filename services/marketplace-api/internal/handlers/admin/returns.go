// Package admin — returns.go: HTTP handler for the admin returns subset
// mounted under /api/v1/admin/stores/:storeId/orders/:orderId/returns and
// /api/v1/admin/stores/:storeId/returns/:id/<action>.
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ReturnsHandler bundles dependencies for the admin returns endpoints.
type ReturnsHandler struct {
	db        *gorm.DB
	svc       *order.ReturnService
	repo      order.ReturnRepository
	orderRepo order.Repository
	orderSvc  *order.Service
	refunds   *orderrefund.Coordinator // optional — nil disables the refunded endpoint (503)
	notify    *notification.Service    // optional — nil-safe
	logger    *slog.Logger
}

// NewReturnsHandler constructs a ReturnsHandler.
func NewReturnsHandler(db *gorm.DB, svc *order.ReturnService, repo order.ReturnRepository, orderRepo order.Repository, orderSvc *order.Service, logger *slog.Logger) *ReturnsHandler {
	return &ReturnsHandler{db: db, svc: svc, repo: repo, orderRepo: orderRepo, orderSvc: orderSvc, logger: logger}
}

// WithNotifier attaches the notification service so return requests fire
// in-app notifications. Nil-safe.
func (h *ReturnsHandler) WithNotifier(n *notification.Service) *ReturnsHandler {
	h.notify = n
	return h
}

// WithRefunds attaches the refund coordinator that moves real money for
// return refunds. Nil-safe — leaving it unset makes MarkRefunded respond
// 503 rather than silently skipping the gateway.
func (h *ReturnsHandler) WithRefunds(c *orderrefund.Coordinator) *ReturnsHandler {
	h.refunds = c
	return h
}

// List handles GET /admin/stores/:storeId/returns.
func (h *ReturnsHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var rows []order.Return
	if err := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND tenant_id = ?", storeID, tenantID).
		Order("requested_at DESC").
		Limit(100).
		Find(&rows).Error; err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := make([]AdminReturnResponse, 0, len(rows))
	for i := range rows {
		_, items, err := h.repo.GetByID(c.Request.Context(), h.db, rows[i].ID)
		if err != nil {
			RespondErr(c, err, h.logger)
			return
		}
		out = append(out, ToAdminReturnResponse(&rows[i], items))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// Get handles GET /admin/stores/:storeId/returns/:id.
func (h *ReturnsHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	r, items, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if r.StoreID.String() != c.Param("storeId") {
		RespondErr(c, apperrors.NotFound("return"), h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminReturnResponse(r, items))
}

// Request handles POST /admin/stores/:storeId/orders/:id/returns.
//
// Allocates a per-store return sequence number, then calls
// ReturnService.Request which validates ordered quantities and writes the
// return + cross-module return_requested event in one tx. The :id path
// param is the order id (gin requires the same param name across the
// whole /orders subtree).
func (h *ReturnsHandler) Request(c *gin.Context) {
	storeID := c.Param("storeId")
	orderID := c.Param("id")
	tenantID := c.GetString("tenant_id")

	storeUUID, err := uuid.Parse(storeID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "must be a uuid"), h.logger)
		return
	}
	orderUUID, err := uuid.Parse(orderID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("orderId", "must be a uuid"), h.logger)
		return
	}
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "must be a uuid"), h.logger)
		return
	}

	var req CreateReturnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	storeVal, _ := c.Get("store")
	storeRow, _ := storeVal.(*stores.Store)
	prefix := storePrefixFromStore(storeRow, storeID)

	// Allocate per-store return sequence in a short tx.
	var seq int64
	if err := h.orderSvc.Unit(c.Request.Context(), func(tx *gorm.DB) error {
		n, err := order.NextDocumentNumber(c.Request.Context(), tx, storeUUID, "return")
		if err != nil {
			return err
		}
		seq = n
		return nil
	}); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Translate wire items to service items (parse OrderItemID uuid).
	items := make([]order.ReturnItem, 0, len(req.Items))
	for _, it := range req.Items {
		oid, err := uuid.Parse(it.OrderItemID)
		if err != nil {
			RespondErr(c, apperrors.ValidationFailed("items.order_item_id", "must be a uuid"), h.logger)
			return
		}
		items = append(items, order.ReturnItem{
			OrderItemID: oid,
			Quantity:    it.Quantity,
			Reason:      it.Reason,
		})
	}

	in := order.RequestInput{
		TenantID:     tenantUUID,
		StoreID:      storeUUID,
		StorePrefix:  prefix,
		ReturnSeq:    seq,
		OrderID:      orderUUID,
		Type:         req.Type,
		Reason:       req.Reason,
		Notes:        req.Notes,
		Items:        items,
		CurrencyCode: req.CurrencyCode,
	}
	r, _, err := h.svc.Request(c.Request.Context(), in)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	full, fullItems, err := h.repo.GetByID(c.Request.Context(), h.db, r.ID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	returnMsg := "A customer requested a return on order " + orderID + "."
	returnResourceType := "return"
	notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
		TenantID:     tenantUUID,
		StoreID:      storeUUID,
		Type:         notification.TypeReturnRequested,
		Title:        "Return requested",
		Message:      &returnMsg,
		ResourceType: &returnResourceType,
		ResourceID:   &full.ID,
	})
	c.JSON(http.StatusCreated, ToAdminReturnResponse(full, fullItems))
}

// Approve handles POST /admin/stores/:storeId/returns/:id/approve. The
// body may carry an optional pickup_details free-text block that the
// customer will see on their order page next to the "Request approved"
// banner (carrier, pickup window, replacement tracking, etc.).
func (h *ReturnsHandler) Approve(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	var req ApproveReturnRequest
	// Body is optional — empty POST keeps pickup_details unset.
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.Approve(c.Request.Context(), id, req.PickupDetails); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	r, items, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminReturnResponse(r, items))
}

// SetPickupDetails handles PATCH /admin/stores/:storeId/returns/:id/pickup.
// Used to refine the logistics note after approval without re-approving —
// the typical flow is: approve without details → couriers confirm slot →
// PATCH with the slot.
func (h *ReturnsHandler) SetPickupDetails(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	var req ApproveReturnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if err := h.svc.SetPickupDetails(c.Request.Context(), id, req.PickupDetails); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	r, items, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminReturnResponse(r, items))
}

// Reject handles POST /admin/stores/:storeId/returns/:id/reject.
func (h *ReturnsHandler) Reject(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	var req RejectReturnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if err := h.svc.Reject(c.Request.Context(), id, req.Reason); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	r, items, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminReturnResponse(r, items))
}

// MarkReceived handles POST /admin/stores/:storeId/returns/:id/received.
func (h *ReturnsHandler) MarkReceived(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	if err := h.svc.MarkReceived(c.Request.Context(), id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	r, items, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminReturnResponse(r, items))
}

// MarkRefunded handles POST /admin/stores/:storeId/returns/:id/refunded.
// This now moves real money — the request goes through
// orderrefund.Coordinator (ScopeID = the return id, giving each return its
// own idempotency key namespace), which resolves the order's captured
// payment, calls the gateway, and derives partial vs full payment_status
// from the amount. The caller-supplied payment_status field is ignored
// (server-derived). Once the coordinator succeeds, ReturnService.StampRefundedOnly
// stamps the return as refunded — the coordinator's own tx already bumped
// orders.refunded_amount, so this second step only owns the return
// aggregate's state.
//
// Self-heal: Coordinator.Refund is idempotent on ScopeID, so if
// StampRefundedOnly fails after the money moved, retrying this endpoint
// re-runs Coordinator.Refund as a no-op (AlreadyDone=true) and completes
// the stamp on retry.
func (h *ReturnsHandler) MarkRefunded(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	var req MarkRefundedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if h.refunds == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "service_unavailable",
			"message": "Refunds are not configured.",
		})
		return
	}

	r, _, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	amount := req.Amount
	if _, rerr := h.refunds.Refund(c.Request.Context(), orderrefund.RefundCommand{
		OrderID: r.OrderID,
		Amount:  &amount,
		Reason:  req.Reason,
		Actor:   "user:" + c.GetString("user_id"),
		ScopeID: id.String(),
	}); rerr != nil {
		RespondErr(c, rerr, h.logger)
		return
	}

	if err := h.svc.StampRefundedOnly(c.Request.Context(), id, req.Amount, req.Reason); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	full, items, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminReturnResponse(full, items))
}
