# Dashboard D1 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship the admin dashboard landing page with stat cards (revenue, orders, customers), 7-day revenue sparkline, setup checklist derived from real data, recent orders, top products, and low stock alerts.

**Architecture:** New `internal/handlers/admin/dashboard.go` handler with aggregation queries. Single GET endpoint cached 60s. Recharts sparkline on frontend. No new migration — queries existing tables.

**Tech Stack:** Go 1.26, Gin, GORM (raw SQL for aggregations). Next.js 16, React 19, Recharts, Tailwind.

**Design Authority:** `docs/superpowers/specs/2026-04-10-dashboard-support-help-design.md` §4 (D1 — Dashboard), §2 (Sidebar Update), §7 (Security), §8 (Testing). `mark8ly/.impeccable.md` — Paper · Ink · Moss design context.

---

## Status

> **Pending.** All tasks open.

---

## Scope check

Adds `services/marketplace-api/internal/handlers/admin/dashboard.go` (single handler file with aggregation queries against existing tables). Wires the handler into `routes.go` and `main.go`. On the frontend: replaces the placeholder `apps/admin/app/dashboard/page.tsx` with a real data-driven dashboard consuming the new endpoint. Adds Recharts sparkline, stat cards, setup checklist, recent orders, top products, and low stock alerts. Updates sidebar navigation to remove Analytics section and add Support section per spec §2.

Spec sections authoritative for this milestone:
- Design spec §4.1 (API endpoint, response shape, queries)
- Design spec §4.2 (Setup checklist — items, queries, links, auto-hide logic)
- Design spec §4.3 (UI layout hierarchy)
- Design spec §4.4 (Design tokens, component styling)
- Design spec §2 (Sidebar navigation update)
- Design spec §7 (Security — scoped by store_id + tenant_id)
- Design spec §8 (Testing — D1 section)
- `mark8ly/.impeccable.md` — Paper · Ink · Moss design context

**Out of scope (deferred):**
- Real-time dashboard updates (WebSocket) — 60s cache is sufficient
- Advanced analytics (cohort analysis, funnel visualization, export)
- Customer count query (customer_profiles table may not exist yet — return 0 gracefully)
- Review count query (reviews table may not exist yet — return 0 gracefully)

---

## Decisions locked (from the spec — do NOT re-debate)

1. **Single endpoint:** `GET /admin/stores/:storeId/dashboard` returns all dashboard data in one response. No separate endpoints for stats, orders, products, etc.
2. **In-memory cache:** 60s TTL, keyed by store_id. No Redis. Simple `sync.Map` with timestamp.
3. **No new migration:** All queries against existing tables (orders, order_items, products, product_variants, payment_gateway_configs, shipping_carrier_configs, stores, supported_countries).
4. **Revenue uses `grand_total`:** From orders table, excluding cancelled orders.
5. **Sparkline library:** Recharts `<Line>` component. Single moss-700 stroke, no fill, no axes, no grid. ~80px tall.
6. **Checklist auto-hides:** When all 8 items complete, the checklist card is not rendered.
7. **Sidebar update:** Remove Analytics section entirely. Add Support section with Tickets + Help Center children. 6 sections total.
8. **Design system:** Paper · Ink · Moss tokens, Source Serif 4 for serif numerals, Source Sans 3 for body, `@tesserix/web` primitives, `@repo/ui` components. No new hex values.

---

## File structure produced by D1

### Modified backend files

```
services/marketplace-api/
  internal/handlers/admin/
    dashboard.go                   NEW — DashboardHandler + aggregation queries
    routes.go                      MODIFIED — add DashboardHandler to Deps, wire GET route
  cmd/marketplace-api/
    main.go                        MODIFIED — instantiate DashboardHandler, add to adminDeps
```

### Modified frontend files

```
apps/admin/
  app/dashboard/
    page.tsx                       MODIFIED — replace placeholder with real dashboard
  components/dashboard/
    StatCard.tsx                    NEW — reusable stat card component
    RevenueSparkline.tsx           NEW — Recharts 7-day sparkline
    SetupChecklist.tsx             NEW — checklist with progress bar
    RecentOrders.tsx               NEW — 5 most recent orders table
    TopProducts.tsx                NEW — 5 top products by revenue
    LowStockAlerts.tsx             NEW — low stock product variants
  components/shell/
    AdminShell.tsx                 MODIFIED — update sidebar navigation
  lib/api/
    marketplace-api.ts             MODIFIED — add dashboard types + fetch function
```

---

## Task 0 — Pre-flight checks

- [ ] **0.1** Verify `mark8ly/.impeccable.md` exists (design context file)
- [ ] **0.2** Verify `services/marketplace-api/internal/handlers/admin/routes.go` compiles — run `cd services/marketplace-api && go build ./...`
- [ ] **0.3** Verify `apps/admin` builds — run `cd apps/admin && npx next build` (or `npm run build`)
- [ ] **0.4** Verify Recharts is already a dependency in `apps/admin/package.json`. If not, run `cd apps/admin && npm install recharts`

---

## Task 1 — Backend: DashboardHandler with aggregation queries

Create `services/marketplace-api/internal/handlers/admin/dashboard.go`.

### 1.1 — Response types

- [ ] Define response structs matching spec §4.1 JSON shape:

```go
// File: services/marketplace-api/internal/handlers/admin/dashboard.go
package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// DashboardResponse is the top-level envelope for GET /dashboard.
type DashboardResponse struct {
	Stats          DashboardStats     `json:"stats"`
	RecentOrders   []RecentOrder      `json:"recent_orders"`
	TopProducts    []TopProduct       `json:"top_products"`
	LowStock       []LowStockItem     `json:"low_stock"`
	SetupChecklist SetupChecklist     `json:"setup_checklist"`
}

type DashboardStats struct {
	RevenueToday     string    `json:"revenue_today"`
	RevenueWeek      string    `json:"revenue_week"`
	RevenueMonth     string    `json:"revenue_month"`
	RevenueChangePct float64   `json:"revenue_change_pct"`
	RevenueTrend     []float64 `json:"revenue_trend"`
	OrdersToday      int       `json:"orders_today"`
	OrdersPending    int       `json:"orders_pending"`
	OrdersFulfilled  int       `json:"orders_fulfilled"`
	OrdersCancelled  int       `json:"orders_cancelled"`
	CustomersTotal   int       `json:"customers_total"`
	CustomersNewWeek int       `json:"customers_new_this_week"`
	PendingReviews   int       `json:"pending_reviews"`
}

type RecentOrder struct {
	ID            string `json:"id"`
	OrderNumber   string `json:"order_number"`
	CustomerEmail string `json:"customer_email"`
	GrandTotal    string `json:"grand_total"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

