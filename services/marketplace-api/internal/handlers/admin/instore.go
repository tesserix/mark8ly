package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/checkoutcore"
	"github.com/mark8ly/marketplace-api/internal/instore"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// InStoreRecorder records a sale; satisfied by *instore.Service.
type InStoreRecorder interface {
	RecordSale(ctx context.Context, in instore.SaleInput) (*order.CreateResult, error)
}

// InStoreHandler serves the in-store sale screen (#1103): its catalogue,
// recording a sale, and the caller's own sales today.
type InStoreHandler struct {
	db     *gorm.DB
	sales  InStoreRecorder
	roles  authz.Client
	logger *slog.Logger
}

// NewInStoreHandler builds the handler. roles answers whether the caller is
// staff or above, which may always give discounts and custom prices.
func NewInStoreHandler(db *gorm.DB, sales InStoreRecorder, roles authz.Client, logger *slog.Logger) *InStoreHandler {
	return &InStoreHandler{db: db, sales: sales, roles: roles, logger: logger}
}

// InStoreSaleLineRequest is a catalogue line (variant_id) or a custom line
// (custom_title + custom_price). A catalogue line carries no price.
type InStoreSaleLineRequest struct {
	VariantID   *string          `json:"variant_id"`
	Quantity    int              `json:"quantity"`
	CustomTitle *string          `json:"custom_title"`
	CustomPrice *decimal.Decimal `json:"custom_price"`
}

// InStoreSaleRequest is POST /admin/stores/:storeId/in-store/sales.
type InStoreSaleRequest struct {
	IdempotencyKey     string                   `json:"idempotency_key" binding:"required"`
	Lines              []InStoreSaleLineRequest `json:"lines"`
	Discount           decimal.Decimal          `json:"discount"`
	PaymentMethod      string                   `json:"payment_method"`
	PaymentMethodLabel *string                  `json:"payment_method_label"`
	CashTendered       *decimal.Decimal         `json:"cash_tendered"`
	Note               *string                  `json:"note"`
	CustomerEmail      string                   `json:"customer_email"`
	// Fulfilled defaults to true: handed over at the counter.
	Fulfilled *bool `json:"fulfilled"`
}

// RecordSale handles POST /admin/stores/:storeId/in-store/sales.
func (h *InStoreHandler) RecordSale(c *gin.Context) {
	var req InStoreSaleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	store, storeID, tenantID, ok := h.scope(c)
	if !ok {
		return
	}
	userID := c.GetString("user_id")

	canDiscount := store.CashierDiscountsEnabled
	if !canDiscount {
		staff, err := h.roles.Check(c.Request.Context(), userID, string(authz.RoleStaff), tenantID.String())
		if err != nil {
			RespondErr(c, err, h.logger)
			return
		}
		canDiscount = staff
	}

	lines := make([]instore.SaleLine, 0, len(req.Lines))
	for _, l := range req.Lines {
		lines = append(lines, instore.SaleLine{VariantID: l.VariantID, Quantity: l.Quantity,
			CustomTitle: l.CustomTitle, CustomPrice: l.CustomPrice})
	}
	res, err := h.sales.RecordSale(c.Request.Context(), instore.SaleInput{
		TenantID: tenantID, StoreID: storeID, Store: store,
		IdempotencyKey: req.IdempotencyKey, Lines: lines, Discount: req.Discount,
		PaymentMethod: req.PaymentMethod, PaymentMethodLabel: req.PaymentMethodLabel,
		CashTendered: req.CashTendered, Note: req.Note, CustomerEmail: req.CustomerEmail,
		SoldByUserID: userID, CanDiscount: canDiscount,
		Fulfilled: req.Fulfilled == nil || *req.Fulfilled,
	})
	var oos checkoutcore.OutOfStockError
	if errors.As(err, &oos) {
		c.AbortWithStatusJSON(http.StatusConflict, map[string]any{
			"error":      "out_of_stock",
			"message":    "not enough stock for one of the items",
			"variant_id": oos.VariantID,
		})
		return
	}
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	status := http.StatusCreated
	if res.Reused {
		status = http.StatusOK
	}
	c.JSON(status, ToAdminOrderResponse(res.Order, res.Items, res.Addrs))
}

// InStoreCatalogVariant is one sellable variant on the sale screen.
type InStoreCatalogVariant struct {
	ID              string          `json:"id"`
	OptionSummary   *string         `json:"option_summary,omitempty"`
	Price           decimal.Decimal `json:"price"`
	CurrencyCode    string          `json:"currency_code"`
	Available       int             `json:"available"`
	InventoryPolicy string          `json:"inventory_policy"`
}

// InStoreCatalogProduct is one product on the sale screen.
type InStoreCatalogProduct struct {
	ID       string                  `json:"id"`
	Title    string                  `json:"title"`
	ImageURL *string                 `json:"image_url,omitempty"`
	Variants []InStoreCatalogVariant `json:"variants"`
}

