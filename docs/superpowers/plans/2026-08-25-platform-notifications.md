# `GET /admin/notifications` (#332) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve the platform console a cross-tenant, read-only log of in-app notifications at `GET /api/v1/platform/admin/notifications`.

**Architecture:** A new cross-store `ListPlatform` method on `internal/notification`'s repository (the existing store-scoped methods are fail-safe and stay untouched), a projecting handler in `internal/handlers/platformadmin` copied from `tickets.go`, and a `created_at` index so the cross-store ordering has something to use. No writes, so no audit emitter and no operator/capability enforcement.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, `golang-migrate`, `testify/require`.

**Spec:** `docs/superpowers/specs/2026-08-24-platform-notifications-design.md`

## Global Constraints

- **Service is `marketplace-api`.** Every path below is relative to `services/marketplace-api/`. Do not touch `platform-api` — it has its own migration sequence and its own `notification` package.
- **Envelope is exactly** `{"data": [...], "pagination": {"page","limit","total"}}`. Never `meta`.
- **Empty results are `200` + `[]`.** Allocate with `make([]notificationRow, 0, n)` — a nil slice marshals to `null` and defeats the console's `?? []`.
- **`pagination.limit` reports the effective (clamped) limit**, so `total / limit` is a correct page count.
- **`limit`: default 50, clamp 500.** Oversized clamps; missing takes the default; neither is ever an error.
- **Timestamps RFC3339, UTC, with offset:** `t.UTC().Format(time.RFC3339)`.
- **Ids bare** — no `mark8ly:` prefix. The platform API namespaces on arrival.
- **Never send a `source` field.** The platform API stamps it and overwrites the body.
- **Project, do not pass through.** `toNotificationRow` maps field by field. A column added to `notification.Notification` tomorrow must not reach the console automatically.
- **`message` and `status` must never appear in the response.** `message` is the interpolated body; there is no delivery status in this estate (see #348).
- **Integration tests:** `//go:build integration`, `-p 1`, and the LAN IP DSN — `postgres://dev:dev@192.168.1.110:5432/marketplace_db`. **Never `localhost`**; a native Postgres squats on `127.0.0.1`. The env var this repo reads is `TEST_DATABASE_URL`.
- **`go vet -tags=integration ./...` is part of every verification step.** The default toolchain never compiles build-tagged files.
- **`go test` must be run from the service root (`./...`), not path-scoped.** `go test ./internal/...` excludes the root package, and with it the schema-version guard that stops a crashloop.

---

### Task 1: Migration `000102` and the schema-version bump

The two existing indexes on `notifications` are both `(store_id, …)`. This endpoint orders by `created_at DESC` across every store, with nothing to use. `ExpectedSchemaVersion` must move with the migration: `AssertVersion` (`pkg/migrate/migrate.go:110-122`) requires **exact equality** and refuses startup on a mismatch, which crashloops every pod on rollout.

**Files:**
- Create: `services/marketplace-api/migrations/000102_notifications_created_at_index.up.sql`
- Create: `services/marketplace-api/migrations/000102_notifications_created_at_index.down.sql`
- Modify: `services/marketplace-api/migrations.go:17`
- Test: `services/marketplace-api/migrations_test.go` (already exists — it is the failing test)

**Interfaces:**
- Consumes: nothing.
- Produces: `notif_created_at_idx` on `notifications (created_at DESC)`; `marketplaceapi.ExpectedSchemaVersion == 102`.

- [ ] **Step 1: Write the failing test**

No new test is written. `TestExpectedSchemaVersionMatchesHighestMigration` in `services/marketplace-api/migrations_test.go` already asserts this, and adding the migration files is what makes it fail. Create the two files:

`migrations/000102_notifications_created_at_index.up.sql`:
```sql
-- The platform console's cross-tenant notification log (#332) orders by
-- created_at DESC across every store. Both existing indexes are
-- store-scoped — notif_store_unread_idx (store_id, is_read, created_at DESC)
-- and notif_store_recent_idx (store_id, created_at DESC) — so neither can
-- serve a query with no store_id predicate.
--
-- Same reason migration 000101 added idx_audit_logs_created_at for #276.
CREATE INDEX notif_created_at_idx ON notifications (created_at DESC);
```

`migrations/000102_notifications_created_at_index.down.sql`:
```sql
DROP INDEX IF EXISTS notif_created_at_idx;
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/marketplace-api && go test -count=1 -run TestExpectedSchemaVersionMatchesHighestMigration ./ -v`

Expected: FAIL — `ExpectedSchemaVersion = 101, but highest migration on disk is 102 — bump the constant in migrations.go to match`.

Confirm from the verbose output that the line reads `--- FAIL`. A `--- SKIP` is not a failure and is one character away in a wall of output.

- [ ] **Step 3: Bump the constant**

In `services/marketplace-api/migrations.go`, change line 17:

```go
const ExpectedSchemaVersion uint = 102
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/marketplace-api && go test -count=1 -run TestExpectedSchemaVersionMatchesHighestMigration ./ -v`

Expected: PASS.

- [ ] **Step 5: Apply the migration locally and confirm the index exists**

`make dev` is broken in this repo (migrate containers fail with `exec: "up": executable file not found`). Apply directly:

```bash
cd services/marketplace-api
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' go run ./cmd/migrate up
# psql is NOT installed on this machine; docker is. The LAN IP is
# reachable from inside the container, `localhost` would not be.
docker run --rm postgres:15 psql \
  'postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  -c "select indexname from pg_indexes where tablename='notifications'"
```

Expected: the output lists `notif_created_at_idx` alongside the four pre-existing indexes (`notifications_pkey`, `notif_store_unread_idx`, `notif_store_recent_idx`, `notif_recipient_unread_idx`). If it is absent, the migration did not apply — do not proceed.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/migrations/000102_notifications_created_at_index.up.sql \
        services/marketplace-api/migrations/000102_notifications_created_at_index.down.sql \
        services/marketplace-api/migrations.go
git commit -m "feat(notifications): cross-store created_at index and schema version 102 (#332)"
```

---

### Task 2: `notification.ListPlatform` — the cross-store query

`ListByStore` hardcodes `store_id = ?` (`internal/notification/repository.go:72-74`) and matches **nothing** for a zero UUID. That is the safe failure for a merchant-facing query. **Do not loosen it.** Widening a zero `StoreID` to mean "all stores" would invert a fail-safe into fail-open on the merchant path — one forgotten field away from a merchant reading another store's notifications. This is #329's ruling for `ticket.ListFilter`, unchanged.

`notifications` has **no foreign keys** (only `notification_preferences.store_id` references `stores`), so seeding requires no parent `stores` row.

**Files:**
- Modify: `services/marketplace-api/internal/notification/repository.go`
- Create: `services/marketplace-api/internal/notification/platform_list_integration_test.go`

**Interfaces:**
- Consumes: `notification.Notification` (`internal/notification/models.go:34-50`), `notification.ListResult` (`repository.go:24-27`).
- Produces:
  ```go
  const MaxPlatformPageSize = 500
  const DefaultPlatformPageSize = 50
  const AudienceStore    = "store"
  const AudienceCustomer = "customer"

  type PlatformListFilter struct {
      TenantID        *uuid.UUID
      StoreID         *uuid.UUID
      Type            string
      Audience        string
      RecipientUserID string
      Read            *bool
      From            *time.Time
      To              *time.Time
      Page            int
      Limit           int
  }

  // On Repository:
  ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error)
  ```

- [ ] **Step 1: Write the failing tests**

Create `services/marketplace-api/internal/notification/platform_list_integration_test.go`:

```go
//go:build integration

