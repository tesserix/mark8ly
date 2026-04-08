# Orders M1 — schema migration, GORM models, and atomic document sequence

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the `0002_orders_initial` migration (nine tables: orders, order_items, order_addresses, order_events, returns, return_items, abandoned_carts, document_number_seq, pending_events), the matching GORM models, and the atomic `document_number_seq` upsert helper — gated by a 50-concurrent-goroutine benchmark proving p99 full-create-transaction latency stays under 50ms on a db-f1-micro-equivalent Postgres. No business logic, no HTTP, no state machine beyond raw CHECK constraints. The exit gate of this milestone is a hard fork: if the benchmark fails, the sequencing strategy is reworked before any other orders milestone starts.

**Architecture:** Migration-first. Nine tables with full constraints, partial indexes for the hot query paths (tab list with `(store_id, status, placed_at DESC)`, prefix search via `varchar_pattern_ops` B-tree, outbox drainer via partial index on unpublished rows). GORM models mirror the schema 1:1 with `gorm:"column:..."` tags. Atomic document number generation via single-statement `INSERT ... ON CONFLICT ... DO UPDATE ... RETURNING` — the row lock is held for microseconds, not across the create transaction. Products slice 1 is a hard prerequisite: this plan depends on the `marketplace-api` service binary, `pkg/db`, `pkg/migrate`, `pkg/testdb`, the `0000_extensions` migration (for `pg_trgm`), and the shared `set_updated_at()` trigger function existing in `marketplace_db`.

**Tech Stack:** Go 1.26, GORM v1.25, Postgres 15, golang-migrate, testify, `github.com/shopspring/decimal`, `github.com/google/uuid`. No new external dependencies beyond what Products slice 1 already vendors.

**Spec reference:** `docs/superpowers/specs/2026-04-09-orders-feature-slice-1-design.md` — authoritative sections for this milestone: §4 (database schema), §4.1 (schema design notes), §6.5 step 3 (atomic order number statement), §9 M1 exit criteria, §13 open question 1 (store prefix source), §14 DoD items 1–3.

**Out of scope for M1** (handled later):
- Order state machine Go type + exhaustive transition tests → M2
- Repository query methods beyond raw CRUD → M2
- `order.Service`, `return.Service`, `abandoned_cart.Service` → M2
- Outbox drainer goroutine → M2 (the `pending_events` table lands here, the drainer does not)
- FGA model additions → M3
- HTTP handlers, DTOs, error envelope codes → M4
- Storefront checkout integration → M5
- Prometheus metrics → end of M5 observability pass

---

## Hard prerequisites from Products slice 1

Orders M1 needs only **Products M1 (service scaffold)** and **Products M2 (schema baseline with extensions + the shared trigger function)**. It does NOT depend on Products M3–M7. The orders branch is stacked on the products branch — when products M1+M2 merge to main, orders M1 can rebase off main cleanly.

Before Task 1 runs, these must exist in the working tree:

1. **(Products M1)** `services/marketplace-api/` with `cmd/marketplace-api/main.go`, `cmd/migrate/main.go`, `pkg/db`, `pkg/migrate`, `pkg/testdb`, `migrations.go` with `go:embed migrations/*.sql`.
2. **(Products M1)** `marketplace_db` Postgres database exists in local dev (`docker-compose up` spins it up) and migrates cleanly.
3. **(Products M2)** `services/marketplace-api/migrations/0000_extensions.up.sql` exists and contains `CREATE EXTENSION IF NOT EXISTS pg_trgm;`. Orders M1 **hard-depends on this** — it does not conditionally create the extension.
4. **(Products M2)** `services/marketplace-api/migrations/0001_products_initial.up.sql` exists and defines the shared `set_updated_at()` trigger function. Orders M1 **reuses** this function and does not redefine it.
5. **(Products M1)** CI workflow runs `go test ./...` for `services/marketplace-api` against a real Postgres 15 container. This plan adds tests to the same module and relies on the same CI path — no CI config changes in this plan.

**Task 0 verifies all five before any new files are touched.** If any is missing, pause and ask the human: either the upstream products milestone hasn't landed yet on the branch this is stacked on, or the stack base is wrong.

---

## Decisions locked for this milestone

1. **Migration filename:** `0002_orders_initial.up.sql` / `.down.sql`. Sequential after products' `0001_products_initial`. No gap for a hypothetical future migration between them.
2. **Nine tables in one transaction.** The whole migration is wrapped in `BEGIN; ... COMMIT;` so a mid-migration failure rolls back cleanly. The `down` migration drops tables in reverse-dependency order inside the same pattern.
3. **No seed data.** Test fixtures live in test code, not in the migration.
4. **Store prefix source deferred.** §13 open question 1 of the spec is not resolved in M1. The GORM models accept `order_number` as an opaque `varchar(40)` string. Order number *generation* (which needs the prefix) is implemented in M2. M1 round-trip tests insert hand-crafted order numbers like `M-TEST-260409-00001`.
5. **`document_number_seq` helper is a pure SQL function wrapped by a tiny Go helper.** The Go helper — `order.NextDocumentNumber(ctx, tx, storeID uuid.UUID, kind string, day time.Time) (int, error)` — lives in `internal/order/number.go` and is used in M1 **only by the benchmark test**. It will be reused by `order.Service.Create` in M2. Placing the helper in `internal/order/` now (vs a shared `internal/sequence/` package) is intentional: orders is the first caller and the only slice-1 caller. If returns or another module needs it later, promote to `internal/sequence/` — but do not pre-abstract.
6. **Benchmark is the hard exit gate.** If the 50-goroutine test fails or p99 exceeds 50ms on the CI Postgres container, M1 does not ship. The two fallback strategies — per-store Postgres sequence created at store onboarding, or Redis counter — are documented in §11 risks of the spec. The decision to switch is escalated to the human; this plan does not pre-authorize it.
7. **`idempotency_key` is `NOT NULL`.** Every order must carry one. Slice 1's only create path is the storefront checkout, which will always supply `cart_session_id`. A "synthetic" idempotency key for test fixtures is `fmt.Sprintf("test-%s", uuid.NewString())`.
8. **`refunded_amount DEFAULT 0`** and the `CHECK (refunded_amount <= grand_total)` constraint land in M1. The atomic refund `UPDATE` statement is implemented in M2; M1 only proves the column + constraint exist and reject bad data.
9. **Partial index on `pending_events`** lands in M1 even though no rows are written yet. Cheaper to ship the index with the table than to alter later.
10. **Soft delete is the only delete path for `orders`.** M1 tests explicitly assert that hard-deleting an order with a linked return fails (due to the `return_items → order_items ON DELETE RESTRICT` chain) and documents this invariant in the model GoDoc.
11. **`order_addresses` has no `updated_at`.** Immutable snapshot; no trigger registered. GORM model uses `CreatedAt` only.
12. **GORM model tags use explicit `column:` names** rather than relying on GORM's default camelCase→snake_case conversion. Defensive against future GORM config changes.
13. **`decimal.Decimal` for all money fields** — matches products M2. No `float64`, no `string`, no `int64` cents. The GORM serializer is the one products already wired up.
14. **Benchmark runs in CI.** It's a standard Go test (`go test -run TestDocumentNumberSeq_Concurrent -v`) with a subtest that captures p99 via `sort.Float64s` + index. Not a `go test -bench`. Goal: reproducible pass/fail, not microbenchmark-grade precision.