// Catalog handles GET /admin/stores/:storeId/in-store/catalog: active,
// published products only, with nothing a cashier should not see (no cost
// price, no drafts).
func (h *InStoreHandler) Catalog(c *gin.Context) {
	_, storeID, _, ok := h.scope(c)
	if !ok {
		return
	}
	var rows []struct {
		ProductID       string
		Title           string
		ImageURL        *string
		VariantID       string
		OptionSummary   *string
		Price           decimal.Decimal
		CurrencyCode    string
		Available       int
		InventoryPolicy string
	}
	err := h.db.WithContext(c.Request.Context()).Raw(`
		SELECT p.id::text AS product_id, p.title,
		       (SELECT pm.url FROM product_media pm WHERE pm.product_id = p.id
		         ORDER BY pm.position, pm.id LIMIT 1) AS image_url,
		       v.id::text AS variant_id,
		       (SELECT string_agg(pov.value, ' / ' ORDER BY po.position, pov.position)
		          FROM variant_option_values vov
		          JOIN product_option_values pov ON pov.id = vov.option_value_id
		          JOIN product_options po ON po.id = pov.option_id
		         WHERE vov.variant_id = v.id) AS option_summary,
		       v.price, v.currency_code, v.inventory_policy,
		       GREATEST(
		         COALESCE((SELECT SUM(vs.quantity) FROM variant_stock vs WHERE vs.variant_id = v.id), 0)
		       - COALESCE((SELECT SUM(sh.qty) FROM stock_holds sh WHERE sh.variant_id = v.id
		                    AND sh.state = 'held' AND sh.expires_at > now()), 0), 0) AS available
		  FROM products p
		  JOIN product_variants v ON v.product_id = p.id AND v.deleted_at IS NULL
		 WHERE p.store_id = ? AND p.deleted_at IS NULL AND p.status = 'active'
		   AND p.published_at IS NOT NULL AND p.published_at <= now()
		 ORDER BY p.title, p.id, v.position, v.id`, storeID).Scan(&rows).Error
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := []InStoreCatalogProduct{}
	for _, r := range rows {
		if len(out) == 0 || out[len(out)-1].ID != r.ProductID {
			out = append(out, InStoreCatalogProduct{ID: r.ProductID, Title: r.Title, ImageURL: r.ImageURL})
		}
		p := &out[len(out)-1]
		p.Variants = append(p.Variants, InStoreCatalogVariant{ID: r.VariantID, OptionSummary: r.OptionSummary,
			Price: r.Price, CurrencyCode: r.CurrencyCode, Available: r.Available, InventoryPolicy: r.InventoryPolicy})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// InStoreSaleSummary is one of the caller's sales today.
type InStoreSaleSummary struct {
	ID            string          `json:"id"`
	OrderNumber   string          `json:"order_number"`
	GrandTotal    decimal.Decimal `json:"grand_total"`
	CurrencyCode  string          `json:"currency_code"`
	PaymentMethod *string         `json:"payment_method,omitempty"`
	PlacedAt      time.Time       `json:"placed_at"`
	ReceiptEmail  string          `json:"receipt_email,omitempty"` // masked
}

// MySales handles GET /admin/stores/:storeId/in-store/sales: the caller's
// own in-store sales since midnight in the store's timezone. A cashier sees
// no one else's.
func (h *InStoreHandler) MySales(c *gin.Context) {
	store, storeID, _, ok := h.scope(c)
	if !ok {
		return
	}
	tz := store.Timezone
	if _, err := time.LoadLocation(tz); tz == "" || err != nil {
		tz = "UTC"
	}
	var rows []struct {
		ID            string
		OrderNumber   string
		GrandTotal    decimal.Decimal
		CurrencyCode  string
		PaymentMethod *string
		PlacedAt      time.Time
		CustomerEmail string
	}
	err := h.db.WithContext(c.Request.Context()).Raw(`
		SELECT id::text AS id, order_number, grand_total, currency_code, payment_method, placed_at, customer_email
		  FROM orders
		 WHERE store_id = ? AND channel = 'in_store' AND sold_by_user_id = ? AND deleted_at IS NULL
		   AND placed_at >= (date_trunc('day', now() AT TIME ZONE ?) AT TIME ZONE ?)
		 ORDER BY placed_at DESC
		 LIMIT 200`, storeID, c.GetString("user_id"), tz, tz).Scan(&rows).Error
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := make([]InStoreSaleSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, InStoreSaleSummary{ID: r.ID, OrderNumber: r.OrderNumber, GrandTotal: r.GrandTotal,
			CurrencyCode: r.CurrencyCode, PaymentMethod: r.PaymentMethod, PlacedAt: r.PlacedAt,
			ReceiptEmail: maskEmail(r.CustomerEmail)})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *InStoreHandler) scope(c *gin.Context) (*stores.Store, uuid.UUID, uuid.UUID, bool) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "must be a uuid"), h.logger)
		return nil, uuid.Nil, uuid.Nil, false
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "must be a uuid"), h.logger)
		return nil, uuid.Nil, uuid.Nil, false
	}
	v, _ := c.Get("store")
	store, _ := v.(*stores.Store)
	if store == nil {
		RespondErr(c, apperrors.NotFound("store"), h.logger)
		return nil, uuid.Nil, uuid.Nil, false
	}
	return store, storeID, tenantID, true
}

// maskEmail shows enough to recognise a receipt went out: "j***@gmail.com".
func maskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at < 1 {
		return ""
	}
	return e[:1] + "***" + e[at:]
}
