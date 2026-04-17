// Package admin — metrics.go: range-scoped analytics endpoints that power
// the dashboard's tabbed analytics band (Sales / Orders / Customers /
// Reviews). Each endpoint accepts ?range=7d|30d|90d and returns a zero-filled
// time series plus summary KPIs. Results are cached per (tab, store, range)
// for 5 minutes to absorb rapid range-switching without hammering the DB.
package admin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ---------- Range parsing ----------

// rangeWindow is the resolved UTC window for a ?range= query param.
// Start is inclusive (beginning of first day), End is exclusive
// (beginning of the day after today), so `placed_at >= Start AND placed_at
// < End` captures the intended calendar days.
type rangeWindow struct {
	days  int
	start time.Time // inclusive
	end   time.Time // exclusive
}

// prev returns the matching window immediately preceding this one, used
// for prior-period delta calculations (e.g. AOV change).
func (r rangeWindow) prev() rangeWindow {
	return rangeWindow{
		days:  r.days,
		start: r.start.AddDate(0, 0, -r.days),
		end:   r.start,
	}
}

func parseRange(c *gin.Context) (rangeWindow, string, error) {
	raw := c.DefaultQuery("range", "30d")
	days, ok := map[string]int{"7d": 7, "30d": 30, "90d": 90}[raw]
	if !ok {
		return rangeWindow{}, "", apperrors.ValidationFailed("range", "must be one of: 7d, 30d, 90d")
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	tomorrow := today.Add(24 * time.Hour)
	start := today.AddDate(0, 0, -(days - 1))
	return rangeWindow{days: days, start: start, end: tomorrow}, raw, nil
}

// ---------- Cache ----------

type metricsCacheEntry struct {
	payload   any
	expiresAt time.Time
}

const metricsCacheTTL = 5 * time.Minute

func (h *DashboardHandler) getCachedMetrics(key string) (any, bool) {
	val, ok := h.metricsCache.Load(key)
	if !ok {
		return nil, false
	}
	entry := val.(*metricsCacheEntry)
	if time.Now().After(entry.expiresAt) {
		h.metricsCache.Delete(key)
		return nil, false
	}
	return entry.payload, true
}

func (h *DashboardHandler) setCachedMetrics(key string, payload any) {
	h.metricsCache.Store(key, &metricsCacheEntry{
		payload:   payload,
		expiresAt: time.Now().Add(metricsCacheTTL),
	})
}

// ---------- Shared DTOs ----------

// TimeSeriesPoint is a single (date, value) pair. Dates are YYYY-MM-DD UTC.
// Float64 is used for both currency and count series so the frontend has a
// single shape to consume.
type TimeSeriesPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// resolveStoreAndTenant parses the :storeId path param and tenant_id from
// the Gin context. Consolidates the boilerplate used by every handler.
func resolveStoreAndTenant(c *gin.Context) (uuid.UUID, uuid.UUID, error) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		return uuid.Nil, uuid.Nil, apperrors.ValidationFailed("storeId", "invalid uuid")
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, apperrors.ValidationFailed("tenant_id", "invalid uuid")
	}
	return storeID, tenantID, nil
}

// zeroFilledSeries issues a PostgreSQL generate_series-backed query that
// returns one row per calendar day in the window, with aggregated value.
// aggSQL must be a COALESCE expression over an aggregate (e.g.
// `COALESCE(SUM(o.grand_total), 0)`). tableAlias must match the JOIN alias.
// The caller owns the JOIN + WHERE construction via joinSQL and extraWhere.
func zeroFilledSeries(
	ctx context.Context,
	db *gorm.DB,
	aggSQL, joinSQL, extraWhere string,
	storeID, tenantID uuid.UUID,
	win rangeWindow,
) ([]TimeSeriesPoint, error) {
	var rows []struct {
		Day   time.Time
		Value float64
	}
	// Inclusive start, inclusive last day — generate_series with step '1 day'
	// produces the count-1 days between start and end. We pass win.end - 1 day
	// as the last day for the series grid.
	lastDay := win.end.Add(-24 * time.Hour)
	sql := fmt.Sprintf(`
		SELECT gs.day::date AS day, %s AS value
		FROM generate_series($3::date, $4::date, '1 day'::interval) AS gs(day)
		LEFT JOIN %s
			ON DATE(source.placed_at) = gs.day::date
			AND source.store_id = $1
			AND source.tenant_id = $2
			%s
		GROUP BY gs.day
		ORDER BY gs.day
	`, aggSQL, joinSQL, extraWhere)

	if err := db.WithContext(ctx).Raw(sql, storeID, tenantID, win.start, lastDay).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]TimeSeriesPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, TimeSeriesPoint{
			Date:  r.Day.Format("2006-01-02"),
			Value: r.Value,
		})
	}
	return out, nil
}

