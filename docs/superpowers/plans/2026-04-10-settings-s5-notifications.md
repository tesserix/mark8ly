# Settings S5 — Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship notification preferences, notification table, bell dropdown with unread count badge (30s poll), and notification listener that creates notifications from outbox events.

**Architecture:** New `internal/notification/` package (models, repository, listener). Migration 000017. Listener goroutine subscribes to outbox events. Bell dropdown in AdminShell topbar.

**Tech Stack:** Go 1.26, Gin, GORM. Next.js 16, React 19, Tailwind.

**Spec reference:** `docs/superpowers/specs/2026-04-10-settings-tier1-tier2-design.md` — sections §2.3, §3.5, §4.5, §5.2 (bell dropdown), §5.1 (notifications page), §6.4, §7.2, §8 (S5 tests).

**Prerequisite:** Migration 000016 (S3 subscriptions) must exist. The outbox publisher (§14.6) must be running — S5 taps into the same outbox event stream.

---

## File structure produced by S5

```
services/marketplace-api/
├── migrations/
│   ├── 000017_notifications.up.sql                     # NEW
│   └── 000017_notifications.down.sql                   # NEW
├── internal/
│   ├── notification/
│   │   ├── models.go                                   # NEW — NotificationPreference, Notification GORM models
│   │   ├── repository.go                               # NEW — CRUD for notifications + preferences
│   │   ├── repository_test.go                          # NEW — repository unit tests
│   │   ├── listener.go                                 # NEW — goroutine that converts outbox events to notifications
│   │   ├── listener_test.go                            # NEW — listener unit tests
│   │   ├── handler.go                                  # NEW — HTTP handlers for notifications + preferences
│   │   └── handler_test.go                             # NEW — handler integration tests
│   ├── outbox/
│   │   └── models.go                                   # MODIFY — add new event type constants
│   └── authz/
│       └── notification_roles.go                       # NEW — role constants
├── internal/handlers/admin/
│   └── routes.go                                       # MODIFY — add notification + preference routes
├── cmd/marketplace-api/
│   └── main.go                                         # MODIFY — wire notification handler + start listener goroutine

apps/admin/
├── lib/api/
│   └── notification-api.ts                             # NEW — typed API client
├── app/settings/notifications/
│   ├── page.tsx                                        # NEW — notification preferences page
│   └── actions.ts                                      # NEW — server actions (updatePreferences)
├── components/settings/
│   └── NotificationPreferencesClient.tsx               # NEW — toggle grid for preferences
├── components/shell/
│   ├── AdminShell.tsx                                  # MODIFY — replace static bell with NotificationBell
│   └── NotificationBell.tsx                            # NEW — bell dropdown with unread badge + 30s poll
```

---

## Task 0: Verify prerequisites

**Files:** none (read-only)

- [ ] **Step 1: Verify current migration version**

```bash
ls services/marketplace-api/migrations/ | tail -5
```

Expected: latest is `000016_subscriptions` or whatever S3 shipped. The new migration is `000017`. Adjust if needed.

- [ ] **Step 2: Verify outbox publisher is wired**

```bash
grep -n "outbox.New" services/marketplace-api/cmd/marketplace-api/main.go
```

Expected: should show the outbox publisher construction around line 326. The notification listener will tap into the same outbox_events table.

- [ ] **Step 3: Verify outbox event types**

```bash
grep "Event.*=" services/marketplace-api/internal/outbox/models.go
```

Expected: shows existing event constants like `EventOrderPlaced`, `EventReturnRequested`, etc. We'll add notification-relevant event mappings.

No commit. Task 0 is read-only.

---

## Task 1: Migration — notification tables

**Files:**
- Create: `services/marketplace-api/migrations/000017_notifications.up.sql`
- Create: `services/marketplace-api/migrations/000017_notifications.down.sql`

### TDD: RED

- [ ] **Step 1: Write the up migration**

Create `services/marketplace-api/migrations/000017_notifications.up.sql`:

```sql
BEGIN;

CREATE TABLE notification_preferences (
    id              UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID    NOT NULL,
    store_id        UUID    NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    preferences     JSONB   NOT NULL DEFAULT '{
        "new_order": true,
        "low_stock": true,
        "return_requested": true,
        "payment_received": true,
        "review_submitted": true
    }'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE TABLE notifications (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    type            VARCHAR(40)   NOT NULL,
    title           VARCHAR(200)  NOT NULL,
    message         TEXT,
    resource_type   VARCHAR(40),
    resource_id     UUID,
    is_read         BOOLEAN       NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX notif_store_unread_idx ON notifications (store_id, is_read, created_at DESC);
CREATE INDEX notif_store_recent_idx ON notifications (store_id, created_at DESC);

COMMIT;
```

- [ ] **Step 2: Write the down migration**

Create `services/marketplace-api/migrations/000017_notifications.down.sql`:

```sql
BEGIN;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS notification_preferences;
COMMIT;
```

### GREEN

- [ ] **Step 3: Apply migration**

```bash
cd services/marketplace-api && DATABASE_URL="postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable" go run ./cmd/migrate up
```

- [ ] **Step 4: Verify tables exist**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT table_name FROM information_schema.tables WHERE table_name IN ('notification_preferences','notifications') ORDER BY 1;"
```

Expected: `notification_preferences` and `notifications`.

**Commit:** `feat(notification): add migration 000017 for notification_preferences and notifications tables`

---

## Task 2: GORM models + repository

**Files:**
- Create: `services/marketplace-api/internal/notification/models.go`
- Create: `services/marketplace-api/internal/notification/repository.go`
- Create: `services/marketplace-api/internal/notification/repository_test.go`

### TDD: RED — Write tests first

- [ ] **Step 1: Create models.go**

Create `services/marketplace-api/internal/notification/models.go`:

```go
package notification

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Notification types — must match the spec §2.3.
const (
	TypeNewOrder              = "new_order"
	TypeLowStock              = "low_stock"
	TypeReturnRequested       = "return_requested"
	TypePaymentReceived       = "payment_received"
	TypeReviewSubmitted       = "review_submitted"
	TypeDomainVerified        = "domain_verified"
	TypeDomainError           = "domain_error"
	TypeSubscriptionExpiring  = "subscription_expiring"
	TypeSubscriptionCancelled = "subscription_cancelled"
)

// AllPreferenceTypes lists the notification types that can be toggled by the user.
var AllPreferenceTypes = []string{
	TypeNewOrder,
	TypeLowStock,
	TypeReturnRequested,
	TypePaymentReceived,
	TypeReviewSubmitted,
}

// NotificationPreference is the GORM model for notification_preferences.
type NotificationPreference struct {
	ID          uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID     uuid.UUID      `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	Preferences datatypes.JSON `gorm:"column:preferences;type:jsonb;not null"                   json:"preferences"`
	CreatedAt   time.Time      `gorm:"column:created_at;not null;default:now()"                  json:"created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;not null;default:now()"                  json:"updated_at"`
}

func (NotificationPreference) TableName() string { return "notification_preferences" }

// DefaultPreferences returns the default notification preferences JSON.
func DefaultPreferences() map[string]bool {
	return map[string]bool{
		TypeNewOrder:        true,
		TypeLowStock:        true,
		TypeReturnRequested: true,
		TypePaymentReceived: true,
		TypeReviewSubmitted: true,
	}
}

// Notification is the GORM model for the notifications table.
type Notification struct {
	ID           uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID      uuid.UUID  `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	Type         string     `gorm:"column:type;type:varchar(40);not null"                     json:"type"`
	Title        string     `gorm:"column:title;type:varchar(200);not null"                   json:"title"`
	Message      *string    `gorm:"column:message;type:text"                                  json:"message,omitempty"`
	ResourceType *string    `gorm:"column:resource_type;type:varchar(40)"                     json:"resource_type,omitempty"`
	ResourceID   *uuid.UUID `gorm:"column:resource_id;type:uuid"                              json:"resource_id,omitempty"`
	IsRead       bool       `gorm:"column:is_read;not null;default:false"                     json:"is_read"`
	CreatedAt    time.Time  `gorm:"column:created_at;not null;default:now()"                  json:"created_at"`
}

