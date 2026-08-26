package platformadmin

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
//
// Pending counts rows the publisher will still attempt: published_at IS
// NULL AND error IS NULL. Errored counts rows it has given up on:
// published_at IS NULL AND error IS NOT NULL, which #336 made a real state
// by teaching the publisher to write outbox_events.error instead of marking
// a dropped event published.
//
// The two are separate because they need different reactions. A pending
// backlog drains on its own and is measured by age; an errored row never
// drains — only an operator clears it — so counting it as pending would
// make OldestPendingAgeSeconds grow forever and leave this surface
// permanently degraded on a condition draining cannot fix.
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

// CampaignSendsHealth is the measured state of campaigns.
type CampaignSendsHealth struct {
	Sending               int64
	SendingStaleHeartbeat int64
}

// StripeWebhooksHealth is the measured state of stripe_webhook_events.
// Inbound only — receiving webhooks normally says nothing about whether
// our own outbound Stripe API calls are succeeding, which is why
// stripe_api is a separate, uninstrumented registry entry.
type StripeWebhooksHealth struct {
	Unprocessed                 int64
	OldestUnprocessedAgeSeconds int64
	ManualReviewRequired        int64
}

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

// dependencyRow is one entry in the response. Metrics is omitempty so an
// uninstrumented or unknown dependency ships no metrics key at all — a
// zeroed block would be indistinguishable from a healthy one.
type dependencyRow struct {
	Name    string           `json:"name"`
	Status  string           `json:"status"`
	Metrics map[string]int64 `json:"metrics,omitempty"`
}

// HealthHandler serves GET /admin/health (#289) — "is this product
// working", as distinct from /health (is the process alive) and /ready
// (can it serve). Those two are correctly scoped and unchanged.
type HealthHandler struct {
	src    HealthSource
	logger *slog.Logger
	now    func() time.Time
}

// NewHealthHandler constructs the handler. logger may be nil.
func NewHealthHandler(src HealthSource, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{src: src, logger: logger, now: time.Now}
}

// Register mounts the route on the supplied group.
func (h *HealthHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/health", h.health)
}

// logCheckFailed records the real error server-side. It is never echoed to
// the caller — same discipline /ready already applies, so DSN fragments and
// driver error text do not leave the process.
func (h *HealthHandler) logCheckFailed(name string, err error) {
	if h.logger != nil {
		h.logger.Error("health check failed", "dependency", name, "err", err)
	}
}

func (h *HealthHandler) health(c *gin.Context) {
	ctx := c.Request.Context()
	asOf := h.now()

	// Gather first, keyed by name. A check that errors is absent from this
	// map, which is what makes it `unknown` below rather than a zeroed `ok`.
	measured := make(map[string]dependencyRow, 4)

	if v, err := h.src.Outbox(ctx, asOf); err != nil {
		h.logCheckFailed("outbox", err)
	} else {
		status := StatusOK
		// Errored degrades regardless of age: a terminally-failed event is
		// a silent divergence between this service and whatever consumes
		// the watermark, and it does not resolve on its own.
		if v.Errored > 0 ||
			time.Duration(v.OldestPendingAgeSeconds)*time.Second >= OutboxPendingThreshold {
			status = StatusDegraded
		}
		measured["outbox"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"pending":                    v.Pending,
			"oldest_pending_age_seconds": v.OldestPendingAgeSeconds,
			"errored":                    v.Errored,
		}}
	}

	if v, err := h.src.CSVJobs(ctx, asOf); err != nil {
		h.logCheckFailed("csv_import_jobs", err)
	} else {
		status := StatusOK
		if v.RunningStaleHeartbeat > 0 {
			status = StatusDegraded
		}
		measured["csv_import_jobs"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"queued":                  v.Queued,
			"running_stale_heartbeat": v.RunningStaleHeartbeat,
		}}
	}

	if v, err := h.src.CampaignSends(ctx, asOf); err != nil {
		h.logCheckFailed("campaign_sends", err)
	} else {
		status := StatusOK
		if v.SendingStaleHeartbeat > 0 {
			status = StatusDegraded
		}
		measured["campaign_sends"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"sending":                 v.Sending,
			"sending_stale_heartbeat": v.SendingStaleHeartbeat,
		}}
	}

	if v, err := h.src.StripeWebhooks(ctx, asOf); err != nil {
		h.logCheckFailed("stripe_webhooks", err)
	} else {
		status := StatusOK
		if v.ManualReviewRequired > 0 ||
			time.Duration(v.OldestUnprocessedAgeSeconds)*time.Second >= StripeUnprocessedThreshold {
			status = StatusDegraded
		}
		measured["stripe_webhooks"] = dependencyRow{Status: status, Metrics: map[string]int64{
			"unprocessed":                    v.Unprocessed,
			"oldest_unprocessed_age_seconds": v.OldestUnprocessedAgeSeconds,
			"manual_review_required":         v.ManualReviewRequired,
		}}
	}

	// Emit in registry order. Membership comes from the registry, never
	// from what the gather stage happened to produce.
	rows := make([]dependencyRow, 0, len(DependencyRegistry))
	for _, key := range DependencyRegistry {
		if !key.Instrumented {
			rows = append(rows, dependencyRow{Name: key.Name, Status: StatusNotInstrumented})
			continue
		}
		row, ok := measured[key.Name]
		if !ok {
			// Registered as instrumented, but no measurement — the check
			// errored, or a registry entry was added without a gather.
			// Either way the honest answer is `unknown` with no metrics.
			rows = append(rows, dependencyRow{Name: key.Name, Status: StatusUnknown})
			continue
		}
		row.Name = key.Name
		rows = append(rows, row)
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"checked_at":   asOf.UTC().Format(time.RFC3339),
		"dependencies": rows,
	}})
}
