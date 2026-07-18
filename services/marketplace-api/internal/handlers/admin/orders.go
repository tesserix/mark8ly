// Package admin — orders.go: HTTP handler for the admin orders subset
// mounted under /api/v1/admin/stores/:storeId/orders. Each method
// extracts path params + auth context, binds the wire DTO, invokes the
// order service, and renders via ToAdminOrderResponse or RespondErr.
package admin

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tax"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// OrdersHandler bundles dependencies for the admin order endpoints.
type OrdersHandler struct {
	db        *gorm.DB
	svc       *order.Service
	repo      order.Repository
	docMailer *orderdoc.Service        // optional — nil disables auto-emails
	audit     *audit.Emitter           // optional — nil-safe; Emit is no-op when nil
	notify    *notification.Service    // optional — nil-safe; notification.Emit skips when nil
	refunds   *orderrefund.Coordinator // optional — nil disables the refund endpoint (503)
	// cancelForOrderFn, when set, cancels the order's shipments at the carrier
	// (best-effort). Wired from shipmentcancel.Executor.CancelForOrder via
	// WithShipmentCanceller. A func seam keeps the handler unit-testable.
	cancelForOrderFn func(ctx context.Context, orderID uuid.UUID)
	logger           *slog.Logger
}

// NewOrdersHandler constructs an OrdersHandler. docMailer is optional;
// when nil, lifecycle emails are skipped (useful for tests + early
// boot ordering when the mailer hasn't been wired yet).
func NewOrdersHandler(db *gorm.DB, svc *order.Service, repo order.Repository, docMailer *orderdoc.Service, logger *slog.Logger) *OrdersHandler {
	return &OrdersHandler{db: db, svc: svc, repo: repo, docMailer: docMailer, logger: logger}
}

// WithAudit attaches an audit emitter so order lifecycle events show up
// in Settings -> Audit Logs. Nil-safe — handlers without an emitter
// still work; they just don't record audit rows.
func (h *OrdersHandler) WithAudit(e *audit.Emitter) *OrdersHandler {
	h.audit = e
	return h
}

// WithNotifier attaches the notification service so order lifecycle
// events fire in-app notifications (respecting merchant preferences).
// Nil-safe — notification.Emit is a no-op without a service.
func (h *OrdersHandler) WithNotifier(n *notification.Service) *OrdersHandler {
	h.notify = n
	return h
}

// WithRefunds attaches the refund coordinator that moves real money.
// Nil-safe by omission — without it, Refund returns 503 rather than
// silently no-opping, since a refund request that appears to succeed
// without touching the gateway would be a correctness bug, not a
// degraded-mode fallback.
func (h *OrdersHandler) WithRefunds(c *orderrefund.Coordinator) *OrdersHandler {
	h.refunds = c
	return h
}

// dispatchInvoiceEmail fires the invoice email on a detached background
// context — the merchant's confirm response should never block on
// SendGrid latency or fail because email dispatch failed.
func (h *OrdersHandler) dispatchInvoiceEmail(orderID uuid.UUID) {
	if h.docMailer == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.docMailer.SendInvoice(ctx, orderID); err != nil {
			h.logger.Warn("orderdoc: invoice email dispatch failed",
				"order_id", orderID, "err", err)
		}
	}()
}

// dispatchCancellationEmail fires the cancellation notification on a
// detached context. Mirrors dispatchInvoiceEmail's never-block contract.
func (h *OrdersHandler) dispatchCancellationEmail(orderID uuid.UUID, reason string, byCustomer bool) {
	if h.docMailer == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.docMailer.SendCancellation(ctx, orderID, reason, byCustomer); err != nil {
			h.logger.Warn("orderdoc: cancellation email dispatch failed",
				"order_id", orderID, "err", err)
		}
	}()
}

// dispatchRefundEmail fires the refund-issued notification on a
// detached context.
func (h *OrdersHandler) dispatchRefundEmail(orderID uuid.UUID, refundAmount, totalRefundedAfter decimal.Decimal) {
	if h.docMailer == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.docMailer.SendRefund(ctx, orderID, refundAmount, totalRefundedAfter); err != nil {
			h.logger.Warn("orderdoc: refund email dispatch failed",
				"order_id", orderID, "err", err)
		}
	}()
}

// WithShipmentCanceller wires the best-effort carrier shipment-cancel for the
// non-paid cancel path (paid cancels reach the carrier via the auto-refund →
// coordinator hook). Nil-safe by omission.
func (h *OrdersHandler) WithShipmentCanceller(fn func(ctx context.Context, orderID uuid.UUID)) *OrdersHandler {
	h.cancelForOrderFn = fn
	return h
}