type TopProduct struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Revenue   string `json:"revenue"`
	UnitsSold int    `json:"units_sold"`
	ImageURL  string `json:"image_url"`
}

type LowStockItem struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	VariantTitle      string `json:"variant_title"`
	Quantity          int    `json:"quantity"`
	LowStockThreshold int   `json:"low_stock_threshold"`
}

type SetupChecklist struct {
	HasStore            bool `json:"has_store"`
	HasProduct          bool `json:"has_product"`
	HasPaymentProvider  bool `json:"has_payment_provider"`
	HasShippingCarrier  bool `json:"has_shipping_carrier"`
	HasTaxConfigured    bool `json:"has_tax_configured"`
	HasCustomDomain     bool `json:"has_custom_domain"`
	HasStorefrontTheme  bool `json:"has_storefront_theme"`
	HasTestOrder        bool `json:"has_test_order"`
}
```

### 1.2 — In-memory cache

- [ ] Add a simple TTL cache using `sync.Map`:

```go
// cacheEntry holds a dashboard response with a TTL.
type cacheEntry struct {
	data      DashboardResponse
	expiresAt time.Time
}

// DashboardHandler bundles dependencies for the admin dashboard endpoint.
type DashboardHandler struct {
	db     *gorm.DB
	logger *slog.Logger
	cache  sync.Map // map[string]cacheEntry keyed by storeID
}

// NewDashboardHandler constructs a DashboardHandler.
func NewDashboardHandler(db *gorm.DB, logger *slog.Logger) *DashboardHandler {
	return &DashboardHandler{db: db, logger: logger}
}

const dashboardCacheTTL = 60 * time.Second
```

### 1.3 — GET handler with cache check

- [ ] Implement the `Get` method:

```go
// Get handles GET /admin/stores/:storeId/dashboard.
func (h *DashboardHandler) Get(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	// Check cache.
	if entry, ok := h.cache.Load(storeID); ok {
		if ce, valid := entry.(cacheEntry); valid && time.Now().Before(ce.expiresAt) {
			c.JSON(http.StatusOK, ce.data)
			return
		}
	}

	resp, err := h.buildDashboard(c.Request.Context(), storeID, tenantID)
	if err != nil {
		h.logger.Error("dashboard: build failed", "err", err, "store_id", storeID)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			envelope("internal", "failed to load dashboard", nil))
		return
	}

	h.cache.Store(storeID, cacheEntry{data: resp, expiresAt: time.Now().Add(dashboardCacheTTL)})
	c.JSON(http.StatusOK, resp)
}
```

### 1.4 — Aggregation queries

- [ ] Implement `buildDashboard` with all sub-queries. Each sub-query is a private method for readability. All queries are scoped by `store_id` AND `tenant_id`:

```go
func (h *DashboardHandler) buildDashboard(ctx context.Context, storeID, tenantID string) (DashboardResponse, error) {
	var resp DashboardResponse
	var err error

	// Stats — revenue.
	resp.Stats, err = h.queryStats(ctx, storeID, tenantID)
	if err != nil {
		return resp, fmt.Errorf("stats: %w", err)
	}

	// Recent orders.
	resp.RecentOrders, err = h.queryRecentOrders(ctx, storeID, tenantID)
	if err != nil {
		return resp, fmt.Errorf("recent_orders: %w", err)
	}

	// Top products.
	resp.TopProducts, err = h.queryTopProducts(ctx, storeID, tenantID)
	if err != nil {
		return resp, fmt.Errorf("top_products: %w", err)
	}

	// Low stock.
	resp.LowStock, err = h.queryLowStock(ctx, storeID, tenantID)
	if err != nil {
		return resp, fmt.Errorf("low_stock: %w", err)
	}

	// Setup checklist.
	resp.SetupChecklist, err = h.queryChecklist(ctx, storeID, tenantID)
	if err != nil {
		return resp, fmt.Errorf("checklist: %w", err)
	}

	return resp, nil
}
```

### 1.5 — Revenue queries

- [ ] Implement revenue aggregation queries. Use `context.Context` from the request for DB calls:

```go
import "context"

