// Package storefront — order_detail.go: GET /storefront/stores/:storeSlug/orders/:id
// and POST /storefront/stores/:storeSlug/orders/:id/cancel.
// Returns a customer-facing order view (no admin-only fields like cost_price).
package storefront

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tax"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// OrderDetailHandler serves the public order view for storefront
// customers and exposes self-service actions (cancel, request return).
type OrderDetailHandler struct {
	db         *gorm.DB
	orderRepo  order.Repository
	orderSvc   *order.Service       // optional — when nil, /cancel returns 503
	returnSvc  *order.ReturnService // optional — when nil, /returns returns 503
	returnRepo order.ReturnRepository
	docMailer  *orderdoc.Service // optional — when nil, the cancel email is skipped
	// notify is an optional notification service. When set, customer-
	// initiated return requests emit notification.TypeReturnRequested so
	// admins see them in their bell just like admin-initiated ones.
	notify *notification.Service
	// refunds is an optional refund coordinator. When set, a paid order
	// is auto-refunded on customer self-cancel (spec §4). Nil-safe —
	// without it, self-cancel simply skips the refund step (pre-existing
	// behavior).
	refunds *orderrefund.Coordinator
	logger  *slog.Logger
}

// NewOrderDetailHandler constructs an OrderDetailHandler. orderSvc,
// returnSvc, returnRepo and docMailer are optional — passing nil disables
// the relevant routes.
func NewOrderDetailHandler(db *gorm.DB, orderRepo order.Repository, orderSvc *order.Service, docMailer *orderdoc.Service, logger *slog.Logger) *OrderDetailHandler {
	return &OrderDetailHandler{db: db, orderRepo: orderRepo, orderSvc: orderSvc, docMailer: docMailer, logger: logger}
}

// WithReturns attaches the return service + repository so the
// self-service return request route is enabled.
func (h *OrderDetailHandler) WithReturns(svc *order.ReturnService, repo order.ReturnRepository) *OrderDetailHandler {
	h.returnSvc = svc
	h.returnRepo = repo
	return h
}

// WithNotifier attaches the notification service so customer-initiated
// return requests fire admin bell entries (notification.TypeReturnRequested).
// Nil-safe; passing nil simply suppresses notifications on this path.
func (h *OrderDetailHandler) WithNotifier(n *notification.Service) *OrderDetailHandler {
	h.notify = n
	return h
}

// WithRefunds attaches the refund coordinator so a paid order is
// auto-refunded when the customer self-cancels it (spec §4). Nil-safe —
// passing nil (or never calling this) simply skips the refund step.
func (h *OrderDetailHandler) WithRefunds(c *orderrefund.Coordinator) *OrderDetailHandler {
	h.refunds = c
	return h
}

// storefrontOrderResponse is the customer-facing DTO.
type storefrontOrderResponse struct {
	ID            string `json:"id"`
	OrderNumber   string `json:"order_number"`
	Status        string `json:"status"`
	PaymentStatus string `json:"payment_status"`
	// CustomerEmail is the email captured at checkout. Surfaced so the
	// invoice/receipt PDF (rendered by Next.js on the storefront) can
	// show the buyer's contact line in the bill-to block. Earlier this
	// field was never returned, leaving customer-rendered PDFs without
	// a contact email.
	CustomerEmail string `json:"customer_email"`
	CustomerName  string `json:"customer_name,omitempty"`
	Subtotal      string `json:"subtotal"`
	ShippingTotal string `json:"shipping_total"`
	TaxTotal      string `json:"tax_total"`
	// DiscountTotal is the sum of coupon + loyalty + manual discounts on
	// the order. Surfaced so customer-rendered PDFs can show the
	// discount line in the totals block (admin-rendered ones already
	// do via AdminOrder.discount_total).
	DiscountTotal string `json:"discount_total"`
	GrandTotal    string `json:"grand_total"`
	// RefundedAmount surfaces partial + full refunds on the customer
	// account page. Always present (zero-value "0.00" when no refunds).
	RefundedAmount string `json:"refunded_amount"`
	// TaxLines is the per-jurisdiction breakdown of the tax_total.
	// India GST splits into CGST + SGST (intra-state) or IGST (inter-
	// state); flat-rate countries get a single VAT/GST line; TaxJar
	// returns state + county + city + special breakdowns. Empty when
	// the order pre-dates the persistence wiring.
	TaxLines        []storefrontTaxLineResponse   `json:"tax_lines"`
	CurrencyCode    string                        `json:"currency_code"`
	Items           []storefrontOrderItemResponse `json:"items"`
	ShippingAddress *storefrontAddressResponse    `json:"shipping_address"`
	BillingAddress  *storefrontAddressResponse    `json:"billing_address,omitempty"`
	Shipment        *storefrontShipmentResponse   `json:"shipment,omitempty"`
	// Shipments is every parcel on the order, oldest first. A multi-warehouse
	// order ships as more than one (#177), and the singular Shipment above
	// shows only the most recent — a customer reading it alone would silently
	// lose the other tracking numbers.
	//
	// Shipment is kept, unchanged, because apps/storefront reads it in seven
	// places including invoice rendering, and this service deploys
	// independently of that app. It is retired once nothing reads it.
	Shipments []storefrontShipmentResponse `json:"shipments"`
	Timeline  []storefrontTimelineEntry    `json:"timeline"`
	PlacedAt  string                       `json:"placed_at"`
}

