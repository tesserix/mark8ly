# `GET /admin/inbox` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Aggregate mark8ly's five "waiting on a human" queues into one console-renderable endpoint, so that work which currently has no interface anywhere becomes visible.

**Architecture:** A new `internal/inbox` package with one `Provider` per kind behind a narrow interface, an aggregator that fans out and merges in memory, and a thin handler in `internal/handlers/platformadmin`. Adding a sixth kind is one file plus one registration, with no aggregator change.

**Tech Stack:** Go 1.26, Gin, GORM v1.25, PostgreSQL 15, `stretchr/testify`, `google/uuid`, build tag `integration`.

**Spec:** `docs/superpowers/specs/2026-08-27-admin-inbox-design.md` — read it first; it records four decisions and the reasoning behind each.

## Global Constraints

- Module `github.com/mark8ly/marketplace-api`; all work under `services/marketplace-api/`.
- Single-line conventional commits. No signature, no `Co-Authored-By`.
- **Never push, open a PR, merge, deploy, or switch branches.**
- `gofmt -l .` empty; `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` clean.
- Integration tests need a database. **Use an isolated container, never the shared dev DB** — another session uses `localhost:5432` and two integration suites against one Postgres corrupt each other:
  ```bash
  docker run -d --name m8-inbox-db -e POSTGRES_USER=dev -e POSTGRES_PASSWORD=dev \
    -e POSTGRES_DB=marketplace_db -p 55434:5432 postgres:15
  docker exec m8-inbox-db psql -U dev -d marketplace_db -c 'CREATE EXTENSION IF NOT EXISTS pgcrypto;'
  cd services/marketplace-api && \
    DATABASE_URL='postgres://dev:dev@192.168.1.110:55434/marketplace_db?sslmode=disable' go run ./cmd/migrate up
  ```
  `pgcrypto` must be enabled manually — migration 058 uses `gen_random_bytes`. Then:
  `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55434/marketplace_db?sslmode=disable'`
- LAN IP `192.168.1.110`, never `localhost`. Always `-p 1` on integration runs.
- **A skipped test still exits 0.** `testdb.NewDB`/`NewTx` SKIP when `TEST_DATABASE_URL` is unset. Confirm `--- PASS` by name; a `--- SKIP` proves nothing.
- **Never run `./internal/billing/tax/revalidation/...`** — it deadlocks (#396) and hangs for the full test timeout.
- Seed parents with `testdb.SeedStore(t, db, tenantID, storeID)` — caller-supplied tenant, always the same tenant as the child row.
- Do NOT invent capability names. See the spec's `actions` section and `platformadmin/middleware.go:36-51`.

## Verified source facts — do not re-derive

| kind | table / type | filter | timestamp |
|---|---|---|---|
| `sea_manual_review` | `sea_manual_review_queue` | `status IN ('pending','in_review')` | `queued_at`, `sla_due_at` |
| `migration_fast_path` | `migration.Review` (`repository.go:27`) | `status = 'pending'` | `created_at` |
| `erasure_request` | `customer_erasure_requests` | `status = 'pending'` | `requested_at` |
| `arbitrage_appeal` | `arbitrage.SubscriptionArbitrageAudit`, table `subscription_arbitrage_audit` | `resolution = 'ongoing'` | `flagged_at` |
| `onboarding_stalled` | `onboardingfunnel.Client.ListSessions` | `Abandoned` / `IdleHours` | `LastActivityAt` |

`sea_manual_review_queue` status CHECK: `('pending','in_review','approved','rejected')`.
`customer_erasure_requests` status CHECK: `('pending','processed','rejected')`.
`arbitrage.ResolutionOngoing = "ongoing"` (`models.go:13`).
`onboardingfunnel.Session` carries `ID, Email, Status, LastActivityAt, Abandoned, IdleHours, TenantID`.

## File Structure

| File | Responsibility |
|---|---|
| `internal/inbox/models.go` *(create)* | `Item`, `Action`, `Filter`, `Severity` and its derivation. No I/O. |
| `internal/inbox/provider.go` *(create)* | The `Provider` interface and the kind constants. |
| `internal/inbox/sea_review.go` *(create)* | `sea_manual_review` provider. |
| `internal/inbox/erasure.go` *(create)* | `erasure_request` provider. |
| `internal/inbox/arbitrage.go` *(create)* | `arbitrage_appeal` provider. |
| `internal/inbox/migration_fastpath.go` *(create)* | `migration_fast_path` provider. |
| `internal/inbox/onboarding.go` *(create)* | `onboarding_stalled` provider, wrapping the existing HTTP client. |
| `internal/inbox/aggregator.go` *(create)* | Fan-out, merge, sort, paginate, degrade. |
| `internal/handlers/platformadmin/inbox.go` *(create)* | Handler: parse, call, render the house envelope. |
| `internal/handlers/platformadmin/routes.go` *(modify)* | One nil-safe `Deps` field, one conditional mount. |

---

### Task 1: `internal/inbox` models, severity, and the Provider interface

**Files:**
- Create: `services/marketplace-api/internal/inbox/models.go`
- Create: `services/marketplace-api/internal/inbox/provider.go`
- Test: `services/marketplace-api/internal/inbox/models_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces, relied on by every later task:
  - `type Item struct{ ID, Kind, Title, Subtitle string; WaitingSince time.Time; DueAt *time.Time; Severity string; Href string; Actions []Action }`
  - `type Action struct{ ID, Label string; Destructive bool }`
  - `type Filter struct{ Kind, TenantID, Status string; Page, Limit int }`
  - `func DeriveSeverity(dueAt *time.Time, now time.Time) string`
  - `type Provider interface{ Kind() string; List(context.Context, Filter) ([]Item, error); Count(context.Context, Filter) (int64, error) }`
  - Kind constants `KindSEAManualReview`, `KindMigrationFastPath`, `KindErasureRequest`, `KindArbitrageAppeal`, `KindOnboardingStalled`

- [ ] **Step 1: Write the failing test**

Create `internal/inbox/models_test.go`:

```go
package inbox_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/inbox"
)

func TestDeriveSeverity(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	ptr := func(d time.Duration) *time.Time { t := now.Add(d); return &t }

	cases := []struct {
		name  string
		dueAt *time.Time
		want  string
	}{
		{"no due date is normal", nil, inbox.SeverityNormal},
		{"far future is normal", ptr(72 * time.Hour), inbox.SeverityNormal},
		{"exactly 24h out is warning", ptr(24 * time.Hour), inbox.SeverityWarning},
		{"inside 24h is warning", ptr(time.Hour), inbox.SeverityWarning},
		{"exactly now is critical", ptr(0), inbox.SeverityCritical},
		{"past due is critical", ptr(-time.Minute), inbox.SeverityCritical},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, inbox.DeriveSeverity(tc.dueAt, now))
		})
	}
}
```

The boundary cases are the point. "Exactly at the boundary" is where an off-by-one in a comparison operator hides, and severity drives what an operator triages first.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
go test ./internal/inbox/... -run TestDeriveSeverity -v
```