func (Notification) TableName() string { return "notifications" }

// NotificationResponse is the wire DTO for a notification.
type NotificationResponse struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	Title        string  `json:"title"`
	Message      *string `json:"message,omitempty"`
	ResourceType *string `json:"resource_type,omitempty"`
	ResourceID   *string `json:"resource_id,omitempty"`
	IsRead       bool    `json:"is_read"`
	CreatedAt    string  `json:"created_at"`
}

// ToResponse converts a Notification to its wire DTO.
func (n Notification) ToResponse() NotificationResponse {
	resp := NotificationResponse{
		ID:           n.ID.String(),
		Type:         n.Type,
		Title:        n.Title,
		Message:      n.Message,
		ResourceType: n.ResourceType,
		IsRead:       n.IsRead,
		CreatedAt:    n.CreatedAt.Format(time.RFC3339),
	}
	if n.ResourceID != nil {
		s := n.ResourceID.String()
		resp.ResourceID = &s
	}
	return resp
}
```

- [ ] **Step 2: Create repository.go**

Create `services/marketplace-api/internal/notification/repository.go`:

```go
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Repository provides data access for notifications and preferences.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ──────────────────────────────────────────────────────────────────────────
// Notification preferences
// ──────────────────────────────────────────────────────────────────────────

// GetPreferences returns the notification preferences for a store.
// If none exist, returns the defaults (not persisted until explicitly saved).
func (r *Repository) GetPreferences(ctx context.Context, storeID uuid.UUID) (map[string]bool, error) {
	var pref NotificationPreference
	err := r.db.WithContext(ctx).Where("store_id = ?", storeID).First(&pref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultPreferences(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("notification: get preferences: %w", err)
	}

	var prefs map[string]bool
	if err := json.Unmarshal(pref.Preferences, &prefs); err != nil {
		return nil, fmt.Errorf("notification: parse preferences: %w", err)
	}
	return prefs, nil
}

// UpsertPreferences creates or updates the notification preferences for a store.
func (r *Repository) UpsertPreferences(ctx context.Context, tenantID, storeID uuid.UUID, prefs map[string]bool) error {
	data, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("notification: marshal preferences: %w", err)
	}

	pref := NotificationPreference{
		TenantID:    tenantID,
		StoreID:     storeID,
		Preferences: datatypes.JSON(data),
	}

	result := r.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Assign(NotificationPreference{Preferences: datatypes.JSON(data)}).
		FirstOrCreate(&pref)
	if result.Error != nil {
		return fmt.Errorf("notification: upsert preferences: %w", result.Error)
	}
	return nil
}

// IsTypeEnabled checks if a notification type is enabled for a store.
func (r *Repository) IsTypeEnabled(ctx context.Context, storeID uuid.UUID, notifType string) (bool, error) {
	prefs, err := r.GetPreferences(ctx, storeID)
	if err != nil {
		return false, err
	}
	enabled, exists := prefs[notifType]
	if !exists {
		return true, nil // default to enabled for unknown types
	}
	return enabled, nil
}

// ──────────────────────────────────────────────────────────────────────────
// Notifications
// ──────────────────────────────────────────────────────────────────────────

// Create inserts a new notification.
func (r *Repository) Create(ctx context.Context, n *Notification) error {
	if err := r.db.WithContext(ctx).Create(n).Error; err != nil {
		return fmt.Errorf("notification: create: %w", err)
	}
	return nil
}

// ListRecent returns the most recent notifications for a store (max 50).
// If unreadOnly is true, only unread notifications are returned.
func (r *Repository) ListRecent(ctx context.Context, storeID uuid.UUID, unreadOnly bool) ([]Notification, error) {
	var notifications []Notification
	q := r.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Order("created_at DESC").
		Limit(50)
	if unreadOnly {
		q = q.Where("is_read = ?", false)
	}

	if err := q.Find(&notifications).Error; err != nil {
		return nil, fmt.Errorf("notification: list recent: %w", err)
	}
	return notifications, nil
}

// UnreadCount returns the number of unread notifications for a store.
func (r *Repository) UnreadCount(ctx context.Context, storeID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Notification{}).
		Where("store_id = ? AND is_read = ?", storeID, false).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("notification: unread count: %w", err)
	}
	return count, nil
}

// MarkRead marks a single notification as read.
func (r *Repository) MarkRead(ctx context.Context, notifID, storeID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Model(&Notification{}).
		Where("id = ? AND store_id = ?", notifID, storeID).
		Update("is_read", true)
	if result.Error != nil {
		return fmt.Errorf("notification: mark read: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("notification: mark read: not found")
	}
	return nil
}

// MarkAllRead marks all unread notifications as read for a store.
func (r *Repository) MarkAllRead(ctx context.Context, storeID uuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&Notification{}).
		Where("store_id = ? AND is_read = ?", storeID, false).
		Update("is_read", true)
	if result.Error != nil {
		return 0, fmt.Errorf("notification: mark all read: %w", result.Error)
	}
	return result.RowsAffected, nil
}
```

- [ ] **Step 3: Write repository tests**

Create `services/marketplace-api/internal/notification/repository_test.go`:

```go
package notification_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/notification"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&notification.NotificationPreference{},
		&notification.Notification{},
	))
	return db
}

// ──────────────────────────────────────────────────────────────────────────
// Preferences tests
// ──────────────────────────────────────────────────────────────────────────

func TestGetPreferences_DefaultsWhenNone(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	prefs, err := repo.GetPreferences(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.True(t, prefs["new_order"])
	assert.True(t, prefs["low_stock"])
	assert.True(t, prefs["return_requested"])
	assert.True(t, prefs["payment_received"])
	assert.True(t, prefs["review_submitted"])
}

func TestUpsertPreferences_SaveAndRetrieve(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	tenantID := uuid.New()
	prefs := map[string]bool{
		"new_order":        true,
		"low_stock":        false,
		"return_requested": true,
		"payment_received": false,
		"review_submitted": true,
	}

	err := repo.UpsertPreferences(context.Background(), tenantID, storeID, prefs)
	require.NoError(t, err)

	got, err := repo.GetPreferences(context.Background(), storeID)
	require.NoError(t, err)
	assert.False(t, got["low_stock"])
	assert.True(t, got["new_order"])
	assert.False(t, got["payment_received"])
}

func TestUpsertPreferences_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	tenantID := uuid.New()

	// First save.
	require.NoError(t, repo.UpsertPreferences(context.Background(), tenantID, storeID, map[string]bool{
		"new_order": true, "low_stock": true,
	}))

	// Update.
	require.NoError(t, repo.UpsertPreferences(context.Background(), tenantID, storeID, map[string]bool{
		"new_order": false, "low_stock": true,
	}))

	got, err := repo.GetPreferences(context.Background(), storeID)
	require.NoError(t, err)
	assert.False(t, got["new_order"])
	assert.True(t, got["low_stock"])
}

func TestIsTypeEnabled_Default(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	enabled, err := repo.IsTypeEnabled(context.Background(), uuid.New(), "new_order")
	require.NoError(t, err)
	assert.True(t, enabled)
}

func TestIsTypeEnabled_DisabledType(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	require.NoError(t, repo.UpsertPreferences(context.Background(), uuid.New(), storeID, map[string]bool{
		"new_order": false,
	}))

	enabled, err := repo.IsTypeEnabled(context.Background(), storeID, "new_order")
	require.NoError(t, err)
	assert.False(t, enabled)
}

// ──────────────────────────────────────────────────────────────────────────
// Notification tests
// ──────────────────────────────────────────────────────────────────────────