---

## File structure produced by M1

```
services/marketplace-api/
├── migrations/
│   ├── 0002_orders_initial.up.sql        # NEW — nine tables, indexes, triggers
│   └── 0002_orders_initial.down.sql      # NEW — drops in reverse-dep order
├── internal/
│   └── order/
│       ├── models.go                     # NEW — Order, OrderItem, OrderAddress, OrderEvent
│       ├── models_returns.go             # NEW — Return, ReturnItem
│       ├── models_abandoned_cart.go      # NEW — AbandonedCart
│       ├── models_outbox.go              # NEW — PendingEvent, DocumentNumberSeq
│       ├── number.go                     # NEW — atomic NextDocumentNumber helper
│       ├── number_test.go                # NEW — unit + concurrent benchmark gate
│       ├── models_test.go                # NEW — round-trip + constraint integration tests
│       └── doc.go                        # NEW — package GoDoc with invariants
└── migrations_test.go                    # MODIFY — append "up → down → up" test for 0002
```

No other files are created or modified. No Dockerfile changes, no CI config changes, no infra changes.

---

## Task decomposition

Tasks are strictly serial except where noted. Each task ends with a commit. No task creates files outside the tree in §"File structure produced by M1".

### Task 0: Verify Products slice 1 prerequisites

**Files:** none (verification only)

- [ ] **Step 1: Verify marketplace-api service exists**

Run:
```bash
ls services/marketplace-api/cmd/marketplace-api/main.go \
   services/marketplace-api/cmd/migrate/main.go \
   services/marketplace-api/pkg/db \
   services/marketplace-api/pkg/migrate \
   services/marketplace-api/pkg/testdb \
   services/marketplace-api/migrations.go
```
Expected: all paths print without error. If any `ls: No such file`, **STOP**. Products slice 1 M1 has not landed. Ask the human to merge it.

- [ ] **Step 2: Verify `0000_extensions` migration shipped with pg_trgm**

Run:
```bash
grep -l "pg_trgm" services/marketplace-api/migrations/0000_extensions.up.sql
```
Expected: prints the file path. If empty, **STOP** — the extension prereq is missing.

- [ ] **Step 3: Verify `0001_products_initial` shipped with the shared trigger function**

Run:
```bash
grep -n "CREATE OR REPLACE FUNCTION set_updated_at" services/marketplace-api/migrations/0001_products_initial.up.sql
```
Expected: one match. If zero, **STOP** — this plan cannot reuse the trigger function that doesn't exist.

- [ ] **Step 4: Verify migrate up → down → up cycles cleanly against local Postgres**

Run (from repo root, with `docker-compose up -d postgres` already running):
```bash
cd services/marketplace-api && \
  go run ./cmd/migrate -direction up && \
  go run ./cmd/migrate -direction down && \
  go run ./cmd/migrate -direction up
```
Expected: each command prints "migration complete" (or equivalent from the existing tool) and exits 0. If any fails, the products migration base is broken — **STOP** and report.

- [ ] **Step 5: Verify `marketplace_db` is at the expected Products slice 1 schema version**

Run:
```bash
psql -h localhost -U dev -d marketplace_db -c \
  "SELECT version FROM marketplace_db_schema_migrations ORDER BY version DESC LIMIT 1;"
```
Expected: prints `1` (the products migration). If 0 or missing, migrations did not run. If > 1, something else is ahead of us — **STOP** and investigate.

No commit — Task 0 is verification only.

---

### Task 1: Write `0002_orders_initial.up.sql` — tables + CHECK constraints

**Files:**
- Create: `services/marketplace-api/migrations/0002_orders_initial.up.sql`

- [ ] **Step 1: Scaffold the migration file with BEGIN/COMMIT**

Create the file with this skeleton:
```sql
-- 0002_orders_initial.up.sql
-- Orders slice 1: orders, order_items, order_addresses, order_events,
-- returns, return_items, abandoned_carts, document_number_seq, pending_events.
-- Spec: docs/superpowers/specs/2026-04-09-orders-feature-slice-1-design.md §4

BEGIN;

-- tables go here

COMMIT;
```

- [ ] **Step 2: Add the `orders` table**

Paste the `CREATE TABLE orders (...)` block from spec §4, including all CHECK constraints:
- `orders_number_per_store_unique`
- `orders_idempotency_per_store_unique`
- `orders_status_valid` with values `('pending','confirmed','fulfilled','cancelled')` — note `refunded` is deliberately excluded
- `orders_payment_status_valid` with values `('pending','authorized','paid','failed','refunded','partially_refunded')`
- `orders_fulfillment_status_valid` with values `('unfulfilled','partial','fulfilled')`
- `orders_grand_total_non_negative`
- `orders_subtotal_non_negative`
- `orders_refunded_non_negative`
- `orders_refunded_not_exceeding_total`
- `orders_currency_format`
- `orders_cancelled_requires_timestamp`

Do not add any indexes yet — they go in Task 2.

- [ ] **Step 3: Add the `order_items`, `order_addresses`, `order_events` tables**

Append from spec §4 verbatim:
- `order_items` with `title_snapshot`, `sku_snapshot`, `option_summary`, `unit_price`, `line_total`, `currency_code`, `image_url`. Note no FK on `product_id` / `variant_id` (intentional — see §4.1).
- `order_addresses` with `kind IN ('shipping','billing')`, country format CHECK, `UNIQUE (order_id, kind)`.
- `order_events` with `kind varchar(40)`, `actor_id`/`actor_email` nullable, `payload jsonb NOT NULL DEFAULT '{}'::jsonb`.

- [ ] **Step 4: Add the `returns`, `return_items`, `abandoned_carts` tables**

From spec §4:
- `returns` with all status CHECK values, `UNIQUE (store_id, return_number)`, refund non-negative, currency format.
- `return_items` with `ON DELETE CASCADE` on `returns.id` and `ON DELETE RESTRICT` on `order_items.id` — this is the chain that makes hard-delete of an order impossible when returns exist.
- `abandoned_carts` **including** the `cart_session_id varchar(100) NOT NULL` column AND the `updated_at timestamptz NOT NULL DEFAULT now()` column directly on the CREATE TABLE (the spec shows them as ALTERs for readability; in the migration they are inline).

