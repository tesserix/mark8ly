package platformadmin

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
)

// errNotImplementedYet is removed in Task 3 when the remaining two checks
// land. It exists only so this file compiles as its own commit.
var errNotImplementedYet = errors.New("platformadmin: health check not implemented yet")

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
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE published_at IS NULL)                    AS pending,
			COALESCE(EXTRACT(EPOCH FROM (? - MIN(created_at)
				FILTER (WHERE published_at IS NULL)))::bigint, 0)           AS oldest_pending_age_seconds
		FROM outbox_events`, asOf).Scan(&out).Error
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
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = 'queued')  AS queued,
			COUNT(*) FILTER (WHERE status = 'running'
				AND heartbeat_at IS NOT NULL
				AND heartbeat_at <= ?)                 AS running_stale_heartbeat
		FROM csv_import_jobs`, asOf.Add(-csvjob.OrphanWindow)).Scan(&out).Error
	if err != nil {
		return CSVJobsHealth{}, err
	}
	return out, nil
}

func (s *dbHealthSource) CampaignSends(context.Context, time.Time) (CampaignSendsHealth, error) {
	return CampaignSendsHealth{}, errNotImplementedYet
}

func (s *dbHealthSource) StripeWebhooks(context.Context, time.Time) (StripeWebhooksHealth, error) {
	return StripeWebhooksHealth{}, errNotImplementedYet
}