// storefrontTaxLineResponse is the public view of one persisted
// order_tax_lines row.
type storefrontTaxLineResponse struct {
	Description  string `json:"description"`
	Rate         string `json:"rate"`
	Amount       string `json:"amount"`
	Jurisdiction string `json:"jurisdiction,omitempty"`
}

// storefrontShipmentResponse is the public view of a shipment. The
// label_url and carrier-internal ids are kept admin-only; customers
// get the tracking number + status only. DeliveredAt is surfaced once
// the shipment flips to "delivered" so receipt PDFs can stamp the
// real delivery moment instead of falling back to order.updated_at.
type storefrontShipmentResponse struct {
	Carrier           string `json:"carrier"`
	Service           string `json:"service,omitempty"`
	TrackingNumber    string `json:"tracking_number,omitempty"`
	Status            string `json:"status"`
	EstimatedDelivery string `json:"estimated_delivery,omitempty"`
	DeliveredAt       string `json:"delivered_at,omitempty"`
	ShippedAt         string `json:"shipped_at,omitempty"`
}

// storefrontTimelineEntry is one row in the customer-facing order
// timeline. Kind maps 1:1 to order_events.kind; description is a
// human-readable one-liner pulled from the payload when present.
type storefrontTimelineEntry struct {
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	Status         string `json:"status,omitempty"`
	Carrier        string `json:"carrier,omitempty"`
	TrackingNumber string `json:"tracking_number,omitempty"`
	OccurredAt     string `json:"occurred_at"`
}

type storefrontOrderItemResponse struct {
	TitleSnapshot string `json:"title_snapshot"`
	SKUSnapshot   string `json:"sku_snapshot"`
	OptionSummary string `json:"option_summary,omitempty"`
	UnitPrice     string `json:"unit_price"`
	Quantity      int    `json:"quantity"`
	LineTotal     string `json:"line_total"`
	CurrencyCode  string `json:"currency_code"`
	ImageURL      string `json:"image_url,omitempty"`
}