func (h *DashboardHandler) queryStats(ctx context.Context, storeID, tenantID string) (DashboardStats, error) {
	var stats DashboardStats
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekStart := todayStart.AddDate(0, 0, -7)
	monthStart := todayStart.AddDate(0, -1, 0)
	prevWeekStart := weekStart.AddDate(0, 0, -7)

	db := h.db.WithContext(ctx)

	// Revenue today.
	var revToday decimal.Decimal
	if err := db.Raw(`SELECT COALESCE(SUM(grand_total), 0) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled' AND created_at >= ?`,
		storeID, tenantID, todayStart).Scan(&revToday).Error; err != nil {
		return stats, fmt.Errorf("revenue_today: %w", err)
	}
	stats.RevenueToday = revToday.StringFixed(2)

	// Revenue this week.
	var revWeek decimal.Decimal
	if err := db.Raw(`SELECT COALESCE(SUM(grand_total), 0) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled' AND created_at >= ?`,
		storeID, tenantID, weekStart).Scan(&revWeek).Error; err != nil {
		return stats, fmt.Errorf("revenue_week: %w", err)
	}
	stats.RevenueWeek = revWeek.StringFixed(2)

	// Revenue this month.
	var revMonth decimal.Decimal
	if err := db.Raw(`SELECT COALESCE(SUM(grand_total), 0) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled' AND created_at >= ?`,
		storeID, tenantID, monthStart).Scan(&revMonth).Error; err != nil {
		return stats, fmt.Errorf("revenue_month: %w", err)
	}
	stats.RevenueMonth = revMonth.StringFixed(2)

	// Revenue change pct — compare this week vs previous week.
	var revPrevWeek decimal.Decimal
	if err := db.Raw(`SELECT COALESCE(SUM(grand_total), 0) FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled'
		AND created_at >= ? AND created_at < ?`,
		storeID, tenantID, prevWeekStart, weekStart).Scan(&revPrevWeek).Error; err != nil {
		return stats, fmt.Errorf("revenue_prev_week: %w", err)
	}
	if !revPrevWeek.IsZero() {
		pct := revWeek.Sub(revPrevWeek).Div(revPrevWeek).Mul(decimal.NewFromInt(100))
		stats.RevenueChangePct, _ = pct.Float64()
	}

	// Revenue trend — 7 days, one value per day.
	stats.RevenueTrend = make([]float64, 7)
	type dayRevenue struct {
		Day     int
		Revenue decimal.Decimal
	}
	var dayRows []dayRevenue
	if err := db.Raw(`SELECT EXTRACT(DOW FROM created_at)::int AS day,
		COALESCE(SUM(grand_total), 0) AS revenue
		FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled'
		AND created_at >= ?
		GROUP BY DATE(created_at)
		ORDER BY DATE(created_at)`,
		storeID, tenantID, weekStart).Scan(&dayRows).Error; err != nil {
		return stats, fmt.Errorf("revenue_trend: %w", err)
	}
	// Map into the 7-slot array ordered by date.
	type trendRow struct {
		DayDate string          `gorm:"column:day_date"`
		Revenue decimal.Decimal `gorm:"column:revenue"`
	}
	var trendRows []trendRow
	if err := db.Raw(`SELECT DATE(created_at) AS day_date,
		COALESCE(SUM(grand_total), 0) AS revenue
		FROM orders
		WHERE store_id = ? AND tenant_id = ? AND status != 'cancelled'
		AND created_at >= ?
		GROUP BY DATE(created_at)
		ORDER BY DATE(created_at)`,
		storeID, tenantID, weekStart).Scan(&trendRows).Error; err != nil {
		return stats, fmt.Errorf("revenue_trend: %w", err)
	}
	// Build a date-indexed map and fill the 7-slot array.
	trendMap := make(map[string]decimal.Decimal, len(trendRows))
	for _, tr := range trendRows {
		trendMap[tr.DayDate] = tr.Revenue
	}
	for i := 0; i < 7; i++ {
		d := weekStart.AddDate(0, 0, i).Format("2006-01-02")
		if rev, ok := trendMap[d]; ok {
			stats.RevenueTrend[i], _ = rev.Float64()
		}
	}

	// Orders today — by status.
	type orderStatusCount struct {
		Status string
		Count  int
	}
	var statusCounts []orderStatusCount
	if err := db.Raw(`SELECT status, COUNT(*) AS count FROM orders
		WHERE store_id = ? AND tenant_id = ? AND created_at >= ?
		GROUP BY status`,
		storeID, tenantID, todayStart).Scan(&statusCounts).Error; err != nil {
		return stats, fmt.Errorf("orders_today: %w", err)
	}
	for _, sc := range statusCounts {
		stats.OrdersToday += sc.Count
		switch sc.Status {
		case "pending":
			stats.OrdersPending += sc.Count
		case "confirmed":
			stats.OrdersPending += sc.Count
		case "fulfilled":
			stats.OrdersFulfilled += sc.Count
		case "cancelled":
			stats.OrdersCancelled += sc.Count
		}
	}

	// Customers total — query customer_profiles if table exists, else 0.
	// Uses raw SQL with error swallowing since the table may not exist yet.
	var customerTotal int
	if err := db.Raw(`SELECT COUNT(*) FROM customer_profiles
		WHERE store_id = ? AND tenant_id = ?`,
		storeID, tenantID).Scan(&customerTotal).Error; err != nil {
		// Table may not exist — gracefully return 0.
		h.logger.Debug("dashboard: customer_profiles query failed (table may not exist)", "err", err)
		customerTotal = 0
	}
	stats.CustomersTotal = customerTotal

	// Customers new this week.
	var customersNewWeek int
	if err := db.Raw(`SELECT COUNT(*) FROM customer_profiles
		WHERE store_id = ? AND tenant_id = ? AND created_at >= ?`,
		storeID, tenantID, weekStart).Scan(&customersNewWeek).Error; err != nil {
		customersNewWeek = 0
	}
	stats.CustomersNewWeek = customersNewWeek

	// Pending reviews — query reviews if table exists, else 0.
	var pendingReviews int
	if err := db.Raw(`SELECT COUNT(*) FROM reviews
		WHERE store_id = ? AND tenant_id = ? AND status = 'pending'`,
		storeID, tenantID).Scan(&pendingReviews).Error; err != nil {
		h.logger.Debug("dashboard: reviews query failed (table may not exist)", "err", err)
		pendingReviews = 0
	}
	stats.PendingReviews = pendingReviews

	return stats, nil
}
```

**IMPORTANT:** The revenue trend query runs twice in the snippet above — the first `dayRows` block is dead code from an earlier draft. Delete the first `dayRows` block and only keep the `trendRows` block. The implementing agent must use only the `trendRows` approach.

### 1.6 — Recent orders query

- [ ] Implement:

```go
func (h *DashboardHandler) queryRecentOrders(ctx context.Context, storeID, tenantID string) ([]RecentOrder, error) {
	type row struct {
		ID            string          `gorm:"column:id"`
		OrderNumber   string          `gorm:"column:order_number"`
		CustomerEmail string          `gorm:"column:customer_email"`
		GrandTotal    decimal.Decimal `gorm:"column:grand_total"`
		Status        string          `gorm:"column:status"`
		CreatedAt     time.Time       `gorm:"column:created_at"`
	}
	var rows []row
	err := h.db.WithContext(ctx).Raw(`SELECT id, order_number, customer_email, grand_total, status, created_at
		FROM orders
		WHERE store_id = ? AND tenant_id = ?
		ORDER BY created_at DESC
		LIMIT 5`,
		storeID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]RecentOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, RecentOrder{
			ID:            r.ID,
			OrderNumber:   r.OrderNumber,
			CustomerEmail: r.CustomerEmail,
			GrandTotal:    r.GrandTotal.StringFixed(2),
			Status:        r.Status,
			CreatedAt:     r.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}
```

### 1.7 — Top products query

- [ ] Implement:

```go
func (h *DashboardHandler) queryTopProducts(ctx context.Context, storeID, tenantID string) ([]TopProduct, error) {
	type row struct {
		ID        string          `gorm:"column:id"`
		Title     string          `gorm:"column:title"`
		Revenue   decimal.Decimal `gorm:"column:revenue"`
		UnitsSold int             `gorm:"column:units_sold"`
		ImageURL  *string         `gorm:"column:image_url"`
	}
	var rows []row
	err := h.db.WithContext(ctx).Raw(`
		SELECT p.id, p.title,
		       COALESCE(SUM(oi.line_total), 0) AS revenue,
		       COALESCE(SUM(oi.quantity), 0) AS units_sold,
		       (SELECT pm.url FROM product_media pm WHERE pm.product_id = p.id ORDER BY pm.position LIMIT 1) AS image_url
		FROM products p
		JOIN order_items oi ON oi.product_id = p.id
		JOIN orders o ON o.id = oi.order_id AND o.status != 'cancelled'
		WHERE p.store_id = ? AND p.tenant_id = ?
		GROUP BY p.id, p.title
		ORDER BY revenue DESC
		LIMIT 5`,
		storeID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]TopProduct, 0, len(rows))
	for _, r := range rows {
		img := ""
		if r.ImageURL != nil {
			img = *r.ImageURL
		}
		out = append(out, TopProduct{
			ID:        r.ID,
			Title:     r.Title,
			Revenue:   r.Revenue.StringFixed(2),
			UnitsSold: r.UnitsSold,
			ImageURL:  img,
		})
	}
	return out, nil
}
```

### 1.8 — Low stock query

- [ ] Implement:

```go
func (h *DashboardHandler) queryLowStock(ctx context.Context, storeID, tenantID string) ([]LowStockItem, error) {
	type row struct {
		ID                string `gorm:"column:id"`
		Title             string `gorm:"column:title"`
		VariantTitle      string `gorm:"column:variant_title"`
		Quantity          int    `gorm:"column:quantity"`
		LowStockThreshold int   `gorm:"column:low_stock_threshold"`
	}
	var rows []row
	err := h.db.WithContext(ctx).Raw(`
		SELECT pv.id, p.title,
		       COALESCE(pv.sku, 'Default') AS variant_title,
		       pv.inventory_quantity AS quantity,
		       pv.low_stock_threshold
		FROM product_variants pv
		JOIN products p ON p.id = pv.product_id
		WHERE pv.store_id = ? AND pv.tenant_id = ?
		  AND pv.low_stock_threshold IS NOT NULL
		  AND pv.inventory_quantity <= pv.low_stock_threshold
		  AND pv.inventory_quantity > 0
		ORDER BY pv.inventory_quantity ASC
		LIMIT 10`,
		storeID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]LowStockItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, LowStockItem{
			ID:                r.ID,
			Title:             r.Title,
			VariantTitle:      r.VariantTitle,
			Quantity:          r.Quantity,
			LowStockThreshold: r.LowStockThreshold,
		})
	}
	return out, nil
}
```

### 1.9 — Setup checklist query

- [ ] Implement. Each item is an independent EXISTS query. Gracefully handle missing tables:

```go
func (h *DashboardHandler) queryChecklist(ctx context.Context, storeID, tenantID string) (SetupChecklist, error) {
	var cl SetupChecklist
	db := h.db.WithContext(ctx)

	// 1. Has store — always true if we're here (storeMiddleware validated).
	cl.HasStore = true

	// 2. Has product.
	var productCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM products WHERE store_id = ? AND tenant_id = ? LIMIT 1`,
		storeID, tenantID).Scan(&productCount).Error; err != nil {
		return cl, fmt.Errorf("checklist products: %w", err)
	}
	cl.HasProduct = productCount > 0

	// 3. Has active payment provider.
	var paymentCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM payment_gateway_configs
		WHERE store_id = ? AND tenant_id = ? AND is_active = true LIMIT 1`,
		storeID, tenantID).Scan(&paymentCount).Error; err != nil {
		// Table may not exist if payments migration hasn't run — treat as false.
		h.logger.Debug("dashboard: payment_gateway_configs query failed", "err", err)
		paymentCount = 0
	}
	cl.HasPaymentProvider = paymentCount > 0

	// 4. Has active shipping carrier.
	var shippingCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM shipping_carrier_configs
		WHERE store_id = ? AND tenant_id = ? AND enabled = true LIMIT 1`,
		storeID, tenantID).Scan(&shippingCount).Error; err != nil {
		h.logger.Debug("dashboard: shipping_carrier_configs query failed", "err", err)
		shippingCount = 0
	}
	cl.HasShippingCarrier = shippingCount > 0

	// 5. Tax configured — always true per spec (supported_countries seeded).
	cl.HasTaxConfigured = true

	// 6. Has custom domain.
	var domainCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM custom_domains
		WHERE store_id = ? AND tenant_id = ? AND status = 'active' LIMIT 1`,
		storeID, tenantID).Scan(&domainCount).Error; err != nil {
		h.logger.Debug("dashboard: custom_domains query failed", "err", err)
		domainCount = 0
	}
	cl.HasCustomDomain = domainCount > 0

	// 7. Has storefront theme — false until storefront customization ships.
	cl.HasStorefrontTheme = false

	// 8. Has test order.
	var orderCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM orders WHERE store_id = ? AND tenant_id = ? LIMIT 1`,
		storeID, tenantID).Scan(&orderCount).Error; err != nil {
		return cl, fmt.Errorf("checklist orders: %w", err)
	}
	cl.HasTestOrder = orderCount > 0

	return cl, nil
}
```