Expected: build failure — `no required module provides package .../internal/inbox`.

- [ ] **Step 3: Write `models.go`**

```go
// Package inbox aggregates mark8ly's queues that are waiting on a human into
// one shape the platform console can render without per-product knowledge.
//
// Each queue is a Provider. The aggregator fans out across them, merges, and
// paginates; it holds no per-kind knowledge, so adding a queue is one file
// plus one registration.
package inbox

import (
	"time"
)

// Severity is derived from an item's due date at read time, never stored.
const (
	SeverityNormal   = "normal"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// warningWindow is how far ahead of DueAt an item starts reading as warning.
const warningWindow = 24 * time.Hour

// Action is something an operator may invoke on an item.
//
// Actions are derived from the item's own STATE, not from the caller's
// capability: the console's capability vocabulary is not settled, and
// platformadmin/middleware.go's CapabilityValueChecked is false for that
// reason. #287 declined to invent capability names; so does this. When the
// vocabulary lands, filter here and flip that switch.
type Action struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Destructive bool   `json:"destructive"`
}

// Item is one unit of work waiting on a human, in the shape every kind shares.
type Item struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Title        string     `json:"title"`
	Subtitle     string     `json:"subtitle"`
	WaitingSince time.Time  `json:"waiting_since"`
	DueAt        *time.Time `json:"due_at,omitempty"`
	Severity     string     `json:"severity"`
	Href         string     `json:"href"`
	Actions      []Action   `json:"actions"`
}

// Filter narrows a listing. An empty Kind means every kind.
type Filter struct {
	Kind     string
	TenantID string
	Status   string
	Page     int
	Limit    int
}

// DeriveSeverity maps a due date to a severity at read time.
//
// An item at or past its due date is critical; one inside warningWindow is
// warning; everything else, including an item with no due date at all, is
// normal. Only sea_manual_review carries a due date today.
func DeriveSeverity(dueAt *time.Time, now time.Time) string {
	if dueAt == nil {
		return SeverityNormal
	}
	if !now.Before(*dueAt) {
		return SeverityCritical
	}
	if dueAt.Sub(now) <= warningWindow {
		return SeverityWarning
	}
	return SeverityNormal
}
```

- [ ] **Step 4: Write `provider.go`**

```go
package inbox

import "context"

// Kinds mark8ly emits. Each maps to exactly one Provider.
const (
	KindSEAManualReview   = "sea_manual_review"
	KindMigrationFastPath = "migration_fast_path"
	KindErasureRequest    = "erasure_request"
	KindArbitrageAppeal   = "arbitrage_appeal"
	KindOnboardingStalled = "onboarding_stalled"
)

// Provider is one queue's view of the work waiting in it.
//
// List returns items already ordered by the provider's own natural order;
// the aggregator re-sorts across providers, so a provider need only be
// internally consistent. Count answers the same Filter List would.
type Provider interface {
	Kind() string
	List(ctx context.Context, f Filter) ([]Item, error)
	Count(ctx context.Context, f Filter) (int64, error)
}
```

- [ ] **Step 5: Run the test to verify it passes**

```bash
cd services/marketplace-api
go test ./internal/inbox/... -run TestDeriveSeverity -v
```

Expected: `--- PASS` for all six subtests, by name.

- [ ] **Step 6: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./...
```

Expected: `gofmt -l .` prints nothing; the rest exit 0.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/inbox/
git commit -m "feat(inbox): add item model, severity derivation and Provider interface"
```

---

### Task 2: The `sea_manual_review` provider

The flagship. Its migration states that entering this queue **immediately pauses the 14-day validation clock on the associated subscription**, under a 5-business-day SLA, and nothing reads the table today. Its test must prove the real behaviour, not that a row round-trips.

**Files:**
- Create: `services/marketplace-api/internal/inbox/sea_review.go`
- Test: `services/marketplace-api/internal/inbox/sea_review_integration_test.go`

**Interfaces:**
- Consumes: `Item`, `Action`, `Filter`, `DeriveSeverity`, `Provider`, `KindSEAManualReview` from Task 1.
- Produces: `func NewSEAReviewProvider(db *gorm.DB, now func() time.Time) *SEAReviewProvider`

- [ ] **Step 1: Write the failing test**

Create `internal/inbox/sea_review_integration_test.go`:

```go
//go:build integration

package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedSEARow(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, status string, queuedAt, slaDueAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := db.Exec(`
		INSERT INTO sea_manual_review_queue
			(id, tenant_id, store_id, country, tax_id, business_name, queue_reason, status, sla_due_at, queued_at)
		VALUES (?, ?, ?, 'MY', 'MY123456789', 'Acme Pte Ltd', 'format_unrecognised', ?, ?, ?)`,
		id, tenantID, storeID, status, slaDueAt, queuedAt,
	).Error
	require.NoError(t, err)
	return id
}

func TestSEAProvider_BreachedSLAIsCriticalAndCarriesDueAt(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	// Queued 8 days ago against a 5-business-day SLA that expired yesterday.
	id := seedSEARow(t, db, tenantID, storeID, "pending", now.Add(-8*24*time.Hour), now.Add(-24*time.Hour))

	p := inbox.NewSEAReviewProvider(db, func() time.Time { return now })
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)

	got := items[0]
	require.Equal(t, id.String(), got.ID)
	require.Equal(t, inbox.KindSEAManualReview, got.Kind)
	require.Equal(t, "Acme Pte Ltd", got.Title)
	require.Equal(t, inbox.SeverityCritical, got.Severity, "a breached SLA must read critical")
	require.NotNil(t, got.DueAt, "due_at is required on every sea_manual_review item")
	require.WithinDuration(t, now.Add(-24*time.Hour), *got.DueAt, time.Second)
	require.Equal(t, []inbox.Action{
		{ID: "approve", Label: "Approve", Destructive: false},
		{ID: "reject", Label: "Reject", Destructive: true},
	}, got.Actions)
}

func TestSEAProvider_ResolvedRowsAreAbsentNotReturnedAsResolved(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	seedSEARow(t, db, tenantID, storeID, "approved", now.Add(-time.Hour), now.Add(time.Hour))
	seedSEARow(t, db, tenantID, storeID, "rejected", now.Add(-time.Hour), now.Add(time.Hour))
	wanted := seedSEARow(t, db, tenantID, storeID, "in_review", now.Add(-time.Hour), now.Add(time.Hour))

	p := inbox.NewSEAReviewProvider(db, func() time.Time { return now })
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1, "only pending and in_review are waiting on a human")
	require.Equal(t, wanted.String(), items[0].ID)

	n, err := p.Count(context.Background(), inbox.Filter{})
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "Count must answer the same filter as List")
}
```