- [ ] **Step 5: Add the `document_number_seq` and `pending_events` tables + the `abandoned_carts_set_updated_at` trigger**

```sql
CREATE TABLE document_number_seq (
    store_id uuid        NOT NULL,
    kind     varchar(10) NOT NULL,
    day      date        NOT NULL,
    last_seq integer     NOT NULL DEFAULT 0,
    PRIMARY KEY (store_id, kind, day),
    CONSTRAINT document_number_seq_kind_valid CHECK (kind IN ('order','return'))
);

CREATE TABLE pending_events (
    id               uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid         NOT NULL,
    store_id         uuid         NOT NULL,
    topic            varchar(80)  NOT NULL,
    payload          jsonb        NOT NULL,
    attempts         integer      NOT NULL DEFAULT 0,
    last_error       text,
    next_attempt_at  timestamptz  NOT NULL DEFAULT now(),
    published_at     timestamptz,
    dead_lettered_at timestamptz,
    created_at       timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT pending_events_attempts_non_negative CHECK (attempts >= 0)
);

CREATE TRIGGER orders_set_updated_at          BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER returns_set_updated_at         BEFORE UPDATE ON returns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER abandoned_carts_set_updated_at BEFORE UPDATE ON abandoned_carts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

Note: `order_items`, `order_addresses`, `order_events`, `return_items`, `document_number_seq`, `pending_events` do NOT get `updated_at` triggers — they are immutable (snapshot / append-only) or counter rows.

- [ ] **Step 6: Run the migration up against local Postgres**

```bash
cd services/marketplace-api && go run ./cmd/migrate -direction up
```
Expected: exits 0.

- [ ] **Step 7: Verify all nine tables exist**

```bash
psql -h localhost -U dev -d marketplace_db -c "\dt" | \
  grep -E 'orders|order_items|order_addresses|order_events|returns|return_items|abandoned_carts|document_number_seq|pending_events'
```
Expected: all nine tables listed.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/migrations/0002_orders_initial.up.sql
git commit -m "feat(marketplace-api): add 0002 orders initial tables (up)"
```

---

### Task 2: Add indexes to `0002_orders_initial.up.sql`

**Files:**
- Modify: `services/marketplace-api/migrations/0002_orders_initial.up.sql`

- [ ] **Step 1: Roll back the migration so we can re-run it with the indexes**

```bash
cd services/marketplace-api && go run ./cmd/migrate -direction down
```
Expected: `marketplace_db_schema_migrations` is back at version 1. Verify with the psql query from Task 0 step 5.

- [ ] **Step 2: Append the `orders` indexes**

Paste inside the transaction, after the `CREATE TABLE orders` block, before the next table:
```sql
CREATE INDEX orders_store_placed_idx
    ON orders (store_id, placed_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX orders_store_status_placed_idx
    ON orders (store_id, status, placed_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX orders_customer_idx
    ON orders (customer_id)
    WHERE deleted_at IS NULL AND customer_id IS NOT NULL;
CREATE INDEX orders_email_idx
    ON orders (lower(customer_email))
    WHERE deleted_at IS NULL;
CREATE INDEX orders_number_idx
    ON orders (store_id, order_number varchar_pattern_ops)
    WHERE deleted_at IS NULL;
```
Note: no `orders_tenant_idx` — deliberately dropped per spec §4.1.

- [ ] **Step 3: Append the remaining table indexes**

After each respective `CREATE TABLE`:
```sql
-- order_items
CREATE INDEX order_items_order_idx   ON order_items (order_id);
CREATE INDEX order_items_variant_idx ON order_items (variant_id) WHERE variant_id IS NOT NULL;

-- order_events
CREATE INDEX order_events_order_idx  ON order_events (order_id, created_at DESC);

-- returns
CREATE INDEX returns_order_idx        ON returns (order_id);
CREATE INDEX returns_store_status_idx ON returns (store_id, status);

-- return_items
CREATE INDEX return_items_return_idx ON return_items (return_id);

-- abandoned_carts
CREATE UNIQUE INDEX abandoned_carts_session_unique       ON abandoned_carts (store_id, cart_session_id);
CREATE INDEX        abandoned_carts_tenant_idx           ON abandoned_carts (tenant_id);
CREATE INDEX        abandoned_carts_store_last_active_idx ON abandoned_carts (store_id, last_active_at DESC);
CREATE INDEX        abandoned_carts_email_idx            ON abandoned_carts (lower(customer_email))
    WHERE customer_email IS NOT NULL;

-- pending_events
CREATE INDEX pending_events_pending_idx
    ON pending_events (next_attempt_at)
    WHERE published_at IS NULL AND dead_lettered_at IS NULL;
```

- [ ] **Step 4: Run the migration up**

```bash
cd services/marketplace-api && go run ./cmd/migrate -direction up
```
Expected: exits 0.

- [ ] **Step 5: Verify all indexes exist**

```bash
psql -h localhost -U dev -d marketplace_db -c "\di" | \
  grep -E 'orders_store_placed_idx|orders_store_status_placed_idx|orders_number_idx|pending_events_pending_idx|abandoned_carts_session_unique'
```
Expected: all five listed.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/migrations/0002_orders_initial.up.sql
git commit -m "feat(marketplace-api): add 0002 orders partial indexes"
```

---

### Task 3: Write `0002_orders_initial.down.sql`

**Files:**
- Create: `services/marketplace-api/migrations/0002_orders_initial.down.sql`

- [ ] **Step 1: Create the down migration**

Reverse-dependency order. Drop triggers before tables (implicit when tables drop, but explicit is clearer for reviewers).

```sql
-- 0002_orders_initial.down.sql
BEGIN;

DROP TABLE IF EXISTS pending_events;
DROP TABLE IF EXISTS document_number_seq;
DROP TABLE IF EXISTS abandoned_carts;
DROP TABLE IF EXISTS return_items;
DROP TABLE IF EXISTS returns;
DROP TABLE IF EXISTS order_events;
DROP TABLE IF EXISTS order_addresses;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;

-- set_updated_at() and pg_trgm are owned by upstream migrations. Do NOT drop.

COMMIT;
```

- [ ] **Step 2: Run down → up → down → up to prove it cycles**

```bash
cd services/marketplace-api && \
  go run ./cmd/migrate -direction down && \
  go run ./cmd/migrate -direction up && \
  go run ./cmd/migrate -direction down && \
  go run ./cmd/migrate -direction up
