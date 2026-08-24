//go:build integration

package platformadmin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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

func TestOutboxHealthCountsPendingAndErrored(t *testing.T) {
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

	// Pending and errored, 2 minutes old.
	require.NoError(t, db.Exec(`INSERT INTO outbox_events
		(tenant_id, aggregate, aggregate_id, event_type, payload, created_at, error)
		VALUES (?, 'order', ?, 'order.placed', '{}'::jsonb, ?, 'boom')`,
		tenant, uuid.NewString(), healthAsOf.Add(-2*time.Minute)).Error)

	got, err := src.Outbox(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Pending, "published row must not count as pending")
	require.Equal(t, int64(1), got.Errored)
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

	got, err := src.CSVJobs(context.Background(), healthAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Queued)
	require.Equal(t, int64(1), got.RunningStaleHeartbeat,
		"a heartbeat exactly OrphanWindow old is stale; one millisecond younger is not")
}