// cancelShipmentsForNonPaid fires the direct shipment cancel only when the
// order did NOT go through the auto-refund path (paid orders). This is the
// single trigger site for cancels, so there's no double-hit with the
// coordinator's post-refund hook.
func (h *OrdersHandler) cancelShipmentsForNonPaid(ctx context.Context, orderID uuid.UUID, paymentStatus string) {
	if h.cancelForOrderFn == nil || paymentStatus == string(order.PaymentStatusPaid) {
		return
	}
	h.cancelForOrderFn(ctx, orderID)
}

// List handles GET /admin/stores/:storeId/orders.
//
// Pagination is offset-based (page/page_size) to match products. The
// service layer doesn't yet expose a List method, so this handler runs
// the SELECT directly via the shared *gorm.DB — acceptable because
// orders pagination is read-only and has no side effects.
func (h *OrdersHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var q ListOrdersQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		RespondErr(c, apperrors.ValidationFailed("query", err.Error()), h.logger)
		return
	}
	q.Defaults()

	tx := h.db.WithContext(c.Request.Context()).
		Model(&order.Order{}).
		Where("store_id = ? AND tenant_id = ? AND deleted_at IS NULL", storeID, tenantID)
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.PaymentStatus != "" {
		tx = tx.Where("payment_status = ?", q.PaymentStatus)
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		// Partial-match across the columns an operator typically
		// remembers. ILIKE means the query is case-insensitive and a
		// few typed characters produce useful narrowing without
		// requiring a specific format.
		like := "%" + s + "%"
		tx = tx.Where(
			"order_number ILIKE ? OR customer_name ILIKE ? OR customer_email ILIKE ?",
			like, like, like,
		)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	var rows []order.Order
	if err := tx.
		Order("placed_at DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// H8 fix: batch-load items + addresses in 2 queries instead of 2*N.
	orderIDs := make([]uuid.UUID, len(rows))
	for i := range rows {
		orderIDs[i] = rows[i].ID
	}
	itemsByOrder, addrsByOrder, err := order.LoadChildrenBatch(c.Request.Context(), h.db, orderIDs)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	resp := make([]AdminOrderResponse, 0, len(rows))
	for i := range rows {
		resp = append(resp, ToAdminOrderResponse(&rows[i], itemsByOrder[rows[i].ID], addrsByOrder[rows[i].ID]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": resp,
		"meta": gin.H{
			"page":        q.Page,
			"page_size":   q.PageSize,
			"total":       total,
			"total_pages": ceilDiv(total, int64(q.PageSize)),
		},
	})
}

// Get handles GET /admin/stores/:storeId/orders/:id.
func (h *OrdersHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	o, items, addrs, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if o.StoreID.String() != c.Param("storeId") {
		RespondErr(c, apperrors.NotFound("order"), h.logger)
		return
	}
	resp := ToAdminOrderResponse(o, items, addrs)
	// Hydrate the per-jurisdiction tax breakdown so the invoice/receipt
	// PDFs (which fetch via this endpoint) can render CGST/SGST/IGST/VAT
	// lines instead of a single aggregate tax_total. Empty for orders
	// placed before the persistence wiring landed — the PDF falls back
	// to the single Tax line in that case.
	taxRepo := tax.NewRepository()
	if rows, err := taxRepo.GetTaxLines(c.Request.Context(), h.db, id); err == nil && len(rows) > 0 {
		out := make([]AdminOrderTaxLineResponse, 0, len(rows))
		for _, r := range rows {
			out = append(out, AdminOrderTaxLineResponse{
				Description:  r.Description,
				Rate:         r.Rate,
				Amount:       r.Amount,
				Jurisdiction: r.Jurisdiction,
			})
		}
		resp.TaxLines = out
	}
	c.JSON(http.StatusOK, resp)
}

// Create handles POST /admin/stores/:storeId/orders. Admin-side order
// creation is rare; storefront checkout uses a separate path in M5.
//
// Allocates a per-store sequence number inside the same Unit() that
// runs the create, so the human order number is consistent with the DB.
func (h *OrdersHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	storeUUID, err := uuid.Parse(storeID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "must be a uuid"), h.logger)
		return
	}
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "must be a uuid"), h.logger)
		return
	}
	storeVal, _ := c.Get("store")
	storeRow, _ := storeVal.(*stores.Store)
	prefix := storePrefixFromStore(storeRow, storeID)

	// Sequence allocation happens inside Service.Create's transaction
	// (C6 fix: atomic with order insert to prevent burned numbers).
	in := order.CreateInput{
		TenantID:       tenantUUID,
		StoreID:        storeUUID,
		StorePrefix:    prefix,
		OrderNumberSeq: 0, // allocated inside Create tx
		IdempotencyKey: req.IdempotencyKey,
		CustomerEmail:  req.CustomerEmail,
		CustomerName:   req.CustomerName,
		Items:          toServiceItems(req.Items),
		Shipping:       toServiceAddress(req.Shipping),
		Billing:        toServiceAddress(req.Billing),
		Subtotal:       req.Subtotal,
		ShippingTotal:  req.ShippingTotal,
		TaxTotal:       req.TaxTotal,
		DiscountTotal:  req.DiscountTotal,
		GrandTotal:     req.GrandTotal,
		CurrencyCode:   req.CurrencyCode,
		Notes:          req.Notes,
	}
	if req.CustomerID != nil {
		if cid, err := uuid.Parse(*req.CustomerID); err == nil {
			in.CustomerID = &cid
		}
	}
	result, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
	}
	if !result.Reused {
		h.audit.Emit(c, audit.Event{
			Action:       "order.created",
			ResourceType: "order",
			ResourceID:   result.Order.ID.String(),
			Metadata: map[string]any{
				"order_number": result.Order.OrderNumber,
				"grand_total":  result.Order.GrandTotal,
				"currency":     result.Order.CurrencyCode,
			},
		})
	}
	c.JSON(status, ToAdminOrderResponse(result.Order, result.Items, result.Addrs))
}