func TestCreate_And_ListRecent(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	tenantID := uuid.New()

	n := &notification.Notification{
		TenantID: tenantID,
		StoreID:  storeID,
		Type:     notification.TypeNewOrder,
		Title:    "New order #1001",
	}
	require.NoError(t, repo.Create(context.Background(), n))

	list, err := repo.ListRecent(context.Background(), storeID, false)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "New order #1001", list[0].Title)
	assert.False(t, list[0].IsRead)
}

func TestListRecent_UnreadOnly(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	tenantID := uuid.New()

	// Create 2 notifications, mark 1 read.
	n1 := &notification.Notification{TenantID: tenantID, StoreID: storeID, Type: "new_order", Title: "Order 1"}
	n2 := &notification.Notification{TenantID: tenantID, StoreID: storeID, Type: "new_order", Title: "Order 2"}
	require.NoError(t, repo.Create(context.Background(), n1))
	require.NoError(t, repo.Create(context.Background(), n2))
	require.NoError(t, repo.MarkRead(context.Background(), n1.ID, storeID))

	list, err := repo.ListRecent(context.Background(), storeID, true)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "Order 2", list[0].Title)
}

func TestUnreadCount(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	tenantID := uuid.New()

	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Create(context.Background(), &notification.Notification{
			TenantID: tenantID, StoreID: storeID, Type: "new_order", Title: "Order",
		}))
	}

	count, err := repo.UnreadCount(context.Background(), storeID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestMarkRead(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	n := &notification.Notification{TenantID: uuid.New(), StoreID: storeID, Type: "new_order", Title: "Order"}
	require.NoError(t, repo.Create(context.Background(), n))

	require.NoError(t, repo.MarkRead(context.Background(), n.ID, storeID))

	count, _ := repo.UnreadCount(context.Background(), storeID)
	assert.Equal(t, int64(0), count)
}

func TestMarkRead_WrongStore(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	n := &notification.Notification{TenantID: uuid.New(), StoreID: storeID, Type: "new_order", Title: "Order"}
	require.NoError(t, repo.Create(context.Background(), n))

	err := repo.MarkRead(context.Background(), n.ID, uuid.New())
	assert.Error(t, err, "should fail when store_id doesn't match")
}

func TestMarkAllRead(t *testing.T) {
	db := setupTestDB(t)
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	tenantID := uuid.New()

	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(context.Background(), &notification.Notification{
			TenantID: tenantID, StoreID: storeID, Type: "new_order", Title: "Order",
		}))
	}

	affected, err := repo.MarkAllRead(context.Background(), storeID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), affected)

	count, _ := repo.UnreadCount(context.Background(), storeID)
	assert.Equal(t, int64(0), count)
}
```

### GREEN

- [ ] **Step 4: Run tests**

```bash
cd services/marketplace-api && go test ./internal/notification/... -v -count=1
```

All tests must pass.

**Commit:** `feat(notification): add GORM models, repository, and repository tests`

---

## Task 3: Notification listener

**Files:**
- Create: `services/marketplace-api/internal/notification/listener.go`
- Create: `services/marketplace-api/internal/notification/listener_test.go`
- Modify: `services/marketplace-api/internal/outbox/models.go`

### TDD: RED

- [ ] **Step 1: Add notification-relevant event constants to outbox models**

In `services/marketplace-api/internal/outbox/models.go`, add to the EventType constants block:

```go
	// Notification-relevant events (mapped in notification.Listener).
	EventPaymentReceived = "payment.received"
	EventLowStock        = "inventory.low_stock"
	EventReviewSubmitted = "review.submitted"
```

- [ ] **Step 2: Create listener.go**

Create `services/marketplace-api/internal/notification/listener.go`:

```go
package notification

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
)

// eventMapping maps outbox event types to notification types + title templates.
type eventMapping struct {
	NotifType    string
	TitleFn      func(payload map[string]any) string
	ResourceType string
}

var eventMappings = map[string]eventMapping{
	outbox.EventOrderPlaced: {
		NotifType:    TypeNewOrder,
		TitleFn:      func(p map[string]any) string { return "New order " + strVal(p, "order_number") },
		ResourceType: "order",
	},
	outbox.EventReturnRequested: {
		NotifType:    TypeReturnRequested,
		TitleFn:      func(p map[string]any) string { return "Return requested for order " + strVal(p, "order_number") },
		ResourceType: "return",
	},
	outbox.EventPaymentReceived: {
		NotifType:    TypePaymentReceived,
		TitleFn:      func(p map[string]any) string { return "Payment received for order " + strVal(p, "order_number") },
		ResourceType: "order",
	},
	outbox.EventLowStock: {
		NotifType:    TypeLowStock,
		TitleFn:      func(p map[string]any) string { return "Low stock: " + strVal(p, "product_name") },
		ResourceType: "product",
	},
	outbox.EventReviewSubmitted: {
		NotifType:    TypeReviewSubmitted,
		TitleFn:      func(p map[string]any) string { return "New review on " + strVal(p, "product_name") },
		ResourceType: "product",
	},
}

func strVal(p map[string]any, key string) string {
	if v, ok := p[key].(string); ok {
		return v
	}
	return "(unknown)"
}

// Listener subscribes to outbox events and creates notifications based on
// preferences. Runs as a goroutine alongside the outbox publisher.
type Listener struct {
	repo     *Repository
	db       *gorm.DB
	logger   *slog.Logger
	interval time.Duration
	batch    int
}

// ListenerConfig configures a Listener.
type ListenerConfig struct {
	DB       *gorm.DB
	Logger   *slog.Logger
	Interval time.Duration // default 5s
	Batch    int           // default 50
}

// NewListener constructs a Listener.
func NewListener(cfg ListenerConfig) *Listener {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.Batch == 0 {
		cfg.Batch = 50
	}
	return &Listener{
		repo:     NewRepository(cfg.DB),
		db:       cfg.DB,
		logger:   cfg.Logger,
		interval: cfg.Interval,
		batch:    cfg.Batch,
	}
}

// Start runs the listener loop until ctx is cancelled. Returns a channel
// that closes when the loop exits (same pattern as outbox.Publisher.Start).
func (l *Listener) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(l.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := l.tick(ctx); err != nil && l.logger != nil {
					l.logger.Error("notification listener tick failed", "err", err)
				}
			}
		}
	}()
	return done
}

// tick processes a batch of published outbox events and creates notifications.
// It reads published events created after the last notification for efficiency.
//
// Strategy: scan outbox_events WHERE published_at IS NOT NULL AND created_at > last_seen
// (polling the same table the publisher already processes). This avoids needing
// a separate channel or Pub/Sub — the listener just trails behind the publisher.
func (l *Listener) tick(ctx context.Context) error {
	// Find the latest notification created_at to use as a cursor.
	var lastSeen time.Time
	var lastNotif Notification
	err := l.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(1).
		First(&lastNotif).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == nil {
		lastSeen = lastNotif.CreatedAt
	}

	// Read published outbox events after the cursor.
	var events []outbox.OutboxEvent
	err = l.db.WithContext(ctx).
		Where("published_at IS NOT NULL AND created_at > ?", lastSeen).
		Order("created_at ASC").
		Limit(l.batch).
		Find(&events).Error
	if err != nil {
		return err
	}

	for _, evt := range events {
		mapping, ok := eventMappings[evt.EventType]
		if !ok {
			continue // not a notification-worthy event
		}

		// Parse payload to get store_id and resource info.
		var payload map[string]any
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			l.logger.Warn("notification listener: unparseable payload", "event_id", evt.ID, "err", err)
			continue
		}

		storeIDStr, _ := payload["store_id"].(string)
		if storeIDStr == "" {
			continue
		}
		storeID, err := uuid.Parse(storeIDStr)
		if err != nil {
			continue
		}
		tenantID, _ := uuid.Parse(evt.TenantID)

		// Check if this notification type is enabled for the store.
		enabled, err := l.repo.IsTypeEnabled(ctx, storeID, mapping.NotifType)
		if err != nil {
			l.logger.Error("notification listener: check preferences", "err", err)
			continue
		}
		if !enabled {
			continue
		}

		// Build notification.
		notif := &Notification{
			TenantID: tenantID,
			StoreID:  storeID,
			Type:     mapping.NotifType,
			Title:    mapping.TitleFn(payload),
		}

		// Set resource type.
		rt := mapping.ResourceType
		notif.ResourceType = &rt

		// Set resource_id from aggregate_id.
		if rid, err := uuid.Parse(evt.AggregateID); err == nil {
			notif.ResourceID = &rid
		}

		// Set message from payload description if available.
		if msg, ok := payload["description"].(string); ok {
			notif.Message = &msg
		}

		if err := l.repo.Create(ctx, notif); err != nil {
			l.logger.Error("notification listener: create notification", "err", err)
		}
	}

	return nil
}