```
Expected: all four commands exit 0. If any fails, fix the down migration before proceeding.

- [ ] **Step 3: Verify the products-owned function `set_updated_at()` still exists after down**

Run `go run ./cmd/migrate -direction down` one more time, then:
```bash
psql -h localhost -U dev -d marketplace_db -c \
  "SELECT proname FROM pg_proc WHERE proname = 'set_updated_at';"
```
Expected: one row. If zero, the down migration incorrectly dropped the shared function — fix and re-verify.

Re-run `go run ./cmd/migrate -direction up` to leave the database in the migrated state.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/0002_orders_initial.down.sql
git commit -m "feat(marketplace-api): add 0002 orders down migration"
```

---

### Task 4: Append `0002` to `migrations_test.go` up/down cycling test

**Files:**
- Modify: `services/marketplace-api/migrations_test.go`

- [ ] **Step 1: Read the existing test to understand its shape**

```bash
cat services/marketplace-api/migrations_test.go
```
Identify the test that asserts "up → down → up cycles cleanly" for `0001_products_initial`. It should be a `TestMigrations_UpDownUp` or similar with a test case list.

- [ ] **Step 2: Add the `0002_orders_initial` case**

Extend the test table (or equivalent) to include the new migration file:
```go
// In the test case slice:
{
    name:    "0002_orders_initial",
    version: 2,
    tables: []string{
        "orders", "order_items", "order_addresses", "order_events",
        "returns", "return_items", "abandoned_carts",
        "document_number_seq", "pending_events",
    },
},
```

The test should:
1. Run migrate up through version 2
2. Assert all nine tables exist
3. Run migrate down to version 1
4. Assert none of the nine tables exist (but products tables still do)
5. Run migrate up back to version 2
6. Assert all nine tables exist again

- [ ] **Step 3: Run the test**

```bash
cd services/marketplace-api && go test -run TestMigrations -v .
```
Expected: PASS with the new subtest visible.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations_test.go
git commit -m "test(marketplace-api): cover 0002 orders in migration cycle test"
```

---

### Task 5: Write GORM models for orders tables

**Files:**
- Create: `services/marketplace-api/internal/order/models.go`
- Create: `services/marketplace-api/internal/order/doc.go`

- [ ] **Step 1: Write the package `doc.go` with invariants**

```go
// Package order owns the orders, order_items, order_addresses, order_events,
// returns, return_items, abandoned_carts, document_number_seq, and pending_events
// tables in marketplace_db.
//
// Invariants (enforced at the DB layer; repeated here so readers don't need to
// reread the migration to understand them):
//
//   - orders.status is the operational lifecycle and NEVER includes 'refunded'.
//     Money state lives on orders.payment_status exclusively.
//   - orders.refunded_amount is the atomic running refund total. Refunds are
//     recorded via a single UPDATE ... WHERE refunded_amount + $new <= grand_total
//     statement (implemented in M2). M1 only proves the column exists.
//   - order_items is a price snapshot. product_id/variant_id have NO foreign keys
//     so products can be hard-deleted without corrupting order history. DO NOT
//     add those foreign keys later.
//   - order_addresses is immutable. No updated_at column, no trigger.
//   - returns and return_items hold an ON DELETE RESTRICT chain back to
//     order_items, which makes hard-deleting an order with returns IMPOSSIBLE.
//     Soft delete via orders.deleted_at is the only delete path in slice 1.
//   - document_number_seq is incremented via atomic upsert; the row lock is held
//     for a single statement, NOT across the full create transaction.
//
// See docs/superpowers/specs/2026-04-09-orders-feature-slice-1-design.md §4.1.
package order
```

- [ ] **Step 2: Write `models.go` with `Order`, `OrderItem`, `OrderAddress`, `OrderEvent`**

```go
package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// Order is the root aggregate. One row per checkout.
type Order struct {
	ID                uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID          uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID           uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	OrderNumber       string          `gorm:"column:order_number;type:varchar(40);not null"`
	IdempotencyKey    string          `gorm:"column:idempotency_key;type:varchar(100);not null"`
	CustomerID        *uuid.UUID      `gorm:"column:customer_id;type:uuid"`
	CustomerEmail     string          `gorm:"column:customer_email;type:varchar(320);not null"`
	CustomerName      *string         `gorm:"column:customer_name;type:varchar(200)"`
	Status            string          `gorm:"column:status;type:varchar(20);not null;default:pending"`
	PaymentStatus     string          `gorm:"column:payment_status;type:varchar(20);not null;default:pending"`
	FulfillmentStatus string          `gorm:"column:fulfillment_status;type:varchar(20);not null;default:unfulfilled"`
	Subtotal          decimal.Decimal `gorm:"column:subtotal;type:numeric(12,2);not null"`
	ShippingTotal     decimal.Decimal `gorm:"column:shipping_total;type:numeric(12,2);not null;default:0"`
	TaxTotal          decimal.Decimal `gorm:"column:tax_total;type:numeric(12,2);not null;default:0"`
	DiscountTotal     decimal.Decimal `gorm:"column:discount_total;type:numeric(12,2);not null;default:0"`
	GrandTotal        decimal.Decimal `gorm:"column:grand_total;type:numeric(12,2);not null"`
	RefundedAmount    decimal.Decimal `gorm:"column:refunded_amount;type:numeric(12,2);not null;default:0"`
	CurrencyCode      string          `gorm:"column:currency_code;type:char(3);not null"`
	PaymentProvider   *string         `gorm:"column:payment_provider;type:varchar(40)"`
	PaymentIntentID   *string         `gorm:"column:payment_intent_id;type:varchar(200)"`
	Notes             *string         `gorm:"column:notes;type:text"`
	PlacedAt          time.Time       `gorm:"column:placed_at;not null;default:now()"`
	CancelledAt       *time.Time      `gorm:"column:cancelled_at"`
	FulfilledAt       *time.Time      `gorm:"column:fulfilled_at"`
	CreatedAt         time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt         time.Time       `gorm:"column:updated_at;not null;default:now()"`
	DeletedAt         *time.Time      `gorm:"column:deleted_at"`
}

func (Order) TableName() string { return "orders" }

// OrderItem is a price snapshot. product_id/variant_id are intentionally not FKs.
type OrderItem struct {
	ID             uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID        uuid.UUID       `gorm:"column:order_id;type:uuid;not null"`
	ProductID      *uuid.UUID      `gorm:"column:product_id;type:uuid"`
	VariantID      *uuid.UUID      `gorm:"column:variant_id;type:uuid"`
	TitleSnapshot  string          `gorm:"column:title_snapshot;type:varchar(300);not null"`
	SKUSnapshot    string          `gorm:"column:sku_snapshot;type:varchar(100);not null"`
	OptionSummary  *string         `gorm:"column:option_summary;type:varchar(300)"`
	UnitPrice      decimal.Decimal `gorm:"column:unit_price;type:numeric(12,2);not null"`
	Quantity       int             `gorm:"column:quantity;type:integer;not null"`
	LineTotal      decimal.Decimal `gorm:"column:line_total;type:numeric(12,2);not null"`
	CurrencyCode   string          `gorm:"column:currency_code;type:char(3);not null"`
	ImageURL       *string         `gorm:"column:image_url;type:text"`
	CreatedAt      time.Time       `gorm:"column:created_at;not null;default:now()"`
}

