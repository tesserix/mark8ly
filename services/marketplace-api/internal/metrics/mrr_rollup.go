package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// mrrRow is the result of the MRR aggregation query — one row per (plan, currency).
type mrrRow struct {
	Plan     string
	Currency string
	// TotalMinor is the sum of plan_amount_minor for active subscriptions in this bucket.
	TotalMinor int64
}

// mrrFXReader is the minimal interface MRRRollup needs from the FX repository.
// Allows substitution with a test stub.
type mrrFXReader interface {
	GetAll(ctx context.Context) (map[string]decimal.Decimal, error)
}

// MRRRollup is a prometheus.Collector that computes the Monthly Recurring
// Revenue (MRR) in USD at scrape time by joining the store_subscriptions table
// with fx_rates. It emits a single GaugeVec metric:
//
//	mark8ly_subscription_mrr_usd{plan, currency}
//
// If the underlying query fails the collector emits a zero value and logs the
// error — it never breaks the scrape.
type MRRRollup struct {
	db     *gorm.DB
	fx     mrrFXReader
	logger *slog.Logger
	desc   *prometheus.Desc
}

// NewMRRRollup constructs an MRRRollup collector. Register it with
// prometheus.MustRegister (or a test registry) to activate scrape-time
// collection.
func NewMRRRollup(db *gorm.DB, fx mrrFXReader, logger *slog.Logger) *MRRRollup {
	if logger == nil {
		logger = slog.Default()
	}
	return &MRRRollup{
		db:     db,
		fx:     fx,
		logger: logger,
		desc: prometheus.NewDesc(
			"mark8ly_subscription_mrr_usd",
			"Monthly Recurring Revenue in USD, aggregated from active subscriptions and fx_rates.",
			[]string{"plan", "currency"},
			nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (m *MRRRollup) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.desc
}

// Collect implements prometheus.Collector. It runs a single aggregate query
// with a 5-second timeout. On failure it emits a zero gauge for each
// (plan, currency) pair rather than omitting the metric entirely, so
// dashboards show "0" rather than a gap.
func (m *MRRRollup) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch FX rates first; fall back to empty map (USD-only) on error.
	rates, err := m.fx.GetAll(ctx)
	if err != nil {
		m.logger.Warn("mrr_rollup: failed to fetch fx rates, USD-denominated subscriptions only",
			"err", err)
		rates = map[string]decimal.Decimal{}
	}
	// Ensure USD identity rate is always present.
	if _, ok := rates["USD"]; !ok {
		rates["USD"] = decimal.NewFromInt(1)
	}

	// Guard: nil DB means no DB connection available (e.g. unit tests without
	// a real Postgres). Emit a zero and return cleanly.
	if m.db == nil {
		m.logger.Warn("mrr_rollup: no db connection, emitting 0")
		ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, 0, "unknown", "USD")
		return
	}

	// Aggregate active subscriptions by plan + billing_currency.
	var rows []mrrRow
	queryErr := m.db.WithContext(ctx).Raw(`
		SELECT
			plan,
			COALESCE(billing_currency, 'USD') AS currency,
			COALESCE(SUM(plan_amount_minor), 0) AS total_minor
		FROM store_subscriptions
		WHERE status = 'active'
		  AND plan_amount_minor IS NOT NULL
		GROUP BY plan, COALESCE(billing_currency, 'USD')
	`).Scan(&rows).Error

	if queryErr != nil {
		m.logger.Error("mrr_rollup: aggregate query failed", "err", queryErr)
		// Emit a zero so the scrape doesn't time out waiting for a metric that
		// never arrives.
		ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, 0, "unknown", "USD")
		return
	}

	if len(rows) == 0 {
		// No active paying subscriptions — emit zero so dashboards don't gap.
		ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, 0, "all", "USD")
		return
	}

	for _, row := range rows {
		usdRate, ok := rates[row.Currency]
		if !ok {
			// No FX rate for this currency — emit zero for this bucket.
			m.logger.Warn("mrr_rollup: no fx rate for currency, emitting 0",
				"currency", row.Currency)
			ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, 0, row.Plan, row.Currency)
			continue
		}

		// Convert minor units (cents) to major, then to USD.
		major := decimal.NewFromInt(row.TotalMinor).Div(decimal.NewFromInt(100))
		mrrUSD, _ := major.Mul(usdRate).Float64()

		ch <- prometheus.MustNewConstMetric(
			m.desc,
			prometheus.GaugeValue,
			mrrUSD,
			row.Plan,
			row.Currency,
		)
	}
}