// Confirm handles POST /admin/stores/:storeId/orders/:id/confirm.
func (h *OrdersHandler) Confirm(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	// H7 fix: verify the order belongs to this store + tenant.
	if err := h.verifyOrderOwnership(c, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	var req ConfirmOrderRequest
	_ = c.ShouldBindJSON(&req) // body is optional

	var paymentTarget *order.PaymentStatus
	if req.PaymentStatus != nil {
		ps := order.PaymentStatus(*req.PaymentStatus)
		paymentTarget = &ps
	}
	if err := h.svc.Confirm(c.Request.Context(), nil, id, paymentTarget, req.Reason); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.audit.Emit(c, audit.Event{
		Action:       "order.confirmed",
		ResourceType: "order",
		ResourceID:   id.String(),
		Metadata:     map[string]any{"reason": req.Reason},
	})
	// Order accepted — fire the invoice email. Detached so a SendGrid
	// outage never blocks the confirm response.
	h.dispatchInvoiceEmail(id)
	o, items, addrs, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminOrderResponse(o, items, addrs))
}

// MarkFulfilled handles POST /admin/stores/:storeId/orders/:id/fulfill.
func (h *OrdersHandler) MarkFulfilled(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	if err := h.verifyOrderOwnership(c, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if err := h.svc.MarkFulfilled(c.Request.Context(), nil, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.audit.Emit(c, audit.Event{
		Action:       "order.fulfilled",
		ResourceType: "order",
		ResourceID:   id.String(),
	})
	o, items, addrs, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminOrderResponse(o, items, addrs))
}

// Cancel handles POST /admin/stores/:storeId/orders/:id/cancel.
func (h *OrdersHandler) Cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	if err := h.verifyOrderOwnership(c, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	var req CancelOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), nil, id, req.Reason); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.audit.Emit(c, audit.Event{
		Action:       "order.cancelled",
		ResourceType: "order",
		ResourceID:   id.String(),
		Severity:     audit.SeverityWarning,
		Metadata:     map[string]any{"reason": req.Reason},
	})
	// Cancellation succeeded — fire the customer email. Detached so a
	// SendGrid blip never blocks the cancel response.
	h.dispatchCancellationEmail(id, req.Reason, false)
	o, items, addrs, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	// Auto-refund a paid order on cancellation (spec §4). Best-effort: a
	// gateway blip leaves the order cancelled + a pending ledger row for the
	// sweeper, so the merchant is never blocked on the cancel response.
	if h.refunds != nil && o.PaymentStatus == string(order.PaymentStatusPaid) {
		if _, rerr := h.refunds.Refund(c.Request.Context(), orderrefund.RefundCommand{
			OrderID: id, Amount: nil, Reason: "order cancelled",
			Actor: "user:" + c.GetString("user_id"), ScopeID: "cancel",
		}); rerr != nil {
			h.logger.Warn("cancel auto-refund deferred", "order_id", id, "err", rerr)
		}
	}
	// Cancel the order's shipment at the carrier. Paid orders reach the carrier
	// through the auto-refund → coordinator hook above; only the non-paid path
	// needs a direct call here. Detached + best-effort: never blocks or fails
	// the cancel response.
	{
		oid := id
		ps := o.PaymentStatus
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.cancelShipmentsForNonPaid(ctx, oid, ps)
		}()
	}
	cancelMsg := "Order " + o.OrderNumber + " was cancelled."
	notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
		TenantID:     o.TenantID,
		StoreID:      o.StoreID,
		Type:         notification.TypeOrderCancelled,
		Title:        "Order cancelled",
		Message:      &cancelMsg,
		ResourceType: stringPtr("order"),
		ResourceID:   &o.ID,
	})
	c.JSON(http.StatusOK, ToAdminOrderResponse(o, items, addrs))
}

