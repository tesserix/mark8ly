//go:build integration

package outbox_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now())`,
		id, tenantID, "test-"+id[:8], "Test Store", "US", "USD", "UTC", "active",
		strings.Repeat("0", 64)).Error
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

func TestIntegration_Publisher_MissingStoreID_DropFloor(t *testing.T) {
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
	if got.PublishedAt == nil {
		t.Fatalf("expected malformed event to still be marked published")
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
