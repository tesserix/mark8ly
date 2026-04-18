package dispatch

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/webhookevents"
)

// PagerDuty is the alerting interface used by Cron to signal unresolvable
// orphan events. HTTPPagerDuty implements this for production use; tests
// supply a simple fake.
type PagerDuty interface {
	Trigger(ctx context.Context, summary string) error
}

// CronConfig holds the dependencies and tuning parameters for the orphan cron.
type CronConfig struct {
	DB             *gorm.DB
	Repo           webhookevents.Repository
	Dispatcher     *Dispatcher
	PagerDuty      PagerDuty
	StaleThreshold time.Duration // §17.7: 1h
	Interval       time.Duration // §17.7: 5m
	MaxRetries     int
}

// Cron schedules periodic orphan resolution and fires PagerDuty alerts for
// any events that remain unresolved beyond StaleThreshold.
type Cron struct {
	cfg CronConfig
	sch *cron.Cron
}

// NewCron constructs a Cron with defaults applied for any zero-valued fields.
func NewCron(cfg CronConfig) *Cron {
	if cfg.StaleThreshold == 0 {
		cfg.StaleThreshold = time.Hour
	}
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 6
	}
	return &Cron{cfg: cfg}
}

// Start schedules RunOnce at cfg.Interval. The scheduler runs in-process;
// for production HA consider GCP Cloud Scheduler + an HTTP trigger.
func (c *Cron) Start(ctx context.Context) error {
	c.sch = cron.New()
	_, err := c.sch.AddFunc(
		fmt.Sprintf("@every %s", c.cfg.Interval.String()),
		func() { _ = c.RunOnce(ctx) },
	)
	if err != nil {
		return err
	}
	c.sch.Start()
	return nil
}

// Stop halts the underlying cron scheduler. Safe to call on a nil scheduler.
func (c *Cron) Stop() {
	if c.sch != nil {
		c.sch.Stop()
	}
}

// RunOnce drives the orphan resolver and then pages on any unresolved event
// older than StaleThreshold.
func (c *Cron) RunOnce(ctx context.Context) error {
	resolver := NewOrphanResolver(OrphanConfig{
		DB:         c.cfg.DB,
		Repo:       c.cfg.Repo,
		Dispatcher: c.cfg.Dispatcher,
		MaxRetries: c.cfg.MaxRetries,
	})
	if err := resolver.RunOnce(ctx); err != nil {
		return err
	}

	var stale []webhookevents.StripeWebhookEvent
	err := c.cfg.DB.WithContext(ctx).
		Where("store_id IS NULL AND processed_at IS NULL AND manual_review_required = false").
		Where("received_at < now() - make_interval(secs => ?)", int64(c.cfg.StaleThreshold/time.Second)).
		Find(&stale).Error
	if err != nil {
		return err
	}

	for _, e := range stale {
		if c.cfg.PagerDuty != nil {
			_ = c.cfg.PagerDuty.Trigger(ctx, fmt.Sprintf(
				"Stripe webhook orphan >%s: event_id=%s type=%s",
				c.cfg.StaleThreshold, e.EventID, e.EventType,
			))
		}
	}
	return nil
}