### 1.10 — Verify build

- [ ] Run `cd services/marketplace-api && go build ./...` — must compile with zero errors.

---

## Task 2 — Backend: Wire dashboard handler into routes and main

### 2.1 — Add DashboardHandler to Deps

- [ ] Edit `services/marketplace-api/internal/handlers/admin/routes.go`. Add `DashboardHandler` to the `Deps` struct:

```go
// In the Deps struct, add after CouponHandler:
DashboardHandler *DashboardHandler
```

### 2.2 — Wire the route

- [ ] In the `RegisterAdmin` function in `routes.go`, add the dashboard route inside the `storeRoute` group block. Place it before the `products` group:

```go
// Dashboard — D1. Placed before products group.
if deps.DashboardHandler != nil {
	storeRoute.GET("/dashboard",
		deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
		deps.DashboardHandler.Get)
}
```

### 2.3 — Instantiate in main.go

- [ ] Edit `services/marketplace-api/cmd/marketplace-api/main.go`. Inside the `if m == mode.Admin || m == mode.Both` block, after the coupon handler wiring and before `adminDeps = admin.Deps{`, add:

```go
// Dashboard handler (D1).
dashboardHandler := admin.NewDashboardHandler(conn, log)
```

- [ ] Add `DashboardHandler: dashboardHandler,` to the `adminDeps = admin.Deps{...}` struct literal.

### 2.4 — Verify build

- [ ] Run `cd services/marketplace-api && go build ./...` — must compile.

---

## Task 3 — Frontend: Dashboard API types and fetch function

### 3.1 — Add types to marketplace-api.ts

- [ ] Edit `apps/admin/lib/api/marketplace-api.ts`. Add the dashboard response types at the end of the file (before any existing default export, or at the bottom):