func (OrderItem) TableName() string { return "order_items" }

// OrderAddress is an immutable snapshot. No UpdatedAt.
type OrderAddress struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID     uuid.UUID `gorm:"column:order_id;type:uuid;not null"`
	Kind        string    `gorm:"column:kind;type:varchar(10);not null"` // 'shipping' | 'billing'
	Name        string    `gorm:"column:name;type:varchar(200);not null"`
	Line1       string    `gorm:"column:line1;type:varchar(300);not null"`
	Line2       *string   `gorm:"column:line2;type:varchar(300)"`
	City        string    `gorm:"column:city;type:varchar(200);not null"`
	Region      *string   `gorm:"column:region;type:varchar(200)"`
	PostalCode  *string   `gorm:"column:postal_code;type:varchar(40)"`
	CountryCode string    `gorm:"column:country_code;type:char(2);not null"`
	Phone       *string   `gorm:"column:phone;type:varchar(40)"`
}

func (OrderAddress) TableName() string { return "order_addresses" }

// OrderEvent is append-only. Payload is opaque JSONB.
type OrderEvent struct {
	ID         uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OrderID    uuid.UUID      `gorm:"column:order_id;type:uuid;not null"`
	Kind       string         `gorm:"column:kind;type:varchar(40);not null"`
	ActorID    *uuid.UUID     `gorm:"column:actor_id;type:uuid"`
	ActorEmail *string        `gorm:"column:actor_email;type:varchar(320)"`
	Payload    datatypes.JSON `gorm:"column:payload;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt  time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (OrderEvent) TableName() string { return "order_events" }
```

- [ ] **Step 3: Run `go build`**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0. If the `decimal` or `datatypes` imports fail, verify they are already in `go.mod` from products slice 1. If not, `go get` them.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/order/models.go services/marketplace-api/internal/order/doc.go
git commit -m "feat(marketplace-api): add GORM models for orders aggregate"
```

---

### Task 6: Write GORM models for returns

**Files:**
- Create: `services/marketplace-api/internal/order/models_returns.go`

- [ ] **Step 1: Write `Return` and `ReturnItem`**

```go
package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Return struct {
	ID           uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID        `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID      uuid.UUID        `gorm:"column:store_id;type:uuid;not null"`
	OrderID      uuid.UUID        `gorm:"column:order_id;type:uuid;not null"`
	ReturnNumber string           `gorm:"column:return_number;type:varchar(40);not null"`
	Status       string           `gorm:"column:status;type:varchar(20);not null;default:requested"`
	Reason       *string          `gorm:"column:reason;type:varchar(80)"`
	Notes        *string          `gorm:"column:notes;type:text"`
	RefundAmount *decimal.Decimal `gorm:"column:refund_amount;type:numeric(12,2)"`
	CurrencyCode string           `gorm:"column:currency_code;type:char(3);not null"`
	RequestedAt  time.Time        `gorm:"column:requested_at;not null;default:now()"`
	ReceivedAt   *time.Time       `gorm:"column:received_at"`
	RefundedAt   *time.Time       `gorm:"column:refunded_at"`
	CreatedAt    time.Time        `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt    time.Time        `gorm:"column:updated_at;not null;default:now()"`
}

func (Return) TableName() string { return "returns" }

type ReturnItem struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ReturnID    uuid.UUID `gorm:"column:return_id;type:uuid;not null"`
	OrderItemID uuid.UUID `gorm:"column:order_item_id;type:uuid;not null"`
	Quantity    int       `gorm:"column:quantity;type:integer;not null"`
	Reason      *string   `gorm:"column:reason;type:varchar(80)"`
}

func (ReturnItem) TableName() string { return "return_items" }
```

- [ ] **Step 2: Run `go build`**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/models_returns.go
git commit -m "feat(marketplace-api): add GORM models for returns"
```

---

### Task 7: Write GORM models for abandoned carts, document_number_seq, pending_events

**Files:**
- Create: `services/marketplace-api/internal/order/models_abandoned_cart.go`
- Create: `services/marketplace-api/internal/order/models_outbox.go`

- [ ] **Step 1: Write `AbandonedCart`**

`models_abandoned_cart.go`:
```go
package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

type AbandonedCart struct {
	ID               uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID         uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID          uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	CartSessionID    string          `gorm:"column:cart_session_id;type:varchar(100);not null"`
	CustomerEmail    *string         `gorm:"column:customer_email;type:varchar(320)"`
	CustomerName     *string         `gorm:"column:customer_name;type:varchar(200)"`
	CustomerID       *uuid.UUID      `gorm:"column:customer_id;type:uuid"`
	ItemCount        int             `gorm:"column:item_count;type:integer;not null"`
	Subtotal         decimal.Decimal `gorm:"column:subtotal;type:numeric(12,2);not null"`
	CurrencyCode     string          `gorm:"column:currency_code;type:char(3);not null"`
	ItemsSnapshot    datatypes.JSON  `gorm:"column:items_snapshot;type:jsonb;not null"`
	RecoveryURL      *string         `gorm:"column:recovery_url;type:text"`
	LastActiveAt     time.Time       `gorm:"column:last_active_at;not null"`
	RecoverySentAt   *time.Time      `gorm:"column:recovery_sent_at"`
	ConvertedOrderID *uuid.UUID      `gorm:"column:converted_order_id;type:uuid"`
	CreatedAt        time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt        time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (AbandonedCart) TableName() string { return "abandoned_carts" }
```

- [ ] **Step 2: Write `DocumentNumberSeq` and `PendingEvent`**

