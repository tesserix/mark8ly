package platformadmin

import (
	"context"
	"time"
)

// Dependency status values. `unknown` exists so a check whose query FAILED
// can never be rendered as `ok` — the same rule the platform-api clients
// enforce with ErrUnavailable: an error and an empty result must never
// collapse into each other. `not_instrumented` is deliberately a separate
// value: "we did not look" and "we looked and the lookup broke" are
// different facts about the system.
const (
	StatusOK              = "ok"
	StatusDegraded        = "degraded"
	StatusUnknown         = "unknown"
	StatusNotInstrumented = "not_instrumented"
)

// Thresholds with no existing authority elsewhere in the system. The two
// heartbeat windows are NOT here — they are read from csvjob.OrphanWindow
// and campaign.StaleDuration, which already govern the recovery scans.
const (
	OutboxPendingThreshold     = 5 * time.Minute
	StripeUnprocessedThreshold = 15 * time.Minute
)

// dependencyKey is one dependency the console may be told about.
type dependencyKey struct {
	Name         string
	Instrumented bool
}

// DependencyRegistry declares EVERY dependency mark8ly knows about, and
// drives the payload — the handler does not decide membership with
// conditionals. Same reasoning as KPIRegistry in kpis.go: a dependency
// must not be able to fall silently out of the response.
//
// The five uninstrumented entries are not omitted and not `ok`. Nothing in
// the system records a last-run for the scheduled jobs, and no outcome log
// exists for the outbound integrations, so any status other than
// not_instrumented would be asserting something nothing records.
// Configuration presence is NOT health: a non-empty STRIPE_BILLING_SECRET_KEY
// says a deploy was configured, nothing more.
var DependencyRegistry = []dependencyKey{
	{Name: "outbox", Instrumented: true},
	{Name: "csv_import_jobs", Instrumented: true},
	{Name: "campaign_sends", Instrumented: true},
	{Name: "stripe_webhooks", Instrumented: true},
	{Name: "scheduled_jobs", Instrumented: false},
	{Name: "platform_api", Instrumented: false},
	{Name: "stripe_api", Instrumented: false},
	{Name: "email_delivery", Instrumented: false},
	{Name: "object_storage", Instrumented: false},
}

// OutboxHealth is the measured state of outbox_events.
type OutboxHealth struct {
	Pending                 int64
	OldestPendingAgeSeconds int64
	Errored                 int64
}

// CSVJobsHealth is the measured state of csv_import_jobs.
type CSVJobsHealth struct {
	Queued                int64
	RunningStaleHeartbeat int64
}

// CampaignSendsHealth is the measured state of campaigns. Fields land in Task 3.
type CampaignSendsHealth struct{}

// StripeWebhooksHealth is the measured state of stripe_webhook_events. Fields land in Task 3.
type StripeWebhooksHealth struct{}

// HealthSource measures the four instrumented dependencies. Every method
// takes asOf from the caller and compares against it in SQL rather than
// using Postgres now(), so a test can place a fixture on the exact
// boundary instant.
type HealthSource interface {
	Outbox(ctx context.Context, asOf time.Time) (OutboxHealth, error)
	CSVJobs(ctx context.Context, asOf time.Time) (CSVJobsHealth, error)
	CampaignSends(ctx context.Context, asOf time.Time) (CampaignSendsHealth, error)
	StripeWebhooks(ctx context.Context, asOf time.Time) (StripeWebhooksHealth, error)
}
