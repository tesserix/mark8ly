package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// mrrGroup is one row of the active-subscription aggregation: a count of
// active subscriptions sharing a (plan, subscription_period, billing_currency).
//
// store_subscriptions does NOT store a price — there is no amount column on
// the table at all. The amount therefore has to come from the compiled price
// catalog (internal/billing/pricing), keyed by exactly these three fields,
// which is why the SQL side only counts.
type mrrGroup struct {
	Plan          string
	Period        string
	Currency      string
	Subscriptions int64
}

// mrrGroupReader reads active-subscription counts grouped by
// (plan, period, currency). The gorm-backed implementation is the only
// production one; tests substitute a stub.
type mrrGroupReader interface {
	activeSubscriptionGroups(ctx context.Context) ([]mrrGroup, error)
}

// gormMRRGroupReader is the production mrrGroupReader, backed by Postgres.
type gormMRRGroupReader struct {
	db *gorm.DB
}

func (r gormMRRGroupReader) activeSubscriptionGroups(ctx context.Context) ([]mrrGroup, error) {
	if r.db == nil {
		return nil, fmt.Errorf("mrr_rollup: no db connection")
	}
	var rows []mrrGroup
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			plan                                        AS plan,
			COALESCE(subscription_period, 'monthly')    AS period,
			COALESCE(billing_currency, 'USD')           AS currency,
			COUNT(*)                                    AS subscriptions
		FROM store_subscriptions
		WHERE status = 'active'
		GROUP BY plan,
		         COALESCE(subscription_period, 'monthly'),
		         COALESCE(billing_currency, 'USD')
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// mrrFXReader is the minimal interface MRRRollup needs from the FX repository.
// Allows substitution with a test stub.
type mrrFXReader interface {
	GetAll(ctx context.Context) (map[string]decimal.Decimal, error)
}

// MRRRollup is a prometheus.Collector that computes Monthly Recurring Revenue
// (MRR) in USD at scrape time. It counts active subscriptions per
// (plan, period, currency) in SQL, resolves the per-subscription price from
// the compiled price catalog in Go, normalises annual prices to a monthly
// figure, and converts to USD with fx_rates. It emits a single GaugeVec:
//
//	mark8ly_subscription_mrr_usd{plan, currency}
//
// Currency buckets are never summed together: each (plan, currency) is its own
// series, matching the metric's label set.
//
// If the underlying query fails the collector emits a zero value and logs the
// error — it never breaks the scrape, and it never panics.
type MRRRollup struct {
	reader mrrGroupReader
	fx     mrrFXReader
	logger *slog.Logger
	desc   *prometheus.Desc
}

// NewMRRRollup constructs an MRRRollup collector. Register it with
// prometheus.MustRegister (or a test registry) to activate scrape-time
// collection.
func NewMRRRollup(db *gorm.DB, fx mrrFXReader, logger *slog.Logger) *MRRRollup {
	return newMRRRollupWithReader(gormMRRGroupReader{db: db}, fx, logger)
}

// newMRRRollupWithReader is the seam the tests use to supply canned rows
// without a database.
func newMRRRollupWithReader(reader mrrGroupReader, fx mrrFXReader, logger *slog.Logger) *MRRRollup {
	if logger == nil {
		logger = slog.Default()
	}
	return &MRRRollup{
		reader: reader,
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

// minorPerMajor converts minor units (cents, paise, sen…) to major units.
// The price catalog stores every currency at ×100, including the zero-decimal
// ones, so this divisor is uniform — see the zero-decimal note in
// internal/billing/pricing/catalog.go.
var minorPerMajor = decimal.NewFromInt(100)

var monthsPerYear = decimal.NewFromInt(12)

// monthlyMinorFor resolves the per-subscription price for one group from the
// compiled catalog and normalises it to a MONTHLY figure.
//
// MRR means monthly: an annual price is divided by 12, otherwise a yearly
// subscription would be counted at twelve times its monthly contribution and
// every dashboard built on this metric would be wrong.
//
// The division is deliberately performed in decimal and NOT rounded to whole
// minor units. Several annual prices do not divide evenly by 12 (e.g. USD
// $182/yr → $15.1666…/mo), and rounding each group to whole cents before
// multiplying by the subscription count would push a systematic bias into the
// total that grows with the number of subscribers. decimal.Div keeps 16
// decimal places; the only rounding left is the final float64 conversion at
// the gauge boundary, which is far below one cent.
//
// It never panics: pricing.MustGet is deliberately not used here because a
// missing catalog entry inside a Prometheus scrape must degrade, not crash.
func monthlyMinorFor(plan, period, currency string) (decimal.Decimal, bool) {
	p := pricing.Plan(strings.ToLower(strings.TrimSpace(plan)))
	per := pricing.Period(strings.ToLower(strings.TrimSpace(period)))
	// The catalog keys currencies in lowercase ISO 4217; the column is CHAR(3)
	// with no enforced case.
	cur := strings.ToLower(strings.TrimSpace(currency))

	amt, ok := pricing.DevelopedAmount(p, per, cur)
	if !ok {
		amt, ok = pricing.PPPAmount(p, per, cur)
	}
	if !ok {
		return decimal.Zero, false
	}

	minor := decimal.NewFromInt(amt.UnitAmountMinor)
	if per == pricing.PeriodAnnual {
		minor = minor.Div(monthsPerYear)
	}
	return minor, true
}

// bucket is the emitted label pair. Several period groups (monthly + annual)
// collapse into one series, so they must be summed before emission or
// Prometheus rejects the scrape with duplicate label values.
type bucket struct {
	plan     string
	currency string
}

// Collect implements prometheus.Collector. It runs a single aggregate query
// with a 5-second timeout. On failure, on an empty table, or when nothing at
// all could be priced, it emits a zero gauge rather than omitting the metric
// entirely, so dashboards show "0" rather than a gap.
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
	// Ensure USD identity rate is always present. Copy first: the map may be
	// owned by the FX repository or by a caller.
	normalisedRates := make(map[string]decimal.Decimal, len(rates)+1)
	for k, v := range rates {
		normalisedRates[strings.ToUpper(strings.TrimSpace(k))] = v
	}
	if _, ok := normalisedRates["USD"]; !ok {
		normalisedRates["USD"] = decimal.NewFromInt(1)
	}

	groups, err := m.reader.activeSubscriptionGroups(ctx)
	if err != nil {
		m.logger.Error("mrr_rollup: aggregate query failed", "err", err)
		// Emit a zero so the series does not gap on a transient DB failure.
		ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, 0, "unknown", "USD")
		return
	}

	if len(groups) == 0 {
		// No active subscriptions — emit zero so dashboards don't gap.
		ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, 0, "all", "USD")
		return
	}

	totals := make(map[bucket]decimal.Decimal, len(groups))
	// unpriceable collects every (plan, period, currency) the catalog could not
	// price. These are EXCLUDED from the gauge rather than emitted as zero: a
	// zero-valued series is indistinguishable from a plan that genuinely earns
	// nothing, which is exactly how a revenue metric under-reports without
	// anyone noticing. An absent series plus this log is visible.
	var unpriceable []string
	var unpricedSubscriptions int64

	for _, g := range groups {
		currency := strings.ToUpper(strings.TrimSpace(g.Currency))
		if currency == "" {
			currency = "USD"
		}
		plan := strings.TrimSpace(g.Plan)

		monthlyMinor, ok := monthlyMinorFor(plan, g.Period, currency)
		if !ok {
			unpriceable = append(unpriceable,
				fmt.Sprintf("%s/%s/%s(n=%d)", plan, strings.TrimSpace(g.Period), currency, g.Subscriptions))
			unpricedSubscriptions += g.Subscriptions
			continue
		}

		key := bucket{plan: plan, currency: currency}
		rate, ok := normalisedRates[currency]
		if !ok {
			// No FX rate for this currency — this bucket contributes 0 USD, but
			// the series is still emitted so its absence is not mistaken for a
			// missing plan.
			m.logger.Warn("mrr_rollup: no fx rate for currency, contributing 0",
				"currency", currency, "plan", plan)
			if _, seen := totals[key]; !seen {
				totals[key] = decimal.Zero
			}
			continue
		}

		contribution := monthlyMinor.
			Mul(decimal.NewFromInt(g.Subscriptions)).
			Div(minorPerMajor).
			Mul(rate)
		totals[key] = totals[key].Add(contribution)
	}

	if len(unpriceable) > 0 {
		// One log line per scrape, naming every offending key.
		sort.Strings(unpriceable)
		m.logger.Warn("mrr_rollup: active subscriptions the price catalog cannot price, excluded from the gauge",
			"groups", strings.Join(unpriceable, ","),
			"group_count", len(unpriceable),
			"subscription_count", unpricedSubscriptions)
	}

	if len(totals) == 0 {
		// Every group was unpriceable — still emit a zero so the scrape has a
		// sample and the dashboard does not hole.
		ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, 0, "all", "USD")
		return
	}

	// Deterministic emission order keeps test output and logs stable.
	keys := make([]bucket, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].plan != keys[j].plan {
			return keys[i].plan < keys[j].plan
		}
		return keys[i].currency < keys[j].currency
	})

	for _, k := range keys {
		usd, _ := totals[k].Float64()
		ch <- prometheus.MustNewConstMetric(m.desc, prometheus.GaugeValue, usd, k.plan, k.currency)
	}
}