`models_outbox.go`:
```go
package order

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// DocumentNumberSeq is the per-store per-day counter for orders and returns.
// Never UPDATE-or-SELECT-FOR-UPDATE this table directly — always go through
// NextDocumentNumber() which uses an atomic upsert.
type DocumentNumberSeq struct {
	StoreID uuid.UUID `gorm:"column:store_id;type:uuid;primaryKey"`
	Kind    string    `gorm:"column:kind;type:varchar(10);primaryKey"`
	Day     time.Time `gorm:"column:day;type:date;primaryKey"`
	LastSeq int       `gorm:"column:last_seq;type:integer;not null;default:0"`
}

func (DocumentNumberSeq) TableName() string { return "document_number_seq" }

// PendingEvent is the transactional outbox row.
type PendingEvent struct {
	ID             uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID       uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID        uuid.UUID      `gorm:"column:store_id;type:uuid;not null"`
	Topic          string         `gorm:"column:topic;type:varchar(80);not null"`
	Payload        datatypes.JSON `gorm:"column:payload;type:jsonb;not null"`
	Attempts       int            `gorm:"column:attempts;type:integer;not null;default:0"`
	LastError      *string        `gorm:"column:last_error;type:text"`
	NextAttemptAt  time.Time      `gorm:"column:next_attempt_at;not null;default:now()"`
	PublishedAt    *time.Time     `gorm:"column:published_at"`
	DeadLetteredAt *time.Time     `gorm:"column:dead_lettered_at"`
	CreatedAt      time.Time      `gorm:"column:created_at;not null;default:now()"`
}

func (PendingEvent) TableName() string { return "pending_events" }
```

- [ ] **Step 3: Run `go build`**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/order/models_abandoned_cart.go \
        services/marketplace-api/internal/order/models_outbox.go
git commit -m "feat(marketplace-api): add GORM models for abandoned carts and outbox"
```

---

### Task 8: Round-trip integration test — full order graph

**Files:**
- Create: `services/marketplace-api/internal/order/models_test.go`

- [ ] **Step 1: Write the failing test**

```go
package order_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestOrderGraph_RoundTrip(t *testing.T) {
	db := testdb.New(t)

	tenantID := uuid.New()
	storeID := uuid.New()

	o := order.Order{
		TenantID:       tenantID,
		StoreID:        storeID,
		OrderNumber:    "M-TEST-260409-00001",
		IdempotencyKey: "test-" + uuid.NewString(),
		CustomerEmail:  "buyer@example.com",
		Subtotal:       decimal.NewFromInt(100),
		GrandTotal:     decimal.NewFromInt(100),
		CurrencyCode:   "EUR",
		PlacedAt:       time.Now(),
	}
	require.NoError(t, db.Create(&o).Error)

	item := order.OrderItem{
		OrderID:       o.ID,
		TitleSnapshot: "Porcelain bowl",
		SKUSnapshot:   "BOWL-M-NATURAL",
		UnitPrice:     decimal.NewFromInt(50),
		Quantity:      2,
		LineTotal:     decimal.NewFromInt(100),
		CurrencyCode:  "EUR",
	}
	require.NoError(t, db.Create(&item).Error)

	ship := order.OrderAddress{
		OrderID:     o.ID,
		Kind:        "shipping",
		Name:        "A Buyer",
		Line1:       "1 Main St",
		City:        "Dublin",
		CountryCode: "IE",
	}
	require.NoError(t, db.Create(&ship).Error)

	evt := order.OrderEvent{
		OrderID: o.ID,
		Kind:    "status_changed",
		Payload: datatypes.JSON([]byte(`{"from":null,"to":"pending"}`)),
	}
	require.NoError(t, db.Create(&evt).Error)

	var back order.Order
	require.NoError(t, db.First(&back, "id = ?", o.ID).Error)
	require.Equal(t, o.OrderNumber, back.OrderNumber)
	require.True(t, o.GrandTotal.Equal(back.GrandTotal))
	require.Equal(t, "pending", back.Status)
	require.Equal(t, "pending", back.PaymentStatus)
	require.Equal(t, "unfulfilled", back.FulfillmentStatus)
	require.True(t, back.RefundedAmount.IsZero())
}
```

- [ ] **Step 2: Run it, expect PASS**

```bash
cd services/marketplace-api && go test -run TestOrderGraph_RoundTrip -v ./internal/order/
```
Expected: PASS. If fail, read the error and fix — most likely a missing column tag or testdb helper signature mismatch.

- [ ] **Step 3: Add a soft-delete isolation test**

Append to the test file:
```go
func TestOrder_SoftDelete_HidesFromDefaultQueries(t *testing.T) {
	db := testdb.New(t)
	o := newTestOrder(t, db)

	now := time.Now()
	require.NoError(t, db.Model(&o).Update("deleted_at", now).Error)

	// Default list query must filter via `deleted_at IS NULL`
	var rows []order.Order
	require.NoError(t, db.Where("store_id = ? AND deleted_at IS NULL", o.StoreID).Find(&rows).Error)
	require.Empty(t, rows)
}
```
Add a `newTestOrder` helper that returns a minimal valid `order.Order`.

- [ ] **Step 4: Run both tests**

```bash
cd services/marketplace-api && go test -v ./internal/order/
```
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/order/models_test.go
git commit -m "test(marketplace-api): order graph round-trip + soft-delete isolation"
```

---

### Task 9: Constraint tests — CHECK guards reject bad data

**Files:**
- Modify: `services/marketplace-api/internal/order/models_test.go`

- [ ] **Step 1: Add test — `status = 'refunded'` is rejected (slice 1 excludes it)**

```go
func TestOrder_Status_RefundedIsRejected(t *testing.T) {
	db := testdb.New(t)
	o := newTestOrder(t, db)
	err := db.Model(&o).Update("status", "refunded").Error
	require.Error(t, err)
}
```

- [ ] **Step 2: Add test — `refunded_amount > grand_total` is rejected**

```go
func TestOrder_RefundedAmount_CannotExceedGrandTotal(t *testing.T) {
	db := testdb.New(t)
	o := newTestOrder(t, db) // GrandTotal = 100
	err := db.Model(&o).Update("refunded_amount", decimal.NewFromInt(101)).Error
	require.Error(t, err)
}
```

- [ ] **Step 3: Add test — `(store_id, idempotency_key)` uniqueness**

```go
func TestOrder_IdempotencyKey_UniquePerStore(t *testing.T) {
	db := testdb.New(t)
	o1 := newTestOrder(t, db)

	o2 := o1
	o2.ID = uuid.Nil
	o2.OrderNumber = "M-TEST-260409-00002"
	// same StoreID, same IdempotencyKey → must fail
	err := db.Create(&o2).Error
	require.Error(t, err)
}
```

- [ ] **Step 4: Add test — return_items ON DELETE RESTRICT blocks hard-delete of an order**

```go
func TestOrder_HardDelete_BlockedByReturnItems(t *testing.T) {
	db := testdb.New(t)
	o := newTestOrder(t, db)
	item := newTestOrderItem(t, db, o.ID)

	r := order.Return{
		TenantID: o.TenantID, StoreID: o.StoreID, OrderID: o.ID,
		ReturnNumber: "R-TEST-260409-00001", CurrencyCode: "EUR",
	}
	require.NoError(t, db.Create(&r).Error)

	ri := order.ReturnItem{ReturnID: r.ID, OrderItemID: item.ID, Quantity: 1}
	require.NoError(t, db.Create(&ri).Error)

	// Now try to hard-delete the order — must fail because return_items RESTRICTs
	err := db.Unscoped().Delete(&o).Error
	require.Error(t, err, "expected RESTRICT from return_items chain")
}
```