// ---------- Sales ----------

// SalesMetricsResponse powers the "Sales" analytics tab.
type SalesMetricsResponse struct {
	Range               string            `json:"range"`
	RevenueSeries       []TimeSeriesPoint `json:"revenue_series"`         // gross
	NetRevenueSeries    []TimeSeriesPoint `json:"net_revenue_series"`     // gross - refunded
	GrossRevenue        float64           `json:"gross_revenue"`
	TotalRefunded       float64           `json:"total_refunded"`
	NetRevenue          float64           `json:"net_revenue"`
	AOV                 float64           `json:"aov"`
	AOVPrev             float64           `json:"aov_prev"`
	AOVDeltaPct         *float64          `json:"aov_delta_pct"` // nil when prev==0
	CouponRedemptions   int64             `json:"coupon_redemptions"`
	CouponDiscountTotal float64           `json:"coupon_discount_total"`
}

// GetSalesMetrics handles GET /admin/stores/:storeId/dashboard/metrics/sales.
func (h *DashboardHandler) GetSalesMetrics(c *gin.Context) {
	storeID, tenantID, err := resolveStoreAndTenant(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	win, raw, err := parseRange(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	key := fmt.Sprintf("sales:%s:%s:%s", tenantID, storeID, raw)
	if cached, ok := h.getCachedMetrics(key); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	ctx := c.Request.Context()
	db := h.db.WithContext(ctx)
	resp := SalesMetricsResponse{Range: raw}

	// Gross revenue series.
	revSeries, err := zeroFilledSeries(ctx, h.db,
		`COALESCE(SUM(source.grand_total), 0)`,
		`orders AS source`,
		`AND source.status != 'cancelled' AND source.deleted_at IS NULL`,
		storeID, tenantID, win)
	if err != nil {
		RespondErr(c, fmt.Errorf("sales revenue series: %w", err), h.logger)
		return
	}
	resp.RevenueSeries = revSeries

	// Net revenue series (gross - refunded).
	netSeries, err := zeroFilledSeries(ctx, h.db,
		`COALESCE(SUM(source.grand_total - source.refunded_amount), 0)`,
		`orders AS source`,
		`AND source.status != 'cancelled' AND source.deleted_at IS NULL`,
		storeID, tenantID, win)
	if err != nil {
		RespondErr(c, fmt.Errorf("sales net revenue series: %w", err), h.logger)
		return
	}
	resp.NetRevenueSeries = netSeries

	// Totals + AOV for current period.
	var totals struct {
		Gross    float64
		Refunded float64
		Net      float64
		AOV      float64
	}
	db.Raw(`
		SELECT
			COALESCE(SUM(grand_total), 0)                      AS gross,
			COALESCE(SUM(refunded_amount), 0)                  AS refunded,
			COALESCE(SUM(grand_total - refunded_amount), 0)    AS net,
			COALESCE(AVG(grand_total), 0)                      AS aov
		FROM orders
		WHERE store_id = ? AND tenant_id = ?
			AND status != 'cancelled' AND deleted_at IS NULL
			AND placed_at >= ? AND placed_at < ?
	`, storeID, tenantID, win.start, win.end).Scan(&totals)
	resp.GrossRevenue = totals.Gross
	resp.TotalRefunded = totals.Refunded
	resp.NetRevenue = totals.Net
	resp.AOV = totals.AOV

	// AOV for the prior period (for delta).
	prev := win.prev()
	var prevAOV float64
	db.Raw(`
		SELECT COALESCE(AVG(grand_total), 0)
		FROM orders
		WHERE store_id = ? AND tenant_id = ?
			AND status != 'cancelled' AND deleted_at IS NULL
			AND placed_at >= ? AND placed_at < ?
	`, storeID, tenantID, prev.start, prev.end).Scan(&prevAOV)
	resp.AOVPrev = prevAOV
	if prevAOV > 0 {
		delta := ((totals.AOV - prevAOV) / prevAOV) * 100
		resp.AOVDeltaPct = &delta
	}

	// Coupon redemptions + discount total.
	var coupon struct {
		Count int64
		Total float64
	}
	// coupon_usage may not exist in every deployment; swallow errors silently.
	if err := db.Raw(`
		SELECT COUNT(*) AS count, COALESCE(SUM(cu.discount_amount), 0) AS total
		FROM coupon_usage cu
		JOIN coupons c ON c.id = cu.coupon_id
		WHERE c.store_id = ? AND c.tenant_id = ?
			AND cu.created_at >= ? AND cu.created_at < ?
	`, storeID, tenantID, win.start, win.end).Scan(&coupon).Error; err != nil {
		h.logger.Debug("coupon usage query skipped", "err", err)
	}
	resp.CouponRedemptions = coupon.Count
	resp.CouponDiscountTotal = coupon.Total

	h.setCachedMetrics(key, resp)
	c.JSON(http.StatusOK, resp)
}

// ---------- Orders ----------

// OrderStatusBreakdown is the per-status count for the range.
type OrderStatusBreakdown struct {
	Pending    int64 `json:"pending"`
	Confirmed  int64 `json:"confirmed"`
	InProgress int64 `json:"in_progress"`
	Fulfilled  int64 `json:"fulfilled"`
	Cancelled  int64 `json:"cancelled"`
	Returned   int64 `json:"returned"`
	Refunded   int64 `json:"refunded"`
}

// OrderStatusSeriesPoint is a daily row with stacked status counts.
type OrderStatusSeriesPoint struct {
	Date       string `json:"date"`
	Pending    int64  `json:"pending"`
	Confirmed  int64  `json:"confirmed"`
	InProgress int64  `json:"in_progress"`
	Fulfilled  int64  `json:"fulfilled"`
	Cancelled  int64  `json:"cancelled"`
}

// OrdersMetricsResponse powers the "Orders" analytics tab.
type OrdersMetricsResponse struct {
	Range            string                   `json:"range"`
	OrdersSeries     []TimeSeriesPoint        `json:"orders_series"`
	StatusSeries     []OrderStatusSeriesPoint `json:"status_series"`
	StatusTotals     OrderStatusBreakdown     `json:"status_totals"`
	AvgHoursFulfill  *float64                 `json:"avg_hours_to_fulfill"` // nil when no fulfilled orders
	FulfillmentRate  float64                  `json:"fulfillment_rate"`     // 0..1
	CancelRate       float64                  `json:"cancel_rate"`          // 0..1
	TotalOrders      int64                    `json:"total_orders"`
}

// GetOrdersMetrics handles GET /admin/stores/:storeId/dashboard/metrics/orders.
func (h *DashboardHandler) GetOrdersMetrics(c *gin.Context) {
	storeID, tenantID, err := resolveStoreAndTenant(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	win, raw, err := parseRange(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	key := fmt.Sprintf("orders:%s:%s:%s", tenantID, storeID, raw)
	if cached, ok := h.getCachedMetrics(key); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	ctx := c.Request.Context()
	db := h.db.WithContext(ctx)
	resp := OrdersMetricsResponse{Range: raw}

	// Orders-per-day series (all statuses).
	ordersSeries, err := zeroFilledSeries(ctx, h.db,
		`COALESCE(COUNT(source.id), 0)`,
		`orders AS source`,
		`AND source.deleted_at IS NULL`,
		storeID, tenantID, win)
	if err != nil {
		RespondErr(c, fmt.Errorf("orders series: %w", err), h.logger)
		return
	}
	resp.OrdersSeries = ordersSeries

	// Status series — one row per day with counts per status (5 most relevant).
	lastDay := win.end.Add(-24 * time.Hour)
	var statusRows []struct {
		Day        time.Time
		Pending    int64
		Confirmed  int64
		InProgress int64
		Fulfilled  int64
		Cancelled  int64
	}
	db.Raw(`
		SELECT
			gs.day::date AS day,
			COUNT(*) FILTER (WHERE o.status = 'pending')      AS pending,
			COUNT(*) FILTER (WHERE o.status = 'confirmed')    AS confirmed,
			COUNT(*) FILTER (WHERE o.status = 'in_progress')  AS in_progress,
			COUNT(*) FILTER (WHERE o.status = 'fulfilled')    AS fulfilled,
			COUNT(*) FILTER (WHERE o.status = 'cancelled')    AS cancelled
		FROM generate_series(?::date, ?::date, '1 day'::interval) AS gs(day)
		LEFT JOIN orders o
			ON DATE(o.placed_at) = gs.day::date
			AND o.store_id = ? AND o.tenant_id = ?
			AND o.deleted_at IS NULL
		GROUP BY gs.day
		ORDER BY gs.day
	`, win.start, lastDay, storeID, tenantID).Scan(&statusRows)

	resp.StatusSeries = make([]OrderStatusSeriesPoint, 0, len(statusRows))
	for _, r := range statusRows {
		resp.StatusSeries = append(resp.StatusSeries, OrderStatusSeriesPoint{
			Date:       r.Day.Format("2006-01-02"),
			Pending:    r.Pending,
			Confirmed:  r.Confirmed,
			InProgress: r.InProgress,
			Fulfilled:  r.Fulfilled,
			Cancelled:  r.Cancelled,
		})
	}

	// Status totals over the whole range.
	var statusTotals []struct {
		Status string
		Cnt    int64
	}
	db.Raw(`
		SELECT status, COUNT(*) AS cnt
		FROM orders
		WHERE store_id = ? AND tenant_id = ?
			AND deleted_at IS NULL
			AND placed_at >= ? AND placed_at < ?
		GROUP BY status
	`, storeID, tenantID, win.start, win.end).Scan(&statusTotals)
	for _, r := range statusTotals {
		resp.TotalOrders += r.Cnt
		switch r.Status {
		case "pending":
			resp.StatusTotals.Pending = r.Cnt
		case "confirmed":
			resp.StatusTotals.Confirmed = r.Cnt
		case "in_progress":
			resp.StatusTotals.InProgress = r.Cnt
		case "fulfilled":
			resp.StatusTotals.Fulfilled = r.Cnt
		case "cancelled":
			resp.StatusTotals.Cancelled = r.Cnt
		case "returned":
			resp.StatusTotals.Returned = r.Cnt
		case "refunded":
			resp.StatusTotals.Refunded = r.Cnt
		}
	}

	// Fulfillment + cancel rates.
	if resp.TotalOrders > 0 {
		resp.FulfillmentRate = float64(resp.StatusTotals.Fulfilled) / float64(resp.TotalOrders)
		resp.CancelRate = float64(resp.StatusTotals.Cancelled) / float64(resp.TotalOrders)
	}

	// Average hours from placed_at → fulfilled_at for orders fulfilled in window.
	var avgHours *float64
	var result struct {
		Avg *float64
	}
	db.Raw(`
		SELECT AVG(EXTRACT(EPOCH FROM (fulfilled_at - placed_at)) / 3600.0) AS avg
		FROM orders
		WHERE store_id = ? AND tenant_id = ?
			AND fulfilled_at IS NOT NULL
			AND placed_at >= ? AND placed_at < ?
			AND deleted_at IS NULL
	`, storeID, tenantID, win.start, win.end).Scan(&result)
	avgHours = result.Avg
	resp.AvgHoursFulfill = avgHours

	h.setCachedMetrics(key, resp)
	c.JSON(http.StatusOK, resp)
}

// ---------- Customers ----------

// CustomerSegmentPoint splits the daily count into new-vs-returning.
type CustomerSegmentPoint struct {
	Date      string `json:"date"`
	New       int64  `json:"new"`
	Returning int64  `json:"returning"`
}

// TopCustomer is a single row in the top-spending customers list.
type TopCustomer struct {
	Email      string  `json:"email"`
	Spend      float64 `json:"spend"`
	OrderCount int64   `json:"order_count"`
}

// CustomersMetricsResponse powers the "Customers" analytics tab.
type CustomersMetricsResponse struct {
	Range            string                 `json:"range"`
	Series           []CustomerSegmentPoint `json:"series"`
	NewTotal         int64                  `json:"new_total"`
	ReturningTotal   int64                  `json:"returning_total"`
	UniqueBuyers     int64                  `json:"unique_buyers"`
	TopCustomers     []TopCustomer          `json:"top_customers"`
}

// GetCustomersMetrics handles GET /admin/stores/:storeId/dashboard/metrics/customers.
func (h *DashboardHandler) GetCustomersMetrics(c *gin.Context) {
	storeID, tenantID, err := resolveStoreAndTenant(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	win, raw, err := parseRange(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	key := fmt.Sprintf("customers:%s:%s:%s", tenantID, storeID, raw)
	if cached, ok := h.getCachedMetrics(key); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	ctx := c.Request.Context()
	db := h.db.WithContext(ctx)
	resp := CustomersMetricsResponse{Range: raw}

	// New-vs-returning per day. Rank each order against the customer's full
	// history for the store — rank 1 = first-ever order (new), rank > 1 =
	// returning. The outer query trims to the requested window.
	lastDay := win.end.Add(-24 * time.Hour)
	var rows []struct {
		Day       time.Time
		NewCnt    int64
		RetCnt    int64
	}
	db.WithContext(ctx).Raw(`
		SELECT day, new_cnt, ret_cnt FROM (
			SELECT gs.day::date AS day,
				COUNT(*) FILTER (WHERE r.order_rank = 1) AS new_cnt,
				COUNT(*) FILTER (WHERE r.order_rank > 1) AS ret_cnt
			FROM generate_series(?::date, ?::date, '1 day'::interval) AS gs(day)
			LEFT JOIN (
				SELECT placed_at,
					RANK() OVER (PARTITION BY store_id, customer_email ORDER BY placed_at) AS order_rank
				FROM orders
				WHERE store_id = ? AND tenant_id = ?
					AND deleted_at IS NULL
					AND status != 'cancelled'
			) r ON DATE(r.placed_at) = gs.day::date
			GROUP BY gs.day
		) s
		ORDER BY day
	`, win.start, lastDay, storeID, tenantID).Scan(&rows)

	resp.Series = make([]CustomerSegmentPoint, 0, len(rows))
	for _, r := range rows {
		resp.Series = append(resp.Series, CustomerSegmentPoint{
			Date:      r.Day.Format("2006-01-02"),
			New:       r.NewCnt,
			Returning: r.RetCnt,
		})
		resp.NewTotal += r.NewCnt
		resp.ReturningTotal += r.RetCnt
	}

	// Unique buyers in window.
	db.Raw(`
		SELECT COUNT(DISTINCT customer_email)
		FROM orders
		WHERE store_id = ? AND tenant_id = ?
			AND deleted_at IS NULL
			AND status != 'cancelled'
			AND placed_at >= ? AND placed_at < ?
	`, storeID, tenantID, win.start, win.end).Scan(&resp.UniqueBuyers)

	// Top customers by spend in the range. Initialized as an empty (not
	// nil) slice so the JSON response is `[]` rather than `null` when no
	// customers match — the frontend relies on `.length`.
	tops := []TopCustomer{}
	db.Raw(`
		SELECT customer_email AS email,
			COALESCE(SUM(grand_total - refunded_amount), 0) AS spend,
			COUNT(*) AS order_count
		FROM orders
		WHERE store_id = ? AND tenant_id = ?
			AND deleted_at IS NULL
			AND status != 'cancelled'
			AND placed_at >= ? AND placed_at < ?
			AND customer_email IS NOT NULL AND customer_email != ''
		GROUP BY customer_email
		ORDER BY spend DESC
		LIMIT 5
	`, storeID, tenantID, win.start, win.end).Scan(&tops)
	resp.TopCustomers = tops

	h.setCachedMetrics(key, resp)
	c.JSON(http.StatusOK, resp)
}

// ---------- Reviews ----------

// RatingDistribution holds counts per star bucket (1..5).
type RatingDistribution struct {
	R1 int64 `json:"r1"`
	R2 int64 `json:"r2"`
	R3 int64 `json:"r3"`
	R4 int64 `json:"r4"`
	R5 int64 `json:"r5"`
}

// ReviewsMetricsResponse powers the "Reviews" analytics tab.
type ReviewsMetricsResponse struct {
	Range        string             `json:"range"`
	AvgRating    float64            `json:"avg_rating"`
	TotalReviews int64              `json:"total_reviews"`
	Distribution RatingDistribution `json:"distribution"`
	Series       []TimeSeriesPoint  `json:"series"`
}

// GetReviewsMetrics handles GET /admin/stores/:storeId/dashboard/metrics/reviews.
//
// Reviews are keyed by created_at, not placed_at, so this handler uses a
// bespoke generate_series join rather than the shared helper.
func (h *DashboardHandler) GetReviewsMetrics(c *gin.Context) {
	storeID, tenantID, err := resolveStoreAndTenant(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	win, raw, err := parseRange(c)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	key := fmt.Sprintf("reviews:%s:%s:%s", tenantID, storeID, raw)
	if cached, ok := h.getCachedMetrics(key); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	ctx := c.Request.Context()
	db := h.db.WithContext(ctx)
	resp := ReviewsMetricsResponse{Range: raw}

	// Average + total for approved reviews in window.
	var totals struct {
		Avg   float64
		Count int64
	}
	db.Raw(`
		SELECT
			COALESCE(AVG(rating), 0) AS avg,
			COUNT(*)                 AS count
		FROM reviews
		WHERE store_id = ? AND tenant_id = ?
			AND status = 'approved'
			AND deleted_at IS NULL
			AND created_at >= ? AND created_at < ?
	`, storeID, tenantID, win.start, win.end).Scan(&totals)
	resp.AvgRating = totals.Avg
	resp.TotalReviews = totals.Count

	// Rating distribution (all time, not range-restricted — distribution is
	// most useful as a stable snapshot of the store's overall reputation).
	var buckets []struct {
		Rating int
		Cnt    int64
	}
	db.Raw(`
		SELECT rating, COUNT(*) AS cnt
		FROM reviews
		WHERE store_id = ? AND tenant_id = ?
			AND status = 'approved'
			AND deleted_at IS NULL
		GROUP BY rating
	`, storeID, tenantID).Scan(&buckets)
	for _, b := range buckets {
		switch b.Rating {
		case 1:
			resp.Distribution.R1 = b.Cnt
		case 2:
			resp.Distribution.R2 = b.Cnt
		case 3:
			resp.Distribution.R3 = b.Cnt
		case 4:
			resp.Distribution.R4 = b.Cnt
		case 5:
			resp.Distribution.R5 = b.Cnt
		}
	}

	// Reviews per day in window.
	lastDay := win.end.Add(-24 * time.Hour)
	var rows []struct {
		Day time.Time
		Cnt int64
	}
	db.WithContext(ctx).Raw(`
		SELECT gs.day::date AS day, COUNT(r.id) AS cnt
		FROM generate_series(?::date, ?::date, '1 day'::interval) AS gs(day)
		LEFT JOIN reviews r
			ON DATE(r.created_at) = gs.day::date
			AND r.store_id = ? AND r.tenant_id = ?
			AND r.status = 'approved'
			AND r.deleted_at IS NULL
		GROUP BY gs.day
		ORDER BY gs.day
	`, win.start, lastDay, storeID, tenantID).Scan(&rows)
	resp.Series = make([]TimeSeriesPoint, 0, len(rows))
	for _, r := range rows {
		resp.Series = append(resp.Series, TimeSeriesPoint{
			Date:  r.Day.Format("2006-01-02"),
			Value: float64(r.Cnt),
		})
	}

	h.setCachedMetrics(key, resp)
	c.JSON(http.StatusOK, resp)
}