Add `"gorm.io/gorm"` to the import block — `seedSEARow` takes a `*gorm.DB`.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55434/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/inbox/... -run TestSEAProvider -v
```

Expected: build failure — `undefined: inbox.NewSEAReviewProvider`. A `--- SKIP` means your database URL is wrong; fix it before continuing.

- [ ] **Step 3: Write `sea_review.go`**

```go
package inbox

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// SEAReviewProvider surfaces sea_manual_review_queue.
//
// This queue matters more than its size suggests: migration
// 000065_sea_manual_review_queue states that any ID entering it immediately
// pauses the 14-day validation clock on the associated subscription, under a
// 5-business-day SLA. Until this endpoint, nothing read the table.
type SEAReviewProvider struct {
	db  *gorm.DB
	now func() time.Time
}

func NewSEAReviewProvider(db *gorm.DB, now func() time.Time) *SEAReviewProvider {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &SEAReviewProvider{db: db, now: now}
}

func (p *SEAReviewProvider) Kind() string { return KindSEAManualReview }

type seaRow struct {
	ID           string
	BusinessName string
	Country      string
	QueueReason  string
	Status       string
	SLADueAt     time.Time
	QueuedAt     time.Time
}

// waiting is the status set that means a human still owes this row a decision.
// approved and rejected are resolved and must be absent entirely, not returned
// carrying a resolved status.
const seaWaitingStatuses = "('pending','in_review')"

func (p *SEAReviewProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	q := p.db.WithContext(ctx).
		Table("sea_manual_review_queue").
		Select("id::text AS id, business_name, country, queue_reason, status, sla_due_at, queued_at").
		Where("status IN " + seaWaitingStatuses)
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	q = q.Order("sla_due_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Page > 1 && f.Limit > 0 {
		q = q.Offset((f.Page - 1) * f.Limit)
	}

	var rows []seaRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}

	now := p.now()
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		due := r.SLADueAt
		items = append(items, Item{
			ID:           r.ID,
			Kind:         KindSEAManualReview,
			Title:        r.BusinessName,
			Subtitle:     r.Country + " tax ID " + r.Status,
			WaitingSince: r.QueuedAt,
			DueAt:        &due,
			Severity:     DeriveSeverity(&due, now),
			Href:         "/admin/tax/manual-review/" + r.ID,
			Actions: []Action{
				{ID: "approve", Label: "Approve", Destructive: false},
				{ID: "reject", Label: "Reject", Destructive: true},
			},
		})
	}
	return items, nil
}

func (p *SEAReviewProvider) Count(ctx context.Context, f Filter) (int64, error) {
	q := p.db.WithContext(ctx).
		Table("sea_manual_review_queue").
		Where("status IN " + seaWaitingStatuses)
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var n int64
	return n, q.Count(&n).Error
}
```

Note `due := r.SLADueAt` before taking its address — taking `&r.SLADueAt` inside a range loop aliases the loop variable, and every item would end up sharing the last row's due date.

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55434/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/inbox/... -run TestSEAProvider -v
```

Expected: `--- PASS: TestSEAProvider_BreachedSLAIsCriticalAndCarriesDueAt` and
`--- PASS: TestSEAProvider_ResolvedRowsAreAbsentNotReturnedAsResolved`, by name.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/inbox/
git commit -m "feat(inbox): add the sea_manual_review provider"
```

---

### Task 3: The `erasure_request` and `arbitrage_appeal` providers

Two providers together because they are the same shape: a local table read filtered to one unresolved status, with no due date.

**Files:**
- Create: `services/marketplace-api/internal/inbox/erasure.go`
- Create: `services/marketplace-api/internal/inbox/arbitrage.go`
- Test: `services/marketplace-api/internal/inbox/erasure_integration_test.go`
- Test: `services/marketplace-api/internal/inbox/arbitrage_integration_test.go`

**Interfaces:**
- Consumes: everything from Task 1.
- Produces:
  - `func NewErasureProvider(db *gorm.DB) *ErasureProvider`
  - `func NewArbitrageProvider(db *gorm.DB) *ArbitrageProvider`

**Schema constraints these tests must respect** — verified, do not re-derive:
- `customer_erasure_requests` has **`UNIQUE (store_id, customer_email)`**. Seeding several rows into
  one store therefore needs a distinct email per row; the helper below derives one from the row id.
  An earlier draft of this plan reused one address and would have failed on insert.
- `subscription_arbitrage_audit` has **no** unique constraint beyond its primary key, so repeated
  rows on one store are fine there.

**Design note — read before writing.** Neither provider sets `DueAt`. GDPR's 30-day window is real, and an unprocessed erasure request is exactly #259's complaint, but `customer_erasure_requests` has no due column and deriving a statutory deadline inside a read endpoint invents policy in the wrong place. The spec records this deliberately. Do not add one.

- [ ] **Step 1: Write the failing tests**

Create `internal/inbox/erasure_integration_test.go`:

```go
//go:build integration

package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedErasure(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, status string, requestedAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	// customer_erasure_requests has UNIQUE (store_id, customer_email), so the
	// email must be distinct per row within a store. Derive it from the row id.
	email := "buyer-" + id.String()[:8] + "@example.com"
	require.NoError(t, db.Exec(`
		INSERT INTO customer_erasure_requests
			(id, tenant_id, store_id, customer_email, requested_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, tenantID, storeID, email, requestedAt, status,
	).Error)
	return id
}

func TestErasureProvider_OnlyPendingAndNoDueDate(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	seedErasure(t, db, tenantID, storeID, "processed", now.Add(-48*time.Hour))
	seedErasure(t, db, tenantID, storeID, "rejected", now.Add(-48*time.Hour))
	wanted := seedErasure(t, db, tenantID, storeID, "pending", now.Add(-72*time.Hour))

	p := inbox.NewErasureProvider(db)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, wanted.String(), items[0].ID)
	require.Equal(t, inbox.KindErasureRequest, items[0].Kind)
	require.Contains(t, items[0].Title, "@example.com", "title is the customer email")
	require.Nil(t, items[0].DueAt,
		"no derived GDPR deadline — the table has no due column and this endpoint does not invent policy")
	require.Equal(t, inbox.SeverityNormal, items[0].Severity)
	require.WithinDuration(t, now.Add(-72*time.Hour), items[0].WaitingSince, time.Second)
}
```

Create `internal/inbox/arbitrage_integration_test.go`:

```go
//go:build integration

package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedArbitrage(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, resolution string, flaggedAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO subscription_arbitrage_audit
			(id, subscription_id, tenant_id, store_id, resolved_price_tier, resolution, flagged_at)
		VALUES (?, ?, ?, ?, 'sea', ?, ?)`,
		id, uuid.New(), tenantID, storeID, resolution, flaggedAt,
	).Error)
	return id
}