func stringPtr(s string) *string { return &s }

// Refund handles POST /admin/stores/:storeId/orders/:id/refund. This now
// moves real money — the request goes through orderrefund.Coordinator,
// which resolves the captured payment, calls the gateway, and derives
// whether the result is a partial or full refund from the amount. The
// caller-supplied payment_status field is ignored (see RefundOrderRequest).
func (h *OrdersHandler) Refund(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	if err := h.verifyOrderOwnership(c, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	var req RefundOrderRequest
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
	if req.RefundRequestID == "" {
		RespondErr(c, apperrors.ValidationFailed("refund_request_id", "refund_request_id is required (stable idempotency key per refund attempt)"), h.logger)
		return
	}
	res, rerr := h.refunds.Refund(c.Request.Context(), orderrefund.RefundCommand{
		OrderID: id,
		Amount:  req.Amount,
		Reason:  req.Reason,
		Actor:   "user:" + c.GetString("user_id"),
		ScopeID: req.RefundRequestID,
	})
	if rerr != nil {
		RespondErr(c, rerr, h.logger)
		return
	}
	o, items, addrs, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	h.audit.Emit(c, audit.Event{
		Action:       "order.refunded",
		ResourceType: "order",
		ResourceID:   id.String(),
		Severity:     audit.SeverityWarning,
		Metadata: map[string]any{
			"refund_amount":      res.Amount,
			"refunded_total":     o.RefundedAmount,
			"payment_status":     res.PaymentStatus,
			"provider_refund_id": res.ProviderRefundID,
			"reason":             req.Reason,
		},
	})
	// Refund recorded — fire the customer email with this refund's
	// amount + the running total. The post-refund order row already
	// reflects the new refunded_amount.
	h.dispatchRefundEmail(id, res.Amount, o.RefundedAmount)
	c.JSON(http.StatusOK, RefundOrderResponse{
		AdminOrderResponse: ToAdminOrderResponse(o, items, addrs),
		ProviderRefundID:   res.ProviderRefundID,
		AlreadyDone:        res.AlreadyDone,
	})
}

// EmailInvoice handles POST /admin/stores/:storeId/orders/:id/invoice/email
// — re-sends the invoice email on demand (e.g. customer lost the original).
func (h *OrdersHandler) EmailInvoice(c *gin.Context) {
	h.emailDocument(c, false)
}

// EmailReceipt handles POST /admin/stores/:storeId/orders/:id/receipt/email.
// Receipts are gated on shipment delivery — sending one before then is a
// 409 because the document doesn't exist yet.
func (h *OrdersHandler) EmailReceipt(c *gin.Context) {
	h.emailDocument(c, true)
}

// emailDocumentRequest is the optional body for the resend endpoints.
// `note` is a short personal message the merchant can attach on resend
// — rendered as a "Note from {store}" block in the email body. Empty
// string suppresses the block, matching the canonical template.
type emailDocumentRequest struct {
	Note string `json:"note"`
}

func (h *OrdersHandler) emailDocument(c *gin.Context, asReceipt bool) {
	if h.docMailer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "mailer_unavailable",
			"message": "Email dispatch is not configured for this deployment.",
		})
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	if err := h.verifyOrderOwnership(c, id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Optional body — admin can attach a note on resend. Body is
	// optional so an empty POST keeps working unchanged.
	var req emailDocumentRequest
	_ = c.ShouldBindJSON(&req)
	note := strings.TrimSpace(req.Note)
	// Cap the note length so a runaway paste doesn't blow the email
	// body envelope. 800 chars is plenty for a "thanks for shopping
	// again" or "we've waived the shipping fee" message.
	const maxNote = 800
	if len(note) > maxNote {
		note = note[:maxNote]
	}

	if asReceipt {
		// Don't email a receipt for an order that hasn't been delivered.
		// Look up the latest shipment and bail if it isn't delivered yet.
		var status string
		row := h.db.WithContext(c.Request.Context()).
			Table("shipments").
			Select("status").
			Where("order_id = ?", id).
			Order("created_at DESC").
			Limit(1).
			Row()
		if err := row.Scan(&status); err != nil || status != "delivered" {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "not_delivered",
				"message": "Receipt is issued only after the shipment has been delivered.",
			})
			return
		}
	}

	o, _, _, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil || o == nil {
		RespondErr(c, apperrors.NotFound("order"), h.logger)
		return
	}

	opts := orderdoc.SendOptions{AdminNote: note}
	var sendErr error
	if asReceipt {
		sendErr = h.docMailer.SendReceiptWithOptions(c.Request.Context(), id, opts)
	} else {
		sendErr = h.docMailer.SendInvoiceWithOptions(c.Request.Context(), id, opts)
	}
	if sendErr != nil {
		h.logger.Warn("orderdoc: manual resend failed",
			"order_id", id, "receipt", asReceipt, "err", sendErr)
		// Missing customer email gets a 422 with an actionable hint
		// instead of a generic 502 — the merchant can fix it (set the
		// email on the order) and retry, whereas a true gateway 502 is
		// transient and they should retry as-is.
		if orderdoc.IsMissingCustomerEmail(sendErr) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "no_recipient",
				"message": "This order has no customer email on file. Add one before resending the document.",
			})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "send_failed",
			"message": sendErr.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true, "recipient": o.CustomerEmail})
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