package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedNotification inserts one notifications row via GORM. The table has NO
// foreign keys (only notification_preferences.store_id references stores),
// so no parent store row is needed. NOT NULL columns are tenant_id,
// store_id, type, title — see migrations/000016_notifications.up.sql.
func seedNotification(t *testing.T, db *gorm.DB, n notification.Notification) notification.Notification {
	t.Helper()
	if n.Title == "" {
		n.Title = "seeded title"
	}
	if n.Type == "" {
		n.Type = notification.TypeNewOrder
	}
	require.NoError(t, db.Create(&n).Error)
	return n
}

// The whole point of the method: two notifications under two different
// stores in two different tenants must both come back from one unfiltered
// call. A single-store fixture would pass against ListByStore too, and
// prove nothing about this method.
func TestListPlatform_SpansStoresAndTenants(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantA, StoreID: storeA, Title: "Alpha title"})
	seedNotification(t, db, notification.Notification{TenantID: tenantB, StoreID: storeB, Title: "Beta title"})

	got, err := repo.ListPlatform(context.Background(), db, notification.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total)

	titles := map[string]bool{}
	for _, n := range got.Notifications {
		titles[n.Title] = true
	}
	require.True(t, titles["Alpha title"], "notification from tenant A / store A must appear")
	require.True(t, titles["Beta title"], "notification from tenant B / store B must appear")
}

// store_id and tenant_id NARROW; neither is a required scope. Both
// directions are asserted, because a filter that always applies and a
// filter that never applies both pass a one-sided test.
func TestListPlatform_TenantAndStoreNarrowRatherThanScope(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantA, StoreID: storeA, Title: "Alpha title"})
	seedNotification(t, db, notification.Notification{TenantID: tenantB, StoreID: storeB, Title: "Beta title"})

	all, err := repo.ListPlatform(context.Background(), db, notification.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), all.Total, "unset filters must return every store")

	byStore, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{StoreID: &storeA, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byStore.Total)
	require.Equal(t, "Alpha title", byStore.Notifications[0].Title)

	byTenant, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{TenantID: &tenantB, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byTenant.Total)
	require.Equal(t, "Beta title", byTenant.Notifications[0].Title)
}

// Audience discriminates on recipient_user_id IS NULL. BOTH kinds of row
// are seeded: with only one kind present, a filter that always applies and
// one that never applies give the same answer.
func TestListPlatform_AudienceDiscriminatesOnRecipient(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	uid := "gip-uid-customer-1"
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Store row"})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Customer row", RecipientUserID: &uid})

	store, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Audience: notification.AudienceStore, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), store.Total)
	require.Equal(t, "Store row", store.Notifications[0].Title)

	customer, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Audience: notification.AudienceCustomer, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), customer.Total)
	require.Equal(t, "Customer row", customer.Notifications[0].Title)

	// An unrecognised audience is ignored, not an error, and must not
	// silently behave like one of the two real values.
	both, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Audience: "nonsense", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), both.Total)
}

// read filters on is_read. Both a read and an unread row are seeded, for
// the same reason as the audience test above.
func TestListPlatform_ReadFilter(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Unread row", IsRead: false})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Read row", IsRead: true})

	yes, no := true, false

	readOnly, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Read: &yes, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), readOnly.Total)
	require.Equal(t, "Read row", readOnly.Notifications[0].Title)

	unreadOnly, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Read: &no, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), unreadOnly.Total)
	require.Equal(t, "Unread row", unreadOnly.Notifications[0].Title)
}

// type filters on the type column, with a second type present so a
// no-op WHERE cannot pass.
func TestListPlatform_TypeFilter(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Order row", Type: notification.TypeNewOrder})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Stock row", Type: notification.TypeLowStock})

	got, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Type: string(notification.TypeLowStock), Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, "Stock row", got.Notifications[0].Title)
}

// recipient_user_id is an exact match, with a SECOND customer row under a
// different uid so a query that ignores the filter cannot pass.
func TestListPlatform_RecipientUserIDFilter(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	uidA, uidB := "gip-uid-aaa", "gip-uid-bbb"
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "For A", RecipientUserID: &uidA})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "For B", RecipientUserID: &uidB})

	got, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{RecipientUserID: uidB, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, "For B", got.Notifications[0].Title)
}