func TestArbitrageProvider_OnlyOngoing(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	seedArbitrage(t, db, tenantID, storeID, "upheld", now.Add(-time.Hour))
	wanted := seedArbitrage(t, db, tenantID, storeID, "ongoing", now.Add(-6*time.Hour))

	p := inbox.NewArbitrageProvider(db)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, wanted.String(), items[0].ID)
	require.Equal(t, inbox.KindArbitrageAppeal, items[0].Kind)
	require.Nil(t, items[0].DueAt)
}
```

If `upheld` is not a valid `Resolution` value, read `internal/arbitrage/models.go` and use one that is — `ResolutionOngoing = "ongoing"` is confirmed at `models.go:13`; pick any other declared constant for the resolved row.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55434/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/inbox/... -run 'TestErasureProvider|TestArbitrageProvider' -v
```

Expected: build failure — `undefined: inbox.NewErasureProvider`.

- [ ] **Step 3: Write `erasure.go`**

```go
package inbox

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ErasureProvider surfaces customer_erasure_requests that no one has acted on.
//
// The table is append-only and, before this endpoint, had no reader at all —
// see #259. Items carry no DueAt: GDPR's 30-day window is real, but the table
// has no due column and deriving a statutory deadline in a read endpoint would
// be inventing policy in the wrong place.
type ErasureProvider struct{ db *gorm.DB }

func NewErasureProvider(db *gorm.DB) *ErasureProvider { return &ErasureProvider{db: db} }

func (p *ErasureProvider) Kind() string { return KindErasureRequest }

type erasureRow struct {
	ID            string
	CustomerEmail string
	RequestedAt   time.Time
}

func (p *ErasureProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	q := p.db.WithContext(ctx).
		Table("customer_erasure_requests").
		Select("id::text AS id, customer_email, requested_at").
		Where("status = ?", "pending")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	q = q.Order("requested_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Page > 1 && f.Limit > 0 {
		q = q.Offset((f.Page - 1) * f.Limit)
	}

	var rows []erasureRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, Item{
			ID:           r.ID,
			Kind:         KindErasureRequest,
			Title:        r.CustomerEmail,
			Subtitle:     "Erasure requested",
			WaitingSince: r.RequestedAt,
			Severity:     SeverityNormal,
			Href:         "/admin/erasure/" + r.ID,
			Actions: []Action{
				{ID: "process", Label: "Process erasure", Destructive: true},
				{ID: "reject", Label: "Reject", Destructive: false},
			},
		})
	}
	return items, nil
}

func (p *ErasureProvider) Count(ctx context.Context, f Filter) (int64, error) {
	q := p.db.WithContext(ctx).Table("customer_erasure_requests").Where("status = ?", "pending")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	var n int64
	return n, q.Count(&n).Error
}
```

- [ ] **Step 4: Write `arbitrage.go`**

```go
package inbox

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
)

// ArbitrageProvider surfaces geo-pricing arbitrage flags still under review.
type ArbitrageProvider struct{ db *gorm.DB }

func NewArbitrageProvider(db *gorm.DB) *ArbitrageProvider { return &ArbitrageProvider{db: db} }

func (p *ArbitrageProvider) Kind() string { return KindArbitrageAppeal }

type arbitrageRow struct {
	ID                string
	ResolvedPriceTier string
	MismatchReason    *string
	FlaggedAt         time.Time
}

func (p *ArbitrageProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	q := p.db.WithContext(ctx).
		Table("subscription_arbitrage_audit").
		Select("id::text AS id, resolved_price_tier, mismatch_reason, flagged_at").
		Where("resolution = ?", string(arbitrage.ResolutionOngoing))
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	q = q.Order("flagged_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Page > 1 && f.Limit > 0 {
		q = q.Offset((f.Page - 1) * f.Limit)
	}

	var rows []arbitrageRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		subtitle := "Price tier " + r.ResolvedPriceTier
		if r.MismatchReason != nil && *r.MismatchReason != "" {
			subtitle = *r.MismatchReason
		}
		items = append(items, Item{
			ID:           r.ID,
			Kind:         KindArbitrageAppeal,
			Title:        "Arbitrage flag " + r.ID[:8],
			Subtitle:     subtitle,
			WaitingSince: r.FlaggedAt,
			Severity:     SeverityNormal,
			Href:         "/admin/arbitrage/" + r.ID,
			Actions: []Action{
				{ID: "uphold", Label: "Uphold flag", Destructive: false},
				{ID: "dismiss", Label: "Dismiss flag", Destructive: true},
			},
		})
	}
	return items, nil
}

func (p *ArbitrageProvider) Count(ctx context.Context, f Filter) (int64, error) {
	q := p.db.WithContext(ctx).
		Table("subscription_arbitrage_audit").
		Where("resolution = ?", string(arbitrage.ResolutionOngoing))
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	var n int64
	return n, q.Count(&n).Error
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55434/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/inbox/... -run 'TestErasureProvider|TestArbitrageProvider' -v
```

Expected: `--- PASS: TestErasureProvider_OnlyPendingAndNoDueDate` and
`--- PASS: TestArbitrageProvider_OnlyOngoing`, by name.

- [ ] **Step 6: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/inbox/
git commit -m "feat(inbox): add the erasure_request and arbitrage_appeal providers"
```

---

### Task 4: The `migration_fast_path` provider

**Files:**
- Create: `services/marketplace-api/internal/inbox/migration_fastpath.go`
- Test: `services/marketplace-api/internal/inbox/migration_fastpath_integration_test.go`

**Interfaces:**
- Consumes: everything from Task 1.
- Produces: `func NewMigrationFastPathProvider(db *gorm.DB) *MigrationFastPathProvider`

**Schema constraints, verified — do not re-derive:**
- `evidence_type` is CHECK-constrained to **`('whois_domain','platform_screenshot')`**. An earlier
  draft of this plan used `'invoice'`, which would have failed on insert.
- `status` is CHECK-constrained to `('pending','approved','rejected')`.
- The table has **no** foreign keys and **no** unique constraints beyond its primary key, so no parent
  row needs seeding and repeated rows on one store are fine.

**Context.** `migration.Review` is at `internal/billing/migration/repository.go:27` with `Status` defaulting to `pending`. `migration.Handler.Review` (`handler.go:148`) already implements the decision, and its route is already mounted at `cmd/marketplace-api/main.go:2148` — that is #281's part (b), already done. This provider only surfaces the pending reviews; it does not implement the decision.

- [ ] **Step 1: Write the failing test**

Create `internal/inbox/migration_fastpath_integration_test.go`:

```go
//go:build integration

