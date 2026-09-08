// Package metrics defines the Prometheus counters emitted by the white-label
// mobile-app subsystem (spec §13.5 lifecycle + §18.9 credential access).
//
// These feed P17 dashboards and alerts:
//   - LifecycleTransition spikes in the firebase_archived→credentials_purged
//     pair indicate day-90 completions — wire to a CSM success notification.
//   - CredentialAccessed spikes indicate either a CI/CD rebuild cycle or a
//     credential-dump attempt; alert when the 1m rate exceeds the median
//     over 24h by a wide margin.
//   - LifecycleStepSkipped is the ONLY signal that a teardown step did not
//     happen. LifecycleTransition increments identically whether or not the
//     third-party call succeeded, so without this counter a Play or Firebase
//     outage across the whole cohort is invisible to alerting and shows up
//     only as reason text inside white_label_app_lifecycle rows.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// LifecycleTransition counts white-label app lifecycle transitions as
	// the daily advancer moves a row from one §13.5 status to the next.
	// Labels:
	//   from — current status (e.g. "sunset_scheduled")
	//   to   — target status  (e.g. "downloads_blocked")
	LifecycleTransition = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "white_label_app_lifecycle_transition_total",
			Help: "Count of white-label app lifecycle transitions, labeled by from/to status.",
		},
		[]string{"from", "to"},
	)

	// CredentialAccessed counts every appcreds.Service read of a Secret
	// Manager secret, one increment per Load call. Label:
	//   type — the CredType (apple-asc-api-key | apple-asc-issuer-id |
	//          apple-asc-key-id | google-play-service-account)
	//
	// NB: high-cardinality on `type` is fine — CredType is a closed enum of
	// four values. Do NOT add tenant_id as a label.
	CredentialAccessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "white_label_app_credential_accessed_total",
			Help: "Count of white-label credential reads from Secret Manager, labeled by cred type.",
		},
		[]string{"type"},
	)

	// LifecycleStepSkipped counts teardown steps the advancer recorded as
	// NOT performed while still advancing the row. Labels:
	//   surface — "google_play" | "firebase"
	//   step    — "block_downloads" | "block_downloads_day60_retry" |
	//             "pull_app" | "archive_project"
	//
	// Both labels are closed sets; do NOT add tenant_id or store_id.
	//
	// `pull_app` on google_play is expected to be non-zero forever, not a
	// fault: unpublishing a Play listing has no API. Alert on the OTHERS
	// rising, and on a step_skipped rate that tracks the transition rate
	// (which means the surface is failing for every row, not one).
	LifecycleStepSkipped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "white_label_app_lifecycle_step_skipped_total",
			Help: "Count of white-label teardown steps recorded as not performed, labeled by surface/step.",
		},
		[]string{"surface", "step"},
	)
)

// MustRegister registers the white-label collectors into the supplied
// Prometheus registry. Main calls this once at startup; tests can pass a
// dedicated registry to isolate assertions.
func MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(LifecycleTransition, CredentialAccessed, LifecycleStepSkipped)
}

func init() {
	// Register into the default registry so /metrics exposes them without
	// extra wiring. Tests that need isolation can use a custom registry
	// via MustRegister; they will get the "already registered" panic if
	// they also touch the default — the standard testing idiom is to call
	// prometheus.Unregister() after; see internal/metrics/registry.go.
	prometheus.MustRegister(LifecycleTransition, CredentialAccessed, LifecycleStepSkipped)
}