// ProcessEvent processes a single outbox event for testing. Exposed for unit tests.
func (l *Listener) ProcessEvent(ctx context.Context, evt outbox.OutboxEvent) error {
	mapping, ok := eventMappings[evt.EventType]
	if !ok {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return err
	}

	storeIDStr, _ := payload["store_id"].(string)
	if storeIDStr == "" {
		return nil
	}
	storeID, err := uuid.Parse(storeIDStr)
	if err != nil {
		return err
	}
	tenantID, _ := uuid.Parse(evt.TenantID)

	enabled, err := l.repo.IsTypeEnabled(ctx, storeID, mapping.NotifType)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	rt := mapping.ResourceType
	notif := &Notification{
		TenantID:     tenantID,
		StoreID:      storeID,
		Type:         mapping.NotifType,
		Title:        mapping.TitleFn(payload),
		ResourceType: &rt,
	}
	if rid, err := uuid.Parse(evt.AggregateID); err == nil {
		notif.ResourceID = &rid
	}

	return l.repo.Create(ctx, notif)
}
```

- [ ] **Step 3: Write listener tests**

Create `services/marketplace-api/internal/notification/listener_test.go`:

```go
package notification_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/outbox"
)

func TestListener_ProcessEvent_OrderPlaced(t *testing.T) {
	db := setupTestDB(t)
	// Also migrate outbox table for test data.
	require.NoError(t, db.AutoMigrate(&outbox.OutboxEvent{}))

	listener := notification.NewListener(notification.ListenerConfig{
		DB:     db,
		Logger: slog.Default(),
	})

	storeID := uuid.New()
	tenantID := uuid.New()
	orderID := uuid.New()

	evt := outbox.OutboxEvent{
		ID:          uuid.NewString(),
		TenantID:    tenantID.String(),
		Aggregate:   outbox.AggregateOrder,
		AggregateID: orderID.String(),
		EventType:   outbox.EventOrderPlaced,
		Payload:     datatypes.JSON([]byte(`{"store_id":"` + storeID.String() + `","order_number":"#1001"}`)),
	}

	err := listener.ProcessEvent(context.Background(), evt)
	require.NoError(t, err)

	repo := notification.NewRepository(db)
	notifications, err := repo.ListRecent(context.Background(), storeID, false)
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	assert.Equal(t, notification.TypeNewOrder, notifications[0].Type)
	assert.Equal(t, "New order #1001", notifications[0].Title)
	assert.Equal(t, &orderID, notifications[0].ResourceID)
}

func TestListener_ProcessEvent_ReturnRequested(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&outbox.OutboxEvent{}))

	listener := notification.NewListener(notification.ListenerConfig{
		DB:     db,
		Logger: slog.Default(),
	})

	storeID := uuid.New()
	tenantID := uuid.New()
	returnID := uuid.New()

	evt := outbox.OutboxEvent{
		ID:          uuid.NewString(),
		TenantID:    tenantID.String(),
		Aggregate:   outbox.AggregateReturn,
		AggregateID: returnID.String(),
		EventType:   outbox.EventReturnRequested,
		Payload:     datatypes.JSON([]byte(`{"store_id":"` + storeID.String() + `","order_number":"#2002"}`)),
	}

	err := listener.ProcessEvent(context.Background(), evt)
	require.NoError(t, err)

	repo := notification.NewRepository(db)
	notifications, _ := repo.ListRecent(context.Background(), storeID, false)
	require.Len(t, notifications, 1)
	assert.Equal(t, notification.TypeReturnRequested, notifications[0].Type)
	assert.Contains(t, notifications[0].Title, "#2002")
}

func TestListener_ProcessEvent_DisabledType(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&outbox.OutboxEvent{}))

	listener := notification.NewListener(notification.ListenerConfig{
		DB:     db,
		Logger: slog.Default(),
	})

	storeID := uuid.New()
	tenantID := uuid.New()

	// Disable new_order notifications.
	repo := notification.NewRepository(db)
	require.NoError(t, repo.UpsertPreferences(context.Background(), tenantID, storeID, map[string]bool{
		"new_order": false,
	}))

	evt := outbox.OutboxEvent{
		ID:          uuid.NewString(),
		TenantID:    tenantID.String(),
		Aggregate:   outbox.AggregateOrder,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventOrderPlaced,
		Payload:     datatypes.JSON([]byte(`{"store_id":"` + storeID.String() + `","order_number":"#3003"}`)),
	}

	err := listener.ProcessEvent(context.Background(), evt)
	require.NoError(t, err)

	notifications, _ := repo.ListRecent(context.Background(), storeID, false)
	assert.Len(t, notifications, 0, "disabled notification type should not create notification")
}

func TestListener_ProcessEvent_UnknownEventType(t *testing.T) {
	db := setupTestDB(t)

	listener := notification.NewListener(notification.ListenerConfig{
		DB:     db,
		Logger: slog.Default(),
	})

	evt := outbox.OutboxEvent{
		ID:          uuid.NewString(),
		TenantID:    uuid.NewString(),
		Aggregate:   "unknown",
		AggregateID: uuid.NewString(),
		EventType:   "unknown.event",
		Payload:     datatypes.JSON([]byte(`{"store_id":"` + uuid.NewString() + `"}`)),
	}

	err := listener.ProcessEvent(context.Background(), evt)
	require.NoError(t, err, "unknown event type should be silently ignored")
}

func TestListener_ProcessEvent_MissingStoreID(t *testing.T) {
	db := setupTestDB(t)

	listener := notification.NewListener(notification.ListenerConfig{
		DB:     db,
		Logger: slog.Default(),
	})

	evt := outbox.OutboxEvent{
		ID:          uuid.NewString(),
		TenantID:    uuid.NewString(),
		Aggregate:   outbox.AggregateOrder,
		AggregateID: uuid.NewString(),
		EventType:   outbox.EventOrderPlaced,
		Payload:     datatypes.JSON([]byte(`{"order_number":"#4004"}`)),
	}

	err := listener.ProcessEvent(context.Background(), evt)
	require.NoError(t, err, "missing store_id should be silently ignored")
}
```

### GREEN

- [ ] **Step 4: Run tests**

```bash
cd services/marketplace-api && go test ./internal/notification/... -v -count=1
```

All tests must pass.

**Commit:** `feat(notification): add listener that converts outbox events to notifications based on preferences`

---

## Task 4: Authz roles + HTTP handler

**Files:**
- Create: `services/marketplace-api/internal/authz/notification_roles.go`
- Create: `services/marketplace-api/internal/notification/handler.go`
- Create: `services/marketplace-api/internal/notification/handler_test.go`

### TDD: RED

- [ ] **Step 1: Create notification role constants**

Create `services/marketplace-api/internal/authz/notification_roles.go`:

```go
package authz

// Notification settings — staff and above can view notifications.
// Admin and above can edit notification preferences.

// NotificationsViewRole allows viewing and interacting with notifications.
var NotificationsViewRole = RoleStaff

// NotificationsPreferencesRole allows editing notification preferences.
var NotificationsPreferencesRole = RoleAdmin
```

- [ ] **Step 2: Create handler.go**

Create `services/marketplace-api/internal/notification/handler.go`:

```go
package notification

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