```typescript
// ─────────────────────────────────────────────────────────────────────────
// Dashboard D1
// ─────────────────────────────────────────────────────────────────────────

export interface DashboardStats {
  revenue_today: string;
  revenue_week: string;
  revenue_month: string;
  revenue_change_pct: number;
  revenue_trend: number[];
  orders_today: number;
  orders_pending: number;
  orders_fulfilled: number;
  orders_cancelled: number;
  customers_total: number;
  customers_new_this_week: number;
  pending_reviews: number;
}

export interface DashboardRecentOrder {
  id: string;
  order_number: string;
  customer_email: string;
  grand_total: string;
  status: string;
  created_at: string;
}

export interface DashboardTopProduct {
  id: string;
  title: string;
  revenue: string;
  units_sold: number;
  image_url: string;
}

export interface DashboardLowStockItem {
  id: string;
  title: string;
  variant_title: string;
  quantity: number;
  low_stock_threshold: number;
}

export interface DashboardSetupChecklist {
  has_store: boolean;
  has_product: boolean;
  has_payment_provider: boolean;
  has_shipping_carrier: boolean;
  has_tax_configured: boolean;
  has_custom_domain: boolean;
  has_storefront_theme: boolean;
  has_test_order: boolean;
}

export interface DashboardResponse {
  stats: DashboardStats;
  recent_orders: DashboardRecentOrder[];
  top_products: DashboardTopProduct[];
  low_stock: DashboardLowStockItem[];
  setup_checklist: DashboardSetupChecklist;
}
```

### 3.2 — Add fetch function

- [ ] Add the fetch function in `marketplace-api.ts`:

```typescript
/**
 * Fetches dashboard data for a store. Server-component only.
 * Returns null on 401/403/404, throws on unexpected errors.
 */
export async function fetchDashboard(
  storeId: string,
  session: SessionHeaders,
): Promise<DashboardResponse | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/dashboard`;
  const res = await fetch(url, {
    headers: {
      "X-User-Id": session.userId,
      "X-Tenant-Id": session.tenantId,
      "Content-Type": "application/json",
    },
    next: { revalidate: 60 },
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`Dashboard fetch failed: ${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<DashboardResponse>;
}
```

---

## Task 4 — Frontend: StatCard component

- [ ] Create `apps/admin/components/dashboard/StatCard.tsx`:

```tsx
interface StatCardProps {
  label: string;
  value: string;
  detail?: string;
  changePct?: number;
}

export function StatCard({ label, value, detail, changePct }: StatCardProps) {
  const isPositive = changePct !== undefined && changePct >= 0;
  const isNegative = changePct !== undefined && changePct < 0;

  return (
    <div className="rounded-md bg-background-elevated p-6 shadow-1">
      <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground/60">
        {label}
      </p>
      <div className="mt-3 flex items-baseline gap-3">
        <p
          className="font-serif text-4xl font-medium text-foreground"
          style={{ fontFeatureSettings: '"tnum" 1' }}
        >
          {value}
        </p>
        {changePct !== undefined && (
          <span
            className={`inline-flex rounded-full px-2 py-0.5 text-xs font-semibold ${
              isPositive
                ? "bg-moss-700/10 text-moss-700"
                : "bg-signal/10 text-signal"
            }`}
          >
            {isPositive ? "+" : ""}
            {changePct.toFixed(1)}%
          </span>
        )}
      </div>
      {detail && (
        <p className="mt-2 text-sm text-foreground/60">{detail}</p>
      )}
    </div>
  );
}
```

---

## Task 5 — Frontend: RevenueSparkline component

- [ ] Create `apps/admin/components/dashboard/RevenueSparkline.tsx`:

```tsx
"use client";

import { Line, LineChart, ResponsiveContainer, Tooltip } from "recharts";

interface RevenueSparklineProps {
  data: number[];
}

export function RevenueSparkline({ data }: RevenueSparklineProps) {
  // Build chart data — 7 days ending today.
  const today = new Date();
  const chartData = data.map((value, i) => {
    const date = new Date(today);
    date.setDate(date.getDate() - (6 - i));
    return {
      date: date.toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric" }),
      revenue: value,
    };
  });

  if (data.every((v) => v === 0)) {
    return (
      <div className="flex h-20 items-center justify-center rounded-md bg-background-elevated shadow-1">
        <p className="text-sm text-foreground/60">No revenue data yet</p>
      </div>
    );
  }

  return (
    <div className="rounded-md bg-background-elevated p-6 shadow-1">
      <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground/60">
        Revenue — Last 7 days
      </p>
      <div className="mt-4 h-20">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={chartData}>
            <Line
              type="monotone"
              dataKey="revenue"
              stroke="var(--moss-700, #2D4A2B)"
              strokeWidth={2}
              strokeLinecap="round"
              dot={false}
              activeDot={{ r: 4, fill: "var(--moss-700, #2D4A2B)" }}
            />
            <Tooltip
              contentStyle={{
                background: "var(--background-elevated, #FFFFFF)",
                border: "1px solid var(--border-subtle)",
                borderRadius: "6px",
                fontSize: "13px",
                padding: "8px 12px",
              }}
              formatter={(value: number) => [
                `$${value.toLocaleString("en-US", { minimumFractionDigits: 2 })}`,
                "Revenue",
              ]}
              labelFormatter={(label: string) => label}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
```

---

## Task 6 — Frontend: SetupChecklist component

- [ ] Create `apps/admin/components/dashboard/SetupChecklist.tsx`:

```tsx
"use client";

import { useState } from "react";
import Link from "next/link";
import { Check, ChevronDown } from "lucide-react";

import type { DashboardSetupChecklist } from "@/lib/api/marketplace-api";

interface SetupChecklistProps {
  checklist: DashboardSetupChecklist;
}

interface ChecklistStep {
  key: keyof DashboardSetupChecklist;
  label: string;
  link: string;
}

const steps: ChecklistStep[] = [
  { key: "has_store", label: "Create your store", link: "/settings/stores" },
  { key: "has_product", label: "Add your first product", link: "/products/new" },
  { key: "has_payment_provider", label: "Configure payment provider", link: "/settings/payments" },
  { key: "has_shipping_carrier", label: "Configure shipping carrier", link: "/settings/shipping" },
  { key: "has_tax_configured", label: "Set up tax settings", link: "/settings/tax" },
  { key: "has_custom_domain", label: "Connect a custom domain", link: "/settings/domains" },
  { key: "has_storefront_theme", label: "Customize your storefront", link: "/settings/storefront" },
  { key: "has_test_order", label: "Place a test order", link: "/products" },
];

export function SetupChecklist({ checklist }: SetupChecklistProps) {
  const [isCollapsed, setIsCollapsed] = useState(false);

  const completedCount = steps.filter((s) => checklist[s.key]).length;
  const allComplete = completedCount === steps.length;
  const progressPct = (completedCount / steps.length) * 100;

  // Auto-hide when all complete.
  if (allComplete) return null;

  return (
    <div className="rounded-md bg-background-elevated p-6 shadow-1">
      <button
        type="button"
        onClick={() => setIsCollapsed((prev) => !prev)}
        className="flex w-full items-center justify-between"
      >
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground/60">
            Setup checklist
          </p>
          <p className="mt-1 text-sm text-foreground/60">
            {completedCount} of {steps.length} complete
          </p>
        </div>
        <ChevronDown
          className={`h-4 w-4 text-foreground/40 transition-transform ${
            isCollapsed ? "-rotate-90" : ""
          }`}
        />
      </button>

      {/* Progress bar */}
      <div className="mt-4 h-1.5 w-full overflow-hidden rounded-full bg-ink-900/10">
        <div
          className="h-full rounded-full bg-moss-700 transition-all duration-300"
          style={{ width: `${progressPct}%` }}
        />
      </div>

      {!isCollapsed && (
        <ul className="mt-5 divide-y divide-border-subtle">
          {steps.map((step) => {
            const done = checklist[step.key];
            return (
              <li key={step.key} className="flex items-center gap-4 py-3 first:pt-0 last:pb-0">
                <div
                  className={`flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full ${
                    done ? "bg-moss-700 text-white" : "border border-ink-900/20"
                  }`}
                >
                  {done && <Check className="h-3.5 w-3.5" />}
                </div>
                <div className="flex-1">
                  <span
                    className={`text-sm ${
                      done ? "text-foreground/60 line-through" : "text-foreground"
                    }`}
                  >
                    {step.label}
                  </span>
                </div>
                {!done && (
                  <Link
                    href={step.link}
                    className="text-xs font-medium text-moss-700 hover:underline"
                  >
                    Set up
                  </Link>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
```

---

## Task 7 — Frontend: RecentOrders component

- [ ] Create `apps/admin/components/dashboard/RecentOrders.tsx`:

```tsx
import Link from "next/link";

import type { DashboardRecentOrder } from "@/lib/api/marketplace-api";

interface RecentOrdersProps {
  orders: DashboardRecentOrder[];
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}

const statusColors: Record<string, string> = {
  pending: "bg-amber-100 text-amber-800",
  confirmed: "bg-moss-700/10 text-moss-700",
  fulfilled: "bg-moss-700 text-white",
  cancelled: "bg-ink-900/10 text-ink-900/60",
};

export function RecentOrders({ orders }: RecentOrdersProps) {
  if (orders.length === 0) {
    return (
      <div className="rounded-md bg-background-elevated p-6 shadow-1">
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground/60">
          Recent orders
        </p>
        <p className="mt-6 text-sm text-foreground/60">
          No orders yet. They will appear here when customers start buying.
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-md bg-background-elevated p-6 shadow-1">
      <div className="flex items-center justify-between">
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground/60">
          Recent orders
        </p>
        <Link href="/orders" className="text-xs font-medium text-moss-700 hover:underline">
          View all
        </Link>
      </div>
      <ul className="mt-4 divide-y divide-border-subtle">
        {orders.map((order) => (
          <li key={order.id} className="flex items-center justify-between py-3 first:pt-0 last:pb-0">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <Link
                  href={`/orders/${order.id}`}
                  className="text-sm font-medium text-foreground hover:text-moss-700"
                >
                  {order.order_number}
                </Link>
                <span
                  className={`inline-flex rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase ${
                    statusColors[order.status] ?? "bg-ink-900/10 text-ink-900"
                  }`}
                >
                  {order.status}
                </span>
              </div>
              <p className="mt-0.5 truncate text-xs text-foreground/60">
                {order.customer_email}
              </p>
            </div>
            <div className="text-right">
              <p
                className="font-serif text-sm font-medium text-foreground"
                style={{ fontFeatureSettings: '"tnum" 1' }}
              >
                ${order.grand_total}
              </p>
              <p className="mt-0.5 text-xs text-foreground/60">{timeAgo(order.created_at)}</p>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

---

## Task 8 — Frontend: TopProducts component

- [ ] Create `apps/admin/components/dashboard/TopProducts.tsx`:

```tsx
import Link from "next/link";
import Image from "next/image";

import type { DashboardTopProduct } from "@/lib/api/marketplace-api";

interface TopProductsProps {
  products: DashboardTopProduct[];
}

export function TopProducts({ products }: TopProductsProps) {
  if (products.length === 0) {
    return (
      <div className="rounded-md bg-background-elevated p-6 shadow-1">
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground/60">
          Top products
        </p>
        <p className="mt-6 text-sm text-foreground/60">
          No sales data yet. Top products by revenue will appear here.
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-md bg-background-elevated p-6 shadow-1">
      <div className="flex items-center justify-between">
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground/60">
          Top products
        </p>
        <Link href="/products" className="text-xs font-medium text-moss-700 hover:underline">
          View all
        </Link>
      </div>
      <ul className="mt-4 divide-y divide-border-subtle">
        {products.map((product) => (
          <li key={product.id} className="flex items-center gap-4 py-3 first:pt-0 last:pb-0">
            <div className="h-10 w-10 flex-shrink-0 overflow-hidden rounded bg-paper-100">
              {product.image_url ? (
                <Image
                  src={product.image_url}
                  alt={product.title}
                  width={40}
                  height={40}
                  className="h-full w-full object-cover"
                />
              ) : (
                <div className="flex h-full w-full items-center justify-center text-xs text-foreground/30">
                  --
                </div>
              )}
            </div>
            <div className="min-w-0 flex-1">
              <Link
                href={`/products/${product.id}`}
                className="truncate text-sm font-medium text-foreground hover:text-moss-700"
              >
                {product.title}
              </Link>
              <p className="text-xs text-foreground/60">
                {product.units_sold} units sold
              </p>
            </div>
            <p
              className="font-serif text-sm font-medium text-foreground"
              style={{ fontFeatureSettings: '"tnum" 1' }}
            >
              ${product.revenue}
            </p>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

---

## Task 9 — Frontend: LowStockAlerts component

- [ ] Create `apps/admin/components/dashboard/LowStockAlerts.tsx`:

```tsx
import Link from "next/link";

import type { DashboardLowStockItem } from "@/lib/api/marketplace-api";

interface LowStockAlertsProps {
  items: DashboardLowStockItem[];
}

export function LowStockAlerts({ items }: LowStockAlertsProps) {
  if (items.length === 0) return null;

  return (
    <div className="rounded-md bg-background-elevated p-6 shadow-1">
      <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-signal">
        Low stock alerts
      </p>
      <ul className="mt-4 divide-y divide-border-subtle">
        {items.map((item) => (
          <li
            key={item.id}
            className="flex items-center justify-between border-l-2 border-signal py-3 pl-4 first:pt-0 last:pb-0"
          >
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-foreground">{item.title}</p>
              <p className="text-xs text-foreground/60">{item.variant_title}</p>
            </div>
            <div className="text-right">
              <p className="text-sm font-medium text-signal">
                {item.quantity} left
              </p>
              <p className="text-xs text-foreground/60">
                threshold: {item.low_stock_threshold}
              </p>
            </div>
            <Link
              href={`/products/${item.id}`}
              className="ml-4 text-xs font-medium text-moss-700 hover:underline"
            >
              View
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

---

## Task 10 — Frontend: Replace dashboard page

- [ ] Replace the entire contents of `apps/admin/app/dashboard/page.tsx` with the real dashboard. The page is a server component that fetches data and renders the dashboard components:

```tsx
import { headers } from "next/headers";

import { AdminShell } from "@/components/shell/AdminShell";
import {
  fetchTenant,
  listMemberTenants,
  type TenantRole,
} from "@/lib/api/platform-api";
import { fetchDashboard } from "@/lib/api/marketplace-api";

import { StatCard } from "@/components/dashboard/StatCard";
import { RevenueSparkline } from "@/components/dashboard/RevenueSparkline";
import { SetupChecklist } from "@/components/dashboard/SetupChecklist";
import { RecentOrders } from "@/components/dashboard/RecentOrders";
import { TopProducts } from "@/components/dashboard/TopProducts";
import { LowStockAlerts } from "@/components/dashboard/LowStockAlerts";

export default async function DashboardPage() {
  const h = await headers();
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const userId = h.get("x-session-user-id") ?? "";
  const email = h.get("x-session-email") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = h.get("x-session-store-id") ?? "";

  const [tenant, memberships, dashboard] = await Promise.all([
    fetchTenant(tenantId),
    userId ? listMemberTenants(userId).catch(() => []) : Promise.resolve([]),
    storeId
      ? fetchDashboard(storeId, { userId, tenantId }).catch(() => null)
      : Promise.resolve(null),
  ]);

  const tenantName = tenant?.name ?? "your store";
  const stats = dashboard?.stats;
  const hasData = dashboard !== null;

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto max-w-7xl space-y-8">
        {/* Welcome header */}
        <section>
          <p className="eyebrow">
            Welcome back{email ? `, ${email.split("@")[0]}` : ""}
          </p>
          <h1 className="mt-1 font-serif text-4xl font-medium leading-tight tracking-tight text-foreground sm:text-5xl">
            {tenant ? `${tenant.name}` : "Dashboard"}
          </h1>
        </section>

        {/* Setup checklist — hides when all complete */}
        {hasData && (
          <SetupChecklist checklist={dashboard.setup_checklist} />
        )}

        {/* Stat cards row */}
        <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard
            label="Revenue today"
            value={stats ? `$${stats.revenue_today}` : "$0.00"}
            changePct={stats?.revenue_change_pct}
          />
          <StatCard
            label="Orders today"
            value={stats ? String(stats.orders_today) : "0"}
            detail={
              stats
                ? `${stats.orders_pending} pending · ${stats.orders_fulfilled} fulfilled`
                : undefined
            }
          />
          <StatCard
            label="Total customers"
            value={stats ? String(stats.customers_total) : "0"}
            detail={
              stats && stats.customers_new_this_week > 0
                ? `+${stats.customers_new_this_week} this week`
                : undefined
            }
          />
          <StatCard
            label="Pending reviews"
            value={stats ? String(stats.pending_reviews) : "0"}
            detail={
              stats && stats.pending_reviews === 0
                ? "No pending reviews"
                : undefined
            }
          />
        </section>

        {/* Revenue sparkline */}
        {hasData && (
          <RevenueSparkline data={stats?.revenue_trend ?? []} />
        )}

        {/* Two-column: Recent orders + Top products */}
        <section className="grid gap-4 lg:grid-cols-2">
          <RecentOrders orders={dashboard?.recent_orders ?? []} />
          <TopProducts products={dashboard?.top_products ?? []} />
        </section>

        {/* Low stock alerts — conditional */}
        {hasData && (
          <LowStockAlerts items={dashboard.low_stock} />
        )}
      </div>
    </AdminShell>
  );
}
```

**IMPORTANT: storeId resolution.** The current page reads `x-session-store-id` from headers. If this header is not set by the existing middleware, the implementing agent must check `apps/admin/middleware.ts` to see how the store is resolved. If no `x-session-store-id` header exists, the agent should either:
1. Extract the first store from a stores list API call, OR
2. Read it from the tenant/session data already available.

The agent must adapt the storeId resolution to match the actual middleware behavior. Do NOT hardcode a storeId.

---

## Task 11 — Frontend: Update sidebar navigation

- [ ] Edit `apps/admin/components/shell/AdminShell.tsx`. Replace the `navigation` array with the updated version per spec §2.

Remove the `analytics` section entirely. Update `marketing` children to use real routes. Add `support` section with children. Update `settings` children to include Domains, Subscription, Account, Audit Logs, Notifications.

```typescript
const navigation: NavSection[] = [
  {
    key: "catalog",
    label: "Products",
    icon: Package,
    href: "/products",
  },
  {
    key: "orders",
    label: "Orders",
    icon: ShoppingCart,
    children: [
      { label: "All Orders", href: "/orders" },
      { label: "Returns & Refunds", href: "/orders/returns" },
      { label: "Abandoned Carts", href: "/orders/abandoned-carts" },
    ],
  },
  {
    key: "customers",
    label: "Customers",
    icon: Users,
    children: [
      { label: "All Customers", href: "/customers" },
      { label: "Reviews", href: "/customers/reviews" },
    ],
  },
  {
    key: "marketing",
    label: "Marketing",
    icon: Megaphone,
    children: [
      { label: "Coupons", href: "/marketing/coupons" },
      { label: "Gift Cards", href: "/marketing/gift-cards" },
      { label: "Loyalty", href: "/marketing/loyalty" },
      { label: "Campaigns", href: "/marketing/campaigns" },
    ],
  },
  {
    key: "support",
    label: "Support",
    icon: HelpCircle,
    children: [
      { label: "Tickets", href: "/support/tickets" },
      { label: "Help Center", href: "/support/help" },
    ],
  },
  {
    key: "settings",
    label: "Settings",
    icon: Settings,
    children: [
      { label: "Store Settings", href: "/settings/general" },
      { label: "Storefront", href: "/settings/storefront" },
      { label: "Stores", href: "/settings/stores" },
      { label: "Team", href: "/settings/team" },
      { label: "Payments", href: "/settings/payments" },
      { label: "Shipping", href: "/settings/shipping" },
      { label: "Tax", href: "/settings/tax" },
      { label: "Domains", href: "/settings/domains" },
      { label: "Subscription", href: "/settings/subscription" },
      { label: "Account", href: "/settings/account" },
      { label: "Audit Logs", href: "/settings/audit-logs" },
      { label: "Notifications", href: "/settings/notifications" },
    ],
  },
];
```

- [ ] Update `canonicalChildLabelBySection` — remove the `analytics` entry:

```typescript
const canonicalChildLabelBySection: Record<string, string> = {
  catalog: "Products",
  orders: "All Orders",
  customers: "All Customers",
  marketing: "Coupons",
  support: "Tickets",
  settings: "Store Settings",
};
```

- [ ] Update `getPageTitle` — add support for new routes:

Add these cases before the final `return` in `getPageTitle`:

```typescript
if (pathname.startsWith("/marketing")) {
  return { eyebrow: "Marketing", title: "Marketing" };
}
if (pathname.startsWith("/support/tickets")) {
  return { eyebrow: "Support", title: "Tickets" };
}
if (pathname.startsWith("/support/help")) {
  return { eyebrow: "Support", title: "Help Center" };
}
if (pathname.startsWith("/support")) {
  return { eyebrow: "Support", title: "Support" };
}
```

- [ ] Update `getActiveSectionKey` — the current function returns `null` for `/dashboard`. This is correct since dashboard is the home page and no sidebar section should be highlighted. No change needed here.

- [ ] Remove the `BarChart3` icon import from lucide-react if it is no longer used after removing the analytics section. Check if any other code references it before removing.

---

## Task 12 — Install Recharts (if needed)

- [ ] Check `apps/admin/package.json` for `recharts`. If absent:

```bash
cd apps/admin && npm install recharts
```

Recharts is the only new dependency. No other packages needed.

---

## Task 13 — Verify full build

- [ ] Backend: `cd services/marketplace-api && go build ./...`
- [ ] Frontend: `cd apps/admin && npx next build`
- [ ] Fix any type errors, import issues, or build failures.

---

## Task 14 — Manual smoke test

- [ ] Start the dev stack: `make dev` (or equivalent docker-compose up)
- [ ] Navigate to `/dashboard` — verify:
  - Setup checklist renders with progress bar
  - Stat cards show zeroes for a fresh store
  - Sparkline shows "No revenue data yet" for empty store
  - Recent orders shows empty state
  - Top products shows empty state
  - Low stock alerts section is hidden (no low stock items)
  - Sidebar shows 6 sections: Products, Orders, Customers, Marketing, Support, Settings
  - Analytics section is gone from sidebar
  - Support section shows Tickets + Help Center children

---

## Task 15 — Backend unit tests

- [ ] Create `services/marketplace-api/internal/handlers/admin/dashboard_test.go` with tests:

1. **TestDashboardHandler_Get_EmptyStore** — Seed a store with no orders/products. Verify response has zero stats, empty arrays, and correct checklist (has_store=true, rest false).
2. **TestDashboardHandler_Get_WithData** — Seed orders, products, variants with low stock. Verify revenue sums, order counts, top products ranked correctly, low stock items present.
3. **TestDashboardHandler_Get_CacheHit** — Call twice within 60s. Second call should not hit the database (mock or check query count).
4. **TestDashboardHandler_Get_GracefulMissingTables** — Verify customer_profiles and reviews queries fail gracefully when tables don't exist (stats return 0, no 500 error).

Follow the existing test pattern in the codebase (use `testing` + `testify`, set up test DB with GORM).

---

## Dependency graph

```
Task 0 (pre-flight)
  └─> Task 1 (backend handler) ──> Task 2 (backend wiring)
  └─> Task 12 (install recharts)
       │
       ├─> Task 4 (StatCard)
       ├─> Task 5 (RevenueSparkline)
       ├─> Task 6 (SetupChecklist)
       ├─> Task 7 (RecentOrders)
       ├─> Task 8 (TopProducts)
       └─> Task 9 (LowStockAlerts)
              │
              └─> Task 3 (API types) ──> Task 10 (dashboard page)
                                         Task 11 (sidebar update)
                                           │
                                           └─> Task 13 (verify build)
                                                 └─> Task 14 (smoke test)
                                                       └─> Task 15 (tests)
```

**Parallelizable:** Tasks 4-9 (all dashboard components) can run in parallel. Tasks 1-2 (backend) can run in parallel with Tasks 4-9+11+12 (frontend). Task 3 must complete before Task 10.

---

## Landmines

1. **storeId in dashboard page:** The current placeholder page does NOT read a storeId. The implementing agent must determine how the admin app resolves the active store (likely from middleware or a session header). Check `apps/admin/middleware.ts` for `x-session-store-id` or equivalent.

2. **Missing tables:** `customer_profiles`, `reviews`, `custom_domains`, `storefront_themes` may not exist in the current schema. All queries against these tables MUST gracefully handle errors (return 0/false, log at debug level, never 500).

3. **product_variants tenant scoping:** The `product_variants` table has both `store_id` and `tenant_id` columns. The low stock query must filter by both.

4. **Recharts bundle size:** Recharts is only used on the dashboard page. If tree-shaking is insufficient, consider dynamic import (`next/dynamic`) with `ssr: false` for the sparkline component.

5. **Revenue trend date alignment:** The 7-day trend must produce exactly 7 values. Days with no orders must show 0, not be omitted. The `trendMap` approach in Task 1.5 handles this.

6. **Sidebar `BarChart3` import:** After removing the Analytics section, the `BarChart3` import from lucide-react becomes unused. Remove it to avoid lint warnings.

7. **`getActiveSectionKey` for /dashboard:** Currently returns `null` for `/dashboard`, which means no sidebar section is highlighted. This is the correct behavior — dashboard is the home page, not a section.

8. **Cache invalidation:** The 60s in-memory cache has no explicit invalidation. If a merchant adds their first product and immediately returns to the dashboard, the checklist may still show "Add your first product" for up to 60s. This is acceptable per spec ("60s cache is sufficient").