type storefrontAddressResponse struct {
	Name        string `json:"name"`
	Line1       string `json:"line1"`
	Line2       string `json:"line2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
	CountryCode string `json:"country_code"`
	Phone       string `json:"phone,omitempty"`
}

// GetOrder handles GET /storefront/stores/:storeSlug/orders/:id.
// resolveCallerCustomer identifies which customer a storefront order request is
// acting for, so reads/writes can be scoped to that customer. It prefers a
// logged-in session profile (CustomerProfileKey, set by the optional
// customer-auth middleware); failing that, a trusted backend caller — the Otto
// support assistant, already authenticated by the storefront key — may act for a
// verified customer via the X-Customer-Email header (the conversation's
// OTP/session-verified email). ok=false means no customer is identifiable (e.g.
// the post-checkout confirmation view), which stays store-scoped only.
func resolveCallerCustomer(c *gin.Context) (profileID, email string, ok bool) {
	if pv, exists := c.Get(CustomerProfileKey); exists {
		if p, _ := pv.(*customer.CustomerProfile); p != nil {
			return p.ID.String(), "", true
		}
	}
	if e := strings.TrimSpace(c.GetHeader("X-Customer-Email")); e != "" {
		return "", e, true
	}
	return "", "", false
}

// orderMatchesCaller reports whether order o belongs to the resolved caller —
// by customer profile id (logged-in) or by email (trusted assistant call).
func orderMatchesCaller(o *order.Order, profileID, email string) bool {
	if profileID != "" {
		return o.CustomerID != nil && o.CustomerID.String() == profileID
	}
	if email != "" {
		return strings.EqualFold(strings.TrimSpace(o.CustomerEmail), email)
	}
	return false
}

func (h *OrderDetailHandler) GetOrder(c *gin.Context) {
	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "store_not_found", "message": "store not found",
		})
		return
	}

	idStr := c.Param("id")
	orderID, err := uuid.Parse(idStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_id", "message": "invalid order ID",
		})
		return
	}

	o, items, addrs, err := h.orderRepo.GetByID(c.Request.Context(), h.db, orderID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "order not found",
		})
		return
	}

	// Verify order belongs to this store.
	if o.StoreID.String() != store.ID {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "order not found",
		})
		return
	}

	// Scope to the requesting customer: a logged-in customer or the Otto
	// assistant (acting for a verified customer via X-Customer-Email) may only
	// read THEIR own order. Anonymous storefront-key reads (the post-checkout
	// confirmation view) stay store-scoped. Same not_found on a mismatch so an
	// order id can't be probed for existence.
	if pid, email, ok := resolveCallerCustomer(c); ok && !orderMatchesCaller(o, pid, email) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "order not found",
		})
		return
	}

	shipment := h.loadShipment(c.Request.Context(), orderID)
	timeline := h.loadTimeline(c.Request.Context(), orderID)
	taxLines := h.loadTaxLines(c.Request.Context(), orderID)

	resp := mapOrderToStorefrontResponse(o, items, addrs)
	resp.Shipment = shipment
	resp.Shipments = h.loadShipments(c.Request.Context(), orderID)
	resp.Timeline = timeline
	resp.TaxLines = taxLines
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// loadTaxLines fetches the persisted per-jurisdiction tax breakdown.
// Returns empty (non-nil) on miss so the JSON is `[]` rather than null.
func (h *OrderDetailHandler) loadTaxLines(ctx context.Context, orderID uuid.UUID) []storefrontTaxLineResponse {
	repo := tax.NewRepository()
	rows, err := repo.GetTaxLines(ctx, h.db, orderID)
	if err != nil {
		h.logger.Warn("storefront order: load tax lines", "err", err, "order_id", orderID)
		return []storefrontTaxLineResponse{}
	}
	out := make([]storefrontTaxLineResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, storefrontTaxLineResponse{
			Description:  r.Description,
			Rate:         r.Rate.StringFixed(2),
			Amount:       r.Amount.StringFixed(2),
			Jurisdiction: r.Jurisdiction,
		})
	}
	return out
}

type shipmentRow struct {
	// Real DB columns: shipments has `carrier` (not `provider`) and no
	// `service` column at all — service level is kept in-memory on the
	// canonical shipping.Shipment model (gorm:"-"). Earlier this struct
	// queried non-existent columns and silently returned nil, which made
	// every customer-side receipt download return 409 not_delivered.
	//
	// DeliveredAt + ShippedAt are written by the admin shipment status
	// handler when the row transitions to delivered/in_transit and read
	// here so the receipt PDF can stamp the real delivery moment rather
	// than falling back to order.updated_at as a proxy.
	Carrier           string  `gorm:"column:carrier"`
	TrackingNumber    string  `gorm:"column:tracking_number"`
	Status            string  `gorm:"column:status"`
	EstimatedDelivery *string `gorm:"column:estimated_delivery"`
	ShippedAt         *string `gorm:"column:shipped_at"`
	DeliveredAt       *string `gorm:"column:delivered_at"`
}

func (r shipmentRow) TableName() string { return "shipments" }

func (h *OrderDetailHandler) loadShipment(ctx context.Context, orderID uuid.UUID) *storefrontShipmentResponse {
	var row shipmentRow
	err := h.db.WithContext(ctx).
		Table("shipments").
		Select("carrier", "tracking_number", "status", "estimated_delivery", "shipped_at", "delivered_at").
		Where("order_id = ?", orderID).
		Order("created_at DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil || row.Carrier == "" {
		return nil
	}
	resp := shipmentResponseFrom(row)
	return &resp
}

// shipmentResponseFrom maps a raw shipments row onto the public DTO. Shared
// by loadShipment (most recent parcel) and loadShipments (every parcel) so
// the field mapping can't drift between the two.
func shipmentResponseFrom(row shipmentRow) storefrontShipmentResponse {
	resp := storefrontShipmentResponse{
		Carrier: row.Carrier,
		// Service level isn't persisted, so we leave it empty on the
		// public DTO — the customer card already reads "Standard delivery"
		// from the order's chosen shipping_service.
		TrackingNumber: row.TrackingNumber,
		Status:         row.Status,
	}
	if row.EstimatedDelivery != nil {
		resp.EstimatedDelivery = *row.EstimatedDelivery
	}
	if row.ShippedAt != nil {
		resp.ShippedAt = *row.ShippedAt
	}
	if row.DeliveredAt != nil {
		resp.DeliveredAt = *row.DeliveredAt
	}
	return resp
}

// loadShipments returns every shipment on the order, oldest first.
//
// Ordered ASCENDING by created_at so a parcel keeps its position as later
// ones are added — a customer who bookmarked "parcel 1" should not find it
// renumbered. loadShipment's DESC + Limit(1) is left alone: it answers a
// different question (the most recent parcel) that the singular field still
// promises.
//
// A read failure yields an empty list rather than an error: the order page
// must still render without its tracking numbers, exactly as loadShipment
// already degrades.
func (h *OrderDetailHandler) loadShipments(ctx context.Context, orderID uuid.UUID) []storefrontShipmentResponse {
	var rows []shipmentRow
	err := h.db.WithContext(ctx).
		Table("shipments").
		Select("carrier", "tracking_number", "status", "estimated_delivery", "shipped_at", "delivered_at").
		Where("order_id = ?", orderID).
		Order("created_at ASC, id ASC").
		Scan(&rows).Error
	if err != nil {
		if h.logger != nil {
			h.logger.Error("storefront: load shipments", "order_id", orderID, "err", err)
		}
		return nil
	}

	out := make([]storefrontShipmentResponse, 0, len(rows))
	for _, row := range rows {
		if row.Carrier == "" {
			continue
		}
		out = append(out, shipmentResponseFrom(row))
	}
	return out
}

type timelineRow struct {
	Kind      string `gorm:"column:kind"`
	Payload   []byte `gorm:"column:payload"`
	CreatedAt string `gorm:"column:created_at"`
}

func (timelineRow) TableName() string { return "order_events" }

// customerHiddenEventKinds are order_events kinds that carry operational
// signal for the merchant only and must never appear on the buyer-facing
// storefront timeline. loadTimeline filters them out at the query level so
// a new admin-only kind can't leak just by being written to order_events.
var customerHiddenEventKinds = []string{
	string(order.EventKindPickupFailed),
	string(order.EventKindGiftCardCreditSkipped),
}

func (h *OrderDetailHandler) loadTimeline(ctx context.Context, orderID uuid.UUID) []storefrontTimelineEntry {
	var rows []timelineRow
	if err := h.db.WithContext(ctx).
		Table("order_events").
		Select("kind", "payload", "to_char(created_at, 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') AS created_at").
		Where("order_id = ?", orderID).
		Where("kind NOT IN ?", customerHiddenEventKinds).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return []storefrontTimelineEntry{}
	}

	out := make([]storefrontTimelineEntry, 0, len(rows))
	for _, r := range rows {
		entry := storefrontTimelineEntry{
			Kind:       r.Kind,
			OccurredAt: r.CreatedAt,
		}
		// Best-effort enrichment — shipment events carry extra fields,
		// others have simpler payloads. We only surface what the UI
		// actually renders; unknown payloads are left as bare entries.
		var raw map[string]any
		_ = json.Unmarshal(r.Payload, &raw)
		if raw != nil {
			if s, ok := raw["description"].(string); ok {
				entry.Description = s
			}
			if s, ok := raw["status"].(string); ok {
				entry.Status = s
			}
			if s, ok := raw["carrier"].(string); ok {
				entry.Carrier = s
			}
			if s, ok := raw["tracking_number"].(string); ok {
				entry.TrackingNumber = s
			}
		}
		if entry.Description == "" {
			entry.Description = defaultTimelineDescription(r.Kind)
		}
		out = append(out, entry)
	}
	return out
}

// defaultTimelineDescription is a fallback label for events whose payload
// doesn't carry a pre-formatted description (older rows, returns, etc.).
func defaultTimelineDescription(kind string) string {
	switch kind {
	case "created":
		return "Order placed."
	case "status_changed":
		return "Order status updated."
	case "payment_recorded":
		return "Payment received."
	case "fulfilled":
		return "Order fulfilled."
	case "partially_fulfilled":
		return "Part of your order has shipped."
	case "cancelled":
		return "Order cancelled."
	case "refunded":
		return "Order refunded."
	case "shipment_created":
		return "Shipping label created."
	case "shipment_in_transit":
		return "Package is on its way."
	case "shipment_out_for_delivery":
		return "Out for delivery."
	case "shipment_delivered":
		return "Package delivered."
	case "shipment_exception":
		return "Delivery exception."
	default:
		return kind
	}
}

func mapOrderToStorefrontResponse(o *order.Order, items []order.OrderItem, addrs []order.OrderAddress) storefrontOrderResponse {
	respItems := make([]storefrontOrderItemResponse, 0, len(items))
	for _, it := range items {
		ri := storefrontOrderItemResponse{
			TitleSnapshot: it.TitleSnapshot,
			SKUSnapshot:   it.SKUSnapshot,
			UnitPrice:     it.UnitPrice.StringFixed(2),
			Quantity:      it.Quantity,
			LineTotal:     it.LineTotal.StringFixed(2),
			CurrencyCode:  it.CurrencyCode,
		}
		if it.OptionSummary != nil {
			ri.OptionSummary = *it.OptionSummary
		}
		if it.ImageURL != nil {
			ri.ImageURL = *it.ImageURL
		}
		respItems = append(respItems, ri)
	}

	var shippingAddr, billingAddr *storefrontAddressResponse
	for _, a := range addrs {
		mapped := &storefrontAddressResponse{
			Name:        a.Name,
			Line1:       a.Line1,
			Line2:       derefStr(a.Line2),
			City:        a.City,
			Region:      derefStr(a.Region),
			PostalCode:  derefStr(a.PostalCode),
			CountryCode: a.CountryCode,
			Phone:       derefStr(a.Phone),
		}
		switch a.Kind {
		case "shipping":
			shippingAddr = mapped
		case "billing":
			billingAddr = mapped
		}
	}
	// If the order only has a single shipping address (most checkouts
	// reuse it as the billing address) leave billing nil — the PDF
	// builders fall back to the shipping address when bill_to is unset.

	customerName := ""
	if o.CustomerName != nil {
		customerName = *o.CustomerName
	}

	return storefrontOrderResponse{
		ID:              o.ID.String(),
		OrderNumber:     o.OrderNumber,
		Status:          string(o.Status),
		PaymentStatus:   string(o.PaymentStatus),
		CustomerEmail:   o.CustomerEmail,
		CustomerName:    customerName,
		Subtotal:        o.Subtotal.StringFixed(2),
		ShippingTotal:   decimalStr(o.ShippingTotal),
		TaxTotal:        decimalStr(o.TaxTotal),
		DiscountTotal:   decimalStr(o.DiscountTotal),
		GrandTotal:      o.GrandTotal.StringFixed(2),
		RefundedAmount:  o.RefundedAmount.StringFixed(2),
		CurrencyCode:    o.CurrencyCode,
		Items:           respItems,
		ShippingAddress: shippingAddr,
		BillingAddress:  billingAddr,
		PlacedAt:        o.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func decimalStr(d decimal.Decimal) string {
	if d.IsZero() {
		return "0.00"
	}
	return d.StringFixed(2)
}

// ─────────────────────────────────────────────────────────────────────────
// Self-service cancel
// ─────────────────────────────────────────────────────────────────────────

// CancelRequest is the wire body for POST /storefront/.../orders/:id/cancel.
// Reason is optional — when empty we substitute a generic copy so the
// merchant always sees something in the order_events row.
type CancelRequest struct {
	Reason string `json:"reason"`
}

// Cancel handles POST /storefront/stores/:storeSlug/orders/:id/cancel.
//
// Authn: requires the customer session cookie (OptionalCustomerAuth must
// have populated CustomerProfileKey on the Gin context).
// Authz: the order must belong to the same customer profile + same store.
// Lifecycle guards: same as the admin path (status machine in
// order.Service rejects fulfilled, plus we bounce shipment-in-flight here).
func (h *OrderDetailHandler) Cancel(c *gin.Context) {
	if h.orderSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "service_unavailable",
			"message": "Self-service cancel is not configured for this deployment.",
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Sign in to cancel an order."})
		return
	}
	profile, ok := profileVal.(*customer.CustomerProfile)
	if !ok || profile == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Sign in to cancel an order."})
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id", "message": "invalid order id"})
		return
	}

	// Load + scope the order to the signed-in customer + this store. We
	// deliberately return the same not_found shape for "wrong customer"
	// and "wrong store" so the endpoint doesn't double as an order-id
	// existence oracle.
	o, _, _, err := h.orderRepo.GetByID(c.Request.Context(), h.db, orderID)
	if err != nil || o == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "order not found"})
		return
	}
	if o.StoreID.String() != store.ID || o.CustomerID == nil || o.CustomerID.String() != profile.ID.String() {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "order not found"})
		return
	}

	// Shipment guard — the customer cannot self-cancel once the merchant
	// has cut a shipping label, regardless of whether the parcel is
	// physically moving yet. From that moment the right operation is a
	// return + refund, not a cancellation.
	var shipCount int64
	if err := h.db.WithContext(c.Request.Context()).
		Table("shipments").
		Where("order_id = ?", orderID).
		Count(&shipCount).Error; err == nil && shipCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "shipment_in_flight",
			"message": "Your order has already been picked up for delivery — please reply to your order confirmation email to arrange a return and refund.",
		})
		return
	}

	var req CancelRequest
	_ = c.ShouldBindJSON(&req)
	if req.Reason == "" {
		req.Reason = "Cancelled by customer"
	}

	if err := h.orderSvc.Cancel(c.Request.Context(), nil, orderID, req.Reason); err != nil {
		// Status-machine guard inside service.Cancel returns
		// apperrors.InvalidTransition for non-cancellable states; map to 409.
		if errors.Is(err, apperrors.ErrInvalidTransition) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "not_cancellable",
				"message": "This order is not in a state where it can be cancelled.",
			})
			return
		}
		h.logger.Error("storefront cancel: service error", "err", err, "order_id", orderID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "Could not cancel the order. Please try again."})
		return
	}

	// Auto-refund a paid order on cancellation (spec §4). Best-effort: a
	// gateway blip leaves the order cancelled + a pending ledger row for the
	// sweeper, so the customer is never blocked on the cancel response.
	if h.refunds != nil && o.PaymentStatus == string(order.PaymentStatusPaid) {
		if _, rerr := h.refunds.Refund(c.Request.Context(), orderrefund.RefundCommand{
			OrderID: orderID, Amount: nil, Reason: "order cancelled", Actor: "customer", ScopeID: "cancel",
		}); rerr != nil {
			h.logger.Warn("cancel auto-refund deferred", "order_id", orderID, "err", rerr)
		}
	}

	// Fire the in-app notification so the admin bell picks up a customer
	// self-cancel exactly like an admin-initiated one. Nil-safe — we skip
	// silently if the notifier wasn't wired.
	cancelMsg := "Order " + o.OrderNumber + " was cancelled."
	resourceType := "order"
	notification.Emit(c.Request.Context(), h.notify, h.logger, notification.Notification{
		TenantID:     o.TenantID,
		StoreID:      o.StoreID,
		Type:         notification.TypeOrderCancelled,
		Title:        "Order cancelled",
		Message:      &cancelMsg,
		ResourceType: &resourceType,
		ResourceID:   &o.ID,
	})

	// Fire the cancellation email on a detached context — same fire-and-
	// forget contract as the admin path. byCustomer=true switches the
	// email copy to the self-service tone.
	if h.docMailer != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := h.docMailer.SendCancellation(ctx, orderID, req.Reason, true); err != nil {
				h.logger.Warn("orderdoc: customer cancel email dispatch failed",
					"order_id", orderID, "err", err)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"cancelled": true, "order_id": orderID.String()})
}