package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedFastPath(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, status string, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO migration_fast_path_reviews
			(id, tenant_id, store_id, evidence_type, evidence_url, prior_platform, status, created_at)
		VALUES (?, ?, ?, 'platform_screenshot', 'https://example.com/e.png', 'shopify', ?, ?)`,
		id, tenantID, storeID, status, createdAt,
	).Error)
	return id
}

func TestMigrationFastPathProvider_OnlyPending(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	seedFastPath(t, db, tenantID, storeID, "approved", now.Add(-time.Hour))
	wanted := seedFastPath(t, db, tenantID, storeID, "pending", now.Add(-5*time.Hour))

	p := inbox.NewMigrationFastPathProvider(db)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, wanted.String(), items[0].ID)
	require.Equal(t, inbox.KindMigrationFastPath, items[0].Kind)
	require.Equal(t, "shopify", items[0].Subtitle)
}
```

**Table name is confirmed:** `migration.Review.TableName()` returns `migration_fast_path_reviews`
(`internal/billing/migration/repository.go:43`), created by `migrations/000051_migration_fast_path_reviews.up.sql`.
Do not re-derive it.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55434/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/inbox/... -run TestMigrationFastPathProvider -v
```

Expected: `undefined: inbox.NewMigrationFastPathProvider`.

- [ ] **Step 3: Write `migration_fastpath.go`**

```go
package inbox

import (
	"context"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
)

// MigrationFastPathProvider surfaces merchant-initiated platform migration
// submissions awaiting a CSM decision.
//
// The decision itself is migration.Handler.Review, already mounted on the
// /internal group (see cmd/marketplace-api/main.go). This provider only makes
// the pending queue visible.
type MigrationFastPathProvider struct{ db *gorm.DB }

func NewMigrationFastPathProvider(db *gorm.DB) *MigrationFastPathProvider {
	return &MigrationFastPathProvider{db: db}
}

func (p *MigrationFastPathProvider) Kind() string { return KindMigrationFastPath }

func (p *MigrationFastPathProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	q := p.db.WithContext(ctx).Model(&migration.Review{}).Where("status = ?", "pending")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	q = q.Order("created_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Page > 1 && f.Limit > 0 {
		q = q.Offset((f.Page - 1) * f.Limit)
	}

	var rows []migration.Review
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, Item{
			ID:           r.ID.String(),
			Kind:         KindMigrationFastPath,
			Title:        "Fast-path migration " + r.ID.String()[:8],
			Subtitle:     r.PriorPlatform,
			WaitingSince: r.CreatedAt,
			Severity:     SeverityNormal,
			Href:         "/admin/migration-fast-path/" + r.ID.String(),
			Actions: []Action{
				{ID: "approve", Label: "Approve", Destructive: false},
				{ID: "reject", Label: "Reject", Destructive: true},
			},
		})
	}
	return items, nil
}

func (p *MigrationFastPathProvider) Count(ctx context.Context, f Filter) (int64, error) {
	q := p.db.WithContext(ctx).Model(&migration.Review{}).Where("status = ?", "pending")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	var n int64
	return n, q.Count(&n).Error
}
```

This file does not import `time` — it reads `CreatedAt` straight off `migration.Review`, which already
carries a `time.Time`. If your editor adds the import, remove it; `go build` will tell you.

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:55434/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/inbox/... -run TestMigrationFastPathProvider -v
```

Expected: `--- PASS: TestMigrationFastPathProvider_OnlyPending`.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/inbox/
git commit -m "feat(inbox): add the migration_fast_path provider"
```

---

### Task 5: The `onboarding_stalled` provider

The only remote source. It wraps the existing `onboardingfunnel` client rather than reaching into platform-api's database — this design adds no second cross-service path.

**Files:**
- Create: `services/marketplace-api/internal/inbox/onboarding.go`
- Test: `services/marketplace-api/internal/inbox/onboarding_test.go` (a unit test with a fake client; no database needed)

**Interfaces:**
- Consumes: everything from Task 1.
- Produces:
  - `type SessionLister interface{ ListSessions(ctx context.Context, p onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error) }`
  - `func NewOnboardingProvider(c SessionLister, idleThresholdHours float64) *OnboardingProvider`

**Verified client shape.** `onboardingfunnel.Session` carries `ID, Email, Status, LastActivityAt, Abandoned, IdleHours, TenantID`. `SessionsResult` carries `Sessions, Total, Page, Limit`. `SessionsParams` carries `CreatedFrom, CreatedTo, Status, Abandoned, Page, Limit`.

- [ ] **Step 1: Write the failing test**

Create `internal/inbox/onboarding_test.go`:

```go
package inbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

type fakeSessions struct {
	res *onboardingfunnel.SessionsResult
	err error
}

func (f fakeSessions) ListSessions(context.Context, onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error) {
	return f.res, f.err
}

func TestOnboardingProvider_OnlyStalledSessions(t *testing.T) {
	now := time.Now().UTC()
	c := fakeSessions{res: &onboardingfunnel.SessionsResult{
		Sessions: []onboardingfunnel.Session{
			{ID: "fresh", Email: "a@example.com", LastActivityAt: now.Add(-time.Hour), IdleHours: 1},
			{ID: "stalled", Email: "b@example.com", LastActivityAt: now.Add(-80 * time.Hour), IdleHours: 80},
		},
		Total: 2,
	}}

	p := inbox.NewOnboardingProvider(c, 48)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1, "only sessions idle beyond the threshold are waiting on a human")
	require.Equal(t, "stalled", items[0].ID)
	require.Equal(t, inbox.KindOnboardingStalled, items[0].Kind)
	require.Equal(t, "b@example.com", items[0].Title)
	require.Nil(t, items[0].DueAt)
}

func TestOnboardingProvider_ErrorPropagatesForTheAggregatorToDegrade(t *testing.T) {
	p := inbox.NewOnboardingProvider(fakeSessions{err: errors.New("platform-api unreachable")}, 48)

	_, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.Error(t, err, "the provider must not swallow the error — the aggregator degrades on it")
}
```

The second test matters more than it looks. If this provider swallowed the error and returned an empty slice, the aggregator would report zero stalled onboardings instead of marking the kind degraded — an operator could not tell "none" from "we could not ask", which is the invisible-queue failure this whole endpoint exists to end.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api
go test ./internal/inbox/... -run TestOnboardingProvider -v
```