// Handler provides HTTP handlers for notifications and preferences.
type Handler struct {
	repo   *Repository
	logger *slog.Logger
}

// NewHandler constructs a notification Handler.
func NewHandler(db *gorm.DB, logger *slog.Logger) *Handler {
	return &Handler{
		repo:   NewRepository(db),
		logger: logger,
	}
}

// storeFromCtx extracts the *stores.Store set by StoreMiddleware.
func storeFromCtx(c *gin.Context) *stores.Store {
	v, ok := c.Get("store")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing store context",
		})
		return nil
	}
	s, _ := v.(*stores.Store)
	if s == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing store context",
		})
		return nil
	}
	return s
}

// ──────────────────────────────────────────────────────────────────────────
// Notifications
// ──────────────────────────────────────────────────────────────────────────

// ListNotifications handles GET /admin/stores/:storeId/notifications.
// Query params: ?unread_only=true
func (h *Handler) ListNotifications(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	unreadOnly := c.Query("unread_only") == "true"

	notifications, err := h.repo.ListRecent(c.Request.Context(), store.ID, unreadOnly)
	if err != nil {
		h.logger.Error("list notifications", "store_id", store.ID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to list notifications"})
		return
	}

	resp := make([]NotificationResponse, len(notifications))
	for i, n := range notifications {
		resp[i] = n.ToResponse()
	}

	c.JSON(http.StatusOK, resp)
}

// UnreadCount handles GET /admin/stores/:storeId/notifications/unread-count.
func (h *Handler) UnreadCount(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	count, err := h.repo.UnreadCount(c.Request.Context(), store.ID)
	if err != nil {
		h.logger.Error("unread count", "store_id", store.ID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to count notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

// MarkRead handles PATCH /admin/stores/:storeId/notifications/:id/read.
func (h *Handler) MarkRead(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	notifID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation", "message": "invalid notification ID"})
		return
	}

	if err := h.repo.MarkRead(c.Request.Context(), notifID, store.ID); err != nil {
		h.logger.Error("mark read", "notif_id", notifID, "err", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "notification not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// MarkAllRead handles PATCH /admin/stores/:storeId/notifications/read-all.
func (h *Handler) MarkAllRead(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	affected, err := h.repo.MarkAllRead(c.Request.Context(), store.ID)
	if err != nil {
		h.logger.Error("mark all read", "store_id", store.ID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to mark all read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "affected": affected})
}

// ──────────────────────────────────────────────────────────────────────────
// Notification preferences
// ──────────────────────────────────────────────────────────────────────────

// GetPreferences handles GET /admin/stores/:storeId/notification-preferences.
func (h *Handler) GetPreferences(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	prefs, err := h.repo.GetPreferences(c.Request.Context(), store.ID)
	if err != nil {
		h.logger.Error("get preferences", "store_id", store.ID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to get preferences"})
		return
	}

	c.JSON(http.StatusOK, prefs)
}

// updatePreferencesRequest is the request body for PATCH /notification-preferences.
type updatePreferencesRequest struct {
	Preferences map[string]bool `json:"preferences" binding:"required"`
}

// UpdatePreferences handles PATCH /admin/stores/:storeId/notification-preferences.
func (h *Handler) UpdatePreferences(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	var req updatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation", "message": "preferences object required"})
		return
	}

	// Validate that only known preference types are provided.
	allowedTypes := make(map[string]bool)
	for _, t := range AllPreferenceTypes {
		allowedTypes[t] = true
	}
	for key := range req.Preferences {
		if !allowedTypes[key] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "validation",
				"message": "unknown notification type: " + key,
			})
			return
		}
	}

	if err := h.repo.UpsertPreferences(c.Request.Context(), store.TenantID, store.ID, req.Preferences); err != nil {
		h.logger.Error("update preferences", "store_id", store.ID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to update preferences"})
		return
	}

	c.JSON(http.StatusOK, req.Preferences)
}
```

- [ ] **Step 3: Write handler tests**

Create `services/marketplace-api/internal/notification/handler_test.go`:

```go
package notification_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

func setupHandlerRouter(t *testing.T) (*gin.Engine, *notification.Repository, uuid.UUID) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := setupTestDB(t)
	handler := notification.NewHandler(db, slog.Default())
	repo := notification.NewRepository(db)

	storeID := uuid.New()
	tenantID := uuid.New()

	r := gin.New()
	group := r.Group("/api/v1/admin/stores/:storeId", func(c *gin.Context) {
		c.Set("store", &stores.Store{ID: storeID, TenantID: tenantID})
		c.Set("tenant_id", tenantID.String())
		c.Next()
	})
	group.GET("/notifications", handler.ListNotifications)
	group.GET("/notifications/unread-count", handler.UnreadCount)
	group.PATCH("/notifications/:id/read", handler.MarkRead)
	group.PATCH("/notifications/read-all", handler.MarkAllRead)
	group.GET("/notification-preferences", handler.GetPreferences)
	group.PATCH("/notification-preferences", handler.UpdatePreferences)

	return r, repo, storeID
}

func TestHandler_ListNotifications_Empty(t *testing.T) {
	r, _, storeID := setupHandlerRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/admin/stores/%s/notifications", storeID), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp []any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp, 0)
}

func TestHandler_UnreadCount(t *testing.T) {
	r, repo, storeID := setupHandlerRouter(t)

	// Create 3 notifications.
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(ctx(), &notification.Notification{
			TenantID: uuid.New(), StoreID: storeID, Type: "new_order", Title: "Order",
		}))
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/admin/stores/%s/notifications/unread-count", storeID), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(3), resp["count"])
}

func TestHandler_MarkRead(t *testing.T) {
	r, repo, storeID := setupHandlerRouter(t)

	n := &notification.Notification{
		TenantID: uuid.New(), StoreID: storeID, Type: "new_order", Title: "Order #1",
	}
	require.NoError(t, repo.Create(ctx(), n))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH",
		fmt.Sprintf("/api/v1/admin/stores/%s/notifications/%s/read", storeID, n.ID),
		nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	count, _ := repo.UnreadCount(ctx(), storeID)
	assert.Equal(t, int64(0), count)
}

func TestHandler_MarkAllRead(t *testing.T) {
	r, repo, storeID := setupHandlerRouter(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Create(ctx(), &notification.Notification{
			TenantID: uuid.New(), StoreID: storeID, Type: "new_order", Title: "Order",
		}))
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH",
		fmt.Sprintf("/api/v1/admin/stores/%s/notifications/read-all", storeID),
		nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(5), resp["affected"])
}

func TestHandler_GetPreferences_Defaults(t *testing.T) {
	r, _, storeID := setupHandlerRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET",
		fmt.Sprintf("/api/v1/admin/stores/%s/notification-preferences", storeID),
		nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]bool
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp["new_order"])
	assert.True(t, resp["low_stock"])
}