// storePrefixFromStore derives the 3-char order number prefix from a Store.
// Falls back to a slice of the storeID hex if no store row is on context.
func storePrefixFromStore(s *stores.Store, fallback string) string {
	if s != nil && s.Slug != "" {
		slug := strings.ToUpper(s.Slug)
		// Take alphanumeric chars only.
		out := make([]byte, 0, 3)
		for i := 0; i < len(slug) && len(out) < 3; i++ {
			c := slug[i]
			if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				out = append(out, c)
			}
		}
		if len(out) == 3 {
			return string(out)
		}
	}
	// Fallback: first 3 hex chars of the store ID.
	stripped := strings.ReplaceAll(fallback, "-", "")
	if len(stripped) >= 3 {
		return strings.ToUpper(stripped[:3])
	}
	return "STR"
}

func toServiceItems(items []CreateOrderItemRequest) []order.OrderItem {
	out := make([]order.OrderItem, 0, len(items))
	for _, it := range items {
		row := order.OrderItem{
			TitleSnapshot: it.TitleSnapshot,
			SKUSnapshot:   it.SKUSnapshot,
			OptionSummary: it.OptionSummary,
			UnitPrice:     it.UnitPrice,
			Quantity:      it.Quantity,
			LineTotal:     it.LineTotal,
			CurrencyCode:  it.CurrencyCode,
			ImageURL:      it.ImageURL,
		}
		if it.ProductID != nil {
			if pid, err := uuid.Parse(*it.ProductID); err == nil {
				row.ProductID = &pid
			}
		}
		if it.VariantID != nil {
			if vid, err := uuid.Parse(*it.VariantID); err == nil {
				row.VariantID = &vid
			}
		}
		out = append(out, row)
	}
	return out
}

func toServiceAddress(a AddressRequest) order.OrderAddress {
	return order.OrderAddress{
		Name:        a.Name,
		Line1:       a.Line1,
		Line2:       a.Line2,
		City:        a.City,
		Region:      a.Region,
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
		Phone:       a.Phone,
	}
}

// verifyOrderOwnership checks that the order belongs to the store and tenant
// from the request path/context, preventing cross-tenant mutations (H7 fix).
func (h *OrdersHandler) verifyOrderOwnership(c *gin.Context, orderID uuid.UUID) error {
	o, _, _, err := h.repo.GetByID(c.Request.Context(), h.db, orderID)
	if err != nil {
		return err
	}
	if o.StoreID.String() != c.Param("storeId") {
		return apperrors.NotFound("order")
	}
	tenantID := c.GetString("tenant_id")
	if tenantID != "" && o.TenantID.String() != tenantID {
		return apperrors.NotFound("order")
	}
	return nil
}