Expected: `undefined: inbox.NewOnboardingProvider`.

- [ ] **Step 3: Write `onboarding.go`**

```go
package inbox

import (
	"context"

	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

// SessionLister is the slice of the onboarding funnel client this provider
// needs. Declaring it here rather than importing the concrete client keeps the
// provider unit-testable with a fake and documents the exact dependency.
type SessionLister interface {
	ListSessions(ctx context.Context, p onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error)
}

// OnboardingProvider surfaces onboarding sessions idle beyond a threshold.
//
// It is the only remote provider: the data lives in platform-api and is reached
// through the same HTTP client that already serves /admin/onboarding/sessions.
// Errors are returned, never swallowed — the aggregator marks this kind
// degraded so an operator can tell "none" from "we could not ask".
type OnboardingProvider struct {
	client             SessionLister
	idleThresholdHours float64
}

func NewOnboardingProvider(c SessionLister, idleThresholdHours float64) *OnboardingProvider {
	if idleThresholdHours <= 0 {
		idleThresholdHours = 48
	}
	return &OnboardingProvider{client: c, idleThresholdHours: idleThresholdHours}
}

func (p *OnboardingProvider) Kind() string { return KindOnboardingStalled }

func (p *OnboardingProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	res, err := p.client.ListSessions(ctx, onboardingfunnel.SessionsParams{Page: 1, Limit: limit})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}

	items := make([]Item, 0, len(res.Sessions))
	for _, s := range res.Sessions {
		if s.IdleHours < p.idleThresholdHours {
			continue
		}
		items = append(items, Item{
			ID:           s.ID,
			Kind:         KindOnboardingStalled,
			Title:        s.Email,
			Subtitle:     "Onboarding idle",
			WaitingSince: s.LastActivityAt,
			Severity:     SeverityNormal,
			Href:         "/admin/onboarding/sessions/" + s.ID,
			Actions:      []Action{{ID: "nudge", Label: "Send reminder", Destructive: false}},
		})
	}
	return items, nil
}

func (p *OnboardingProvider) Count(ctx context.Context, f Filter) (int64, error) {
	items, err := p.List(ctx, f)
	if err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}
```

`Count` deliberately reuses `List`: the remote API has no count-only endpoint, and the idle threshold is applied client-side, so a server-reported `Total` would not answer the same question `List` does. Two round-trips is the honest cost.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
go test ./internal/inbox/... -run TestOnboardingProvider -v
```

Expected: `--- PASS: TestOnboardingProvider_OnlyStalledSessions` and
`--- PASS: TestOnboardingProvider_ErrorPropagatesForTheAggregatorToDegrade`.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/inbox/
git commit -m "feat(inbox): add the onboarding_stalled provider"
```

---

### Task 6: The aggregator

**Files:**
- Create: `services/marketplace-api/internal/inbox/aggregator.go`
- Test: `services/marketplace-api/internal/inbox/aggregator_test.go` (unit tests with fake providers — no database)

**Interfaces:**
- Consumes: `Provider`, `Item`, `Filter` from Task 1.
- Produces:
  - `type Result struct{ Items []Item; Total int64; Degraded []string }`
  - `func NewAggregator(providers ...Provider) *Aggregator`
  - `func (a *Aggregator) List(ctx context.Context, f Filter) (Result, error)`
  - `var ErrPageTooDeep = errors.New(...)`
  - `const MaxAggregateItems = 500`

**The four behaviours to get right**, all from the spec:
1. Order by `DueAt` ascending **nulls last**, then `WaitingSince` ascending.
2. Aggregate mode caps at `MaxAggregateItems`; a request past it returns `ErrPageTooDeep`, **never a silently truncated page**.
3. `Filter.Kind` naming exactly one provider delegates to it and bypasses the cap.
4. A provider that errors is omitted, its kind is listed in `Degraded`, and the request still succeeds. All providers failing returns an error.

- [ ] **Step 1: Write the failing tests**

Create `internal/inbox/aggregator_test.go`:

```go
package inbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/inbox"
)

type fakeProvider struct {
	kind  string
	items []inbox.Item
	err   error
}

func (f fakeProvider) Kind() string { return f.kind }
func (f fakeProvider) List(context.Context, inbox.Filter) ([]inbox.Item, error) {
	return f.items, f.err
}
func (f fakeProvider) Count(context.Context, inbox.Filter) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return int64(len(f.items)), nil
}

func at(base time.Time, d time.Duration) *time.Time { t := base.Add(d); return &t }

func TestAggregator_OverdueFirstThenLongestWaitingNullsLast(t *testing.T) {
	now := time.Now().UTC()
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", items: []inbox.Item{
			{ID: "no-due-old", WaitingSince: now.Add(-72 * time.Hour)},
			{ID: "due-later", WaitingSince: now.Add(-time.Hour), DueAt: at(now, 48*time.Hour)},
		}},
		fakeProvider{kind: "k2", items: []inbox.Item{
			{ID: "no-due-new", WaitingSince: now.Add(-time.Hour)},
			{ID: "overdue", WaitingSince: now.Add(-2 * time.Hour), DueAt: at(now, -time.Hour)},
		}},
	)

	res, err := a.List(context.Background(), inbox.Filter{Page: 1, Limit: 10})
	require.NoError(t, err)

	got := make([]string, len(res.Items))
	for i, it := range res.Items {
		got[i] = it.ID
	}
	require.Equal(t, []string{"overdue", "due-later", "no-due-old", "no-due-new"}, got,
		"due dates ascending first, nulls last ordered by longest waiting")
	require.EqualValues(t, 4, res.Total)
	require.Empty(t, res.Degraded)
}

func TestAggregator_DeepPageIsRefusedNotTruncated(t *testing.T) {
	a := inbox.NewAggregator(fakeProvider{kind: "k1"})

	_, err := a.List(context.Background(), inbox.Filter{Page: 11, Limit: 50})
	require.ErrorIs(t, err, inbox.ErrPageTooDeep,
		"page*limit beyond the cap must error, never return a silently truncated page")

	_, err = a.List(context.Background(), inbox.Filter{Page: 10, Limit: 50})
	require.NoError(t, err, "exactly at the cap is allowed")
}

func TestAggregator_SingleKindDelegatesAndBypassesTheCap(t *testing.T) {
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", items: []inbox.Item{{ID: "a"}}},
		fakeProvider{kind: "k2", items: []inbox.Item{{ID: "b"}}},
	)

	res, err := a.List(context.Background(), inbox.Filter{Kind: "k2", Page: 100, Limit: 50})
	require.NoError(t, err, "a single-kind request pages natively and is not capped")
	require.Len(t, res.Items, 1)
	require.Equal(t, "b", res.Items[0].ID)
}

func TestAggregator_OneFailingProviderDegradesRatherThanFails(t *testing.T) {
	a := inbox.NewAggregator(
		fakeProvider{kind: "healthy", items: []inbox.Item{{ID: "a"}}},
		fakeProvider{kind: "broken", err: errors.New("platform-api unreachable")},
	)

	res, err := a.List(context.Background(), inbox.Filter{Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, []string{"broken"}, res.Degraded,
		"a failed source must be named, so an operator can tell 'none' from 'we could not ask'")
}

func TestAggregator_AllProvidersFailingIsAnError(t *testing.T) {
	boom := errors.New("down")
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", err: boom},
		fakeProvider{kind: "k2", err: boom},
	)

	_, err := a.List(context.Background(), inbox.Filter{Page: 1, Limit: 10})
	require.Error(t, err, "nothing can be rendered, so this is a real outage")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api
go test ./internal/inbox/... -run TestAggregator -v
```

