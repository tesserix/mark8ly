//go:build integration

package platformadmin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/csvjob"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// asOf is a fixed instant so every fixture below sits at an exact offset
// from it. Nothing here calls time.Now() — see the plan's global
// constraints on why a caller-supplied asOf is what makes an
// exact-boundary fixture possible.
var healthAsOf = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

// seedStoreForCSV creates the store row csv_import_jobs.store_id must
// reference (csv_import_jobs_store_id_fkey). Mirrors the helper in
// internal/csvjob/repository_test.go.
func seedStoreForCSV(t *testing.T, db *gorm.DB) string {
	t.Helper()
	storeID := uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     uuid.NewString(),
		Slug:         "csv-health-" + storeID[:8],
		Name:         "CSV Health Test Store",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "UTC",
		Status:       stores.StatusActive,
	}
	require.NoError(t, db.Create(s).Error)
	return storeID
}

func TestOutboxHealthCountsPendingAndMeasuresAgeFromAsOf(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	src := platformadmin.NewDBHealthSource(db)
	tenant := uuid.NewString()

	// Published: must not count as pending.
	published := healthAsOf.Add(-time.Hour)
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at, published_at)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?, ?)`,
		tenant, uuid.NewString(), published, published).Error)

	// Pending, 10 minutes old — the oldest pending row.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?)`,
		tenant, uuid.NewString(), healthAsOf.Add(-10*time.Minute)).Error)

	// A second pending row, younger than the 10-minute one, so the age
	// assertion fails if MIN() ignores the pending filter.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at)
		VALUES (?, 'order', ?, 'order.placed', '{}'::jsonb, ?)`,
		tenant, uuid.NewString(), healthAsOf.Add(-2*time.Minute)).Error)

	got, err := src.Outbox(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Pending, "published row must not count as pending")
	require.Equal(t, int64(600), got.OldestPendingAgeSeconds,
		"age must be measured from the caller's asOf, not Postgres now()")
}

// TestCSVJobsStaleHeartbeatIsInclusiveAtTheBoundary puts the fixture ON the
// boundary instant rather than near it. A row exactly OrphanWindow old is
// stale; a row one millisecond younger is not. One millisecond, not one
// nanosecond: timestamptz truncates to microseconds and the two rows would
// otherwise be identical.
func TestCSVJobsStaleHeartbeatIsInclusiveAtTheBoundary(t *testing.T) {
	db := testdb.NewDB(t, "csv_import_jobs", "stores")
	src := platformadmin.NewDBHealthSource(db)
	storeID := seedStoreForCSV(t, db)

	// content_hash must be distinct per row: idx_csv_import_jobs_store_content_hash_active
	// is a unique index on (store_id, content_hash) for active statuses.
	insert := func(status string, heartbeat *time.Time) {
		require.NoError(t, db.Exec(`INSERT INTO csv_import_jobs
			(store_id, user_id, gcs_path, content_hash, status, heartbeat_at)
			VALUES (?, 'u', 'gs://x', ?, ?, ?)`,
			storeID, uuid.NewString(), status, heartbeat).Error)
	}

	exactly := healthAsOf.Add(-csvjob.OrphanWindow)
	justInside := exactly.Add(time.Millisecond)

	insert("running", &exactly)    // stale: age == window
	insert("running", &justInside) // healthy: age < window
	insert("queued", nil)          // queued, never counted as stale
	// worker.go sets status='running' before the heartbeat loop starts, so
	// a worker that dies in that gap leaves heartbeat_at NULL forever. That
	// is the failure this metric exists to report, and the recovery scan
	// resets exactly these rows — they must not read as healthy.
	insert("running", nil)

	got, err := src.CSVJobs(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Queued)
	require.Equal(t, int64(2), got.RunningStaleHeartbeat,
		"a heartbeat exactly OrphanWindow old is stale, one millisecond younger is not, "+
			"and a running job that never heartbeat at all is stale")
}

// The campaign window is campaign.StaleDuration — an exported constant
// that already governs RecoverStuckCampaigns. Same inclusive boundary rule
// and same 1ms offset as the csv test above.
//
// campaigns.tenant_id is NOT NULL and campaigns.store_id has an FK to
// stores(id) — neither is in the plan's sketch insert. Both are added here
// rather than weakening the fixture; seedStoreForCSV is reused even though
// its name says CSV, since it only creates a generic store row.
func TestCampaignSendsStaleHeartbeatIsInclusiveAtTheBoundary(t *testing.T) {
	db := testdb.NewDB(t, "campaigns", "stores")
	src := platformadmin.NewDBHealthSource(db)
	storeID := seedStoreForCSV(t, db)

	exactly := healthAsOf.Add(-campaign.StaleDuration)
	justInside := exactly.Add(time.Millisecond)

	insert := func(status string, heartbeat *time.Time) {
		require.NoError(t, db.Exec(`INSERT INTO campaigns (id, tenant_id, store_id, name, status, heartbeat_at)
			VALUES (?, ?, ?, 'c', ?, ?)`,
			uuid.New(), uuid.New(), storeID, status, heartbeat).Error)
	}
	insert("sending", &exactly)
	insert("sending", &justInside)
	insert("draft", nil)
	// service.go flips status to 'sending' before the heartbeat loop
	// starts; a send that dies in that gap never writes a heartbeat.
	insert("sending", nil)

	got, err := src.CampaignSends(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Sending, "only status='sending' rows count")
	require.Equal(t, int64(2), got.SendingStaleHeartbeat,
		"a heartbeat exactly StaleDuration old is stale, one millisecond younger is not, "+
			"and a sending campaign that never heartbeat at all is stale")
}

func TestStripeWebhooksCountsUnprocessedAndManualReview(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	src := platformadmin.NewDBHealthSource(db)

	insert := func(id string, received time.Time, processed *time.Time, manual bool) {
		require.NoError(t, db.Exec(`INSERT INTO stripe_webhook_events
			(event_id, event_type, payload, received_at, processed_at, manual_review_required)
			VALUES (?, 'invoice.paid', '{}'::jsonb, ?, ?, ?)`,
			id, received, processed, manual).Error)
	}

	done := healthAsOf.Add(-time.Hour)
	insert("evt_done", done, &done, false)                         // processed
	insert("evt_old", healthAsOf.Add(-20*time.Minute), nil, false) // unprocessed, oldest
	insert("evt_manual", healthAsOf.Add(-time.Minute), nil, true)  // needs a human
	// Flagged AND processed: a human dealt with it. Nothing ever sets
	// manual_review_required back to false, so an unscoped count would pin
	// stripe_webhooks to `degraded` forever with no operator remedy.
	insert("evt_manual_done", done, &done, true)

	got, err := src.StripeWebhooks(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Unprocessed, "processed rows must not count")
	require.Equal(t, int64(1), got.ManualReviewRequired,
		"a flagged row that has been processed is resolved and must not count")
	require.Equal(t, int64(1200), got.OldestUnprocessedAgeSeconds)
}

// A terminally-failed row is NOT pending. If it counted as pending, the
// first one would put /admin/health into a degraded state that never
// clears, because oldest_pending_age_seconds would grow forever.
func TestOutboxHealthExcludesFailedRowsFromPendingAndAge(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	src := platformadmin.NewDBHealthSource(db)
	tenant := uuid.NewString()

	// Failed and very old: must not count as pending, and must not drive
	// the age.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at, error)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?, 'payload_unparseable')`,
		tenant, uuid.NewString(), healthAsOf.Add(-72*time.Hour)).Error)

	// Genuinely pending, 5 minutes old.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?)`,
		tenant, uuid.NewString(), healthAsOf.Add(-5*time.Minute)).Error)

	got, err := src.Outbox(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Pending, "a failed row must not count as pending")
	require.Equal(t, int64(1), got.Errored, "the failed row must be counted as errored")
	require.Equal(t, int64(300), got.OldestPendingAgeSeconds,
		"age must ignore failed rows, or the alarm never clears")
}

// Errored counts only unpublished failures. A published row is settled
// whatever its error column happens to hold.
func TestOutboxHealthErroredIsZeroWhenNoFailedRows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	src := platformadmin.NewDBHealthSource(db)
	tenant := uuid.NewString()

	published := healthAsOf.Add(-time.Hour)
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at, published_at)
		VALUES (?, 'product', ?, 'product.created', '{}'::jsonb, ?, ?)`,
		tenant, uuid.NewString(), published, published).Error)

	got, err := src.Outbox(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Pending)
	require.Equal(t, int64(0), got.Errored)
	require.Equal(t, int64(0), got.OldestPendingAgeSeconds)
}
