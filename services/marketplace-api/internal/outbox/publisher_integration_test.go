//go:build integration

package outbox_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func insertStore(t *testing.T, db *gorm.DB, id, tenantID string) {
	t.Helper()
	// storefront_customer_portal_secret is char(64) NOT NULL with no default
	// and no Go hook — callers set it explicitly (internal/stores/models.go).
	// Omitting it made every test using this helper fail at INSERT.
	err := db.Exec(`
		INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status,
		                    storefront_customer_portal_secret, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'), now())`,
		id, tenantID, "test-"+id[:8], "Test Store", "US", "USD", "UTC", "active").Error
	if err != nil {
		t.Fatalf("insertStore: %v", err)
	}
}

func enqueueForStore(t *testing.T, db *gorm.DB, tenantID, storeID string) *outbox.OutboxEvent {
	t.Helper()
	payload := datatypes.JSON([]byte(fmt.Sprintf(`{"store_id":%q}`, storeID)))
	evt := &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventProductCreated,
		Payload:     payload,
	}
	if err := db.Create(evt).Error; err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return evt
}

func TestIntegration_Publisher_BumpsWatermark(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	insertStore(t, db, storeID, tenantID)

	evt := enqueueForStore(t, db, tenantID, storeID)

	pub := outbox.New(outbox.Config{
		Repo:      repo,
		DB:        db,
		Logger:    quietLogger(),
		Interval:  50 * time.Millisecond,
		BatchSize: 100,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := pub.Start(ctx)

	deadline := time.Now().Add(1 * time.Second)
	var wmTS time.Time
	for time.Now().Before(deadline) {
		var ts time.Time
		err := db.Raw(`SELECT products_updated_at FROM store_watermarks WHERE store_id = ?`, storeID).Row().Scan(&ts)
		if err == nil && !ts.IsZero() {
			wmTS = ts
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if wmTS.IsZero() {
		t.Fatalf("store_watermarks row never appeared")
	}

	// Reload event, confirm published_at.
	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got.PublishedAt == nil {
		t.Fatalf("expected published_at to be set")
	}
	if wmTS.Before(got.CreatedAt.Add(-1 * time.Second)) {
		t.Fatalf("watermark %s earlier than event created_at %s", wmTS, got.CreatedAt)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("publisher did not shut down in time")
	}
}

func TestIntegration_Publisher_BatchMultipleEvents_MaxCreatedAt(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	insertStore(t, db, storeID, tenantID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	times := []time.Time{
		now.Add(-5 * time.Minute),
		now.Add(-3 * time.Minute),
		now.Add(-1 * time.Minute),
	}
	var evts []*outbox.OutboxEvent
	for _, ts := range times {
		e := enqueueForStore(t, db, tenantID, storeID)
		if err := db.Exec(`UPDATE outbox_events SET created_at = ? WHERE id = ?`, ts, e.ID).Error; err != nil {
			t.Fatalf("backdate: %v", err)
		}
		evts = append(evts, e)
	}

	pub := outbox.New(outbox.Config{
		Repo:      repo,
		DB:        db,
		Logger:    quietLogger(),
		Interval:  1 * time.Second,
		BatchSize: 100,
	})

	count, err := pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count=3, got %d", count)
	}

	var ts time.Time
	if err := db.Raw(`SELECT products_updated_at FROM store_watermarks WHERE store_id = ?`, storeID).Row().Scan(&ts); err != nil {
		t.Fatalf("read watermark: %v", err)
	}
	want := times[2].Truncate(time.Microsecond)
	if !ts.UTC().Truncate(time.Microsecond).Equal(want) {
		t.Fatalf("watermark = %s, want %s (max created_at)", ts.UTC(), want)
	}

	for _, e := range evts {
		var got outbox.OutboxEvent
		if err := db.First(&got, "id = ?", e.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.PublishedAt == nil {
			t.Fatalf("event %s not published", e.ID)
		}
	}
}

func TestIntegration_Publisher_MissingStoreID_MarksFailedNotPublished(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	insertStore(t, db, storeID, tenantID)

	evt := &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventProductCreated,
		Payload:     datatypes.JSON([]byte(`{"unrelated":"value"}`)),
	}
	if err := db.Create(evt).Error; err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	pub := outbox.New(outbox.Config{
		Repo:      repo,
		DB:        db,
		Logger:    quietLogger(),
		Interval:  1 * time.Second,
		BatchSize: 100,
	})

	count, err := pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PublishedAt != nil {
		t.Fatalf("a dropped event must NOT be marked published, got published_at=%v", got.PublishedAt)
	}
	if got.Error == nil {
		t.Fatalf("error is nil; want %q", outbox.ReasonPayloadMissingStoreID)
	}
	if *got.Error != outbox.ReasonPayloadMissingStoreID {
		t.Fatalf("error = %q, want %q", *got.Error, outbox.ReasonPayloadMissingStoreID)
	}

	var n int64
	if err := db.Raw(`SELECT count(*) FROM store_watermarks`).Scan(&n).Error; err != nil {
		t.Fatalf("count watermarks: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 watermark rows, got %d", n)
	}
}

func TestIntegration_Publisher_CleanShutdown(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	pub := outbox.New(outbox.Config{
		Repo:      repo,
		DB:        db,
		Logger:    quietLogger(),
		Interval:  10 * time.Millisecond,
		BatchSize: 100,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := pub.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("publisher did not shut down within 200ms")
	}
}

func TestIntegration_Publisher_UnparseablePayload_MarksFailedNotPublished(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	insertStore(t, db, storeID, tenantID)

	// jsonb rejects malformed JSON at insert, so the unparseable-to-Go
	// value is a well-formed JSON scalar: valid jsonb, but not the object
	// json.Unmarshal into map[string]any requires.
	evt := &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventProductCreated,
		Payload:     datatypes.JSON([]byte(`"not-an-object"`)),
	}
	if err := db.Create(evt).Error; err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	pub := outbox.New(outbox.Config{
		Repo: repo, DB: db, Logger: quietLogger(),
		Interval: 1 * time.Second, BatchSize: 100,
	})
	if _, err := pub.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PublishedAt != nil {
		t.Fatalf("a dropped event must NOT be marked published, got published_at=%v", got.PublishedAt)
	}
	if got.Error == nil || *got.Error != outbox.ReasonPayloadUnparseable {
		t.Fatalf("error = %v, want %q", got.Error, outbox.ReasonPayloadUnparseable)
	}
}

// The mixed batch is the composition no single-purpose test constructs, and
// it is the whole point of the fix: one good row and one bad row in the SAME
// tick. The good row must publish AND bump its watermark; the bad row must
// be failed AND left unpublished. A regression that reverted the id-building
// inversion would still pass every single-row test in this file.
func TestIntegration_Publisher_MixedBatch_PublishesGoodFailsBad(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	insertStore(t, db, storeID, tenantID)

	good := enqueueForStore(t, db, tenantID, storeID)

	bad := &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventProductUpdated,
		Payload:     datatypes.JSON([]byte(`{"unrelated":"value"}`)),
	}
	if err := db.Create(bad).Error; err != nil {
		t.Fatalf("enqueue bad: %v", err)
	}

	pub := outbox.New(outbox.Config{
		Repo: repo, DB: db, Logger: quietLogger(),
		Interval: 1 * time.Second, BatchSize: 100,
	})
	count, err := pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if count != 2 {
		t.Fatalf("tick saw %d rows, want 2", count)
	}

	var gotGood outbox.OutboxEvent
	if err := db.First(&gotGood, "id = ?", good.ID).Error; err != nil {
		t.Fatalf("reload good: %v", err)
	}
	if gotGood.PublishedAt == nil {
		t.Fatalf("the good event must be published even when a bad one shares its batch")
	}
	if gotGood.Error != nil {
		t.Fatalf("the good event must not be marked failed, got %q", *gotGood.Error)
	}

	var gotBad outbox.OutboxEvent
	if err := db.First(&gotBad, "id = ?", bad.ID).Error; err != nil {
		t.Fatalf("reload bad: %v", err)
	}
	if gotBad.PublishedAt != nil {
		t.Fatalf("the bad event must NOT be published, got published_at=%v", gotBad.PublishedAt)
	}
	if gotBad.Error == nil || *gotBad.Error != outbox.ReasonPayloadMissingStoreID {
		t.Fatalf("bad event error = %v, want %q", gotBad.Error, outbox.ReasonPayloadMissingStoreID)
	}

	// The good row's watermark landed: the drop must not have suppressed it.
	var n int64
	if err := db.Raw(`SELECT count(*) FROM store_watermarks WHERE store_id = ?`, storeID).
		Scan(&n).Error; err != nil {
		t.Fatalf("count watermarks: %v", err)
	}
	if n != 1 {
		t.Fatalf("watermark rows = %d, want 1", n)
	}

	// And the failed row is not retried on the next tick.
	count, err = pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if count != 0 {
		t.Fatalf("second tick saw %d rows, want 0 (failed rows are terminal)", count)
	}
}

// A store_id that is well-formed but absent from `stores` used to raise an FK
// violation on the watermark upsert, which ABORTS the whole Postgres
// transaction — taking the good rows and the failure marks with it and
// leaving the entire batch pending forever. #374.
func TestIntegration_Publisher_StoreNotFound_MarksFailedAndCommits(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	ghostStoreID := uuid.NewString() // deliberately never inserted

	evt := enqueueForStore(t, db, tenantID, ghostStoreID)

	pub := outbox.New(outbox.Config{
		Repo: repo, DB: db, Logger: quietLogger(),
		Interval: 1 * time.Second, BatchSize: 100,
	})
	if _, err := pub.Tick(context.Background()); err != nil {
		t.Fatalf("tick must not error on a missing store: %v", err)
	}

	var got outbox.OutboxEvent
	if err := db.First(&got, "id = ?", evt.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PublishedAt != nil {
		t.Fatalf("a row for a missing store must not be published, got %v", got.PublishedAt)
	}
	if got.Error == nil {
		t.Fatalf("error is nil; want %q", outbox.ReasonStoreNotFound)
	}
	// Exact code. If the constant was added but not allowlisted in
	// sanitizeReason, this reads "unknown" and this line is what catches it.
	if *got.Error != outbox.ReasonStoreNotFound {
		t.Fatalf("error = %q, want %q", *got.Error, outbox.ReasonStoreNotFound)
	}

	var n int64
	if err := db.Raw(`SELECT count(*) FROM store_watermarks`).Scan(&n).Error; err != nil {
		t.Fatalf("count watermarks: %v", err)
	}
	if n != 0 {
		t.Fatalf("watermark rows = %d, want 0", n)
	}
}

// The composition that matters: one good row and one ghost-store row in the
// SAME batch. Before #374 the FK violation rolled back BOTH.
func TestIntegration_Publisher_StoreNotFound_DoesNotRollBackGoodRows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	realStoreID := uuid.NewString()
	insertStore(t, db, realStoreID, tenantID)
	ghostStoreID := uuid.NewString()

	good := enqueueForStore(t, db, tenantID, realStoreID)
	ghost := enqueueForStore(t, db, tenantID, ghostStoreID)

	pub := outbox.New(outbox.Config{
		Repo: repo, DB: db, Logger: quietLogger(),
		Interval: 1 * time.Second, BatchSize: 100,
	})
	count, err := pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if count != 2 {
		t.Fatalf("tick saw %d rows, want 2", count)
	}

	var gotGood outbox.OutboxEvent
	if err := db.First(&gotGood, "id = ?", good.ID).Error; err != nil {
		t.Fatalf("reload good: %v", err)
	}
	if gotGood.PublishedAt == nil {
		t.Fatalf("the good row must publish even when a ghost-store row shares its batch")
	}
	if gotGood.Error != nil {
		t.Fatalf("the good row must not be failed, got %q", *gotGood.Error)
	}

	var gotGhost outbox.OutboxEvent
	if err := db.First(&gotGhost, "id = ?", ghost.ID).Error; err != nil {
		t.Fatalf("reload ghost: %v", err)
	}
	if gotGhost.PublishedAt != nil {
		t.Fatalf("the ghost row must not be published, got %v", gotGhost.PublishedAt)
	}
	if gotGhost.Error == nil || *gotGhost.Error != outbox.ReasonStoreNotFound {
		t.Fatalf("ghost row error = %v, want %q", gotGhost.Error, outbox.ReasonStoreNotFound)
	}

	// The real store's watermark landed — the FK row did not suppress it.
	var n int64
	if err := db.Raw(`SELECT count(*) FROM store_watermarks WHERE store_id = ?`, realStoreID).
		Scan(&n).Error; err != nil {
		t.Fatalf("count watermarks: %v", err)
	}
	if n != 1 {
		t.Fatalf("watermark rows for the real store = %d, want 1", n)
	}

	// And nothing is retried.
	count, err = pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if count != 0 {
		t.Fatalf("second tick saw %d rows, want 0", count)
	}
}

// A store_id that is a non-empty string but not a parseable UUID used to
// reach the store pre-check's `SELECT id FROM stores WHERE id IN ?`, where
// Postgres raises `invalid input syntax for type uuid` — which ABORTS the
// transaction and rolls back the whole batch, good rows included. #374.
func TestIntegration_Publisher_NonUUIDStoreID_MarksFailedAndCommits(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events", "store_watermarks", "stores")
	repo := outbox.NewRepository(db)

	tenantID := uuid.NewString()
	storeID := uuid.NewString()
	insertStore(t, db, storeID, tenantID)

	good := enqueueForStore(t, db, tenantID, storeID)

	bad := &outbox.OutboxEvent{
		TenantID:    tenantID,
		Aggregate:   outbox.AggregateProduct,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventProductUpdated,
		Payload:     datatypes.JSON([]byte(`{"store_id":"store-42"}`)),
	}
	if err := db.Create(bad).Error; err != nil {
		t.Fatalf("enqueue bad: %v", err)
	}

	pub := outbox.New(outbox.Config{
		Repo: repo, DB: db, Logger: quietLogger(),
		Interval: 1 * time.Second, BatchSize: 100,
	})
	if _, err := pub.Tick(context.Background()); err != nil {
		t.Fatalf("tick must not error on a non-UUID store_id: %v", err)
	}

	var gotGood outbox.OutboxEvent
	if err := db.First(&gotGood, "id = ?", good.ID).Error; err != nil {
		t.Fatalf("reload good: %v", err)
	}
	if gotGood.PublishedAt == nil {
		t.Fatalf("the good event must be published even when a non-UUID store_id shares its batch")
	}

	var wmCount int64
	if err := db.Raw(`SELECT count(*) FROM store_watermarks WHERE store_id = ?`, storeID).
		Scan(&wmCount).Error; err != nil {
		t.Fatalf("count watermarks: %v", err)
	}
	if wmCount != 1 {
		t.Fatalf("watermark rows = %d, want 1", wmCount)
	}

	var gotBad outbox.OutboxEvent
	if err := db.First(&gotBad, "id = ?", bad.ID).Error; err != nil {
		t.Fatalf("reload bad: %v", err)
	}
	if gotBad.PublishedAt != nil {
		t.Fatalf("the non-UUID store_id event must NOT be published, got published_at=%v", gotBad.PublishedAt)
	}
	if gotBad.Error == nil || *gotBad.Error != outbox.ReasonPayloadMissingStoreID {
		t.Fatalf("bad event error = %v, want %q", gotBad.Error, outbox.ReasonPayloadMissingStoreID)
	}

	// And the bad row is not retried on the next tick.
	count, err := pub.Tick(context.Background())
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if count != 0 {
		t.Fatalf("second tick saw %d rows, want 0 (failed rows are terminal)", count)
	}
}