Expected: `undefined: inbox.NewAggregator`.

- [ ] **Step 2b: Pin every provider to the interface at compile time**

Nothing currently enforces that the five providers satisfy `Provider`. Their signatures match today,
but a drift would only surface when Task 7 registers them — late, and with a confusing error. Add to
`internal/inbox/provider.go`, below the interface declaration:

```go
// Compile-time proof that every provider satisfies Provider. Without these, a
// signature drift surfaces only where a provider is registered, far from the
// change that caused it.
var (
	_ Provider = (*SEAReviewProvider)(nil)
	_ Provider = (*ErasureProvider)(nil)
	_ Provider = (*ArbitrageProvider)(nil)
	_ Provider = (*MigrationFastPathProvider)(nil)
	_ Provider = (*OnboardingProvider)(nil)
)
```

Run `go build ./...` immediately after adding this. If it fails, a provider has drifted from the
interface and that is a real finding — report it rather than adjusting the assertion to match.

- [ ] **Step 3: Write `aggregator.go`**

```go
package inbox

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// MaxAggregateItems bounds how deep aggregate-mode pagination may go.
//
// Serving page N at limit L requires the first N*L merged items, so cost grows
// with depth. Past this bound the aggregator refuses rather than truncating: a
// short page that looks complete is worse than an error. Narrowing with
// Filter.Kind delegates to a single provider, which pages natively and is not
// capped.
const MaxAggregateItems = 500

// ErrPageTooDeep is returned when page*limit exceeds MaxAggregateItems.
var ErrPageTooDeep = errors.New("inbox: page too deep for aggregate mode; narrow with kind")

// ErrAllSourcesFailed is returned when no provider answered.
var ErrAllSourcesFailed = errors.New("inbox: every source failed")

// Result is one page of merged work, plus the kinds that could not be reached.
type Result struct {
	Items    []Item
	Total    int64
	Degraded []string
}

// Aggregator fans out across providers, merges, sorts and paginates. It holds
// no per-kind knowledge; adding a kind is one registration.
type Aggregator struct{ providers []Provider }

func NewAggregator(providers ...Provider) *Aggregator {
	return &Aggregator{providers: providers}
}

func (a *Aggregator) List(ctx context.Context, f Filter) (Result, error) {
	page, limit := f.Page, f.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 25
	}

	// Single-kind requests delegate: that provider pages natively, so the
	// aggregate cap does not apply.
	if f.Kind != "" {
		for _, p := range a.providers {
			if p.Kind() != f.Kind {
				continue
			}
			items, err := p.List(ctx, f)
			if err != nil {
				return Result{}, fmt.Errorf("inbox: %s: %w", f.Kind, err)
			}
			total, err := p.Count(ctx, f)
			if err != nil {
				return Result{}, fmt.Errorf("inbox: %s count: %w", f.Kind, err)
			}
			return Result{Items: items, Total: total}, nil
		}
		return Result{}, fmt.Errorf("inbox: unknown kind %q", f.Kind)
	}

	if page*limit > MaxAggregateItems {
		return Result{}, ErrPageTooDeep
	}

	// Fetch enough from each provider to fill the requested window after
	// merging, then merge, sort and slice.
	fanout := f
	fanout.Page = 1
	fanout.Limit = page * limit

	var (
		merged   []Item
		total    int64
		degraded []string
		ok       int
	)
	for _, p := range a.providers {
		items, err := p.List(ctx, fanout)
		if err != nil {
			degraded = append(degraded, p.Kind())
			continue
		}
		n, err := p.Count(ctx, fanout)
		if err != nil {
			degraded = append(degraded, p.Kind())
			continue
		}
		merged = append(merged, items...)
		total += n
		ok++
	}
	if ok == 0 && len(a.providers) > 0 {
		return Result{}, ErrAllSourcesFailed
	}

	sortItems(merged)

	start := (page - 1) * limit
	if start > len(merged) {
		start = len(merged)
	}
	end := start + limit
	if end > len(merged) {
		end = len(merged)
	}

	return Result{Items: merged[start:end], Total: total, Degraded: degraded}, nil
}

// sortItems orders overdue work first, then longest-waiting.
//
// Items with a due date sort ahead of items without one — a null due date is
// not "due at the epoch", it is "no deadline", and must not outrank a real
// breached SLA.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch {
		case a.DueAt != nil && b.DueAt != nil:
			if !a.DueAt.Equal(*b.DueAt) {
				return a.DueAt.Before(*b.DueAt)
			}
		case a.DueAt != nil:
			return true
		case b.DueAt != nil:
			return false
		}
		return a.WaitingSince.Before(b.WaitingSince)
	})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
go test ./internal/inbox/... -run TestAggregator -v
```

Expected: five `--- PASS` lines, by name.

- [ ] **Step 5: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/inbox/
git commit -m "feat(inbox): add the aggregator with ordering, page cap and degradation"
```

---

### Task 7: The handler and its registration

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/inbox.go`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes.go` — one `Deps` field, one conditional mount
- Test: `services/marketplace-api/internal/handlers/platformadmin/inbox_test.go`

**Interfaces:**
- Consumes: `inbox.Result`, `inbox.Filter`, `inbox.ErrPageTooDeep` from Task 6.
- Produces: `func NewInboxHandler(agg InboxAggregator, logger *slog.Logger) *InboxHandler` and `(*InboxHandler).Register(g *gin.RouterGroup)`

**Follow the existing pattern.** `outbox.go` is the closest precedent: a narrow interface for the dependency, a `New…Handler` constructor, a `Register` method mounting one route, a row type with JSON tags, and a `parseFilter`. `Deps` fields are nil-safe — a nil dependency leaves the routes unmounted, which is what lets the binary ship before a dependency exists.

- [ ] **Step 1: Write the failing test**

Create `internal/handlers/platformadmin/inbox_test.go`:

```go
package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

