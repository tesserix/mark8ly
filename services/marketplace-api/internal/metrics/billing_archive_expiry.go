package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

// BillingArchiveExpiryCollector is a prometheus.Collector that emits two
// gauges at scrape time by counting rows in billing_archive:
//
//   - mark8ly_billing_archive_expiry_soon  — rows expiring within 30 days
//   - mark8ly_billing_archive_expired      — rows already past archive_expires_at
//
// Register with prometheus.MustRegister(NewBillingArchiveExpiryCollector(...))
// in main.go. The metrics.Subscription.BillingArchiveExpirySoonGauge package-
// level gauge is a simpler alternative for point-in-time checks; this
// collector runs the full query at scrape time so it stays accurate without a
// background cron.
type BillingArchiveExpiryCollector struct {
	db     *gorm.DB
	logger *slog.Logger

	descExpirySoon *prometheus.Desc
	descExpired    *prometheus.Desc
}

// NewBillingArchiveExpiryCollector constructs the collector. Register it with
// the default Prometheus registry in main.go.
func NewBillingArchiveExpiryCollector(db *gorm.DB, logger *slog.Logger) *BillingArchiveExpiryCollector {
	if logger == nil {
		logger = slog.Default()
	}
	return &BillingArchiveExpiryCollector{
		db:     db,
		logger: logger,
		descExpirySoon: prometheus.NewDesc(
			"mark8ly_billing_archive_expiry_soon",
			"Count of billing_archive rows whose archive_expires_at is within the next 30 days.",
			nil, nil,
		),
		descExpired: prometheus.NewDesc(
			"mark8ly_billing_archive_expired",
			"Count of billing_archive rows whose archive_expires_at is already in the past (awaiting sweeper).",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *BillingArchiveExpiryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.descExpirySoon
	ch <- c.descExpired
}

// Collect implements prometheus.Collector. Runs two COUNT queries with a
// short timeout. On error, emits 0 and logs — never breaks the scrape.
func (c *BillingArchiveExpiryCollector) Collect(ch chan<- prometheus.Metric) {
	if c.db == nil {
		ch <- prometheus.MustNewConstMetric(c.descExpirySoon, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.descExpired, prometheus.GaugeValue, 0)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var expirySoon int64
	if err := c.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM billing_archive
		WHERE archive_expires_at < now() + INTERVAL '30 days'
		  AND archive_expires_at >= now()
	`).Scan(&expirySoon).Error; err != nil {
		c.logger.Error("billing_archive_expiry_collector: expiry_soon query failed", "err", err)
		expirySoon = 0
	}

	var expired int64
	if err := c.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM billing_archive
		WHERE archive_expires_at < now()
	`).Scan(&expired).Error; err != nil {
		c.logger.Error("billing_archive_expiry_collector: expired query failed", "err", err)
		expired = 0
	}

	ch <- prometheus.MustNewConstMetric(c.descExpirySoon, prometheus.GaugeValue, float64(expirySoon))
	ch <- prometheus.MustNewConstMetric(c.descExpired, prometheus.GaugeValue, float64(expired))
}
