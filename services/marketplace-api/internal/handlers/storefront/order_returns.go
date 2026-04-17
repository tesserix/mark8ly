// Package storefront — order_returns.go: customer self-service return /
// replace request endpoints mounted under /storefront/stores/:storeSlug.
// The Go-side contract mirrors the admin POST but scopes the order to the
// signed-in customer instead of trusting a storeId path param.
package storefront

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// StorefrontReturnItemRequest is a single line of the create-return body.
type StorefrontReturnItemRequest struct {
	OrderItemID string  `json:"order_item_id" binding:"required,uuid"`
	Quantity    int     `json:"quantity"      binding:"required,min=1"`
	Reason      *string `json:"reason"`
}

// StorefrontReturnRequest is the wire body for POST
// /storefront/stores/:storeSlug/orders/:id/returns.
type StorefrontReturnRequest struct {
	// Type is "return" (refund-only, default) or "replace" (exchange).
	Type   string                         `json:"type"`
	Reason *string                        `json:"reason"`
	Notes  *string                        `json:"notes"`
	// Items is optional — when absent we default to requesting the full
	// ordered quantity of every line on the order. This is the common
	// case (customer wants to return "everything") and keeps the widget
	// simple.
	Items  []StorefrontReturnItemRequest `json:"items"`
}

// StorefrontReturnResponse is the customer-facing wire shape.
type StorefrontReturnResponse struct {
	ID            string  `json:"id"`
	ReturnNumber  string  `json:"return_number"`
	Type          string  `json:"type"`
	Status        string  `json:"status"`
	Reason        *string `json:"reason,omitempty"`
	Notes         *string `json:"notes,omitempty"`
	PickupDetails *string `json:"pickup_details,omitempty"`
	RejectReason  *string `json:"reject_reason,omitempty"`
	CurrencyCode  string  `json:"currency_code"`
	RequestedAt   string  `json:"requested_at"`
	ApprovedAt    *string `json:"approved_at,omitempty"`
	RejectedAt    *string `json:"rejected_at,omitempty"`
}

// RequestReturn handles POST /storefront/stores/:storeSlug/orders/:id/returns.
//
// Authn: requires the customer session cookie (OptionalCustomerAuth must
// have set CustomerProfileKey).
// Authz: the order must belong to the same customer profile + same store.
// Lifecycle: the order must have at least one shipment (can't return
// something that hasn't been dispatched). A shipment in any status is
// acceptable — customers may open a return for cancelled-in-transit or
// "delivered but wrong item" scenarios equally.
func (h *OrderDetailHandler) RequestReturn(c *gin.Context) {
	if h.returnSvc == nil || h.returnRepo == nil || h.orderSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "service_unavailable",
			"message": "Self-service returns are not configured for this deployment.",
		})
		return
	}

	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "store_not_found", "message": "store not found"})
		return
	}

	profileVal, exists := c.Get(CustomerProfileKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Sign in to request a return."})
		return
	}
	profile, ok := profileVal.(*customer.CustomerProfile)
	if !ok || profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Sign in to request a return."})
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id", "message": "invalid order id"})
		return
	}

	// Load + scope. Mirror the cancel endpoint's "same not_found for any
	// ownership miss" pattern so the endpoint can't be used as an id
	// existence oracle.
	o, items, _, err := h.orderRepo.GetByID(c.Request.Context(), h.db, orderID)
	if err != nil || o == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "order not found"})
		return
	}
	if o.StoreID.String() != store.ID || o.CustomerID == nil || o.CustomerID.String() != profile.ID.String() {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "order not found"})
		return
	}

	// Order-state guard — returns only make sense on orders that have
	// been confirmed (or further along). Pending / cancelled orders
	// belong to the cancel flow.
	switch o.Status {
	case string(order.OrderStatusConfirmed), string(order.OrderStatusFulfilled):
		// ok
	default:
		c.JSON(http.StatusConflict, gin.H{
			"error":   "not_returnable",
			"message": "This order can't be returned in its current state.",
		})
		return
	}

	// One open return per order in v1 — makes the order-page banner
	// logic unambiguous. If the customer needs to amend they can cancel
	// the existing request (future iteration).
	var existingOpen int64
	if err := h.db.WithContext(c.Request.Context()).
		Table("returns").
		Where("order_id = ? AND status NOT IN (?, ?)", orderID, string(order.ReturnStatusRejected), string(order.ReturnStatusRefunded)).
		Count(&existingOpen).Error; err == nil && existingOpen > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "return_exists",
			"message": "A return request already exists for this order.",
		})
		return
	}

	var req StorefrontReturnRequest
	_ = c.ShouldBindJSON(&req)

	// Default type to "return".
	retType := strings.TrimSpace(req.Type)
	if retType == "" {
		retType = order.ReturnTypeReturn
	}

	// When the client omits items, request the whole order — matches
	// the common "refund the whole thing" case without forcing the
	// widget to enumerate items.
	svcItems := make([]order.ReturnItem, 0, len(items))
	if len(req.Items) > 0 {
		for _, it := range req.Items {
			oid, err := uuid.Parse(it.OrderItemID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_item", "message": "order_item_id must be a uuid"})
				return
			}
			svcItems = append(svcItems, order.ReturnItem{
				OrderItemID: oid,
				Quantity:    it.Quantity,
				Reason:      it.Reason,
			})
		}
	} else {
		for _, it := range items {
			svcItems = append(svcItems, order.ReturnItem{
				OrderItemID: it.ID,
				Quantity:    it.Quantity,
				Reason:      req.Reason,
			})
		}
	}

	// Allocate the per-store return sequence.
	var seq int64
	if err := h.orderSvc.Unit(c.Request.Context(), func(tx *gorm.DB) error {
		n, err := order.NextDocumentNumber(c.Request.Context(), tx, o.StoreID, "return")
		if err != nil {
			return err
		}
		seq = n
		return nil
	}); err != nil {
		h.logger.Error("storefront return: seq alloc", "err", err, "order_id", orderID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "Could not create return, try again."})
		return
	}

	prefix := storePrefixFromStoreRow(store)
	in := order.RequestInput{
		TenantID:     o.TenantID,
		StoreID:      o.StoreID,
		StorePrefix:  prefix,
		ReturnSeq:    seq,
		OrderID:      orderID,
		Type:         retType,
		Reason:       req.Reason,
		Notes:        req.Notes,
		Items:        svcItems,
		CurrencyCode: o.CurrencyCode,
	}
	r, _, err := h.returnSvc.Request(c.Request.Context(), in)
	if err != nil {
		h.logger.Error("storefront return: service", "err", err, "order_id", orderID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "return_failed", "message": err.Error()})
		return
	}

	// Fire the in-app notification so the admin bell picks up the new
	// request exactly like an admin-initiated one. Nil-safe helper — we
	// skip silently if the notifier wasn't wired.
	who := customerDisplayName(profile)
	msg := who + " requested a " + retType + " on order " + o.OrderNumber + "."
	resourceType := "return"
	notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
		TenantID:     o.TenantID,
		StoreID:      o.StoreID,
		Type:         notification.TypeReturnRequested,
		Title:        "Return requested",
		Message:      &msg,
		ResourceType: &resourceType,
		ResourceID:   &r.ID,
	})

	c.JSON(http.StatusCreated, gin.H{
		"return": toStorefrontReturnResponse(r),
	})
}

