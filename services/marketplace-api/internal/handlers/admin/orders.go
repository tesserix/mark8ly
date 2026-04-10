// Package admin — orders.go: HTTP handler for the admin orders subset
// mounted under /api/v1/admin/stores/:storeId/orders. Each method
// extracts path params + auth context, binds the wire DTO, invokes the
// order service, and renders via ToAdminOrderResponse or RespondErr.
package admin

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// OrdersHandler bundles dependencies for the admin order endpoints.
type OrdersHandler struct {
	db     *gorm.DB
	svc    *order.Service
	repo   order.Repository
	logger *slog.Logger
}

// NewOrdersHandler constructs an OrdersHandler.
func NewOrdersHandler(db *gorm.DB, svc *order.Service, repo order.Repository, logger *slog.Logger) *OrdersHandler {
	return &OrdersHandler{db: db, svc: svc, repo: repo, logger: logger}
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
	c.JSON(http.StatusOK, ToAdminOrderResponse(o, items, addrs))
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
	o, items, addrs, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminOrderResponse(o, items, addrs))
}

// Refund handles POST /admin/stores/:storeId/orders/:id/refund. The
// payment_status target is required because the service layer needs to
// know whether this is a partial or full refund (transition is enforced).
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
	if err := h.svc.RecordRefund(
		c.Request.Context(), nil, id,
		req.Amount, order.PaymentStatus(req.PaymentStatus), req.Reason,
	); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	o, items, addrs, err := h.repo.GetByID(c.Request.Context(), h.db, id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminOrderResponse(o, items, addrs))
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

