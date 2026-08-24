package platformadmin

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/csvjob"
)

// errNoDB is returned by every check when the source has no database.
// It exists so a nil DB degrades to `unknown` rather than panicking:
// (*gorm.DB).WithContext dereferences its receiver, so an unguarded nil
// would take down the request rather than reporting an honest non-answer.
var errNoDB = errors.New("platformadmin: health source has no database")

type dbHealthSource struct{ db *gorm.DB }

// NewDBHealthSource returns a HealthSource reading from Postgres. Every
// measurement is a query, never an in-process counter: the production
// admin Deployment is replicas:1 today, but that is a fact about the
// manifest rather than a guarantee, and a DB-backed answer stays correct
// if the pin is ever lifted.
func NewDBHealthSource(db *gorm.DB) HealthSource { return &dbHealthSource{db: db} }

func (s *dbHealthSource) Outbox(ctx context.Context, asOf time.Time) (OutboxHealth, error) {
	if s.db == nil {
		return OutboxHealth{}, errNoDB
	}
	var out OutboxHealth
	// The pending condition is a WHERE, not a FILTER: outbox_events is
	// never pruned, and only a WHERE lets Postgres use outbox_unpublished_idx
	// (a partial index on published_at IS NULL, migration 000001) instead of
	// scanning every row ever written on a shared db-f1-micro.
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*)                                                        AS pending,
			COALESCE(EXTRACT(EPOCH FROM (? - MIN(created_at)))::bigint, 0)  AS oldest_pending_age_seconds
		FROM outbox_events
		WHERE published_at IS NULL`, asOf).Scan(&out).Error
	if err != nil {
		return OutboxHealth{}, err
	}
	return out, nil
}

func (s *dbHealthSource) CSVJobs(ctx context.Context, asOf time.Time) (CSVJobsHealth, error) {
	if s.db == nil {
		return CSVJobsHealth{}, errNoDB
	}
	var out CSVJobsHealth
	// age >= OrphanWindow is stale, so the comparison is <= on the
	// timestamp. Inclusive at the boundary, matching the plan's uniform
	// rule and pinned by an exact-instant fixture.
	//
	// A NULL heartbeat_at on a 'running' row is stale, not healthy:
	// worker.go sets status='running' BEFORE the heartbeat loop starts, so
	// a worker that dies in that gap leaves the row at NULL forever — the
	// exact condition this metric exists to report. The recovery scan
	// (csvjob/repository.go) treats NULL the same way.
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = 'queued')  AS queued,
			COUNT(*) FILTER (WHERE status = 'running'
				AND (heartbeat_at IS NULL
					OR heartbeat_at <= ?))             AS running_stale_heartbeat
		FROM csv_import_jobs`, asOf.Add(-csvjob.OrphanWindow)).Scan(&out).Error
	if err != nil {
		return CSVJobsHealth{}, err
	}
	return out, nil
}

func (s *dbHealthSource) CampaignSends(ctx context.Context, asOf time.Time) (CampaignSendsHealth, error) {
	if s.db == nil {
		return CampaignSendsHealth{}, errNoDB
	}
	var out CampaignSendsHealth
	// NULL heartbeat_at on a 'sending' row is stale for the same reason as
	// csv jobs above: campaign/service.go flips status to 'sending' before
	// the heartbeat loop starts, and RecoverStuckCampaigns already treats
	// NULL as recoverable.
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = 'sending') AS sending,
			COUNT(*) FILTER (WHERE status = 'sending'
				AND (heartbeat_at IS NULL
					OR heartbeat_at <= ?))             AS sending_stale_heartbeat
		FROM campaigns`, asOf.Add(-campaign.StaleDuration)).Scan(&out).Error
	if err != nil {
		return CampaignSendsHealth{}, err
	}
	return out, nil
}

func (s *dbHealthSource) StripeWebhooks(ctx context.Context, asOf time.Time) (StripeWebhooksHealth, error) {
	if s.db == nil {
		return StripeWebhooksHealth{}, errNoDB
	}
	var out StripeWebhooksHealth
	// All three metrics share `processed_at IS NULL`, so it is a WHERE:
	// the table is never pruned, and the WHERE lets the partial indexes
	// (swe_orphan_idx, swe_manual_review_idx, migration 000043) apply.
	//
	// Scoping manual_review_required to unprocessed rows also stops it
	// being a one-way latch. Nothing ever sets the column back to false,
	// so an unscoped count would pin stripe_webhooks to `degraded` forever
	// after the first flagged event, with no operator remedy. Every other
	// consumer of the column (webhookevents/repository.go,
	// billing/dispatch/cron.go) pairs it with processed_at IS NULL too.
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*)                                                         AS unprocessed,
			COALESCE(EXTRACT(EPOCH FROM (? - MIN(received_at)))::bigint, 0)  AS oldest_unprocessed_age_seconds,
			COUNT(*) FILTER (WHERE manual_review_required)                   AS manual_review_required
		FROM stripe_webhook_events
		WHERE processed_at IS NULL`, asOf).Scan(&out).Error
	if err != nil {
		return StripeWebhooksHealth{}, err
	}
	return out, nil
}