type fakeAgg struct {
	res inbox.Result
	err error
}

func (f fakeAgg) List(context.Context, inbox.Filter) (inbox.Result, error) { return f.res, f.err }

func TestInboxHandler_RendersEnvelopeWithDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{res: inbox.Result{
		Items:    []inbox.Item{{ID: "a", Kind: "erasure_request"}},
		Total:    1,
		Degraded: []string{"onboarding_stalled"},
	}}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data       []inbox.Item `json:"data"`
		Pagination struct {
			Page, Limit int
			Total       int64
		} `json:"pagination"`
		Degraded []string `json:"degraded"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.EqualValues(t, 1, body.Pagination.Total)
	require.Equal(t, []string{"onboarding_stalled"}, body.Degraded,
		"a degraded source must reach the console, not be swallowed")
}

func TestInboxHandler_EmptyIsTwoHundredWithEmptyArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{res: inbox.Result{}}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"data":[]`,
		"empty must serialise as [] not null — the console renders an array")
}

func TestInboxHandler_DeepPageIsFourHundred(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{err: inbox.ErrPageTooDeep}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox?page=99&limit=50", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "kind", "the error must tell the caller how to narrow")
}
```

The `"data":[]` assertion is deliberate. A nil slice marshals to `null` in Go, and a console rendering `null.map(...)` breaks — this is a real bug class, not a style preference.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api
go test ./internal/handlers/platformadmin/... -run TestInboxHandler -v
```

Expected: `undefined: platformadmin.NewInboxHandler`.

- [ ] **Step 3: Write `inbox.go`**

```go
package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// InboxAggregator is the slice of inbox.Aggregator this handler needs.
type InboxAggregator interface {
	List(ctx context.Context, f inbox.Filter) (inbox.Result, error)
}

// InboxHandler answers GET /admin/inbox (#280).
type InboxHandler struct {
	agg    InboxAggregator
	logger *slog.Logger
}

func NewInboxHandler(agg InboxAggregator, logger *slog.Logger) *InboxHandler {
	return &InboxHandler{agg: agg, logger: logger}
}

func (h *InboxHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/inbox", h.List)
}

// inboxListResponse is the house envelope plus Degraded.
//
// Degraded names the kinds that could not be reached. It is omitted when
// empty, so a healthy response is the same shape every other list endpoint
// returns.
type inboxListResponse struct {
	Data       []inbox.Item `json:"data"`
	Pagination pagination   `json:"pagination"`
	Degraded   []string     `json:"degraded,omitempty"`
}

func (h *InboxHandler) List(c *gin.Context) {
	f := h.parseFilter(c)

	res, err := h.agg.List(c.Request.Context(), f)
	if err != nil {
		switch {
		case errors.Is(err, inbox.ErrPageTooDeep):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "page_too_deep",
				"message": "aggregate inbox pagination is bounded; narrow the request with ?kind=",
			})
		default:
			if h.logger != nil {
				h.logger.Error("platform inbox list", "err", err)
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "internal server error",
			})
		}
		return
	}

	// A nil slice marshals to null; the console renders an array.
	items := res.Items
	if items == nil {
		items = []inbox.Item{}
	}

	c.JSON(http.StatusOK, inboxListResponse{
		Data:       items,
		Pagination: pagination{Page: f.Page, Limit: f.Limit, Total: res.Total},
		Degraded:   res.Degraded,
	})
}

func (h *InboxHandler) parseFilter(c *gin.Context) inbox.Filter {
	f := inbox.Filter{
		Kind:     c.Query("kind"),
		TenantID: c.Query("tenant_id"),
		Status:   c.Query("status"),
		Page:     1,
		Limit:    25,
	}
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		f.Page = v
	}
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		if v > 100 {
			v = 100
		}
		f.Limit = v
	}
	return f
}
```

- [ ] **Step 4: Wire it into `routes.go`**

Add to the `Deps` struct, beside `OnboardingFunnel` and following its comment style:

```go
	// Inbox serves /admin/inbox (#280). Nil leaves the route unmounted,
	// matching the nil-safe pattern used for TenantDirectory above.
	Inbox InboxAggregator
```

And in `Register`, beside the other conditional mounts:

```go
	if deps.Inbox != nil {
		NewInboxHandler(deps.Inbox, deps.Logger).Register(g)
	}
```

Read the surrounding code first and match how the neighbouring mounts are written — this plan does not reproduce `Register`'s body, and the existing conditional style is the one to follow.

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd services/marketplace-api
go test ./internal/handlers/platformadmin/... -run TestInboxHandler -v
```

Expected: three `--- PASS` lines, by name.

- [ ] **Step 6: Run the whole package, to be sure the Deps change broke nothing**

```bash
cd services/marketplace-api
go test ./internal/handlers/platformadmin/... 2>&1 | tail -5
```

Expected: `ok`. If the package has pre-existing failures, confirm they are unrelated to this change before continuing.

- [ ] **Step 7: Verify formatting and vet**

```bash
cd services/marketplace-api
gofmt -l . && go build ./... && go vet ./... && go vet -tags=integration ./...
```

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/
git commit -m "feat(platformadmin): add GET /admin/inbox (#280)"
```

---

## Definition of Done

- `GET /admin/inbox` returns the house envelope with `data`, `pagination`, and `degraded` when a source is unreachable.
- All five kinds are represented by a provider, each independently tested.
- Ordering is overdue-first, then longest-waiting, with null due dates last.
- A page beyond `MaxAggregateItems` returns `400` naming `kind` as the way to narrow — never a truncated page.
- `?kind=` delegates to one provider and pages without the cap.
- One provider failing degrades; all failing is a `500`.
- `Deps.Inbox` is nil-safe: a nil aggregator leaves the route unmounted.
- No capability names invented. The `actions` derivation carries a comment pointing at `CapabilityValueChecked`.
- `gofmt`, `go build`, `go vet`, `go vet -tags=integration` all clean.

## Not in this plan

Wiring the aggregator into `cmd/marketplace-api/main.go` — that needs the five providers constructed with real dependencies and belongs with the deployment change. `Deps.Inbox` stays nil until then, which leaves the route unmounted and the binary shippable.

#281's action execution. Capability filtering of `actions` (blocked on the console's vocabulary, #333). A derived GDPR due date for erasure requests.
