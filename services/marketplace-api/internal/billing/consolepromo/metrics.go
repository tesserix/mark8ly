package consolepromo

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metrics for the promo catalog ingest (#726).
//
// # Why these four and not one
//
// The four answer questions that cannot substitute for each other, and the
// distinction is the same one internal/billing/consolecatalog's metrics
// preserve: a sync that has STOPPED RUNNING must not look like a sync that
// KEEPS FINDING NOTHING TO DO.
//
//   - CodesIngested       → how many codes the table currently holds from the
//     console. Legitimately zero when no campaign is running, so on its own
//     it can never distinguish healthy from broken.
//   - LastSuccessTimestamp stale → nothing has been ingested lately, so
//     CodesIngested describes the past. This is the half that keeps a dead
//     ingest from reading as an idle one.
//   - SyncFailuresTotal rising → the ingest IS running and its reads or
//     writes are failing, which the stale timestamp alone cannot tell apart
//     from not running at all.
//   - CodesSkippedTotal by reason → the console published something this
//     service could not map. Labelled by reason because the reason is the
//     whole diagnostic: "bad_percent_off" is a malformed campaign, while
//     "bad_discount_kind" is the console contract having moved underneath us.
//
// CodesExpiredTotal is a counter, not a gauge, because withdrawal is an
// event: the interesting question is "did a sweep expire anything", and a
// gauge would read zero again on the very next sync that expired nothing.
var (
	// CodesIngested is how many codes the LAST SUCCESSFUL sync wrote. Not
	// touched by a failed sync, so its value always describes a real one.
	CodesIngested = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mark8ly_promo_catalog_codes_ingested",
			Help: "Promo codes written by the last successful console ingest. Only meaningful alongside mark8ly_promo_catalog_last_success_timestamp_seconds — a failed sync leaves this at its previous value.",
		},
	)
	// CodesSkippedTotal counts definitions the mapper rejected, by reason.
	// The reason set is closed (see Reason) so this cannot become a
	// cardinality bomb.
	CodesSkippedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mark8ly_promo_catalog_codes_skipped_total",
			Help: "Published promo definitions the mapper rejected, by reason. A rejected definition is skipped and counted; it never aborts the batch.",
		},
		[]string{"reason"},
	)
	// CodesExpiredTotal counts rows expired for having been withdrawn from
	// the catalog. Withdrawn codes are expired (valid_until = now), never
	// deleted — see Syncer.Sync.
	CodesExpiredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mark8ly_promo_catalog_codes_expired_total",
			Help: "Console-sourced promo codes expired because they are no longer published. Expired, never deleted — promo_redemptions references these rows.",
		},
	)
	// LastSuccessTimestamp is when a sync last COMPLETED. A failed sync must
	// never advance it; that is what makes silence detectable.
	LastSuccessTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mark8ly_promo_catalog_last_success_timestamp_seconds",
			Help: "Unix time of the last COMPLETED promo catalog ingest. Never advanced by a failure, so staleness means the ingest is dead or cannot reach the console.",
		},
	)
	// SyncFailuresTotal counts syncs abandoned because the console could not
	// be read or the write failed.
	SyncFailuresTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mark8ly_promo_catalog_sync_failures_total",
			Help: "Promo catalog ingests abandoned because the console read or the database write failed. Distinct from codes skipped — an outage is not a malformed definition.",
		},
	)
)

// MustRegisterMetrics registers the ingest's metrics with reg. Panics on
// duplicate registration — call once at startup.
//
// Call it AFTER the credentials gate, never unconditionally. Registering a
// gauge publishes it at zero immediately, and a LastSuccessTimestamp of zero
// reads as "last ingested in 1970" — so registering on a pod where the
// ingest is switched off would make a staleness alert fire forever on a
// deployment behaving exactly as configured. Absent series mean "not enabled
// here"; present-but-stale means "enabled and not working".
func MustRegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(CodesIngested, CodesSkippedTotal, CodesExpiredTotal,
		LastSuccessTimestamp, SyncFailuresTotal)
}
