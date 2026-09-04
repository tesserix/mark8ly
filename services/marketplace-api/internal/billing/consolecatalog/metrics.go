package consolecatalog

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metrics for the parity monitor (tesserix-home#328 gap 3).
//
// # Why metrics and not a table
//
// Before this file the monitor's only output was a log line, and nothing
// watched it. A parity difference in production was therefore written
// somewhere durable and never read. A persisted table would have had the same
// defect — a table also has to be looked at. What was missing is a difference
// ARRIVING somewhere, which is a metric plus an alert.
//
// This matters more since the compiled fallback became GENERATED from the
// console (#648): a difference now means the console moved and nobody
// regenerated, or someone hand-edited a generated file. Neither is visible in
// day-to-day behaviour, because the fallback is only consulted when the
// console is unreachable. The failure is silent until the day it is
// load-bearing.
//
// # The distinction these three metrics exist to preserve
//
// Result.Compared is deliberately separate from Result.Differences, because
// "a failed read must not look like agreement". A single gauge would re-create
// that defect exactly: if the monitor stopped reaching the console,
// ParityDifferences would hold its last value — or never be set, and read as
// zero — and would say "fine" forever. A monitor that has STOPPED CHECKING
// must not look like a monitor that KEEPS FINDING NOTHING.
//
// So the three are read together, and each has a state where it is the one
// that tells you:
//
//   - ParityDifferences > 0            → the catalog and the fallback disagree.
//   - LastSuccessTimestamp stale       → nothing has been compared lately, so
//     ParityDifferences describes the past and means nothing about now. This
//     is the half that keeps a dead monitor from reading as a clean one.
//   - CheckFailuresTotal rising        → the monitor IS running and is failing
//     its reads. It separates "cannot reach the console" from "not running at
//     all", which the stale timestamp alone cannot tell apart.
//
// Deliberately absent: a "checks succeeded" counter. Its every reading is
// already implied by the timestamp advancing, and it would be zero in exactly
// the states that matter.
var (
	// ParityDifferences is how many differences the LAST COMPLETED comparison
	// found. It is not touched by a failed read, so its value always describes
	// a real comparison — read it together with LastSuccessTimestamp, which
	// says when that comparison was.
	ParityDifferences = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mark8ly_catalog_parity_differences",
			Help: "Differences the last completed console/compiled catalog comparison found. Only meaningful alongside mark8ly_catalog_parity_last_success_timestamp_seconds — a failed read leaves this at its previous value.",
		},
	)
	// LastSuccessTimestamp is when a comparison last COMPLETED — i.e. the
	// console was read and the two sources were compared. A failed read must
	// never advance it; that is what makes silence detectable.
	LastSuccessTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mark8ly_catalog_parity_last_success_timestamp_seconds",
			Help: "Unix time of the last COMPLETED console/compiled catalog comparison. Never advanced by a failed read, so staleness means the monitor is dead or cannot reach the console.",
		},
	)
	// CheckFailuresTotal counts comparisons that could not be made because the
	// console read failed. Counted separately from finding differences: an
	// outage is not a disagreement.
	CheckFailuresTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mark8ly_catalog_parity_check_failures_total",
			Help: "Comparisons abandoned because the console catalog could not be read. Counted separately from differences found — an unreachable console is not a disagreement.",
		},
	)
)

// MustRegisterMetrics registers the parity monitor's metrics with reg.
// Panics on duplicate registration — call once at startup.
//
// Called from startCatalogParityRun AFTER the credentials gate, not
// unconditionally from main. Registering a gauge publishes it at zero
// immediately, and a LastSuccessTimestamp of zero reads as "last compared in
// 1970" — so registering on a pod where the parallel run is switched off
// would make the staleness alert fire forever on a deployment that is
// behaving exactly as configured. Absent series mean "not enabled here";
// present-but-stale means "enabled and not working", which is the thing worth
// paging about.
func MustRegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(ParityDifferences, LastSuccessTimestamp, CheckFailuresTotal)
}