- [ ] **Step 5: Run the whole test file**

```bash
cd services/marketplace-api && go test -v ./internal/order/
```
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/order/models_test.go
git commit -m "test(marketplace-api): CHECK and FK constraint coverage for orders"
```

---

### Task 10: Implement `NextDocumentNumber` atomic helper

**Files:**
- Create: `services/marketplace-api/internal/order/number.go`

- [ ] **Step 1: Write the helper**

```go
package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NextDocumentNumber issues the next per-day sequence number for a (store, kind)
// pair via a single atomic upsert. The row lock is held for exactly one
// statement — do NOT wrap this in SELECT ... FOR UPDATE or hold it across a
// wider transaction.
//
// kind must be one of: "order", "return".
//
// This function MUST be called inside an open transaction (pass the tx.DB() of
// the caller's tx). It does not open its own transaction so that the sequence
// increment is rolled back atomically with the domain write that uses it.
func NextDocumentNumber(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, kind string, day time.Time) (int, error) {
	if kind != "order" && kind != "return" {
		return 0, fmt.Errorf("invalid document kind %q", kind)
	}
	dayOnly := day.UTC().Truncate(24 * time.Hour)

	var lastSeq int
	err := tx.WithContext(ctx).Raw(`
		INSERT INTO document_number_seq (store_id, kind, day, last_seq)
		VALUES (?, ?, ?, 1)
		ON CONFLICT (store_id, kind, day)
		DO UPDATE SET last_seq = document_number_seq.last_seq + 1
		RETURNING last_seq
	`, storeID, kind, dayOnly).Scan(&lastSeq).Error
	if err != nil {
		return 0, fmt.Errorf("next document number: %w", err)
	}
	return lastSeq, nil
}
```

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/number.go
git commit -m "feat(marketplace-api): atomic NextDocumentNumber helper"
```

---

### Task 11: Unit test for `NextDocumentNumber` — happy path

**Files:**
- Create: `services/marketplace-api/internal/order/number_test.go`

- [ ] **Step 1: Write the test**

```go
package order_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestNextDocumentNumber_HappyPath(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	storeID := uuid.New()
	day := time.Now()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for i := 1; i <= 5; i++ {
			n, err := order.NextDocumentNumber(ctx, tx, storeID, "order", day)
			require.NoError(t, err)
			require.Equal(t, i, n)
		}
		return nil
	}))
}

func TestNextDocumentNumber_KindIsolated(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	storeID := uuid.New()
	day := time.Now()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		o1, _ := order.NextDocumentNumber(ctx, tx, storeID, "order", day)
		r1, _ := order.NextDocumentNumber(ctx, tx, storeID, "return", day)
		o2, _ := order.NextDocumentNumber(ctx, tx, storeID, "order", day)
		require.Equal(t, 1, o1)
		require.Equal(t, 1, r1)
		require.Equal(t, 2, o2)
		return nil
	}))
}

func TestNextDocumentNumber_DayRollover(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	storeID := uuid.New()
	day1 := time.Now()
	day2 := day1.AddDate(0, 0, 1)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		a, _ := order.NextDocumentNumber(ctx, tx, storeID, "order", day1)
		b, _ := order.NextDocumentNumber(ctx, tx, storeID, "order", day2)
		require.Equal(t, 1, a)
		require.Equal(t, 1, b)
		return nil
	}))
}

func TestNextDocumentNumber_InvalidKindRejected(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	_, err := order.NextDocumentNumber(ctx, db, uuid.New(), "invoice", time.Now())
	require.Error(t, err)
}
```

- [ ] **Step 2: Run**

```bash
cd services/marketplace-api && go test -run TestNextDocumentNumber -v ./internal/order/
```
Expected: all four PASS.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/number_test.go
git commit -m "test(marketplace-api): NextDocumentNumber happy-path and isolation"
```

---

### Task 12: Concurrent benchmark gate — the M1 exit criterion

**Files:**
- Modify: `services/marketplace-api/internal/order/number_test.go`

This is the hard gate. If it fails, M1 does not ship and the sequencing strategy is reworked.

- [ ] **Step 1: Write the concurrent test**

Append to `number_test.go`:
```go
func TestNextDocumentNumber_Concurrent_NoDuplicates_And_P99Gate(t *testing.T) {
	const (
		goroutines = 50
		perG       = 20
		p99Gate    = 50 * time.Millisecond
	)

	ctx := context.Background()
	db := testdb.New(t)
	storeID := uuid.New()
	day := time.Now()

	// Each goroutine runs its own mini-transaction that inserts an order row +
	// its items + events, simulating the full create-order tx weight. This
	// measures the sequence strategy under realistic hold-time, not a microbench.

	type result struct {
		num     int
		latency time.Duration
		err     error
	}
	ch := make(chan result, goroutines*perG)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				start := time.Now()
				var seq int
				err := db.Transaction(func(tx *gorm.DB) error {
					var err error
					seq, err = order.NextDocumentNumber(ctx, tx, storeID, "order", day)
					if err != nil {
						return err
					}
					// Simulate the rest of the create flow: insert an orders row,
					// two order_items rows, two addresses, one event.
					o := order.Order{
						TenantID:       uuid.New(),
						StoreID:        storeID,
						OrderNumber:    fmt.Sprintf("M-BENCH-%06d", seq),
						IdempotencyKey: "bench-" + uuid.NewString(),
						CustomerEmail:  "buyer@example.com",
						Subtotal:       decimal.NewFromInt(100),
						GrandTotal:     decimal.NewFromInt(100),
						CurrencyCode:   "EUR",
						PlacedAt:       time.Now(),
					}
					if err := tx.Create(&o).Error; err != nil {
						return err
					}
					for j := 0; j < 2; j++ {
						if err := tx.Create(&order.OrderItem{
							OrderID: o.ID, TitleSnapshot: "x", SKUSnapshot: "x",
							UnitPrice: decimal.NewFromInt(50), Quantity: 1,
							LineTotal: decimal.NewFromInt(50), CurrencyCode: "EUR",
						}).Error; err != nil {
							return err
						}
					}
					for _, kind := range []string{"shipping", "billing"} {
						if err := tx.Create(&order.OrderAddress{
							OrderID: o.ID, Kind: kind, Name: "A", Line1: "1", City: "C", CountryCode: "IE",
						}).Error; err != nil {
							return err
						}
					}
					return tx.Create(&order.OrderEvent{
						OrderID: o.ID, Kind: "status_changed",
						Payload: datatypes.JSON([]byte(`{"to":"pending"}`)),
					}).Error
				})
				ch <- result{num: seq, latency: time.Since(start), err: err}
			}
		}()
	}

	wg.Wait()
	close(ch)

	seen := make(map[int]bool)
	latencies := make([]float64, 0, goroutines*perG)
	for r := range ch {
		require.NoError(t, r.err)
		require.False(t, seen[r.num], "duplicate sequence: %d", r.num)
		seen[r.num] = true
		latencies = append(latencies, float64(r.latency))
	}
	require.Len(t, seen, goroutines*perG)

	sort.Float64s(latencies)
	p99 := time.Duration(latencies[int(float64(len(latencies))*0.99)])
	t.Logf("p50=%v p90=%v p99=%v", time.Duration(latencies[len(latencies)/2]),
		time.Duration(latencies[int(float64(len(latencies))*0.9)]), p99)
	require.Less(t, p99, p99Gate,
		"M1 EXIT GATE FAILED: p99 create-tx latency %v exceeds %v. Rework sequencing strategy per spec §11 risks.", p99, p99Gate)
}
```

Note: imports need `sort`, `sync`, `fmt`, `gorm.io/gorm` — add them to the `import` block.

- [ ] **Step 2: Run the gate**

```bash
cd services/marketplace-api && go test -run TestNextDocumentNumber_Concurrent -v ./internal/order/
```
Expected: PASS with a p50/p90/p99 log line. **Note:** on a developer machine with a local Postgres, p99 should be well under 50ms — CI will be the authoritative reading on a db-f1-micro-equivalent container.

**If this test fails:** do not fix the test. Instead, **STOP** and escalate. The sequencing strategy needs to be reworked via one of the §11 fallbacks (per-store Postgres sequence created at store onboarding, or Redis counter). Any such pivot is a spec-level decision and must be surfaced to the human, not patched in-plan.

- [ ] **Step 3: Repeat the run 5 times locally to check stability**

```bash
for i in 1 2 3 4 5; do \
  cd services/marketplace-api && go test -run TestNextDocumentNumber_Concurrent -v ./internal/order/ 2>&1 | grep 'p99='; \