// The from/to window is inclusive on both ends, and the fixture sits on the
// exact boundary instant — the value where a `>` implementation and a `>=`
// implementation disagree. "Close to the edge" is not the edge.
func TestListPlatform_FromToBoundaryIsInclusive(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	boundary := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	seedNotification(t, db, notification.Notification{
		TenantID: tenantID, StoreID: storeID, Title: "On the boundary", CreatedAt: boundary,
	})
	seedNotification(t, db, notification.Notification{
		TenantID: tenantID, StoreID: storeID, Title: "Ten days earlier", CreatedAt: boundary.AddDate(0, 0, -10),
	})

	got, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{From: &boundary, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total, "a row created exactly at `from` must be included")
	require.Equal(t, "On the boundary", got.Notifications[0].Title)

	gotTo, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{To: &boundary, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), gotTo.Total, "a row created exactly at `to` must be included")
}

// Limit clamps at MaxPlatformPageSize rather than refusing, and an unset
// limit takes the default. Asserted on the returned row count with more
// rows present than the clamp would allow is impractical here; instead
// assert the SQL-visible effect: a limit of 1 returns 1 of 2 rows, and
// Total still reports 2 so the console can page.
func TestListPlatform_LimitAppliesAndTotalIgnoresIt(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "First"})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Second"})

	got, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, got.Notifications, 1, "limit must bound the page")
	require.Equal(t, int64(2), got.Total, "total must count every match, not the page")
}

// The existing store-scoped List is fail-safe and must stay that way. If a
// future change makes a zero StoreID mean "all stores", this test fails —
// which is its entire purpose.
func TestListByStore_ZeroStoreIDStillMatchesNothing(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	seedNotification(t, db, notification.Notification{
		TenantID: uuid.New(), StoreID: uuid.New(), Title: "Should not leak",
	})

	got, err := repo.ListByStore(context.Background(), db, notification.ListFilter{})
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Total, "a zero StoreID must match nothing, never everything")
	require.Empty(t, got.Notifications)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/notification/... -v
```

Expected: FAIL to **compile** — `undefined: notification.PlatformListFilter`.

If instead you see `--- SKIP`, `TEST_DATABASE_URL` did not reach the process. Fix that before continuing; a skipped test is not a passing test, and the two are one character apart.

- [ ] **Step 3: Write the implementation**

In `services/marketplace-api/internal/notification/repository.go`, add `"time"` to the imports, then add above the `Repository` interface:

```go
// MaxPlatformPageSize and DefaultPlatformPageSize mirror the ticket and
// audit packages' values so every cross-tenant read on the platform
// surface clamps alike.
const MaxPlatformPageSize = 500
const DefaultPlatformPageSize = 50

// Audience values for PlatformListFilter.Audience. They discriminate on
// recipient_user_id being NULL: a notification either targets the store
// (staff bell, no individual recipient) or one storefront customer.
const (
	AudienceStore    = "store"
	AudienceCustomer = "customer"
)

// PlatformListFilter is the CROSS-STORE filter for the platform console
// (#332). It is deliberately a separate type from ListFilter: that one
// requires a store and matches nothing without it, which is the safe
// failure for a merchant-facing query. Widening it to mean "all stores
// when unset" would make a forgotten field a cross-store leak.
//
// TenantID and StoreID NARROW; neither is a required scope.
type PlatformListFilter struct {
	TenantID        *uuid.UUID
	StoreID         *uuid.UUID
	Type            string
	Audience        string // AudienceStore | AudienceCustomer | "" (any)
	RecipientUserID string
	Read            *bool
	From            *time.Time
	To              *time.Time
	Page            int
	Limit           int
}
```

Add to the `Repository` interface, directly after the `ListByStore` entry:

```go
	// ListPlatform returns a filtered, paginated, CROSS-STORE list of
	// notifications for the platform console. TenantID and StoreID narrow
	// rather than scope — see PlatformListFilter.
	ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error)
