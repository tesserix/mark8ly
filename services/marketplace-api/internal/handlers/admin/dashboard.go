// Package admin — dashboard.go: HTTP handler for the per-store
// dashboard aggregation endpoint (Dashboard D1).
package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"gorm.io/gorm"
)

// ---------- Response DTOs ----------

// DashboardStats holds the top-level numeric indicators.
type DashboardStats struct {
	RevenueToday        float64   `json:"revenue_today"`
	RevenueWeek         float64   `json:"revenue_week"`
	RevenueMonth        float64   `json:"revenue_month"`
	RevenueChangePct    float64   `json:"revenue_change_pct"`
	RevenueTrend        []float64 `json:"revenue_trend"`
	OrdersToday         int64     `json:"orders_today"`
	OrdersPending       int64     `json:"orders_pending"`
	OrdersFulfilled     int64     `json:"orders_fulfilled"`
	OrdersCancelled     int64     `json:"orders_cancelled"`
	CustomersTotal      int64     `json:"customers_total"`
	CustomersNewThisWeek int64    `json:"customers_new_this_week"`
	PendingReviews      int64     `json:"pending_reviews"`
}

// RecentOrder is a summary row for a recent order.
type RecentOrder struct {
	ID            string  `json:"id"`
	OrderNumber   string  `json:"order_number"`
	CustomerEmail string  `json:"customer_email"`
	GrandTotal    float64 `json:"grand_total"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

// TopProduct shows a product ranked by revenue.
type TopProduct struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Revenue   float64 `json:"revenue"`
	UnitsSold int64   `json:"units_sold"`
	ImageURL  *string `json:"image_url"`
}

// LowStockItem shows a variant below its reorder threshold.
type LowStockItem struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	VariantTitle      string `json:"variant_title"`
	Quantity          int    `json:"quantity"`
	LowStockThreshold int   `json:"low_stock_threshold"`
}

// SetupChecklist tracks onboarding completion.
type SetupChecklist struct {
	HasStore           bool `json:"has_store"`
	HasProduct         bool `json:"has_product"`
	HasPaymentProvider bool `json:"has_payment_provider"`
	HasShippingCarrier bool `json:"has_shipping_carrier"`
	HasTaxConfigured   bool `json:"has_tax_configured"`
	HasCustomDomain    bool `json:"has_custom_domain"`
	HasStorefrontTheme bool `json:"has_storefront_theme"`
	HasTestOrder       bool `json:"has_test_order"`
}

// DashboardResponse is the top-level envelope returned by GET .../dashboard.
type DashboardResponse struct {
	Stats          DashboardStats `json:"stats"`
	RecentOrders   []RecentOrder  `json:"recent_orders"`
	TopProducts    []TopProduct   `json:"top_products"`
	LowStock       []LowStockItem `json:"low_stock"`
	SetupChecklist SetupChecklist `json:"setup_checklist"`
}

// ---------- Cache ----------

type cacheEntry struct {
	data      DashboardResponse
	expiresAt time.Time
}

// ---------- Handler ----------

// DashboardHandler serves the admin dashboard aggregation endpoint.
type DashboardHandler struct {
	db     *gorm.DB
	logger *slog.Logger
	cache  sync.Map // key: "tenantID:storeID" → *cacheEntry
}

// NewDashboardHandler constructs a DashboardHandler.
func NewDashboardHandler(db *gorm.DB, logger *slog.Logger) *DashboardHandler {
	return &DashboardHandler{db: db, logger: logger}
}

// Get handles GET /admin/stores/:storeId/dashboard.
func (h *DashboardHandler) Get(c *gin.Context) {
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

	cacheKey := fmt.Sprintf("%s:%s", tenantID, storeID)

	// Check cache.
	if val, ok := h.cache.Load(cacheKey); ok {
		entry := val.(*cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			c.Header("Cache-Control", "private, no-store")
			c.JSON(http.StatusOK, entry.data)
			return
		}
		h.cache.Delete(cacheKey)
	}

	ctx := c.Request.Context()
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -int(todayStart.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	lastWeekStart := weekStart.AddDate(0, 0, -7)

	db := h.db.WithContext(ctx)

	var stats DashboardStats

	// Revenue today.
	db.Raw(`SELECT COALESCE(SUM(grand_total), 0) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled' AND created_at >= ?`,
		storeID, tenantID, todayStart).Scan(&stats.RevenueToday)

	// Revenue this week.
	db.Raw(`SELECT COALESCE(SUM(grand_total), 0) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled' AND created_at >= ?`,
		storeID, tenantID, weekStart).Scan(&stats.RevenueWeek)

	// Revenue this month.
	db.Raw(`SELECT COALESCE(SUM(grand_total), 0) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled' AND created_at >= ?`,
		storeID, tenantID, monthStart).Scan(&stats.RevenueMonth)

	// Revenue change % — compare this week vs last week.
	var lastWeekRevenue float64
	db.Raw(`SELECT COALESCE(SUM(grand_total), 0) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled'
		AND created_at >= ? AND created_at < ?`,
		storeID, tenantID, lastWeekStart, weekStart).Scan(&lastWeekRevenue)

	if lastWeekRevenue > 0 {
		stats.RevenueChangePct = ((stats.RevenueWeek - lastWeekRevenue) / lastWeekRevenue) * 100
	}

	// Revenue trend — last 7 days grouped by date.
	stats.RevenueTrend = make([]float64, 7)
	type dayRevenue struct {
		Day     time.Time
		Revenue float64
	}
	var trendRows []dayRevenue
	sevenDaysAgo := todayStart.AddDate(0, 0, -6)
	db.Raw(`SELECT DATE(created_at) AS day, COALESCE(SUM(grand_total), 0) AS revenue
		FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled'
		AND created_at >= ?
		GROUP BY DATE(created_at)
		ORDER BY day`,
		storeID, tenantID, sevenDaysAgo).Scan(&trendRows)

	trendMap := make(map[string]float64, len(trendRows))
	for _, r := range trendRows {
		trendMap[r.Day.Format("2006-01-02")] = r.Revenue
	}
	for i := 0; i < 7; i++ {
		day := sevenDaysAgo.AddDate(0, 0, i).Format("2006-01-02")
		stats.RevenueTrend[i] = trendMap[day]
	}

	// Orders today — count all for today.
	db.Raw(`SELECT COUNT(*) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND created_at >= ?`,
		storeID, tenantID, todayStart).Scan(&stats.OrdersToday)

	// Orders by status — pending (confirmed), fulfilled, cancelled.
	db.Raw(`SELECT COUNT(*) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status = 'pending'`,
		storeID, tenantID).Scan(&stats.OrdersPending)
	db.Raw(`SELECT COUNT(*) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status = 'fulfilled'`,
		storeID, tenantID).Scan(&stats.OrdersFulfilled)
	db.Raw(`SELECT COUNT(*) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status = 'cancelled'`,
		storeID, tenantID).Scan(&stats.OrdersCancelled)

	// Customers total / new this week — graceful if table doesn't exist.
	if err := db.Raw(`SELECT COUNT(*) FROM customer_profiles
		WHERE store_id = ? AND tenant_id = ?`,
		storeID, tenantID).Scan(&stats.CustomersTotal).Error; err != nil {
		stats.CustomersTotal = 0
	}
	if err := db.Raw(`SELECT COUNT(*) FROM customer_profiles
		WHERE store_id = ? AND tenant_id = ? AND created_at >= ?`,
		storeID, tenantID, weekStart).Scan(&stats.CustomersNewThisWeek).Error; err != nil {
		stats.CustomersNewThisWeek = 0
	}

	// Pending reviews — graceful if table doesn't exist.
	if err := db.Raw(`SELECT COUNT(*) FROM reviews
		WHERE store_id = ? AND tenant_id = ? AND status = 'pending'`,
		storeID, tenantID).Scan(&stats.PendingReviews).Error; err != nil {
		stats.PendingReviews = 0
	}

	// Recent orders — last 5.
	var recentRows []struct {
		ID            uuid.UUID
		OrderNumber   string
		CustomerEmail string
		GrandTotal    float64
		Status        string
		CreatedAt     time.Time
	}
	db.Raw(`SELECT id, order_number, customer_email, grand_total, status, created_at
		FROM orders
		WHERE store_id = ? AND tenant_id = ?
		ORDER BY created_at DESC
		LIMIT 5`,
		storeID, tenantID).Scan(&recentRows)

	recentOrders := make([]RecentOrder, 0, len(recentRows))
	for _, r := range recentRows {
		recentOrders = append(recentOrders, RecentOrder{
			ID:            r.ID.String(),
			OrderNumber:   r.OrderNumber,
			CustomerEmail: r.CustomerEmail,
			GrandTotal:    r.GrandTotal,
			Status:        r.Status,
			CreatedAt:     r.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	// Top products — join order_items with products, sum revenue.
	var topRows []struct {
		ID        uuid.UUID
		Title     string
		Revenue   float64
		UnitsSold int64
		ImageURL  *string
	}
	db.Raw(`SELECT p.id, p.title,
			COALESCE(SUM(oi.line_total), 0) AS revenue,
			COALESCE(SUM(oi.quantity), 0) AS units_sold,
			(SELECT pm.url FROM product_media pm WHERE pm.product_id = p.id ORDER BY pm.position LIMIT 1) AS image_url
		FROM order_items oi
		JOIN products p ON p.id = oi.product_id
		JOIN orders o ON o.id = oi.order_id
		WHERE o.store_id = ? AND o.tenant_id = ? AND o.status != 'cancelled'
		GROUP BY p.id, p.title
		ORDER BY revenue DESC
		LIMIT 5`,
		storeID, tenantID).Scan(&topRows)

	topProducts := make([]TopProduct, 0, len(topRows))
	for _, r := range topRows {
		topProducts = append(topProducts, TopProduct{
			ID:        r.ID.String(),
			Title:     r.Title,
			Revenue:   r.Revenue,
			UnitsSold: r.UnitsSold,
			ImageURL:  r.ImageURL,
		})
	}

	// Low stock items.
	var lowRows []struct {
		ID                uuid.UUID
		Title             string
		VariantTitle      string
		Quantity          int
		LowStockThreshold int
	}
	db.Raw(`SELECT pv.id, p.title, pv.title AS variant_title,
			pv.inventory_quantity AS quantity,
			COALESCE(pv.low_stock_threshold, 10) AS low_stock_threshold
		FROM product_variants pv
		JOIN products p ON p.id = pv.product_id
		WHERE p.store_id = ? AND p.tenant_id = ?
		AND pv.inventory_quantity <= COALESCE(pv.low_stock_threshold, 10)
		AND pv.inventory_quantity > 0
		ORDER BY pv.inventory_quantity ASC
		LIMIT 10`,
		storeID, tenantID).Scan(&lowRows)

	lowStock := make([]LowStockItem, 0, len(lowRows))
	for _, r := range lowRows {
		lowStock = append(lowStock, LowStockItem{
			ID:                r.ID.String(),
			Title:             r.Title,
			VariantTitle:      r.VariantTitle,
			Quantity:          r.Quantity,
			LowStockThreshold: r.LowStockThreshold,
		})
	}

	// Setup checklist — individual EXISTS queries.
	var checklist SetupChecklist
	checklist.HasStore = true // always true if we got here (store middleware passed)

	db.Raw(`SELECT EXISTS(SELECT 1 FROM products WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasProduct)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM payment_settings WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasPaymentProvider)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM shipping_settings WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasShippingCarrier)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM tax_settings WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasTaxConfigured)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM custom_domains WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasCustomDomain)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM store_settings WHERE store_id = ? AND tenant_id = ? AND key = 'theme')`,
		storeID, tenantID).Scan(&checklist.HasStorefrontTheme)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM orders WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasTestOrder)

	resp := DashboardResponse{
		Stats:          stats,
		RecentOrders:   recentOrders,
		TopProducts:    topProducts,
		LowStock:       lowStock,
		SetupChecklist: checklist,
	}

	// Cache for 60s.
	h.cache.Store(cacheKey, &cacheEntry{
		data:      resp,
		expiresAt: time.Now().Add(60 * time.Second),
	})

	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, resp)
}