done
```
Expected: five lines, all showing p99 comfortably under 50ms. If one of the five fails, investigate whether it's noise (e.g. laptop thermal throttle) or a real regression.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/order/number_test.go
git commit -m "test(marketplace-api): concurrent document seq gate (50 goroutines, p99 <50ms)"
```

---

### Task 13: M1 exit checklist + handoff to M2

**Files:**
- none (verification + documentation)

- [ ] **Step 1: Run the full orders test suite**

```bash
cd services/marketplace-api && go test -v ./internal/order/...
```
Expected: all tests from Tasks 5–12 PASS. Note the p99 from the gate test.

- [ ] **Step 2: Run the full marketplace-api test suite to confirm no regression**

```bash
cd services/marketplace-api && go test ./...
```
Expected: all tests (products + orders) PASS.

- [ ] **Step 3: Run `golangci-lint` if the project uses it**

```bash
cd services/marketplace-api && golangci-lint run ./... || true
```
Expected: no new issues in `internal/order/` or the migration.

- [ ] **Step 4: Verify the migration `up → down → up` cycle one more time**

```bash
cd services/marketplace-api && \
  go run ./cmd/migrate -direction down && \
  go run ./cmd/migrate -direction up
```
Expected: both exit 0, `marketplace_db_schema_migrations` back at version 2.

- [ ] **Step 5: Tick the M1 exit criteria from spec §9**

Confirm each item literally:
- [x] `up → down → up` works — Task 4 + Step 4 above
- [x] `document_number_seq` produces unique numbers under 50-goroutine concurrent load — Task 12
- [x] p99 full create-tx latency under 50ms — Task 12
- [x] Full order-graph round-trip integration test passes — Task 8
- [x] Tenant/store constraint tests pass — Task 9
- [x] Hard-delete-with-returns is blocked — Task 9
- [x] `status = 'refunded'` is rejected — Task 9
- [x] `refunded_amount > grand_total` is rejected — Task 9

- [ ] **Step 6: Write a short M1 handoff note**

Create (or append to) `services/marketplace-api/internal/order/README.md`:
```markdown
# marketplace-api · order module

## Status

- **M1 (schema + models + atomic sequence):** landed ← you are here
- M2 (state machine + services + outbox drainer): pending
- M3 (FGA): pending
- M4 (HTTP + DTOs + API tests): pending
- M5 (checkout integration + observability): pending

## M1 benchmark result

- 50 goroutines × 20 ops each (1,000 orders)
- p99 full create-tx latency: **[paste the number from Task 12 step 2]**
- Gate: <50ms → PASS

## Invariants — do not break

See `doc.go`. In short:
- Never add FKs to `order_items.product_id` / `variant_id`.
- Never call `UPDATE orders SET status = ...` outside `order.Service.TransitionStatus` (implemented in M2).
- Never refund via anything other than the atomic `UPDATE ... WHERE refunded_amount + $new <= grand_total` pattern (implemented in M2).
- Never call `SELECT ... FOR UPDATE` on `document_number_seq`; only use `order.NextDocumentNumber`.
- Never hard-delete an order — soft delete only.
```

- [ ] **Step 7: Commit the handoff note**

```bash
git add services/marketplace-api/internal/order/README.md
git commit -m "docs(marketplace-api): M1 handoff note for orders module"
```

---

## Parallelization notes (for `subagent-driven-development`)

All tasks in this plan are strictly serial. Task 12 (the benchmark gate) depends on Tasks 1–11 in order, and there is no productive way to parallelize schema work. Do not dispatch parallel subagents for this plan.

## Estimated effort

Small-to-medium Go engineer who has zero context for this codebase but moderate Postgres + GORM fluency: **one focused day**. The long tail is:
- Figuring out the `testdb.New` signature from products slice 1 (Task 5/8)
- Debugging the `CHECK` constraint tests if the GORM driver swallows the Postgres error class (Task 9)
- Interpreting the first benchmark failure if it happens (Task 12 — escalate rather than tune)

## Exit gate to M2

Do not start Orders M2 until all of the following are true:

1. Every task in this plan is committed.
2. The benchmark test in Task 12 passes **in CI**, not just locally.
3. The benchmark result (p99 number) is written into `internal/order/README.md` and into the Orders M2 plan's "inherited constraints" section.
4. A human has reviewed the p99 result and signed off. If CI p99 is above ~20ms (well below the 50ms gate but worth noticing) flag it in the review — the headroom matters for cluster-scale later.

If any item is false, M2 does not start. If the benchmark fails in CI, follow the §11 fallback path and revise the spec before writing Orders M2.