```

Add the implementation after `ListByStore`:

```go
func (gormRepository) ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error) {
	var result ListResult
	q := db.WithContext(ctx).Model(&Notification{})

	// TenantID and StoreID NARROW. Unset means every tenant and every
	// store, which is the whole point of this method and exactly why it is
	// not ListFilter.
	if f.TenantID != nil {
		q = q.Where("tenant_id = ?", *f.TenantID)
	}
	if f.StoreID != nil {
		q = q.Where("store_id = ?", *f.StoreID)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	// An unrecognised audience narrows nothing rather than erroring,
	// matching how every other unknown parameter on this surface behaves.
	switch f.Audience {
	case AudienceStore:
		q = q.Where("recipient_user_id IS NULL")
	case AudienceCustomer:
		q = q.Where("recipient_user_id IS NOT NULL")
	}
	if f.RecipientUserID != "" {
		q = q.Where("recipient_user_id = ?", f.RecipientUserID)
	}
	if f.Read != nil {
		q = q.Where("is_read = ?", *f.Read)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("notification platform list count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultPlatformPageSize
	}
	if limit > MaxPlatformPageSize {
		limit = MaxPlatformPageSize
	}
	page := f.Page
	if page < 1 {
		page = 1
	}

	if err := q.Order("created_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Find(&result.Notifications).Error; err != nil {
		return result, fmt.Errorf("notification platform list: %w", err)
	}
	return result, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/notification/... -v
```

Expected: every test prints `--- PASS`. **Read the verbose output and confirm each named test above actually ran** — count them. A summary line of `ok` is compatible with everything having skipped.

- [ ] **Step 5: Prove the fail-safe test constrains the code**

A test must fail if the property it names is deleted. Temporarily change `ListByStore`'s predicate in `repository.go:74` from `Where("store_id = ?", f.StoreID)` to `Where("1 = 1")`, re-run:

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration -run TestListByStore_ZeroStoreIDStillMatchesNothing ./internal/notification/... -v
```

Expected: FAIL. **Revert the change** and re-run to confirm PASS. If the mutated version passed, the test proves nothing and must be fixed before continuing.

- [ ] **Step 6: Vet, including the build-tagged files**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: all three exit 0 with no output. (`go.work requires go >= 1.26.6 (running go 1.26.5)` is pre-existing local toolchain drift and is not a failure.)

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/notification/repository.go \
        services/marketplace-api/internal/notification/platform_list_integration_test.go
git commit -m "feat(notification): cross-store ListPlatform for the platform console (#332)"
```

---

### Task 3: The handler and its pinned contract

Copy `internal/handlers/platformadmin/tickets.go` rather than inventing a fourth shape. This is a read: no operator identity, no capability, no audit emitter.

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/notifications.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/notifications_test.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/testdata/notifications_response.json`

**Interfaces:**
- Consumes: `notification.PlatformListFilter`, `notification.ListResult`, `notification.MaxPlatformPageSize`, `notification.DefaultPlatformPageSize`, `notification.AudienceStore`, `notification.AudienceCustomer` (Task 2); `platformadmin.pagination` (`audit_logs.go:54-58`).
- Produces:
  ```go
  type NotificationLister interface {
      ListPlatform(ctx context.Context, db *gorm.DB, f notification.PlatformListFilter) (notification.ListResult, error)
  }
  func NewNotificationsHandler(db *gorm.DB, repo NotificationLister, logger *slog.Logger) *NotificationsHandler
  func (h *NotificationsHandler) Register(g *gin.RouterGroup)   // GET /admin/notifications
  ```

- [ ] **Step 1: Write the failing tests**

Create `services/marketplace-api/internal/handlers/platformadmin/notifications_test.go`:

```go
package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/notification"
)

// stubNotificationLister records the filter it was handed and returns a
// canned result, so the tests can assert on parsing without a database.
type stubNotificationLister struct {
	result    notification.ListResult
	err       error
	gotFilter notification.PlatformListFilter
}

func (s *stubNotificationLister) ListPlatform(_ context.Context, _ *gorm.DB, f notification.PlatformListFilter) (notification.ListResult, error) {
	s.gotFilter = f
	if s.err != nil {
		return notification.ListResult{}, s.err
	}
	if s.result.Notifications == nil {
		s.result.Notifications = []notification.Notification{}
	}
	return s.result, nil
}

func getNotifications(t *testing.T, repo platformadmin.NotificationLister) *httptest.ResponseRecorder {
	t.Helper()
	return getNotificationsWithQuery(t, repo, "")
}

func getNotificationsWithQuery(t *testing.T, repo platformadmin.NotificationLister, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewNotificationsHandler(nil, repo, nil).Register(r.Group(""))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/notifications"+query, nil))
	return rec
}

// Values are DISTINCT and NON-ZERO so an assertion cannot pass on a zero
// fabricated by a missing field. Two rows, two stores, two tenants, one of
// each audience — the shape this endpoint exists to return.
func notificationsFixture() []notification.Notification {
	body := "MUST NOT APPEAR IN THE RESPONSE"
	otherBody := "MUST NOT APPEAR IN THE RESPONSE EITHER"
	resourceType := "order"
	resourceID := uuid.MustParse("cccccccc-1111-1111-1111-111111111111")
	uid := "gip-uid-customer-7"
	return []notification.Notification{
		{
			ID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID:     uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111"),
			StoreID:      uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111"),
			Type:         notification.TypeNewOrder,
			Title:        "New order received",
			Message:      &body,
			ResourceType: &resourceType,
			ResourceID:   &resourceID,
			IsRead:       false,
			CreatedAt:    time.Date(2026, 8, 19, 8, 30, 0, 0, time.UTC),
		},
		{
			ID:              uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			TenantID:        uuid.MustParse("aaaaaaaa-2222-2222-2222-222222222222"),
			StoreID:         uuid.MustParse("bbbbbbbb-2222-2222-2222-222222222222"),
			RecipientUserID: &uid,
			Type:            notification.TypeOrderShipped,
			Title:           "Order confirmed",
			Message:         &otherBody,
			IsRead:          true,
			CreatedAt:       time.Date(2026, 8, 18, 7, 15, 0, 0, time.UTC),
		},
	}
}

// The golden fixture pins the exact contract shape as bytes, catching a
// rename or an unauthorized addition that a struct-shaped assertion would
// happily accept.
func TestNotificationsMatchesPinnedContract(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{result: notification.ListResult{
		Notifications: notificationsFixture(), Total: 2,
	}})
	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/notifications_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// Asserted on the RAW BYTES, not an unmarshalled struct: a struct cannot
// distinguish an absent key from an empty one. `message` is the
// interpolated body #332 exists to keep out; `status` does not exist in
// this estate at all (#348) and must not be faked from is_read.
func TestNotificationsOmitsBodyAndStatus(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{result: notification.ListResult{
		Notifications: notificationsFixture(), Total: 2,
	}})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	require.NotContains(t, body, `"message"`, "the notification body must never reach the console")
	require.NotContains(t, body, "MUST NOT APPEAR IN THE RESPONSE")
	require.NotContains(t, body, `"status"`, "there is no delivery status in this estate — see #348")
	require.NotContains(t, body, `"source"`, "the platform API stamps source and overwrites the body")
}

// audience is always present, so an absent recipient_user_id reads as
// "went to the store" rather than "the lookup failed". Both values are
// exercised because the fixture carries one row of each kind.
func TestNotificationsAudienceIsAlwaysPresent(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{result: notification.ListResult{
		Notifications: notificationsFixture(), Total: 2,
	}})

	var resp struct {
		Data []struct {
			Audience        string `json:"audience"`
			RecipientUserID string `json:"recipient_user_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)
	require.Equal(t, "store", resp.Data[0].Audience)
	require.Empty(t, resp.Data[0].RecipientUserID)
	require.Equal(t, "customer", resp.Data[1].Audience)
	require.Equal(t, "gip-uid-customer-7", resp.Data[1].RecipientUserID)
}

// Empty is 200 + [], never null. A nil slice marshals to null and defeats
// the console's `?? []` precisely when there is no data.
func TestNotificationsEmptyIsAnArray(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"data":[]`)
	require.NotContains(t, rec.Body.String(), `"data":null`)
}

// Every filter reaches the repository with the value the caller sent.
// Each value is DISTINCT so a handler that assigned the wrong field would
// be caught.
func TestNotificationsParsesEveryFilter(t *testing.T) {
	stub := &stubNotificationLister{}
	tenantID := uuid.MustParse("aaaaaaaa-3333-3333-3333-333333333333")
	storeID := uuid.MustParse("bbbbbbbb-3333-3333-3333-333333333333")

	rec := getNotificationsWithQuery(t, stub,
		"?type=low_stock&tenant_id="+tenantID.String()+
			"&store_id="+storeID.String()+
			"&audience=customer&recipient_user_id=gip-uid-zzz&read=true"+
			"&from=2026-08-01T00:00:00Z&to=2026-08-31T00:00:00Z&limit=7&page=3")
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "low_stock", stub.gotFilter.Type)
	require.NotNil(t, stub.gotFilter.TenantID)
	require.Equal(t, tenantID, *stub.gotFilter.TenantID)
	require.NotNil(t, stub.gotFilter.StoreID)
	require.Equal(t, storeID, *stub.gotFilter.StoreID)
	require.Equal(t, "customer", stub.gotFilter.Audience)
	require.Equal(t, "gip-uid-zzz", stub.gotFilter.RecipientUserID)
	require.NotNil(t, stub.gotFilter.Read)
	require.True(t, *stub.gotFilter.Read)
	require.NotNil(t, stub.gotFilter.From)
	require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), stub.gotFilter.From.UTC())
	require.NotNil(t, stub.gotFilter.To)
	require.Equal(t, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), stub.gotFilter.To.UTC())
	require.Equal(t, 7, stub.gotFilter.Limit)
	require.Equal(t, 3, stub.gotFilter.Page)
}

// read=false must reach the repository as a non-nil pointer to false, not
// as nil. A handler that only set the pointer for "true" would return read
// AND unread rows for read=false, and a presence-only assertion would miss
// it.
func TestNotificationsReadFalseIsNotTheSameAsAbsent(t *testing.T) {
	stub := &stubNotificationLister{}
	getNotificationsWithQuery(t, stub, "?read=false")
	require.NotNil(t, stub.gotFilter.Read, "read=false must narrow, not be dropped")
	require.False(t, *stub.gotFilter.Read)

	absent := &stubNotificationLister{}
	getNotifications(t, absent)
	require.Nil(t, absent.gotFilter.Read, "an absent read parameter must not narrow")
}

// An oversized limit clamps rather than refusing, a missing one takes the
// default, and pagination.limit reports the EFFECTIVE limit so the console
// can compute total/limit as a page count.
func TestNotificationsLimitClampsAndReportsEffective(t *testing.T) {
	stub := &stubNotificationLister{result: notification.ListResult{Total: 9000}}
	rec := getNotificationsWithQuery(t, stub, "?limit=100000")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, notification.MaxPlatformPageSize, stub.gotFilter.Limit)

	var resp struct {
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, notification.MaxPlatformPageSize, resp.Pagination.Limit,
		"pagination.limit must be the clamped limit, not the requested one")
	require.Equal(t, 1, resp.Pagination.Page)
	require.Equal(t, int64(9000), resp.Pagination.Total)

	def := &stubNotificationLister{}
	getNotifications(t, def)
	require.Equal(t, notification.DefaultPlatformPageSize, def.gotFilter.Limit)
}

// Garbage never errors — it takes the default, matching #276 and #329.
func TestNotificationsMalformedParametersTakeDefaults(t *testing.T) {
	stub := &stubNotificationLister{}
	rec := getNotificationsWithQuery(t, stub,
		"?limit=banana&page=-4&tenant_id=not-a-uuid&store_id=nope&from=yesterday&read=perhaps")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, notification.DefaultPlatformPageSize, stub.gotFilter.Limit)
	require.Equal(t, 1, stub.gotFilter.Page)
	require.Nil(t, stub.gotFilter.TenantID)
	require.Nil(t, stub.gotFilter.StoreID)
	require.Nil(t, stub.gotFilter.From)
	require.Nil(t, stub.gotFilter.Read)
}

// Explicit from/to wins over since_hours when both are supplied, matching
// #276 and #329. The explicit `from` is TEN DAYS back while since_hours
// asks for one hour, so the two cannot coincide.
func TestNotificationsExplicitFromBeatsSinceHours(t *testing.T) {
	stub := &stubNotificationLister{}
	getNotificationsWithQuery(t, stub, "?since_hours=1&from=2026-08-01T00:00:00Z")
	require.NotNil(t, stub.gotFilter.From)
	require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), stub.gotFilter.From.UTC())
}

// since_hours alone sets a window measured back from now.
func TestNotificationsSinceHoursSetsAWindow(t *testing.T) {
	stub := &stubNotificationLister{}
	before := time.Now()
	getNotificationsWithQuery(t, stub, "?since_hours=24")
	require.NotNil(t, stub.gotFilter.From)
	delta := before.Sub(*stub.gotFilter.From)
	require.InDelta(t, (24 * time.Hour).Seconds(), delta.Seconds(), 60,
		"since_hours=24 must look back roughly 24 hours")
}

// A repository failure is a 500 with a stable code, and the driver's error
// text is never echoed to the caller.
func TestNotificationsRepositoryErrorIs500AndDoesNotLeak(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{
		err: errors.New("dial tcp 10.0.0.1:5432: connection refused"),
	})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "internal_error")
	require.NotContains(t, rec.Body.String(), "connection refused",
		"driver error text must be logged server-side, never echoed")
}
```

Create `services/marketplace-api/internal/handlers/platformadmin/testdata/notifications_response.json` (single line, no trailing newline concerns — `require.JSONEq` parses both sides):

```json
{"data":[{"id":"11111111-1111-1111-1111-111111111111","tenant_id":"aaaaaaaa-1111-1111-1111-111111111111","store_id":"bbbbbbbb-1111-1111-1111-111111111111","type":"new_order","title":"New order received","audience":"store","resource_type":"order","resource_id":"cccccccc-1111-1111-1111-111111111111","is_read":false,"created_at":"2026-08-19T08:30:00Z"},{"id":"22222222-2222-2222-2222-222222222222","tenant_id":"aaaaaaaa-2222-2222-2222-222222222222","store_id":"bbbbbbbb-2222-2222-2222-222222222222","type":"order_shipped","title":"Order confirmed","audience":"customer","recipient_user_id":"gip-uid-customer-7","is_read":true,"created_at":"2026-08-18T07:15:00Z"}],"pagination":{"page":1,"limit":50,"total":2}}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api && go test -count=1 ./internal/handlers/platformadmin/... -run 'TestNotifications' -v
```

Expected: FAIL to compile — `undefined: platformadmin.NewNotificationsHandler`.

- [ ] **Step 3: Write the implementation**

Create `services/marketplace-api/internal/handlers/platformadmin/notifications.go`:

```go
package platformadmin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/notification"
)

// NotificationLister is the subset of notification.Repository this handler
// needs. Narrowed to one method for the same reason as TicketLister in
// tickets.go and EstateCounts in kpis.go.
type NotificationLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f notification.PlatformListFilter) (notification.ListResult, error)
}

// NotificationsHandler serves GET /admin/notifications to the platform
// console — a cross-store, cross-tenant read of the in-app notification
// log.
//
// This is the IN-APP notification bell, not a sent-mail log. #332 asked for
// one; no record of outbound mail exists anywhere in this estate —
// transactional mail is fire-and-forget through internal/email, no provider
// event webhook was ever rebuilt, and campaign_recipients only ever writes
// `sent`. That work is #348. Nothing here reports a delivery outcome, and
// nothing here should be made to look as though it does.
type NotificationsHandler struct {
	db     *gorm.DB
	repo   NotificationLister
	logger *slog.Logger
}

// NewNotificationsHandler constructs the handler. logger may be nil.
func NewNotificationsHandler(db *gorm.DB, repo NotificationLister, logger *slog.Logger) *NotificationsHandler {
	return &NotificationsHandler{db: db, repo: repo, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *NotificationsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/notifications", h.List)
}

// notificationRow is the pinned contract shape.
//
// `message` is DELIBERATELY absent: it is the interpolated body, the only
// field in the row carrying customer detail, and a cross-tenant governance
// surface must not become a way to read every merchant's correspondence.
// Same reasoning that keeps `description` out of #329 and `payload` out of
// #331.
//
// There is no `status` field. No delivery status exists in this estate
// (#348); emitting is_read under that name would put a governance label on
// a metric answering a different question, and an operator would act on it.
//
// `audience` is always present so an absent recipient_user_id reads as
// "this went to the store" rather than "the recipient lookup failed".
type notificationRow struct {
	ID              string  `json:"id"`
	TenantID        string  `json:"tenant_id"`
	StoreID         string  `json:"store_id"`
	Type            string  `json:"type"`
	Title           string  `json:"title"`
	Audience        string  `json:"audience"`
	RecipientUserID *string `json:"recipient_user_id,omitempty"`
	ResourceType    *string `json:"resource_type,omitempty"`
	ResourceID      *string `json:"resource_id,omitempty"`
	IsRead          bool    `json:"is_read"`
	CreatedAt       string  `json:"created_at"`
}

type notificationListResponse struct {
	Data       []notificationRow `json:"data"`
	Pagination pagination        `json:"pagination"`
}

// List handles GET /admin/notifications.
func (h *NotificationsHandler) List(c *gin.Context) {
	filter := h.parseFilter(c)

	result, err := h.repo.ListPlatform(c.Request.Context(), h.db, filter)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform notifications list", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "could not read notifications",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to null, which
	// defeats a caller's `?? []` and crashes their page precisely when
	// there is no data.
	rows := make([]notificationRow, 0, len(result.Notifications))
	for _, n := range result.Notifications {
		rows = append(rows, toNotificationRow(n))
	}

	c.JSON(http.StatusOK, notificationListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(filter.Page, 1),
			Limit: filter.Limit,
			Total: result.Total,
		},
	})
}

// toNotificationRow maps a stored notification to the pinned contract
// shape, FIELD BY FIELD. n.Message is never read — the body's absence is a
// property of this projection, not of what the query happened to select, so
// a column added to notification.Notification tomorrow cannot leak.
func toNotificationRow(n notification.Notification) notificationRow {
	row := notificationRow{
		ID:        n.ID.String(),
		TenantID:  n.TenantID.String(),
		StoreID:   n.StoreID.String(),
		Type:      string(n.Type),
		Title:     n.Title,
		Audience:  notification.AudienceStore,
		IsRead:    n.IsRead,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
	}
	if n.RecipientUserID != nil && *n.RecipientUserID != "" {
		row.Audience = notification.AudienceCustomer
		row.RecipientUserID = n.RecipientUserID
	}
	if n.ResourceType != nil {
		row.ResourceType = n.ResourceType
	}
	if n.ResourceID != nil {
		id := n.ResourceID.String()
		row.ResourceID = &id
	}
	return row
}

// parseFilter never returns an error. A missing parameter takes our
// default, and an oversized limit clamps rather than refusing — matching
// the audit logs contract (#276) and tickets (#329).
func (h *NotificationsHandler) parseFilter(c *gin.Context) notification.PlatformListFilter {
	f := notification.PlatformListFilter{
		Type:            strings.TrimSpace(c.Query("type")),
		RecipientUserID: strings.TrimSpace(c.Query("recipient_user_id")),
		Page:            1,
		Limit:           notification.DefaultPlatformPageSize,
	}

	// An unrecognised audience narrows nothing rather than erroring,
	// matching how every other unknown parameter here behaves.
	switch strings.TrimSpace(c.Query("audience")) {
	case notification.AudienceStore:
		f.Audience = notification.AudienceStore
	case notification.AudienceCustomer:
		f.Audience = notification.AudienceCustomer
	}

	// read=false must narrow to unread rows, so the pointer is set for
	// BOTH values — not only for "true".
	if v := strings.TrimSpace(c.Query("read")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			f.Read = &b
		}
	}

	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = min(n, notification.MaxPlatformPageSize)
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("since_hours")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			from := time.Now().Add(-time.Duration(n) * time.Hour)
			f.From = &from
		}
	}
	// Explicit from/to win over since_hours when both are supplied.
	if t, ok := parseNotificationTime(c.Query("from")); ok {
		f.From = &t
	}
	if t, ok := parseNotificationTime(c.Query("to")); ok {
		f.To = &t
	}
	// tenant_id and store_id NARROW rather than scope — see
	// notification.PlatformListFilter.
	if v := strings.TrimSpace(c.Query("tenant_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.TenantID = &id
		}
	}
	if v := strings.TrimSpace(c.Query("store_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.StoreID = &id
		}
	}
	return f
}

func parseNotificationTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api && go test -count=1 ./internal/handlers/platformadmin/... -run 'TestNotifications' -v
```

Expected: every `TestNotifications*` prints `--- PASS`. Count them against the test file.

- [ ] **Step 5: Prove the golden fixture catches a rename AND an addition**

A fixture that only catches omissions is theatre. Two mutations, reverting after each:

1. **Rename.** In `notifications.go`, change `Title string \`json:"title"\`` to `json:"subject"`. Run `go test -count=1 ./internal/handlers/platformadmin/... -run TestNotificationsMatchesPinnedContract -v`. Expected: FAIL. Revert.
2. **Addition.** Add `Extra string \`json:"extra"\`` to `notificationRow` and set `Extra: "leaked"` in `toNotificationRow`. Run the same test. Expected: FAIL. Revert.

Re-run after reverting both and confirm PASS. If either mutation passed, the fixture proves nothing and must be fixed before continuing.

3. **Body exclusion.** Add `Message *string \`json:"message,omitempty"\`` to `notificationRow` and set `Message: n.Message` in `toNotificationRow`. Run `go test -count=1 ./internal/handlers/platformadmin/... -run TestNotificationsOmitsBodyAndStatus -v`. Expected: FAIL. **Revert.**

- [ ] **Step 6: Vet, including the build-tagged files**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
```

Expected: all exit 0.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/notifications.go \
        services/marketplace-api/internal/handlers/platformadmin/notifications_test.go \
        services/marketplace-api/internal/handlers/platformadmin/testdata/notifications_response.json
git commit -m "feat(platformadmin): GET /admin/notifications — the cross-tenant notification log (#332)"
```

---

### Task 4: Mount the route and wire it in `main.go`

`main.go` constructs `platformadmin.Deps` at **two** sites — `cmd/marketplace-api/main.go:1980` and `:2092`. Both must be updated. #323 records five instances in this estate of wiring that silently unmounted a route because only one site was touched, and nothing tests either site.

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:521-526`, `:1980-1995`, `:2092-2107`
- Test: `services/marketplace-api/internal/handlers/platformadmin/routes_notifications_test.go` (create)

**Interfaces:**
- Consumes: `platformadmin.NotificationLister`, `platformadmin.NewNotificationsHandler` (Task 3); `notification.NewRepository()`.
- Produces: `Deps.Notifications NotificationLister`; the route `GET /api/v1/platform/admin/notifications`.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/handlers/platformadmin/routes_notifications_test.go`:

```go
package platformadmin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/notification"
)

type routeNotificationLister struct{ called bool }

func (s *routeNotificationLister) ListPlatform(_ context.Context, _ *gorm.DB, _ notification.PlatformListFilter) (notification.ListResult, error) {
	s.called = true
	return notification.ListResult{Notifications: []notification.Notification{}}, nil
}

// Register must mount the route when Notifications is supplied. Asserted
// as "not 404" with the secret set, matching TestRegisterTicketsMountsWhenDepPresent
// — this catches a guard that always refuses just as readily as a missing one.
func TestRegisterMountsNotificationsWhenSupplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:          &stubRepo{},
		Notifications: &routeNotificationLister{},
		Secret:        "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/notifications", nil))
	require.NotEqual(t, http.StatusNotFound, rec.Code,
		"the route must be mounted when Notifications is set")

	// A bogus path under the SAME prefix must 404. Without this, the
	// assertion above is also satisfied by a router that answers
	// everything under /api/v1/platform — it is what makes the first
	// check mean "this route exists".
	bogus := httptest.NewRecorder()
	r.ServeHTTP(bogus, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/notifications-nope", nil))
	require.Equal(t, http.StatusNotFound, bogus.Code)
}

// A nil Notifications leaves the route unmounted, matching the nil-safe
// pattern every other optional client-backed route uses.
func TestRegisterLeavesNotificationsUnmountedWhenNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo: &stubRepo{},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/notifications", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

`stubRepo` is the existing minimal `audit.Repository` stub in
`internal/handlers/platformadmin/audit_logs_test.go:23`, already used by
`routes_tickets_test.go`. Reuse it — `Register` returns immediately when
`Repo` is nil, so a non-nil one is required for any route to mount. Do not
add a second stub.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api && go test -count=1 ./internal/handlers/platformadmin/... -run 'TestRegister.*Notifications' -v
```

Expected: FAIL to compile — `unknown field Notifications in struct literal`.

- [ ] **Step 3: Add the dependency and the mount**

In `services/marketplace-api/internal/handlers/platformadmin/routes.go`, add to `Deps` after the `Tickets` field:

```go
	// Notifications serves /admin/notifications (#332), the cross-store
	// in-app notification log. Nil leaves that route unmounted, matching
	// the nil-safe pattern used for the other optional client-backed
	// routes above.
	//
	// This is the notification BELL, not a sent-mail log — no record of
	// outbound email exists anywhere in this estate. See #348.
	Notifications NotificationLister
```

And in `Register`, after the `Tickets` block:

```go
	if deps.Notifications != nil {
		NewNotificationsHandler(deps.DB, deps.Notifications, deps.Logger).Register(group)
	}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd services/marketplace-api && go test -count=1 ./internal/handlers/platformadmin/... -run 'TestRegister.*Notifications' -v
```

Expected: both tests `--- PASS`.

- [ ] **Step 5: Wire both `main.go` sites**

In `services/marketplace-api/cmd/marketplace-api/main.go`, hoist the repository so both the service and the platform surface share one construction. Replace lines 521-526:

```go
	notificationRepo := notification.NewRepository()
	notificationSvc := notification.NewService(notification.ServiceConfig{
		DB:     conn,
		Repo:   notificationRepo,
		Logger: log,
		Pusher: pushPublisher,
	})
```

Then add `Notifications: notificationRepo,` to **both** `platformadmin.Deps` literals — the one at line ~1980 and the one at line ~2092. Add it directly after the `Tickets:` line in each, keeping gofmt's alignment:

```go
			Tickets:               ticketRepo,
			Notifications:         notificationRepo,
```

**Verify both sites were edited** — a single-site edit is the exact defect #323 records five times:

```bash
cd services/marketplace-api && grep -c 'Notifications:         notificationRepo,' cmd/marketplace-api/main.go
```

Expected output: `2`. If it prints `1`, the second site was missed.

- [ ] **Step 6: Build, vet, and run the full suite from the service root**

```bash
cd services/marketplace-api
gofmt -l ./cmd ./internal
go build ./... && go vet ./... && go vet -tags=integration ./...
go test -count=1 ./...
```

Expected: `gofmt -l` prints nothing; the three build/vet commands exit 0; `go test ./...` reports `ok` for every package.

**Run `go test ./...` from the service root, not `./internal/...`.** The root package holds `TestExpectedSchemaVersionMatchesHighestMigration`, and a path-scoped run silently excludes it — which is how a schema-version mismatch reaches a rollout and crashloops every pod.

Check the exit code rather than trusting a summary line:

```bash
cd services/marketplace-api && go test -count=1 ./... > /tmp/mk8-test.log 2>&1; echo "exit=$?"; grep -E '^(FAIL|---)' /tmp/mk8-test.log | head -40
```

Expected: `exit=0`.

- [ ] **Step 7: Run the integration suite**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/notification/... ./internal/handlers/platformadmin/... -v 2>&1 | tee /tmp/mk8-int.log
grep -c -- '--- SKIP' /tmp/mk8-int.log
grep -c -- '--- PASS' /tmp/mk8-int.log
```

`-p 1` is required: these packages share one local Postgres, and parallel execution exhausts its connection limit (`FATAL: sorry, too many clients already`), which presents as data pollution and is not.

Expected: the `--- SKIP` count is `0` for the notification tests, and the `--- PASS` count accounts for every test in Task 2. A skipped test is not a passing test.

Pre-existing failures in `internal/subscription` (`store_subscriptions_store_id_fkey`) are #317 and unrelated — do not attempt to fix them, and do not include that package in the run above.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/routes.go \
        services/marketplace-api/internal/handlers/platformadmin/routes_notifications_test.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(platformadmin): mount /admin/notifications on both wiring sites (#332)"
```

---

## Final whole-branch review

After Task 4, before opening a PR, verify as a whole rather than per-task. Mutation over reading: a review that only reads the diff has repeatedly missed things a two-minute mutation caught.

- [ ] Both `platformadmin.Deps` literals in `main.go` carry `Notifications` (`grep -c` returns `2`).
- [ ] `ExpectedSchemaVersion` is `102` and `go test -count=1 ./...` from the service root passes — confirmed by exit code, not a summary line.
- [ ] `go vet -tags=integration ./...` exits 0.
- [ ] `git diff main --stat` shows no change to `ListByStore`, `ListByCustomer`, or `ListFilter`.
- [ ] The response for the golden fixture contains no `message`, `status`, or `source` key — checked against the raw JSON, not a struct.
- [ ] Every integration test named in Task 2 appears as `--- PASS` in verbose output, not `--- SKIP`.
- [ ] No claim in a code comment asserts a property that was not checked. In particular, the doc comment on `NotificationsHandler` asserts that no sent-mail record exists in the estate — that was established by searching all four services, and it is #348's subject. Do not extend it to claims that were not searched.

## What production can and cannot prove

When reporting on #332, separate the two. **Data-independent:** the route is mounted, an unsigned request returns `401`, a bogus path under the same prefix returns `404` (the second is what makes the first mean anything), and the clamp/default behaviours. **Data-dependent and not provable against the four live demo tenants:** that the cross-tenant fan-out actually spans tenants. An empty `200` is not a passing integration check.

Deploys arrive as image tags (`main-<sha7>`), not git commits — a rollout check matching a mark8ly commit SHA against `tesserix-k8s` can never match. Check `kubectl get stages,promotions -n kargo-mark8ly`.