func TestHandler_UpdatePreferences(t *testing.T) {
	r, _, storeID := setupHandlerRouter(t)

	body, _ := json.Marshal(map[string]any{
		"preferences": map[string]bool{
			"new_order":        true,
			"low_stock":        false,
			"return_requested": true,
			"payment_received": false,
			"review_submitted": true,
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH",
		fmt.Sprintf("/api/v1/admin/stores/%s/notification-preferences", storeID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]bool
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp["low_stock"])
	assert.True(t, resp["new_order"])
}

func TestHandler_UpdatePreferences_UnknownType(t *testing.T) {
	r, _, storeID := setupHandlerRouter(t)

	body, _ := json.Marshal(map[string]any{
		"preferences": map[string]bool{
			"unknown_type": true,
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH",
		fmt.Sprintf("/api/v1/admin/stores/%s/notification-preferences", storeID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func ctx() context.Context { return context.Background() }
```

**Note:** Add `"context"` to the import block.

### GREEN

- [ ] **Step 4: Run tests**

```bash
cd services/marketplace-api && go test ./internal/notification/... -v -count=1
```

All tests must pass.

**Commit:** `feat(notification): add HTTP handlers for notifications, unread count, and preferences`

---

## Task 5: Route wiring + listener startup

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Add NotificationHandler to Deps struct**

In `services/marketplace-api/internal/handlers/admin/routes.go`, add to the `Deps` struct:

```go
NotificationHandler      *notification.Handler    // S5: notifications
```

Add the import:

```go
"github.com/mark8ly/marketplace-api/internal/notification"
```

- [ ] **Step 2: Add notification routes to RegisterAdmin**

In `RegisterAdmin`, after the audit logs block, add:

```go
		// Notifications — S5.
		if deps.NotificationHandler != nil {
			notifs := storeRoute.Group("/notifications")
			{
				notifs.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
					deps.NotificationHandler.ListNotifications)
				notifs.GET("/unread-count",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
					deps.NotificationHandler.UnreadCount)
				notifs.PATCH("/:id/read",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
					deps.NotificationHandler.MarkRead)
				notifs.PATCH("/read-all",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
					deps.NotificationHandler.MarkAllRead)
			}

			notifPrefs := storeRoute.Group("/notification-preferences")
			{
				notifPrefs.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
					deps.NotificationHandler.GetPreferences)
				notifPrefs.PATCH("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsPreferencesRole),
					deps.NotificationHandler.UpdatePreferences)
			}
		}
```

- [ ] **Step 3: Wire handler + start listener in main.go**

In `services/marketplace-api/cmd/marketplace-api/main.go`, in the admin deps construction block:

```go
	// Notification handler (S5).
	notificationHandler := notification.NewHandler(conn, log)
```

Add to the `adminDeps` struct literal:

```go
	NotificationHandler: notificationHandler,
```

Then, in the goroutine startup section (after the outbox publisher start, around line 335), add the notification listener:

```go
	// Notification listener — runs in admin and both modes.
	// Trails behind the outbox publisher, converting published events into
	// notifications based on per-store preferences.
	var notifListenerDone <-chan struct{}
	if m == mode.Admin || m == mode.Both {
		notifListener := notification.NewListener(notification.ListenerConfig{
			DB:       conn,
			Logger:   log,
			Interval: 5 * time.Second,
			Batch:    50,
		})
		notifListenerDone = notifListener.Start(publisherCtx) // shares publisher's ctx for coordinated shutdown
		log.Info("notification listener started")
	}
```

In the graceful shutdown section, wait for the listener:

```go
	if notifListenerDone != nil {
		<-notifListenerDone
		log.Info("notification listener stopped")
	}
```

- [ ] **Step 4: Build check**

```bash
cd services/marketplace-api && go build ./...
```

Must compile without errors.

**Commit:** `feat(notification): wire notification routes, handler, and listener goroutine in main.go`

---

## Task 6: Admin UI — notification preferences page

**Files:**
- Create: `apps/admin/lib/api/notification-api.ts`
- Create: `apps/admin/app/settings/notifications/page.tsx`
- Create: `apps/admin/app/settings/notifications/actions.ts`
- Create: `apps/admin/components/settings/NotificationPreferencesClient.tsx`
- Modify: `apps/admin/components/shell/AdminShell.tsx`

- [ ] **Step 1: Create notification-api.ts**

Create `apps/admin/lib/api/notification-api.ts`:

```typescript
import type { SessionHeaders } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface NotificationData {
  id: string;
  type: string;
  title: string;
  message?: string;
  resource_type?: string;
  resource_id?: string;
  is_read: boolean;
  created_at: string;
}

export interface NotificationPreferences {
  new_order: boolean;
  low_stock: boolean;
  return_requested: boolean;
  payment_received: boolean;
  review_submitted: boolean;
  [key: string]: boolean;
}

export async function getNotifications(
  storeId: string,
  headers: SessionHeaders,
  unreadOnly = false,
): Promise<NotificationData[]> {
  const params = unreadOnly ? "?unread_only=true" : "";
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications${params}`,
    { headers, cache: "no-store" },
  );
  if (!res.ok) {
    throw new Error(`Failed to fetch notifications: ${res.status}`);
  }
  return res.json();
}

export async function getUnreadCount(
  storeId: string,
  headers: SessionHeaders,
): Promise<number> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/unread-count`,
    { headers, cache: "no-store" },
  );
  if (!res.ok) {
    return 0; // fail silently for polling
  }
  const data = await res.json();
  return data.count ?? 0;
}

export async function markNotificationRead(
  storeId: string,
  notificationId: string,
  headers: SessionHeaders,
): Promise<void> {
  await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/${notificationId}/read`,
    { method: "PATCH", headers },
  );
}

export async function markAllNotificationsRead(
  storeId: string,
  headers: SessionHeaders,
): Promise<void> {
  await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/read-all`,
    { method: "PATCH", headers },
  );
}

export async function getNotificationPreferences(
  storeId: string,
  headers: SessionHeaders,
): Promise<NotificationPreferences> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notification-preferences`,
    { headers, cache: "no-store" },
  );
  if (!res.ok) {
    throw new Error(`Failed to fetch preferences: ${res.status}`);
  }
  return res.json();
}

export async function updateNotificationPreferences(
  storeId: string,
  preferences: NotificationPreferences,
  headers: SessionHeaders,
): Promise<NotificationPreferences> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notification-preferences`,
    {
      method: "PATCH",
      headers: { ...headers, "Content-Type": "application/json" },
      body: JSON.stringify({ preferences }),
    },
  );
  if (!res.ok) {
    throw new Error(`Failed to update preferences: ${res.status}`);
  }
  return res.json();
}
```

- [ ] **Step 2: Create server actions**

Create `apps/admin/app/settings/notifications/actions.ts`:

```typescript
"use server";

import { revalidatePath } from "next/cache";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import {
  updateNotificationPreferences,
  type NotificationPreferences,
} from "@/lib/api/notification-api";

export async function updatePreferencesAction(
  storeId: string,
  preferences: NotificationPreferences,
) {
  const { sessionHeaders } = await getServerSessionContext();
  await updateNotificationPreferences(storeId, preferences, sessionHeaders);
  revalidatePath("/settings/notifications");
}
```

- [ ] **Step 3: Create NotificationPreferencesClient.tsx**

Create `apps/admin/components/settings/NotificationPreferencesClient.tsx`:

```tsx
"use client";

import { useState, useTransition } from "react";
import { updatePreferencesAction } from "@/app/settings/notifications/actions";
import type { NotificationPreferences } from "@/lib/api/notification-api";

const preferenceLabels: Record<string, { label: string; description: string }> = {
  new_order: {
    label: "New orders",
    description: "When a customer places a new order",
  },
  low_stock: {
    label: "Low stock alerts",
    description: "When a product variant falls below the low-stock threshold",
  },
  return_requested: {
    label: "Return requests",
    description: "When a customer requests a return or exchange",
  },
  payment_received: {
    label: "Payments received",
    description: "When a payment is successfully processed",
  },
  review_submitted: {
    label: "Product reviews",
    description: "When a customer submits a new product review",
  },
};

interface NotificationPreferencesClientProps {
  storeId: string;
  preferences: NotificationPreferences;
  editable: boolean;
}

export function NotificationPreferencesClient({
  storeId,
  preferences,
  editable,
}: NotificationPreferencesClientProps) {
  const [prefs, setPrefs] = useState<NotificationPreferences>(preferences);
  const [isPending, startTransition] = useTransition();
  const [saved, setSaved] = useState(false);

  function handleToggle(key: string) {
    const updated = { ...prefs, [key]: !prefs[key] };
    setPrefs(updated);
    setSaved(false);

    startTransition(async () => {
      await updatePreferencesAction(storeId, updated);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    });
  }

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        {saved && (
          <p className="text-sm text-[color:var(--moss-700)]">
            Preferences saved.
          </p>
        )}
      </div>

      <div className="divide-y divide-[color:var(--ink-900)]/5">
        {Object.entries(preferenceLabels).map(([key, { label, description }]) => (
          <div
            key={key}
            className="flex items-center justify-between py-4"
          >
            <div className="space-y-0.5">
              <p className="text-sm font-medium text-foreground">{label}</p>
              <p className="text-sm text-foreground-secondary">{description}</p>
            </div>
            <button
              type="button"
              role="switch"
              aria-checked={prefs[key] ?? true}
              disabled={!editable || isPending}
              onClick={() => handleToggle(key)}
              className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-[color:var(--moss-700)] focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 ${
                prefs[key]
                  ? "bg-[color:var(--moss-700)]"
                  : "bg-[color:var(--ink-900)]/20"
              }`}
            >
              <span
                className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
                  prefs[key] ? "translate-x-5" : "translate-x-0"
                }`}
              />
            </button>
          </div>
        ))}
      </div>

      {!editable && (
        <p className="text-sm text-warning">
          Your role can view notification settings but cannot change them.
        </p>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Create page.tsx**

Create `apps/admin/app/settings/notifications/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getNotificationPreferences } from "@/lib/api/notification-api";
import { NotificationPreferencesClient } from "@/components/settings/NotificationPreferencesClient";

export default async function NotificationsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    currentStore,
  } = await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-3xl space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Store setup</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Notifications
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Choose which events trigger in-app notifications in the bell
            dropdown.
          </p>
        </header>

        {currentStore ? (
          <NotificationsContent
            storeId={currentStore.id}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store first.
          </p>
        )}
      </div>
    </AdminShell>
  );
}

async function NotificationsContent({
  storeId,
  editable,
}: {
  storeId: string;
  editable: boolean;
}) {
  const preferences = await getNotificationPreferences(storeId, {} as any);

  return (
    <NotificationPreferencesClient
      storeId={storeId}
      preferences={preferences}
      editable={editable}
    />
  );
}
```

- [ ] **Step 5: Add "Notifications" to sidebar**

In `apps/admin/components/shell/AdminShell.tsx`, add after "Audit Logs" in the settings children:

```typescript
      { label: "Notifications", href: "/settings/notifications" },
```

**Commit:** `feat(notification): add notification preferences page with toggle grid`

---

## Task 7: Bell dropdown component

**Files:**
- Create: `apps/admin/components/shell/NotificationBell.tsx`
- Modify: `apps/admin/components/shell/AdminShell.tsx`

- [ ] **Step 1: Create NotificationBell.tsx**

Create `apps/admin/components/shell/NotificationBell.tsx`:

```tsx
"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Bell } from "lucide-react";
import type { NotificationData } from "@/lib/api/notification-api";

const POLL_INTERVAL = 30_000; // 30 seconds

interface NotificationBellProps {
  storeId: string;
}

// Type-to-icon mapping and resource URL mapping.
const typeIcons: Record<string, string> = {
  new_order: "\uD83D\uDCE6",
  low_stock: "\u26A0\uFE0F",
  return_requested: "\u21A9\uFE0F",
  payment_received: "\uD83D\uDCB0",
  review_submitted: "\u2B50",
  domain_verified: "\uD83C\uDF10",
  domain_error: "\u274C",
  subscription_expiring: "\u23F3",
  subscription_cancelled: "\uD83D\uDEAB",
};

function resourceURL(n: NotificationData): string | null {
  if (!n.resource_type || !n.resource_id) return null;
  switch (n.resource_type) {
    case "order":
      return `/orders/${n.resource_id}`;
    case "return":
      return `/returns/${n.resource_id}`;
    case "product":
      return `/products/${n.resource_id}`;
    default:
      return null;
  }
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor(
    (Date.now() - new Date(dateStr).getTime()) / 1000,
  );
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function NotificationBell({ storeId }: NotificationBellProps) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);
  const [notifications, setNotifications] = useState<NotificationData[]>([]);
  const [loading, setLoading] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Poll unread count every 30s.
  const fetchUnreadCount = useCallback(async () => {
    try {
      const res = await fetch(
        `/api/notifications/unread-count?storeId=${storeId}`,
      );
      if (res.ok) {
        const data = await res.json();
        setUnreadCount(data.count ?? 0);
      }
    } catch {
      // silently fail on poll error
    }
  }, [storeId]);

  useEffect(() => {
    fetchUnreadCount();
    const interval = setInterval(fetchUnreadCount, POLL_INTERVAL);
    return () => clearInterval(interval);
  }, [fetchUnreadCount]);

  // Fetch full notification list when dropdown opens.
  async function handleOpen() {
    setOpen((prev) => !prev);
    if (!open) {
      setLoading(true);
      try {
        const res = await fetch(
          `/api/notifications?storeId=${storeId}`,
        );
        if (res.ok) {
          setNotifications(await res.json());
        }
      } catch {
        // fail silently
      } finally {
        setLoading(false);
      }
    }
  }

  // Mark single notification read + navigate.
  async function handleClick(n: NotificationData) {
    if (!n.is_read) {
      fetch(`/api/notifications/${n.id}/read?storeId=${storeId}`, {
        method: "PATCH",
      }).catch(() => {});
      setNotifications((prev) =>
        prev.map((item) =>
          item.id === n.id ? { ...item, is_read: true } : item,
        ),
      );
      setUnreadCount((prev) => Math.max(0, prev - 1));
    }

    const url = resourceURL(n);
    if (url) {
      setOpen(false);
      router.push(url);
    }
  }

  // Mark all read.
  async function handleMarkAllRead() {
    fetch(`/api/notifications/read-all?storeId=${storeId}`, {
      method: "PATCH",
    }).catch(() => {});
    setNotifications((prev) =>
      prev.map((item) => ({ ...item, is_read: true })),
    );
    setUnreadCount(0);
  }

  // Close on outside click.
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    if (open) {
      document.addEventListener("mousedown", handleClickOutside);
    }
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        type="button"
        onClick={handleOpen}
        className="hidden h-11 w-11 items-center justify-center rounded-md text-foreground-secondary transition-colors hover:bg-paper-100 hover:text-foreground sm:inline-flex"
        aria-label={`Notifications${unreadCount > 0 ? `, ${unreadCount} unread` : ""}`}
        aria-expanded={open}
        aria-haspopup="true"
      >
        <Bell className="h-4 w-4" aria-hidden="true" />
        {unreadCount > 0 && (
          <span className="absolute right-1.5 top-1.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-[color:var(--signal)] px-1 text-[10px] font-bold text-white">
            {unreadCount > 99 ? "99+" : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-2 w-80 rounded-md bg-white shadow-[var(--shadow-2)] ring-1 ring-[color:var(--ink-900)]/10 sm:w-96">
          <div className="flex items-center justify-between border-b border-[color:var(--ink-900)]/10 px-4 py-3">
            <p className="text-sm font-medium text-foreground">
              Notifications
            </p>
            {unreadCount > 0 && (
              <button
                type="button"
                onClick={handleMarkAllRead}
                className="text-xs text-[color:var(--moss-700)] hover:underline"
              >
                Mark all read
              </button>
            )}
          </div>

          <div className="max-h-96 overflow-y-auto">
            {loading ? (
              <div className="px-4 py-8 text-center text-sm text-foreground-secondary">
                Loading...
              </div>
            ) : notifications.length === 0 ? (
              <div className="px-4 py-8 text-center text-sm text-foreground-secondary">
                No notifications yet.
              </div>
            ) : (
              notifications.map((n) => (
                <button
                  key={n.id}
                  type="button"
                  onClick={() => handleClick(n)}
                  className={`flex w-full gap-3 px-4 py-3 text-left transition-colors hover:bg-paper-100 ${
                    !n.is_read ? "bg-[color:var(--moss-700)]/5" : ""
                  }`}
                >
                  <span className="mt-0.5 text-base" aria-hidden="true">
                    {typeIcons[n.type] ?? "\uD83D\uDD14"}
                  </span>
                  <div className="min-w-0 flex-1">
                    <p
                      className={`truncate text-sm ${
                        !n.is_read
                          ? "font-medium text-foreground"
                          : "text-foreground-secondary"
                      }`}
                    >
                      {n.title}
                    </p>
                    {n.message && (
                      <p className="truncate text-xs text-foreground-secondary">
                        {n.message}
                      </p>
                    )}
                    <p className="mt-0.5 text-xs text-foreground-secondary/70">
                      {timeAgo(n.created_at)}
                    </p>
                  </div>
                  {!n.is_read && (
                    <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-[color:var(--moss-700)]" />
                  )}
                </button>
              ))
            )}
          </div>

          <div className="border-t border-[color:var(--ink-900)]/10 px-4 py-2">
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                router.push("/settings/notifications");
              }}
              className="text-xs text-[color:var(--moss-700)] hover:underline"
            >
              Notification settings
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Replace static bell button in AdminShell**

In `apps/admin/components/shell/AdminShell.tsx`, find the static bell button (around line 292-298):

```tsx
              <button
                type="button"
                className="hidden h-11 w-11 items-center justify-center rounded-md text-foreground-secondary transition-colors hover:bg-paper-100 hover:text-foreground sm:inline-flex"
                aria-label="Notifications"
              >
                <Bell className="h-4 w-4" aria-hidden="true" />
              </button>
```

Replace it with:

```tsx
              <NotificationBell storeId={currentStoreId ?? ""} />
```

Where `currentStoreId` is derived from the AdminShell props. Add the import at the top:

```tsx
import { NotificationBell } from "./NotificationBell";
```

If `currentStoreId` doesn't exist as a prop, derive it from the existing `currentStore` prop that's already passed to the page components. You may need to thread it through the AdminShell props:

```tsx
// In AdminShell props interface, add:
currentStoreId?: string;
```

And pass it from each page that renders AdminShell.

**Fallback:** If threading `currentStoreId` is too invasive, use a client-side hook that reads the store from a cookie or context. The simplest approach is to read it from the URL or a zustand store.

- [ ] **Step 3: Create API routes for client-side bell polling**

The NotificationBell component fetches from `/api/notifications/*` (Next.js API routes that proxy to marketplace-api). Create these API routes:

Create `apps/admin/app/api/notifications/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export async function GET(req: NextRequest) {
  const storeId = req.nextUrl.searchParams.get("storeId");
  if (!storeId) {
    return NextResponse.json([], { status: 200 });
  }

  // Forward session headers from the incoming request.
  const headers: Record<string, string> = {};
  for (const [key, value] of req.headers.entries()) {
    if (key.startsWith("x-") || key === "cookie") {
      headers[key] = value;
    }
  }

  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications`,
    { headers, cache: "no-store" },
  );

  const data = await res.json().catch(() => []);
  return NextResponse.json(data, { status: res.status });
}
```

Create `apps/admin/app/api/notifications/unread-count/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export async function GET(req: NextRequest) {
  const storeId = req.nextUrl.searchParams.get("storeId");
  if (!storeId) {
    return NextResponse.json({ count: 0 });
  }

  const headers: Record<string, string> = {};
  for (const [key, value] of req.headers.entries()) {
    if (key.startsWith("x-") || key === "cookie") {
      headers[key] = value;
    }
  }

  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/unread-count`,
    { headers, cache: "no-store" },
  );

  const data = await res.json().catch(() => ({ count: 0 }));
  return NextResponse.json(data);
}
```

Create `apps/admin/app/api/notifications/[id]/read/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export async function PATCH(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const storeId = req.nextUrl.searchParams.get("storeId");
  if (!storeId) {
    return NextResponse.json({ error: "missing storeId" }, { status: 400 });
  }

  const headers: Record<string, string> = {};
  for (const [key, value] of req.headers.entries()) {
    if (key.startsWith("x-") || key === "cookie") {
      headers[key] = value;
    }
  }

  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/${id}/read`,
    { method: "PATCH", headers },
  );

  const data = await res.json().catch(() => ({}));
  return NextResponse.json(data, { status: res.status });
}
```

Create `apps/admin/app/api/notifications/read-all/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export async function PATCH(req: NextRequest) {
  const storeId = req.nextUrl.searchParams.get("storeId");
  if (!storeId) {
    return NextResponse.json({ error: "missing storeId" }, { status: 400 });
  }

  const headers: Record<string, string> = {};
  for (const [key, value] of req.headers.entries()) {
    if (key.startsWith("x-") || key === "cookie") {
      headers[key] = value;
    }
  }

  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/read-all`,
    { method: "PATCH", headers },
  );

  const data = await res.json().catch(() => ({}));
  return NextResponse.json(data, { status: res.status });
}
```

- [ ] **Step 4: Verify frontend builds**

```bash
cd apps/admin && npx next build
```

Must compile without type errors.

**Commit:** `feat(notification): add bell dropdown with unread badge, 30s poll, and notification preferences sidebar link`

---

## Task 8: E2E smoke tests

**Files:**
- Create: `apps/admin/e2e/notifications.spec.ts`

- [ ] **Step 1: Write Playwright tests**

Create `apps/admin/e2e/notifications.spec.ts`:

```typescript
import { test, expect } from "@playwright/test";

test.describe("Notification Settings", () => {
  test("renders notification preferences page", async ({ page }) => {
    await page.goto("/settings/notifications");
    await expect(page.getByText("Notifications")).toBeVisible();
    await expect(page.getByText("New orders")).toBeVisible();
    await expect(page.getByText("Low stock alerts")).toBeVisible();
  });

  test("shows toggle switches for each notification type", async ({ page }) => {
    await page.goto("/settings/notifications");
    const switches = page.getByRole("switch");
    await expect(switches).toHaveCount(5);
  });

  test("sidebar contains Notifications link", async ({ page }) => {
    await page.goto("/settings/notifications");
    const sidebar = page.locator("aside");
    await expect(sidebar.getByText("Notifications")).toBeVisible();
  });
});

test.describe("Notification Bell", () => {
  test("bell icon is visible in header", async ({ page }) => {
    await page.goto("/settings/notifications");
    await expect(
      page.getByRole("button", { name: /notifications/i }),
    ).toBeVisible();
  });

  test("bell dropdown opens on click", async ({ page }) => {
    await page.goto("/settings/notifications");
    await page.getByRole("button", { name: /notifications/i }).click();
    await expect(page.getByText("Notification settings")).toBeVisible();
  });
});
```

- [ ] **Step 2: Run E2E tests**

```bash
cd apps/admin && npx playwright test e2e/notifications.spec.ts
```

**Commit:** `test(notification): add Playwright E2E smoke tests for notification preferences and bell dropdown`

---

## Summary

| Task | What it delivers | Files |
|------|-----------------|-------|
| 0 | Prerequisites check | read-only |
| 1 | Migration 000017 | 2 SQL files |
| 2 | GORM models + repository + tests | 3 Go files |
| 3 | Notification listener + tests | 2 Go files + 1 modified |
| 4 | Authz roles + HTTP handler + tests | 3 Go files |
| 5 | Route wiring + listener startup | 2 Go files modified |
| 6 | Admin UI (preferences page, API client, sidebar) | 5 TS/TSX files |
| 7 | Bell dropdown + API routes | 6 TS/TSX files + 1 modified |
| 8 | E2E smoke test | 1 TS file |

**No environment variables required** — notification system is entirely internal to marketplace-api.

**Key design decisions:**
- Listener polls outbox_events table (same as publisher) — no new Pub/Sub or channels
- Listener trails behind publisher — reads only published events (published_at IS NOT NULL)
- Preferences stored as JSONB — flexible for adding new types without migration
- Bell dropdown polls unread count via Next.js API route proxy — avoids CORS, reuses session
- 30s poll interval — lightweight enough for the db-f1-micro constraint
- Notifications scoped by store_id — no cross-tenant leak
- No sensitive data in notification messages — order numbers only, never amounts
