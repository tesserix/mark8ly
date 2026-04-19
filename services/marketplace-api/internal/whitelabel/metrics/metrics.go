// Package metrics defines the Prometheus counters emitted by the white-label
// mobile-app subsystem (spec §13.5 lifecycle + §18.9 credential access).
//
// These feed P17 dashboards and alerts:
//   - LifecycleTransition spikes in the firebase_archived→credentials_purged
//     pair indicate day-90 completions — wire to a CSM success notification.
//   - CredentialAccessed spikes indicate either a CI/CD rebuild cycle or a
//     credential-dump attempt; alert when the 1m rate exceeds the median
//     over 24h by a wide margin.
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
)

// MustRegister registers the white-label collectors into the supplied
// Prometheus registry. Main calls this once at startup; tests can pass a
// dedicated registry to isolate assertions.
func MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(LifecycleTransition, CredentialAccessed)
}

func init() {
	// Register into the default registry so /metrics exposes them without
	// extra wiring. Tests that need isolation can use a custom registry
	// via MustRegister; they will get the "already registered" panic if
	// they also touch the default — the standard testing idiom is to call
	// prometheus.Unregister() after; see internal/metrics/registry.go.
	prometheus.MustRegister(LifecycleTransition, CredentialAccessed)
}