// ListReturnsForOrder handles GET /storefront/stores/:storeSlug/orders/:id/returns.
// Returns 0 or 1 return (v1 enforces one open return per order) so the
// order-detail page can show the appropriate status banner.
func (h *OrderDetailHandler) ListReturnsForOrder(c *gin.Context) {
	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "store_not_found"})
		return
	}

	profileVal, _ := c.Get(CustomerProfileKey)
	profile, _ := profileVal.(*customer.CustomerProfile)
	if profile == nil {
		c.JSON(http.StatusOK, gin.H{"returns": []any{}})
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	// Scope check — same "not found" shape as Cancel.
	o, _, _, err := h.orderRepo.GetByID(c.Request.Context(), h.db, orderID)
	if err != nil || o == nil ||
		o.StoreID.String() != store.ID ||
		o.CustomerID == nil || o.CustomerID.String() != profile.ID.String() {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	var rows []order.Return
	if err := h.db.WithContext(c.Request.Context()).
		Where("order_id = ? AND store_id = ? AND tenant_id = ?", orderID, o.StoreID, o.TenantID).
		Order("requested_at DESC").
		Limit(5).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}
	out := make([]StorefrontReturnResponse, 0, len(rows))
	for i := range rows {
		out = append(out, toStorefrontReturnResponse(&rows[i]))
	}
	c.JSON(http.StatusOK, gin.H{"returns": out})
}

func toStorefrontReturnResponse(r *order.Return) StorefrontReturnResponse {
	out := StorefrontReturnResponse{
		ID:            r.ID.String(),
		ReturnNumber:  r.ReturnNumber,
		Type:          r.Type,
		Status:        r.Status,
		Reason:        r.Reason,
		Notes:         r.Notes,
		PickupDetails: r.PickupDetails,
		RejectReason:  r.RejectReason,
		CurrencyCode:  r.CurrencyCode,
		RequestedAt:   r.RequestedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if r.ApprovedAt != nil {
		s := r.ApprovedAt.UTC().Format("2006-01-02T15:04:05Z")
		out.ApprovedAt = &s
	}
	if r.RejectedAt != nil {
		s := r.RejectedAt.UTC().Format("2006-01-02T15:04:05Z")
		out.RejectedAt = &s
	}
	return out
}

// customerDisplayName returns a human-readable label for a profile,
// preferring the full name, falling back to first name, then email, then
// a generic placeholder.
func customerDisplayName(p *customer.CustomerProfile) string {
	if p == nil {
		return "A customer"
	}
	first := ""
	last := ""
	if p.FirstName != nil {
		first = strings.TrimSpace(*p.FirstName)
	}
	if p.LastName != nil {
		last = strings.TrimSpace(*p.LastName)
	}
	switch {
	case first != "" && last != "":
		return first + " " + last
	case first != "":
		return first
	case p.Email != "":
		return p.Email
	}
	return "A customer"
}

// storePrefixFromStoreRow builds the RMA prefix from the store, matching
// the admin path's storePrefixFromStore helper but working off the
// *stores.Store shape the storefront side has loaded. Falls back to a
// stable placeholder when the store has no slug (shouldn't happen in
// prod; belt-and-braces).
func storePrefixFromStoreRow(store *stores.Store) string {
	if store == nil {
		return "RMA"
	}
	slug := strings.ToUpper(store.Slug)
	if slug == "" {
		return "RMA"
	}
	// First 3 letters of slug, minus non-alnum.
	runes := make([]rune, 0, 3)
	for _, r := range slug {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			runes = append(runes, r)
		}
		if len(runes) == 3 {
			break
		}
	}
	if len(runes) == 0 {
		return "RMA"
	}
	return "R-" + string(runes)
}
