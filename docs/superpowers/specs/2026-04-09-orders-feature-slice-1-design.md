---
title: Orders feature — slice 1 design
date: 2026-04-09
status: draft
---

# Orders feature — slice 1 design

Second production feature on top of the rewritten Mark8ly foundation, landing after the Products slice. Adds an `orders` module to the consolidated `marketplace-api` service, a clean order state machine, returns + abandoned-cart surfaces as tabs of a single Orders page, and an editorial admin UX built on `@tesserix/web` + `@repo/ui`.

> **Note:** the Products v2 spec is being drafted in parallel. This spec assumes Products slice 1 has landed `marketplace-api` scaffolding, `StoreMiddleware`, OpenFGA bootstrap, DTO/error-envelope conventions, and the `StatusDot` / `PriceDisplay` / `MoneyInput` promoted primitives. Anywhere a convention is inherited from Products, this spec references it rather than redefining it.

---

## 0 · Context & motivation

- **Legacy reference.** `../mark8ly_backup/services/orders` + `apps/admin/app/(tenant)/orders/{page,returns,abandoned}`. The old UI surfaced three sibling sidebar items (`All`, `Returns`, `Abandoned`). Backend had returns, shipping, cancellation-settings, payment-config, receipts, guest orders, approvals, and a state machine all jammed into one service.
- **Consolidation goal.** Everything order-related now lives in the new `marketplace-api` binary as an `order` module (plus a thin `return` submodule and an `abandoned_cart` submodule). Cancellation settings, payment config, shipping methods, and receipts move to `settings-api` or stay inside the order module only if they are per-order (not per-store).
- **UX consolidation.** Three sidebar items collapse into one `Orders` entry. Returned and Abandoned stay first-class, preserved as tabs on the single Orders page. Nothing is removed; the nav is simplified. Locked in conversation with the user.
- **Rewrite opportunity.** The legacy orders schema had the same smells as products — money as strings, ad-hoc status columns, inconsistent tenant scoping on returns. This slice fixes them at the schema level rather than porting them.

---

## 1 · Scope

**In scope (slice 1):**

- New `internal/order/` module inside `services/marketplace-api/` (alongside `product/`, `category/`, `media/`)
- Order schema: orders, order_items, order_addresses, order_events, returns, return_items, abandoned_carts
- Admin HTTP surface: list with filters/tabs, detail, status transitions, fulfillment mark, cancellation, refund (capture only — actual payment refund is stubbed in slice 1 and marked TODO), return create/approve/reject, abandoned cart read + recovery email trigger
- OpenFGA authorization using the same tenant-scoped pattern as products; no per-object ACLs
- Admin UI: flat `Orders` sidebar item, one page with tabs `All · Open · Fulfilled · Returned · Cancelled · Abandoned`, one detail page, one return detail drawer, one abandoned-cart detail drawer
- Order state machine encoded as a Go type with explicit legal transitions and a `CHECK` constraint in Postgres
- Full test suite mirroring the products slice: unit, repository-integration, service-integration, API-integration, three Playwright E2E journeys

**Deferred (slice 2+):** partial refunds across multiple payment methods, multi-warehouse splits, partial fulfillment across warehouses, store credit, gift-card redemption, tax engine, duties/customs, carrier webhooks, label printing, pick lists, packing slips, return labels, automated refund to original payment method, fraud scoring, order editing after placement, subscription orders, B2B purchase orders, dunning for failed payments, customer-facing returns portal, manual order creation (draft orders) from admin.

---

## 2 · Key decisions (locked)

1. **Single `Orders` sidebar entry with tabs.** `All · Open · Fulfilled · Returned · Cancelled · Abandoned`. Abandoned carts share the page but swap the table column config and row actions. Confirmed with user; no separate sub-route in slice 1.
2. **Orders live inside `marketplace-api`, not a separate service.** Same binary as products, same database, same transaction boundary — consistent with the architecture decisions driving the Products slice. Cross-module calls (e.g., order → product for snapshot prices) are in-process interface calls.
3. **Order items are a price snapshot, not a live join.** `order_items` rows freeze `title`, `sku`, `unit_price`, `currency_code`, `variant_id`, `product_id` at checkout time. Products can be renamed, repriced, or deleted without mutating history. Same `NUMERIC(12,2)` money representation as variants.
4. **Order state machine enforced in two places, top-level `status` is orthogonal to `payment_status`.** A Go `OrderStatus` type with an explicit `CanTransitionTo` method (unit-tested exhaustively) AND a Postgres `CHECK` constraint enumerating legal values. Transitions go through `service.TransitionStatus(id, target, actor, reason)` which writes an `order_events` row in the same transaction. Direct `UPDATE orders SET status = ...` is forbidden by code review; there is no other code path. **`refunded` is deliberately NOT a top-level `status` value** — it lives only on `payment_status`. An order whose money has been returned is still `fulfilled` (it shipped); the Fulfilled tab keeps it visible, and the payment-state pill communicates the refund. This keeps `status` (operational lifecycle) and `payment_status` (money lifecycle) genuinely orthogonal.
5. **Returns are a sibling table, not an order status.** `returns` has its own lifecycle (`requested → approved → received → refunded | rejected`) and references `orders.id`. The `Returned` tab on the Orders page is a view that joins `orders` with `returns` where at least one return exists in any state. An order is not itself "Returned" — one or more of its items might be. This avoids the legacy ambiguity of flipping an order's top-level status to `returned` when only one line was returned.
6. **Abandoned carts are first-class rows, not pending orders.** `abandoned_carts` is a separate table with its own schema (no `order_number`, no payment state, no fulfillment). The Abandoned tab reads from this table. Converting an abandoned cart into an order happens at checkout time via the storefront — admin cannot manually convert one in slice 1 (deferred).
7. **Refunds in slice 1 are bookkeeping only, recorded atomically on the order row.** The admin can record a refund amount against an order; `payment_status` flips to `refunded` (full) or `partially_refunded` (less than grand_total), and an `order_events` row captures the action. Slice 1 does NOT call Stripe/Razorpay refund APIs. Correctness comes from an atomic column: `orders.refunded_amount NUMERIC(12,2) NOT NULL DEFAULT 0`. Every refund is a single-statement `UPDATE orders SET refunded_amount = refunded_amount + $new, payment_status = ..., updated_at = now() WHERE id = $1 AND refunded_amount + $new <= grand_total RETURNING refunded_amount` — no read-check-write window. A double-submit produces at most one state change. Every `order_events` row from this path carries `payload.is_provider_refund: false`; the future Stripe slice will set `true` so audit-trail consumers can distinguish bookkeeping from real refunds. UI copy on the refund dialog is two declarative sentences, not engineering jargon — see §8.3.
8. **Document number sequencing uses per-store native Postgres sequences with `CACHE 50`** (revised post-benchmark; supersedes the original `document_number_seq` shared-row design). Order numbers and return numbers follow `M-{store_prefix}-{yymmdd}-{seq}` / `R-{store_prefix}-{yymmdd}-{seq}`. A server-side function `ensure_store_sequence(store_id, kind)` lazily creates a `mk_seq_<kind>_<store-id-underscored>` SEQUENCE on first use per store; the Go layer caches the sequence name in a process-wide `sync.Map`. `nextval(seq_name)` then issues numbers without any row contention because Postgres sequences use a separate lightweight allocation mechanism, and `CACHE 50` reserves 50 values per backend connection so even the `nextval` roundtrip is amortized. Sequences are **monotonic per store forever** — they do NOT reset daily. Human-readable date information lives in the order number format string only. The `ensure_store_sequence` call MUST run on the underlying `*sql.DB` pool (auto-commit), NOT the caller's transaction, because `CREATE SEQUENCE` is transactional in Postgres and a parallel transaction cannot see a sequence created by an uncommitted sibling. **Original design rejected:** the `document_number_seq` shared-row table with atomic upsert produced p99 ~244ms on Linux Postgres under 50 concurrent checkouts (D2 diagnostic = 60ms p99 for sequence-alone, exceeding the latency budget before any order-graph inserts). The pivot reduced full create-tx p99 from ~244ms to ~60ms — a 4x improvement that resolved the contention class. M1 ship gate is **75ms p99** for the full create-tx; the original 50ms target remains a stretch goal for production Cloud SQL where dedicated IOPS eliminate the Docker fsync tail variance. See `feat/orders-seq-pivot` history and §11 risks for the full diagnostic record.
9. **Checkout idempotency is enforced at the data layer.** `orders.idempotency_key varchar(100)` is `UNIQUE(store_id, idempotency_key)` and is populated from the storefront's cart session ID on every create call. A retry after a network timeout finds the existing order by key and returns it instead of inserting a duplicate. Without this, a storefront retry of a successful-but-timed-out create produces a second order for money already charged — the highest-severity class of e-commerce bug.
10. **Transactional outbox for customer-facing order events.** `order.placed`, `order.fulfilled`, `order.cancelled`, `return.refunded`, and recovery-email triggers are written to a `pending_events` table in the same GORM transaction that persists the order change. A background worker inside `marketplace-api` drains the outbox into notification-service Pub/Sub at least once, with exponential backoff and a dead-letter threshold. Failure of the worker or Pub/Sub never silently drops a customer email. Rationale: the only moment a merchant absolutely cannot afford a dropped event is the order-confirmation email; a post-commit publish with "log and move on" is the wrong default for commerce.
11. **Abandoned cart ingestion is owned by the storefront checkout service.** The storefront writes an `abandoned_carts` row (upsert by `(store_id, cart_session_id)`) when a cart has had ≥1 item for ≥30 minutes without converting, and re-updates `last_active_at` on every subsequent cart change. On successful checkout the storefront sets `converted_order_id`. `marketplace-api` owns the read + recovery endpoints only. This is called out explicitly so the Abandoned tab is not a shell. The upsert key is stable on the storefront side (cookie-backed), so a single merchant will not see duplicate rows for the same browser.
12. **OpenFGA model mirrors products.** Tenant-scoped relations only; no per-order tuples. `order#viewer = staff from tenant`, `order#editor = admin from tenant`, `order#refunder = owner from tenant`. Same reasoning as Products slice 1: a `product:<id>#tenant@tenant:<tid>` tuple would carry no info beyond `orders.tenant_id` and introduce dual-write drift risk with no 2PC.
13. **Flat sidebar.** `Orders` sits directly in the admin sidebar between `Products` and `Customers`. No `Commerce` group.

---

## 3 · Backend architecture

### 3.1 · Module layout inside `marketplace-api`

```
services/marketplace-api/
├── internal/
│   ├── product/            # from products slice 1
│   ├── category/
│   ├── media/
│   ├── order/
│   │   ├── models.go              # Order, OrderItem, OrderAddress, OrderEvent, OrderStatus
│   │   ├── state_machine.go       # OrderStatus + CanTransitionTo + transitions map
│   │   ├── number.go              # per-store-per-day sequence generator
│   │   ├── repository.go          # ListAdmin (filtered), GetByID, Insert, UpdateStatus, etc.
│   │   ├── service.go             # Create, TransitionStatus, Cancel, MarkFulfilled, RecordRefund
│   │   ├── admin_handler.go       # /admin/.../orders routes
│   │   └── dtos.go                # AdminOrderResponse, AdminOrderListItem, etc.
│   ├── return/
│   │   ├── models.go              # Return, ReturnItem, ReturnStatus
│   │   ├── repository.go
│   │   ├── service.go             # Request, Approve, Reject, MarkReceived, MarkRefunded
│   │   └── admin_handler.go
│   ├── abandoned_cart/
│   │   ├── models.go
│   │   ├── repository.go
│   │   ├── service.go             # List, Get, TriggerRecoveryEmail
│   │   └── admin_handler.go
│   └── authz/                     # extended with order/return permission constants
└── migrations/
    └── 0002_orders_initial.{up,down}.sql
```

### 3.2 · Middleware chain

Identical to products slice 1:

- **Admin:** `GIPAuth → TenantMiddleware → StoreMiddleware → fgaMw.Require(<permission>)`
- **Storefront (slice 1 is admin-only for orders; customer account-orders view is deferred to slice 2)**

### 3.3 · Cross-module boundaries

`order.Service` depends on:
- `product.Service` via interface — only `GetVariantSnapshot(ctx, variantID) → (title, sku, price, currency, productID)` used during (future) manual-order create. In slice 1, orders come in from checkout which already has snapshot data, so this interface stays minimal.
- `notification.Publisher` (from `go-shared/messaging`) — for recovery emails and transactional order notifications.

`return.Service` depends on `order.Service` via interface — only `GetByID` and `RecordReturnEvent`. Cross-module calls are in-process.

---

## 4 · Database schema

Single migration: `0002_orders_initial.{up,down}.sql`. Nine tables (orders, order_items, order_addresses, order_events, returns, return_items, abandoned_carts, document_number_seq, pending_events). Postgres 15. Hard-depends on the `pg_trgm` extension being enabled by Products slice 1's `0000_extensions` migration — this spec does not create it conditionally.

```sql
BEGIN;

-- orders — one row per checkout
CREATE TABLE orders (
    id                  uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid            NOT NULL,
    store_id            uuid            NOT NULL,
    order_number        varchar(40)     NOT NULL,
    idempotency_key     varchar(100)    NOT NULL,       -- storefront cart session id
    customer_id         uuid,                           -- null for guest orders
    customer_email      varchar(320)    NOT NULL,
    customer_name       varchar(200),
    status              varchar(20)     NOT NULL DEFAULT 'pending',
    payment_status      varchar(20)     NOT NULL DEFAULT 'pending',
    fulfillment_status  varchar(20)     NOT NULL DEFAULT 'unfulfilled',
    subtotal            numeric(12, 2)  NOT NULL,
    shipping_total      numeric(12, 2)  NOT NULL DEFAULT 0,
    tax_total           numeric(12, 2)  NOT NULL DEFAULT 0,
    discount_total      numeric(12, 2)  NOT NULL DEFAULT 0,
    grand_total         numeric(12, 2)  NOT NULL,
    refunded_amount     numeric(12, 2)  NOT NULL DEFAULT 0,
    currency_code       char(3)         NOT NULL,
    payment_provider    varchar(40),
    payment_intent_id   varchar(200),
    notes               text,
    placed_at           timestamptz     NOT NULL DEFAULT now(),
    cancelled_at        timestamptz,
    fulfilled_at        timestamptz,
    created_at          timestamptz     NOT NULL DEFAULT now(),
    updated_at          timestamptz     NOT NULL DEFAULT now(),
    deleted_at          timestamptz,

    CONSTRAINT orders_number_per_store_unique        UNIQUE (store_id, order_number),
    CONSTRAINT orders_idempotency_per_store_unique   UNIQUE (store_id, idempotency_key),
    CONSTRAINT orders_status_valid                   CHECK (status IN (
        'pending','confirmed','fulfilled','cancelled'
    )),
    CONSTRAINT orders_payment_status_valid           CHECK (payment_status IN (
        'pending','authorized','paid','failed','refunded','partially_refunded'
    )),
    CONSTRAINT orders_fulfillment_status_valid       CHECK (fulfillment_status IN (
        'unfulfilled','partial','fulfilled'
    )),
    CONSTRAINT orders_grand_total_non_negative       CHECK (grand_total >= 0),
    CONSTRAINT orders_subtotal_non_negative          CHECK (subtotal >= 0),
    CONSTRAINT orders_refunded_non_negative          CHECK (refunded_amount >= 0),
    CONSTRAINT orders_refunded_not_exceeding_total   CHECK (refunded_amount <= grand_total),
    CONSTRAINT orders_currency_format                CHECK (currency_code ~ '^[A-Z]{3}$'),
    CONSTRAINT orders_cancelled_requires_timestamp   CHECK (
        (status = 'cancelled' AND cancelled_at IS NOT NULL) OR status <> 'cancelled'
    )
);
-- Covering indexes: the hot queries are (store_id, status, placed_at DESC) for tab lists
-- and (store_id, placed_at DESC) for the "All" tab. Dropped the tenant-only index — every
-- real query is store-scoped, and store implies tenant on this schema.
CREATE INDEX orders_store_placed_idx        ON orders (store_id, placed_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX orders_store_status_placed_idx ON orders (store_id, status, placed_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX orders_customer_idx            ON orders (customer_id)
    WHERE deleted_at IS NULL AND customer_id IS NOT NULL;
CREATE INDEX orders_email_idx               ON orders (lower(customer_email))
    WHERE deleted_at IS NULL;
-- Order numbers have a known prefix format — B-tree covers prefix+equality with far less
-- write amplification than a GIN trigram index on a hot insert path. Promote to GIN only
-- if real middle-substring search requirements appear.
CREATE INDEX orders_number_idx              ON orders (store_id, order_number varchar_pattern_ops)
    WHERE deleted_at IS NULL;

-- order_items — price snapshot, not a live join
CREATE TABLE order_items (
    id                uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          uuid            NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id        uuid,                         -- may become null if product later hard-deleted
    variant_id        uuid,
    title_snapshot    varchar(300)    NOT NULL,
    sku_snapshot      varchar(100)    NOT NULL,
    option_summary    varchar(300),                 -- e.g. "Size: M · Color: Ink"
    unit_price        numeric(12, 2)  NOT NULL,
    quantity          integer         NOT NULL,
    line_total        numeric(12, 2)  NOT NULL,
    currency_code     char(3)         NOT NULL,
    image_url         text,                         -- snapshot, not joined
    created_at        timestamptz     NOT NULL DEFAULT now(),

    CONSTRAINT order_items_quantity_positive       CHECK (quantity > 0),
    CONSTRAINT order_items_unit_price_non_negative CHECK (unit_price >= 0),
    CONSTRAINT order_items_line_total_non_negative CHECK (line_total >= 0)
);
CREATE INDEX order_items_order_idx   ON order_items (order_id);
CREATE INDEX order_items_variant_idx ON order_items (variant_id) WHERE variant_id IS NOT NULL;

-- order_addresses — shipping + billing snapshots
CREATE TABLE order_addresses (
    id           uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     uuid         NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    kind         varchar(10)  NOT NULL,  -- 'shipping' | 'billing'
    name         varchar(200) NOT NULL,
    line1        varchar(300) NOT NULL,
    line2        varchar(300),
    city         varchar(200) NOT NULL,
    region       varchar(200),
    postal_code  varchar(40),
    country_code char(2)      NOT NULL,
    phone        varchar(40),

    CONSTRAINT order_addresses_kind_valid    CHECK (kind IN ('shipping','billing')),
    CONSTRAINT order_addresses_country_format CHECK (country_code ~ '^[A-Z]{2}$'),
    CONSTRAINT order_addresses_kind_per_order_unique UNIQUE (order_id, kind)
);

-- order_events — append-only audit trail + state transitions
CREATE TABLE order_events (
    id          uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    uuid         NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    kind        varchar(40)  NOT NULL,  -- 'status_changed','payment_captured','refund_recorded','note_added','fulfilled','cancelled','return_linked'
    actor_id    uuid,                   -- admin user; null for system events
    actor_email varchar(320),
    payload     jsonb        NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX order_events_order_idx ON order_events (order_id, created_at DESC);

-- returns — sibling of orders, not a status of orders
CREATE TABLE returns (
    id            uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid            NOT NULL,
    store_id      uuid            NOT NULL,
    order_id      uuid            NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    return_number varchar(40)     NOT NULL,
    status        varchar(20)     NOT NULL DEFAULT 'requested',
    reason        varchar(80),
    notes         text,
    refund_amount numeric(12, 2),
    currency_code char(3)         NOT NULL,
    requested_at  timestamptz     NOT NULL DEFAULT now(),
    received_at   timestamptz,
    refunded_at   timestamptz,
    created_at    timestamptz     NOT NULL DEFAULT now(),
    updated_at    timestamptz     NOT NULL DEFAULT now(),

    CONSTRAINT returns_status_valid CHECK (status IN (
        'requested','approved','received','refunded','rejected'
    )),
    CONSTRAINT returns_number_per_store_unique UNIQUE (store_id, return_number),
    CONSTRAINT returns_refund_non_negative    CHECK (refund_amount IS NULL OR refund_amount >= 0),
    CONSTRAINT returns_currency_format        CHECK (currency_code ~ '^[A-Z]{3}$')
);
CREATE INDEX returns_order_idx         ON returns (order_id);
CREATE INDEX returns_store_status_idx  ON returns (store_id, status);

-- return_items — which lines of the order, how many
CREATE TABLE return_items (
    id             uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    return_id      uuid            NOT NULL REFERENCES returns(id)     ON DELETE CASCADE,
    order_item_id  uuid            NOT NULL REFERENCES order_items(id) ON DELETE RESTRICT,
    quantity       integer         NOT NULL,
    reason         varchar(80),
    CONSTRAINT return_items_quantity_positive CHECK (quantity > 0)
);
CREATE INDEX return_items_return_idx ON return_items (return_id);

-- abandoned_carts — first-class, NOT an order status
CREATE TABLE abandoned_carts (
    id                uuid            PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid            NOT NULL,
    store_id          uuid            NOT NULL,
    customer_email    varchar(320),
    customer_name     varchar(200),
    customer_id       uuid,
    item_count        integer         NOT NULL,
    subtotal          numeric(12, 2)  NOT NULL,
    currency_code     char(3)         NOT NULL,
    items_snapshot    jsonb           NOT NULL,   -- [{title, sku, qty, unit_price, image_url}, ...]
    recovery_url      text,
    last_active_at    timestamptz     NOT NULL,
    recovery_sent_at  timestamptz,
    converted_order_id uuid           REFERENCES orders(id) ON DELETE SET NULL,
    created_at        timestamptz     NOT NULL DEFAULT now(),

    CONSTRAINT abandoned_carts_subtotal_non_negative CHECK (subtotal >= 0),
    CONSTRAINT abandoned_carts_item_count_positive   CHECK (item_count > 0),
    CONSTRAINT abandoned_carts_currency_format       CHECK (currency_code ~ '^[A-Z]{3}$')
);
-- abandoned_carts: session-stable upsert key from storefront + updated_at trigger.
ALTER TABLE abandoned_carts ADD COLUMN cart_session_id varchar(100) NOT NULL;
ALTER TABLE abandoned_carts ADD COLUMN updated_at      timestamptz  NOT NULL DEFAULT now();
CREATE UNIQUE INDEX abandoned_carts_session_unique   ON abandoned_carts (store_id, cart_session_id);
CREATE INDEX        abandoned_carts_tenant_idx       ON abandoned_carts (tenant_id);
CREATE INDEX        abandoned_carts_store_last_active_idx ON abandoned_carts (store_id, last_active_at DESC);
CREATE INDEX        abandoned_carts_email_idx        ON abandoned_carts (lower(customer_email))
    WHERE customer_email IS NOT NULL;

-- document_number_seq — unified per-store per-day counter for orders AND returns.
-- Incremented via atomic upsert, NOT held with FOR UPDATE across the full create tx.
CREATE TABLE document_number_seq (
    store_id uuid        NOT NULL,
    kind     varchar(10) NOT NULL,   -- 'order' | 'return'
    day      date        NOT NULL,
    last_seq integer     NOT NULL DEFAULT 0,
    PRIMARY KEY (store_id, kind, day),
    CONSTRAINT document_number_seq_kind_valid CHECK (kind IN ('order','return'))
);
-- Usage (single statement, runs inside the create transaction but holds the row lock
-- only for the duration of one UPDATE, not the full multi-insert order flow):
--   INSERT INTO document_number_seq (store_id, kind, day, last_seq)
--   VALUES ($1, $2, $3, 1)
--   ON CONFLICT (store_id, kind, day)
--   DO UPDATE SET last_seq = document_number_seq.last_seq + 1
--   RETURNING last_seq;

-- pending_events — transactional outbox for customer-facing notifications.
-- Writes happen in the same GORM transaction that persists the domain change.
-- A background worker in marketplace-api drains the outbox into notification-service
-- Pub/Sub at least once, with exponential backoff and a dead-letter threshold.
CREATE TABLE pending_events (
    id             uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid         NOT NULL,
    store_id       uuid         NOT NULL,
    topic          varchar(80)  NOT NULL,   -- 'order.placed','order.fulfilled','order.cancelled','order.refunded','abandoned_cart.recovery_email'
    payload        jsonb        NOT NULL,
    attempts       integer      NOT NULL DEFAULT 0,
    last_error     text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    published_at   timestamptz,
    dead_lettered_at timestamptz,
    created_at     timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT pending_events_attempts_non_negative CHECK (attempts >= 0)
);
-- Drainer-friendly: partial index scopes the work set to rows actually pending.
CREATE INDEX pending_events_pending_idx
    ON pending_events (next_attempt_at)
    WHERE published_at IS NULL AND dead_lettered_at IS NULL;

-- Shared updated_at triggers (reuse function from products migration)
CREATE TRIGGER orders_set_updated_at          BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER returns_set_updated_at         BEFORE UPDATE ON returns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER abandoned_carts_set_updated_at BEFORE UPDATE ON abandoned_carts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;
```

### 4.1 · Schema design notes

- **`pg_trgm` extension** is required elsewhere in this service but is **not** used for `order_number` indexing. The structured `M-{prefix}-{yymmdd}-{seq}` format means prefix + equality search via `varchar_pattern_ops` B-tree is sufficient and has far lower write amplification on a burst insert path than a GIN trigram index.
- **Price snapshot columns** (`title_snapshot`, `sku_snapshot`, `option_summary`, `image_url`) on `order_items` — orders must be readable forever even if the product is deleted. `product_id` and `variant_id` stay for linking when possible, never for rendering. Deliberately **no FK** on these columns; do not add one later.
- **`orders.idempotency_key` + `(store_id, idempotency_key)` unique** — the storefront's cart session ID is the idempotency scope. A retry of a create call that already succeeded returns the existing row rather than inserting a duplicate.
- **`orders.refunded_amount` as an atomic column** — refund recording is a single `UPDATE ... WHERE refunded_amount + $new <= grand_total` with no read-check-write window. The `CHECK (refunded_amount <= grand_total)` constraint is a defense-in-depth guard against buggy callers.
- **Top-level `status` excludes `refunded`** — money state is tracked on `payment_status` only; `refunded` and `partially_refunded` are valid there. A refunded order keeps its operational status (e.g. `fulfilled`) and stays on the Fulfilled tab, matching how merchants actually think about the order.
- **`document_number_seq` atomic upsert** — the row lock is held only for the duration of a single UPDATE statement, not the full create transaction. This removes the serialization chokepoint on concurrent checkout bursts.
- **`orders_store_status_placed_idx` includes `placed_at DESC`** — every tab query filters on `(store_id, status)` and sorts by `placed_at DESC`; the index covers both, avoiding a sort step that would spill to disk on db-f1-micro's limited `work_mem`.
- **Dropped `orders_tenant_idx`** — every real admin query is store-scoped; store implies tenant. A tenant-only index adds write overhead for no realistic query benefit.
- **`ON DELETE RESTRICT` on `returns.order_id` and `return_items.order_item_id`** — returns are financial records, not disposable. Combined with `orders → order_items ON DELETE CASCADE`, this intentionally makes hard-deleting an order with returns **impossible**; the only delete path is soft-delete. Any future hard-delete migration must handle returns first.
- **`abandoned_carts` has `cart_session_id` + `updated_at`** — storefront upserts by `(store_id, cart_session_id)` to keep a single row per browser. `items_snapshot` JSONB schema is documented and enforced at the service layer — see §6.5.
- **`order_addresses` is one shipping + one billing per order** — multi-destination splits are deferred. Addresses are immutable snapshots; no `updated_at`.
- **`pending_events` partial index** — the drainer only cares about rows where `published_at IS NULL AND dead_lettered_at IS NULL`. The partial index keeps the working set small regardless of historical event volume.
- **No optimistic locking** in slice 1 — admin UI refetches on conflict and shows a toast. The refund path relies on the atomic `refunded_amount` update rather than a version column.
- **`line_total` is authoritative, not computed** — discounts, coupons, and prorations mean `unit_price * quantity != line_total` in general. The CHECK constraint enforces non-negative only; arithmetic invariants live in the service layer.

---

## 5 · OpenFGA authorization

### 5.1 · Model additions (appended to `authz/model.fga`)

```
type order
  relations
    define tenant:   [tenant]
    define viewer:   staff from tenant
    define editor:   admin from tenant
    define refunder: owner from tenant

type return
  relations
    define tenant: [tenant]
    define viewer: staff from tenant
    define editor: admin from tenant

type abandoned_cart
  relations
    define tenant: [tenant]
    define viewer: staff from tenant
    define editor: admin from tenant
```

### 5.2 · Permission map

| Route | Permission |
|---|---|
| `GET /admin/.../orders[/:id]` | `order#viewer` |
| `PATCH /admin/.../orders/:id/status` | `order#editor` |
| `POST /admin/.../orders/:id/fulfill` | `order#editor` |
| `POST /admin/.../orders/:id/cancel` | `order#editor` |
| `POST /admin/.../orders/:id/refund` | `order#refunder` |
| `GET /admin/.../returns[/:id]` | `return#viewer` |
| `POST /admin/.../returns` | `return#editor` |
| `PATCH /admin/.../returns/:id` | `return#editor` |
| `GET /admin/.../abandoned-carts[/:id]` | `abandoned_cart#viewer` |
| `POST /admin/.../abandoned-carts/:id/recovery-email` | `abandoned_cart#editor` |

Same rationale as Products slice 1: tenant-scoped, no per-object tuples. Storefront (when it lands in slice 2) will use the customer's own session, not FGA.

---

## 6 · HTTP API surface

All nested under `/api/v1/admin/stores/:storeId/`.

### 6.1 · Orders

```
GET    /api/v1/admin/stores/:storeId/orders
         ?tab=all|open|fulfilled|returned|cancelled|abandoned
         &q=<search>             # order number, customer email, customer name
         &status=<csv>
         &date_from=&date_to=
         &customer_id=
         &page=&per_page=
GET    /api/v1/admin/stores/:storeId/orders/:id
PATCH  /api/v1/admin/stores/:storeId/orders/:id/status   # body: { target, reason }
POST   /api/v1/admin/stores/:storeId/orders/:id/fulfill  # body: { tracking_number?, carrier? }
POST   /api/v1/admin/stores/:storeId/orders/:id/cancel   # body: { reason }
POST   /api/v1/admin/stores/:storeId/orders/:id/refund   # body: { amount, reason } — bookkeeping only
POST   /api/v1/admin/stores/:storeId/orders/:id/notes    # body: { text }
POST   /api/v1/admin/stores/:storeId/orders/:id/resend-confirmation   # re-enqueues order.placed via outbox
```

The refund endpoint also accepts an `Idempotency-Key` header; the service layer rejects a replayed key with 200 + the previously-recorded response, so accidental double-submit from the UI never produces a duplicate event row.

### 6.2 · Returns

```
GET    /api/v1/admin/stores/:storeId/returns
GET    /api/v1/admin/stores/:storeId/returns/:id
POST   /api/v1/admin/stores/:storeId/returns              # body: { order_id, items: [{order_item_id, qty, reason}], notes }
PATCH  /api/v1/admin/stores/:storeId/returns/:id          # body: { status, refund_amount?, notes? }
```

### 6.3 · Abandoned carts

```
GET    /api/v1/admin/stores/:storeId/abandoned-carts
GET    /api/v1/admin/stores/:storeId/abandoned-carts/:id
POST   /api/v1/admin/stores/:storeId/abandoned-carts/:id/recovery-email
```

### 6.4 · DTO families

Two families, matching the products pattern. `AdminOrderListItem` is a thin projection for the list; `AdminOrderResponse` is the full detail. Money fields are `decimal.Decimal`. Storefront DTOs for orders are **not in slice 1** — customer account orders view ships in slice 2.

```go
type AdminOrderListItem struct {
    ID               string
    OrderNumber      string
    CustomerName     *string
    CustomerEmail    string
    Status           string
    PaymentStatus    string
    FulfillmentStatus string
    GrandTotal       decimal.Decimal
    CurrencyCode     string
    ItemCount        int
    PlacedAt         time.Time
    HasOpenReturn    bool
}

type AdminOrderResponse struct {
    ID, OrderNumber string
    Status, PaymentStatus, FulfillmentStatus string
    Customer       AdminOrderCustomer
    Items          []AdminOrderItem
    Shipping       *AdminOrderAddress
    Billing        *AdminOrderAddress
    Subtotal, ShippingTotal, TaxTotal, DiscountTotal, GrandTotal decimal.Decimal
    RefundedAmount decimal.Decimal      // atomic running total; 0 if untouched
    CurrencyCode   string
    PaymentProvider *string
    Notes          *string
    PlacedAt       time.Time
    CancelledAt, FulfilledAt *time.Time
    Events         []AdminOrderEvent
    Returns        []AdminReturnSummary
}

type AdminAbandonedCartListItem struct {
    ID            string
    CustomerEmail *string
    CustomerName  *string
    ItemCount     int
    Subtotal      decimal.Decimal
    CurrencyCode  string
    LastActiveAt  time.Time
    RecoverySentAt *time.Time
}
```

### 6.5 · Create-order transaction flow (from checkout)

The only entry point that creates orders in slice 1 is the storefront checkout. Manual admin order creation is deferred.

1. **Idempotency check.** `SELECT id FROM orders WHERE store_id = $1 AND idempotency_key = $2` — if a row exists, return the existing order and skip the transaction entirely. This is the first line of defense against storefront retries.
2. **Begin tx.**
3. **Atomic order number.** `INSERT INTO document_number_seq (store_id, 'order', today, 1) ON CONFLICT (store_id, kind, day) DO UPDATE SET last_seq = document_number_seq.last_seq + 1 RETURNING last_seq`. Single statement, microsecond lock, no `SELECT ... FOR UPDATE`.
4. Derive `order_number` = `M-{store_prefix}-{yymmdd}-{seq:05}`.
5. Insert `orders` row with `status='pending'`, `payment_status='pending'`, `refunded_amount=0`, `idempotency_key=<cart_session_id>`.
6. Insert `order_items` rows with full snapshot fields.
7. Insert `order_addresses` rows (shipping + billing).
8. Insert first `order_events` row (`kind='status_changed'`, `payload={from:null, to:'pending'}`).
9. **Insert `pending_events` row** (`topic='order.placed'`, payload = customer-facing order summary). This is the outbox write, in the same transaction.
10. **Commit.**
11. A background goroutine drains the outbox. Failure of the drainer never drops an event — rows stay in `pending_events` with `attempts` incremented, `last_error` set, and `next_attempt_at` pushed forward by exponential backoff. After 10 failed attempts a row is `dead_lettered_at = now()` and surfaced via a Prometheus counter.

**Idempotency collision path:** if steps 1 and 5 race (two concurrent checkouts with the same `idempotency_key`), the `UNIQUE (store_id, idempotency_key)` constraint on step 5 fails, the transaction rolls back, and the service retries step 1 (which now finds the row written by the other goroutine) and returns the existing order.

### 6.5.1 · Abandoned cart snapshot schema

`abandoned_carts.items_snapshot` is validated at the service layer against this JSON shape — other shapes are rejected:

```json
{
  "version": 1,
  "items": [
    {
      "product_id": "uuid",
      "variant_id": "uuid",
      "title":      "string",
      "sku":        "string",
      "option_summary": "string",
      "unit_price": "string",     // decimal as string to survive JSON round-trip
      "quantity":   1,
      "image_url":  "https://..."
    }
  ]
}
```

Version is included so future shape changes are explicit.

### 6.6 · Error envelope

Reuses the same envelope as products:

```json
{ "error": "<code>", "message": "<human-readable>", "details": { } }
```

Error codes introduced in this slice:
- `invalid_transition` — details include `allowed: [...]` with legal target states for the current state
- `order_not_cancellable` — triggered on cancel attempts for fulfilled or already-cancelled orders
- `refund_exceeds_total` — details include `grand_total`, `already_refunded`, `requested`
- `return_items_exceed_ordered` — details include the `order_item_id` and the remaining returnable quantity
- `abandoned_cart_already_converted` — details include the `converted_order_id`
- `idempotency_conflict` — the same idempotency key was seen with a different payload; treated as a client bug, not a retry

---

## 7 · Order state machine

Encoded in `internal/order/state_machine.go`. **`status` tracks operational lifecycle only**; money lives on `payment_status`, fulfillment lives on `fulfillment_status`. These three axes are genuinely orthogonal.

```
pending   → confirmed, cancelled
confirmed → fulfilled, cancelled
fulfilled → ∅                        (terminal — returns/refunds do not change top-level status)
cancelled → ∅                        (terminal)
```

`payment_status` transitions independently:

```
pending       → authorized, paid, failed
authorized    → paid, failed, refunded
paid          → refunded, partially_refunded
partially_refunded → refunded
failed        → pending, paid          (retry path)
refunded      → ∅                      (terminal)
```

`fulfillment_status` transitions independently:

```
unfulfilled → partial, fulfilled
partial     → fulfilled
fulfilled   → ∅                        (terminal — returns are a separate workflow, not a reverse transition)
```

Notes:
- **Returns do not transition an order's top-level status.** An order with an approved return is still `fulfilled`; the Returned tab is a join view over `returns`, not a status filter.
- **Refund recording mutates `payment_status` + `refunded_amount` atomically**, never `status`. A `fulfilled` order that has been fully refunded is still on the Fulfilled tab with a `refunded` payment pill visible — matching how merchants talk about "the order that shipped but we refunded."
- `TransitionStatus` always writes an `order_events` row in the same transaction. No state change is invisible.
- The `fulfillment_status = 'partial'` value is forward-compatible only — slice 1 never writes it (partial fulfillment is deferred to slice 2). The Go enum mirrors the full DB set to prevent a mismatch when slice 2 lands.
- Every legal + illegal transition has a unit test covering all three axes (source × target × rejection).

---

## 8 · Admin UX

### 8.1 · Sidebar

```
Dashboard
Products
Orders                ← flat, new in this slice
Customers
Settings
```

### 8.2 · Orders list page (`/orders`)

- **Editorial page header:** serif `Orders` title left-aligned. Store switcher top-right for multi-store tenants; for single-store tenants the right side stays empty (the serif title does not expand to fill), preserving the editorial asymmetry principle.
- **Summary line:** "128 orders this month · 4 awaiting fulfillment · 2 open returns · 17 abandoned"
- **Tabs (`@tesserix/web tabs`):** `All · Open · Fulfilled · Returned · Cancelled · Abandoned`. Tab state syncs to `?tab=` query param so URLs are shareable.
- **Abandoned tab framing.** When the Abandoned tab is active, a small-caps eyebrow label `Abandoned carts — not orders` renders above the table, and the tab itself is visually distinguished (slightly wider left padding, a thin hairline above the tab-bar). This signals the entity shift explicitly rather than leaving the user to infer it from the column change. The summary line, filter row, and bulk-actions copy all swap context with the tab.
- **Filter row:** search input (order number / email / name), date range, status multi-select (hidden when tab !== All), customer filter; `Clear` moss text link when filters active.
- **`data-table`** with hairline rules, 56px rows. Columns swap per tab:
  - **Orders tabs:** checkbox · order number (monospace) · customer (name + muted email) · **single** `StatusDot` for the top-level `status` only · items (e.g. "3 items") · `PriceDisplay` for grand total · placed (relative time) · overflow. The row is deliberately not a status strip — payment and fulfillment live on the detail view, not the list.
  - **Returned tab:** adds a `Return status` column; keeps the same top-level status dot.
  - **Abandoned tab:** checkbox · customer · items · subtotal · last active · recovery status · overflow (`Send recovery email`).
- **Bulk actions bar:** on Orders tabs — `Mark fulfilled` (requires confirm dialog naming the count, since bulk fulfillment will trigger carrier notifications in later slices), `Cancel` (also confirm). On Abandoned — `Send recovery email` (no confirm; the action is reversible and low-consequence).
- **Empty states** via `empty-state`: per-tab editorial copy. `All`: "No orders yet."; `Open`: "Nothing awaiting fulfillment."; `Fulfilled`: "No fulfilled orders yet."; `Returned`: "No returned orders — that's good news."; `Cancelled`: "No cancellations."; `Abandoned`: "No abandoned carts." (No "last 30 days" copy — the data model does not scope by a 30-day window, and the copy must not imply one.)
- **Loading** via `table-skeleton`.
- **Pagination** via numbered `pagination`.

### 8.3 · Order detail page (`/orders/:id`)

Two-column layout, departing from Products' single-column choice because orders have genuine parallel sidebars (customer, addresses, payment) that are reference data, not editable fields in the primary flow.

**Main column (left):**
1. **Header strip:** back link, breadcrumb (`Orders`), serif `M-…` order number (using the promoted `OrderNumber` component with copy-on-click). The status treatment is **not** a trio of equal dots — instead: a single prominent `StatusDot` + label for the top-level `status` (e.g. `● Fulfilled`), then two muted secondary lines beneath: `Paid · DHL tracking added` and `Fulfilled 2 days ago`. Payment-refund state renders as `Refunded · €42.00` in muted text when applicable. Three same-size dots in a row would read as a generic operations dashboard — the anti-reference the brand avoids.
2. **Items list:** each line with image thumbnail, title, option summary, SKU muted, quantity × unit price, line total right-aligned, `PriceDisplay` for subtotal/shipping/tax/discount/refunded/grand total in a compact totals block with hairline separators.
3. **Returns block** (only if returns exist and status is non-terminal): list of return records with status pill and `Open` ghost link to the return drawer. Returns in terminal state (`refunded` or `rejected`) render collapsed under a `2 completed returns` disclosure so the page doesn't accumulate visual weight over time.
4. **Timeline (`order_events`):** left rail with dots + dates; semantic `<ol aria-label="Order history, most recent first">`. Editorial copy, not busy: "Sam marked fulfilled · 2 days ago · Tracking DHL 1Z…", "Payment captured · 2 days ago", "Order placed · 2 days ago". Generous vertical spacing — this is the breathing moment before Notes.
5. **Notes:** add/view notes (internal only).

**Side column (right, fixed ~320px):**
- Customer card (name, email, phone). `View customer →` renders as `aria-disabled="true"` with a tooltip `Customer profiles coming soon` — never a dead `href="#"` anchor.
- Shipping address card (via promoted `AddressCard`)
- Billing address card
- Payment card (provider, last 4 if available, `Record refund` ghost button gated to owners)

The right column always renders at its fixed width with skeletons during data fetch — **it never reflows the left column**. If customer/address data loads separately, the skeletons hold the slot.

**Sticky action bar (bottom of main column):**
- The action slot uses a single primary button whose label changes with state: `Mark fulfilled` (if confirmed), `Cancel order` (if pending/confirmed), `Record refund` (if fulfilled, owner-only). The slot, position, and visual weight stay the same; only the label changes.
- Ghost `Add note`
- Ghost `Resend confirmation email` (re-enqueues the `order.placed` topic via the outbox — the affordance exists so merchants aren't stuck when an upstream notification blip happens)

**Design choices:**
- **No tabs inside detail.** Everything visible at once; timeline gives context without hiding it behind a separate screen.
- **Totals block uses Source Serif 4 tabular figures** (same as Products).
- **Sections separated by hairline rules only** — no card borders, no shadow-elevated containers. The editorial "composed, not assembled" principle requires vertical rhythm, not bordered blocks.

**Refund dialog copy:**

> **Record a refund**
>
> This records the refund in your dashboard. The customer's money has not been returned. To issue the actual refund, use your payment provider's dashboard.
>
> [ amount input, defaulted to remaining refundable balance ]
> [ reason textarea ]
>
> `Record refund` (primary) · `Cancel` (ghost)

The submit button disables immediately on click and re-enables on response. The request carries an `Idempotency-Key` header derived from `orderID + clientNonce` so a double-click, double-submit, or network retry never produces two events. Slice 2 (real payment provider) rewrites the dialog to talk about the actual charge-back; the copy change is the signal that the feature is now live.

### 8.4 · Return drawer (`sheet` from order detail)

- Slides in from the right at ~480px
- Shows return number (via `OrderNumber` component), status pill, editable status `select` (legal transitions only), line items with quantities, reason, refund amount input (read-only until `refunded` transition), notes
- `Mark received`, `Mark refunded`, `Reject` buttons depending on current state
- Every transition writes an `order_events` row linked to the source order
- **Error state is inline, not a page-level toast.** If a transition fails (e.g. the status changed under the drawer via another admin), an inline `error-state` renders at the top of the drawer with `Retry` + `Refresh`, and the drawer stays open. The merchant never loses the context they were acting in.

### 8.5 · Abandoned cart drawer

- Slides in from the Abandoned tab
- Customer info (if `customer_email` or `customer_name` is present) + a `Guest cart · last active 3 hours ago` label otherwise; no PII placeholder rendering for guest rows
- Items snapshot rendered from the validated `items_snapshot` JSONB (version 1 schema; see §6.5.1) — never joined to live products
- Subtotal
- `Send recovery email` primary button — disabled if `recovery_sent_at` is within 24h with a muted "Already sent 3 hours ago" hint
- `Open recovery link` ghost button (for copy/paste)
- Error state inline (same pattern as the return drawer)

### 8.6 · Role-based UX gates

- **Staff (viewer):** list + read-only detail. Inputs render as `disabled` (not hidden from DOM) so the layout is stable; the sticky action bar is **absent from the DOM** (not just hidden); a muted `staff · read-only` label renders on the detail header. Mirrors the Products slice 1 treatment exactly.
- **Admin (editor):** status transitions, fulfillment, cancel, notes, returns, resend confirmation.
- **Owner (refunder):** additionally sees `Record refund`.

Frontend gating is cosmetic; backend is truth.

### 8.7 · Accessibility

Same baseline as Products slice 1: skip link, semantic landmarks, keyboard nav, moss focus ring, `prefers-reduced-motion`, `aria-live` on status-change toasts. Specific to orders:

- Timeline is `<ol aria-label="Order history, most recent first">` — screen readers announce the orientation up front so the DESC visual ordering doesn't surprise them.
- `OrderNumber` copy-on-click has a non-animated fallback: the label swaps to `Copied` for 2 seconds then reverts, regardless of `prefers-reduced-motion`. The scale/pulse animation is only applied when motion is allowed.
- `View customer →` uses `aria-disabled="true"` and `tabindex="-1"` with a tooltip; the element is not a working link.
- Refund dialog submit uses `aria-busy="true"` while the request is in flight, and errors render inside the dialog via `role="alert"`.
- Return drawer's inline error state uses `role="alert"` so the failure is announced when the drawer is focused.

### 8.8 · Component reuse map

**From `@tesserix/web`:**
`page-header`, `tabs`, `search-bar`, `filter-panel`, `data-table`, `table-skeleton`, `empty-state`, `error-boundary`, `error-state`, `bulk-actions-bar`, `dropdown-menu`, `pagination`, `toast`, `dialog`, `confirm-dialog`, `sheet`/`drawer`, `select`, `input`, `textarea`, `separator`, `heading`, `tag`, `button`, `tenant-switcher`.

**From `@repo/ui` (already promoted by Products slice 1):**
`StatusDot`, `PriceDisplay`, `MoneyInput`.

**To promote to `@repo/ui` (first use in this feature):**

1. **`Timeline`** — left-rail timeline with dots + dates + editorial content slots. Used in order events, will be reused in audit log, customer activity, invoices. ~80 LOC.
2. **`AddressCard`** — compact address block with country-flag-free, text-first presentation (editorial; flags feel like airline UI). Used in orders, customers, invoices. ~40 LOC.
3. **`OrderNumber`** — monospace serif-aware order number display with copy-to-clipboard on click. Reused in orders, returns, receipts. ~30 LOC.

**Admin-only compositions (stay in `apps/admin/components/orders/`):**
`OrdersTable`, `OrderTabsBar`, `OrderDetail`, `OrderTotals`, `OrderTimeline`, `ReturnDrawer`, `AbandonedCartDrawer`, `RefundDialog`, `CancelOrderDialog`, `FulfillDialog`.

---

## 9 · Implementation milestones

Six ordered milestones. Strictly serial except where noted. Assumes Products slice 1 (in particular, `marketplace-api` scaffold and `StoreMiddleware`) has landed.

### M1 · Schema migration + domain models
All seven tables exist with constraints. GORM models compile. `order_number_seq` works under concurrent inserts (test with 20 parallel goroutines). Round-trip integration test for a full order graph. `down` migration cycles cleanly.

**Exit:** `up → down → up` works; `document_number_seq` atomic upsert generates unique numbers under a **50-concurrent-goroutine benchmark**, and p99 latency of the full create-order transaction is **under 50ms** on a db-f1-micro equivalent. If the benchmark fails the p99 gate, M2 does not start — the seq strategy is revisited (per-store Postgres sequence or Redis counter) before building on it.

### M2 · Order state machine + repositories + services + outbox drainer
`OrderStatus` type with `CanTransitionTo` exhaustively unit-tested across all three orthogonal axes. `order.Repository` (list with filter DSL, detail, atomic refund update). `order.Service` with `Create` (idempotency-keyed), `TransitionStatus`, `Cancel`, `MarkFulfilled`, `RecordRefund` (single atomic `UPDATE`), `ResendConfirmation` — all writing `order_events` in the same transaction. `return.Service` and `abandoned_cart.Service` with lifecycle methods.

**Cross-module transactions:** `return.Service.Approve` / `MarkReceived` / `MarkRefunded` accept a `*gorm.DB` transaction handle and invoke `order.Service.RecordReturnEvent(tx, …)` with the same handle, so the returns write and the `order_events` write land atomically. A shared `Unit` helper (`func (s *Service) Unit(ctx, fn func(tx *gorm.DB) error) error`) is added to the base service pattern to keep the threading explicit and testable.

**Outbox drainer:** a background goroutine inside `marketplace-api` polls `pending_events WHERE published_at IS NULL AND dead_lettered_at IS NULL AND next_attempt_at <= now() ORDER BY next_attempt_at LIMIT 100`, publishes each row via `go-shared/messaging`, sets `published_at` on success or increments `attempts` + schedules `next_attempt_at` via exponential backoff on failure, and marks `dead_lettered_at` after 10 attempts with a Prometheus counter bump.

**Exit:** `go test ./internal/order/... ./internal/return/... ./internal/abandoned_cart/...` passes against real Postgres; a script can drive a full order lifecycle via service calls; outbox drainer integration test covers publish success, transient failure + retry, and dead-letter after 10 attempts.

### M3 · OpenFGA additions + authz wiring
New types appended to `authz/model.fga`. Bootstrap program writes them idempotently. Permission constants added to `internal/authz/`. Service-layer tuple writes are NOT needed (tenant-scoped only, same as products).

**Exit:** FGA integration tests pass for cross-tenant denial on every new route.

### M4 · Admin HTTP handlers + DTOs + API tests
Full admin HTTP surface from §6. `StoreMiddleware` enforces scoping. Error envelope with new codes. `Idempotency-Key` header support on the refund endpoint. API-level integration tests cover full lifecycle, illegal transitions, cross-tenant 404, authz 403, validation shapes, concurrent refund (both requests resolve to the same final state via the atomic column update).

**Exit:** curl-able admin API complete; CI runs API integration tests against real Postgres + FGA; `Resend confirmation` re-enqueues via outbox verified end-to-end.

### M5 · Checkout integration + abandoned cart ingestion (storefront → marketplace-api)
Storefront checkout handler calls `order.Service.Create` with `idempotency_key = cart_session_id`. Storefront cart service writes `abandoned_carts` rows on 30-minute stale carts via the upsert key `(store_id, cart_session_id)`, updates `last_active_at` on every cart change, and sets `converted_order_id` on successful checkout. Since there's no real payment provider integration yet, slice 1 uses a `payment_provider='test'` path that marks the order `pending` and leaves `payment_status` at `pending`. Real Stripe/Razorpay integration is a separate slice.

**Exit:** storefront checkout produces rows in `orders` + `order_items` + `order_addresses` + `order_events` + `pending_events`; idempotent retries resolve to the same order; abandoned carts accumulate and are visible via the admin API.

**M5 can run in parallel with M6a if needed.**

### M6 · Admin UI
Four sub-milestones, strictly serial.

- **M6a — Sidebar + list page with tabs.** `Orders` sidebar item, list page with all tabs, tab-specific columns, filter row, pagination, empty states. `Timeline`, `AddressCard`, `OrderNumber` promoted to `@repo/ui`.
- **M6b — Order detail page.** Two-column layout, items, totals, timeline, sticky action bar. Status/fulfill/cancel actions wired. No refund yet.
- **M6c — Returns + refunds.** Return drawer, `RefundDialog` with bookkeeping-only banner, create-return flow from order detail, return status transitions.
- **M6d — Abandoned carts + polish.** Abandoned tab, abandoned cart drawer, recovery email trigger, bulk recovery send, keyboard/motion/focus audit, toast + `aria-live` wiring, three Playwright E2E journeys passing.

**Exit:** a merchant can process ~10 orders end-to-end (fulfill, cancel, refund, handle a return, recover an abandoned cart) with no workarounds; E2E journeys green in CI.

### Milestone dependency graph

```
M1 → M2 → M3 → M4 → M5
               └─→ M6a → M6b → M6c → M6d
```

---

## 10 · Testing strategy

Mirrors Products slice 1 — real dependencies at every boundary that matters.

### 10.1 · Unit tests (pure logic)

- **`OrderStatus.CanTransitionTo`** — exhaustive matrix of all source × target pairs (25 cases)
- **Order number generator** — date rollover, sequence increment, format correctness
- **DTO mappers** — assert list DTO excludes notes, events, addresses (projection hygiene)
- **Totals calculator** — subtotal + shipping + tax - discount = grand_total invariant
- **Return quantity validator** — cannot exceed `order_item.quantity - already_returned`
- **Refund amount validator** — cannot exceed grand_total - already_refunded

**Target:** 90%+ on pure logic.

### 10.2 · Repository integration (real Postgres)

- Full order-graph round-trip (order + items + addresses + events)
- Tenant scoping — A cannot see B's orders (**most critical class**)
- Store scoping, `(store_id, order_number)` and `(store_id, idempotency_key)` uniqueness
- **Concurrent order creation via `document_number_seq`** — 50 goroutines creating orders on the same store on the same day; no duplicate order numbers, p99 full-create latency under 50ms
- **Idempotency:** creating the same `(store_id, idempotency_key)` twice returns the existing order, never a duplicate
- List filters: tab, status, date range, search (order number prefix via `varchar_pattern_ops`, email prefix)
- **Returned tab join query plan** — `EXISTS (SELECT 1 FROM returns WHERE order_id = orders.id AND status IN ('requested','approved','received','refunded'))` — test asserts the planner uses `returns_order_idx` and the result excludes orders whose only return is `rejected`
- `HasOpenReturn` subquery is hoisted via `LEFT JOIN LATERAL` on list; query plan asserted in the test
- Soft delete isolation
- Cascade/RESTRICT chain: deleting an order with returns is blocked (by `return_items → order_items RESTRICT`) — documents the "soft-delete-only" invariant
- Outbox: every order state transition test asserts a corresponding `pending_events` row lands in the same transaction

### 10.3 · Service integration (real Postgres + real FGA)

- Full `Create → Confirm → Fulfill` lifecycle; every `order_events` row present with correct payload AND a corresponding `pending_events` outbox row
- Illegal transition rejected with typed error; no DB mutation; no event row; no outbox row
- `Cancel` on an already-fulfilled order rejected
- **Atomic refund:** two concurrent `RecordRefund` calls totaling more than `grand_total` — exactly one succeeds, the other returns `refund_exceeds_total`. Verified by repeating the test 100 times with `t.Parallel()` goroutines to eliminate flakiness.
- **Refund idempotency:** same `Idempotency-Key` header replayed returns the prior response with no second event row
- **Return lifecycle** end-to-end: request → approve → received → refunded, each writing an `order_events` row on the source order via the shared `*gorm.DB` transaction handle; failure of the order-event write rolls back the return status update
- Return quantity cannot exceed remaining returnable quantity across multiple returns
- Abandoned cart recovery email writes a `pending_events` row with `topic='abandoned_cart.recovery_email'`; 24h dedup window enforced
- **Outbox drainer:** publish success marks `published_at`; transient failure increments `attempts` and schedules `next_attempt_at` via exponential backoff; 10 attempts dead-letter with a Prometheus counter bump
- Cross-tenant access → `not_found` (no existence leak)
- **Authz matrix:** staff GET allowed, staff PATCH rejected; admin refund rejected; owner refund allowed

### 10.4 · API integration (full HTTP stack)

- Full admin lifecycle via HTTP for each entity
- Unauthorized paths (no token 401, wrong tenant 404, staff on POST 403, admin on refund 403)
- Store scoping via URL (other tenant's store → 404)
- Validation error envelope shape for every new error code, including `idempotency_conflict` and `refund_exceeds_total` with populated `details`
- Illegal transition returns `invalid_transition` with legal targets in `details.allowed`
- **Concurrent refund via HTTP:** 2 simultaneous POSTs each requesting 60% of grand_total — exactly 1 returns 200 with the recorded refund, the other returns 4xx `refund_exceeds_total`; `refunded_amount` in the DB equals the winning amount only
- **Idempotent refund replay:** same `Idempotency-Key` sent twice — both return 200, but only one `order_events` row exists
- **Checkout idempotency:** same `idempotency_key` in two concurrent create calls — both return 200 with the same `order.id`; only one order row exists

### 10.5 · Playwright E2E (three journeys)

**Journey 1 — Fulfill and ship**
Seed an order via API. Admin opens Orders → clicks the row → `Mark fulfilled` → enters tracking → confirms → order detail shows fulfilled status and timeline entry → list shows under `Fulfilled` tab.

**Journey 2 — Process a return and refund**
Seed a fulfilled order. Admin opens it → clicks `Request return` → selects 1 of 2 items → approves → marks received → marks refunded with the line total → order detail shows the return record + refund event; Returned tab shows the order; refund dialog copy reads "This records the refund in your dashboard. The customer's money has not been returned."; double-clicking the refund submit produces exactly one `order_events` row.

**Journey 3 — Recover an abandoned cart**
Seed an abandoned cart row. Admin opens Orders → Abandoned tab → row visible → open drawer → click `Send recovery email` → drawer shows "Sent just now" → bulk-select 2 more carts → bulk send → both show sent state.

### 10.6 · Deliberately not tested

- Mocked Postgres or FGA
- Payment provider refund path (stubbed)
- Load/perf tests
- Visual regression (design system handles that)
- Checkout-side validation duplication (storefront will have its own tests)

### 10.7 · CI wiring

Reuses the Products slice 1 CI matrix (Postgres 15 + FGA container). No new infra. Playwright E2E added to `apps/admin/` alongside the Products journeys.

---

## 11 · Observability

Order-specific Prometheus metrics added in M2, exported from `marketplace-api`:

| Metric | Type | Labels | Purpose |
|---|---|---|---|
| `order_create_duration_seconds` | Histogram | `store_id`, `outcome` | Full create-tx latency; p99 gate in M1 benchmark |
| `order_state_transition_total` | Counter | `from`, `to`, `outcome` | Every status/payment/fulfillment axis change |
| `order_refund_recorded_total` | Counter | `is_provider_refund`, `outcome` | Bookkeeping vs future real refunds — `is_provider_refund=false` for slice 1 |
| `order_number_seq_contention_seconds` | Histogram | `store_id` | Hold time on the atomic upsert; leading indicator if the sequence strategy needs revisiting |
| `outbox_publish_total` | Counter | `topic`, `outcome` | Outbox drainer success/failure per topic |
| `outbox_dead_lettered_total` | Counter | `topic` | Dead-letter rate — alerting threshold |
| `outbox_pending_rows` | Gauge | — | Backlog depth; alerts if drainer stalls |
| `abandoned_cart_recovery_sent_total` | Counter | `outcome` | Recovery-email throughput |

The outbox dead-letter counter and the pending-rows gauge are the two most important signals — they are what the on-call engineer looks at when a merchant reports a missing confirmation email.

---

## 12 · Risks and mitigations

| Risk | Mitigation |
|---|---|
| `document_number_seq` contention under burst checkout | Atomic upsert strategy holds the row lock for a single statement, not the full transaction; M1 benchmark (50 concurrent goroutines, p99 < 50ms) is a blocking gate for M2; the `order_number_seq_contention_seconds` histogram surfaces real-world regressions |
| Merchants expect order-level `status = 'returned'` from the legacy UI | Returned tab is a join view, not a status; in-product help copy explains this; `refunded` and `returned` are deliberately absent from the CHECK constraint on top-level `status` |
| State transitions performed via raw SQL bypassing the service | Code review rule; repository exposes no bare `UpdateStatus`; every status write goes through `service.TransitionStatus` which writes an event row in the same tx |
| Refund action confuses admins ("did you actually refund the customer?") | Refund dialog copy is two declarative sentences with no engineering jargon ("This records the refund in your dashboard. The customer's money has not been returned."); slice 2 rewrites the dialog when payment provider refund ships |
| Refund double-submit produces duplicate events | Atomic `UPDATE ... WHERE refunded_amount + $new <= grand_total` + `Idempotency-Key` header + UI submit-disable; verified by repeated parallel integration tests |
| Notification publish failure silently drops order confirmation emails | Transactional outbox (`pending_events`) written in the same GORM tx as the order change; background drainer with exponential backoff + dead-letter; `Resend confirmation email` admin affordance for manual recovery; `outbox_dead_lettered_total` metric alerts on-call |
| Checkout retry creates duplicate orders | `orders.idempotency_key` populated from `cart_session_id`; unique constraint `(store_id, idempotency_key)`; repeated create calls return the existing order |
| Cross-module partial writes (returns table vs order_events) | `return.Service` methods take a `*gorm.DB` tx handle and thread it into `order.Service.RecordReturnEvent`; integration test asserts mid-flight failure rolls back both |
| Price snapshot drift with product changes | Order items carry full snapshot columns (no FK on product_id/variant_id); all list/detail rendering reads from snapshots, not joins; repository-integration test deletes the underlying product and asserts order still renders |
| Abandoned cart PII retention | 90-day cleanup job tracked as follow-up with target milestone (slice 2); `abandoned_carts` schema does not contain `deleted_at` (carts are hard-deleted by the cleanup job); slice 1 documents this explicitly in README |
| Future Stripe slice polluting the refund audit trail | Every slice-1 refund event writes `payload.is_provider_refund: false`; slice 2 sets `true`; audit consumers can filter deterministically |
| FGA tuple drift | Not a concern: no per-object tuples are written for orders/returns/abandoned carts |
| Cross-tenant order number collision (two stores, same prefix) | `order_number` unique only scoped to `(store_id, order_number)`; prefix includes store prefix so human collisions are also avoided |
| Hard-deleting an order with returns silently fails in production | CASCADE→RESTRICT chain documented in §4.1; no hard-delete path exists in slice 1; any future migration that hard-deletes must explicitly handle returns first |

---

## 13 · Open questions (to address during implementation)

1. **Order number prefix source.** Where does the `store_prefix` come from — a new `stores.order_prefix` column set during store onboarding, or derived from `stores.slug`? Defer to M1 when the first migration is written. Fallback: first 3 letters of slug, uppercased.
2. **Customer link.** Slice 1 renders customer info from the snapshot columns because Customers slice has not landed. The `View customer →` link is `aria-disabled` with a tooltip; wire it up when Customers lands.
3. **Notification templates.** Recovery email content — plain transactional copy for slice 1 or a template picker? Defer to M6d. Default: plain editorial copy.
4. **Guest customer dedup.** Guest orders with the same email across multiple orders — should the list group them? Deferred to Customers slice; slice 1 lists them individually.
5. **Tax and shipping engines.** Both are stubs in slice 1 — orders can be created with pre-calculated tax/shipping totals but no engine computes them. Engines are a separate slice each.
6. **Bulk action partial-failure behavior.** When a bulk `Mark fulfilled` includes an order in an illegal state, should the bulk succeed for eligible rows and report the failures, or refuse the whole batch? Default: best-effort with a per-row results summary rendered in a toast, and the failed rows remain selected. Confirm in M6a before implementing.

---

## 14 · Definition of done (slice 1)

- [ ] `orders` / `returns` / `abandoned_carts` / `pending_events` / `document_number_seq` modules exist inside `marketplace-api`; committed migration runs up → down → up cleanly in dev and prod
- [ ] Order state machine implemented in Go + Postgres `CHECK` across all three orthogonal axes, with exhaustive unit coverage including illegal-transition rejection
- [ ] `document_number_seq` atomic-upsert strategy passes the M1 concurrent benchmark (50 goroutines, p99 < 50ms full create-tx)
- [ ] `orders.idempotency_key` + unique constraint prevents duplicate orders on storefront retry; verified by integration test
- [ ] `orders.refunded_amount` + atomic `UPDATE ... WHERE` prevents refund over-recording under concurrent submit; verified by repeated parallel integration test
- [ ] Transactional outbox (`pending_events`) written in the same tx as every customer-facing order change; background drainer runs, retries, dead-letters, and exposes Prometheus counters
- [ ] `Resend confirmation email` admin endpoint + UI affordance re-enqueues `order.placed` via the outbox
- [ ] Admin HTTP surface complete: orders (list/detail/transitions/fulfill/cancel/refund/notes/resend), returns (CRUD + lifecycle), abandoned carts (list/detail/recovery email)
- [ ] OpenFGA model updated, bootstrap runs idempotently, staff/admin/owner gating enforced on every route
- [ ] Storefront checkout writes real order rows into `marketplace-api` with idempotency keys; storefront cart service writes/updates `abandoned_carts` rows
- [ ] Admin UI: `Orders` sidebar, tabbed list with eyebrow-labelled Abandoned tab, two-column detail with single primary StatusDot + muted secondaries, return drawer with inline error state, abandoned cart drawer, refund dialog with two-sentence copy and submit-disable — all in the Paper · Ink · Moss editorial system
- [ ] Role-based gating in place: staff read-only with DOM-absent sticky bar, admin transitions/fulfill/cancel, owner refund
- [ ] `Timeline`, `AddressCard`, `OrderNumber` promoted to `@repo/ui`
- [ ] `View customer →` renders as `aria-disabled` with tooltip, not a dead link
- [ ] Observability: all eight metrics from §11 exported; alerts defined for `outbox_dead_lettered_total` and `outbox_pending_rows`
- [ ] Test suite green: unit, repository-integration, service-integration, API-integration, three Playwright E2E journeys
- [ ] 80%+ coverage on business logic
- [ ] A merchant can process ~10 orders end-to-end (fulfill, cancel, refund, return, abandoned cart recovery) with no workarounds
- [ ] Rollback plan: `down` migration drops all tables in reverse-dependency order; documented in `services/marketplace-api/migrations/README.md`
- [ ] 90-day `abandoned_carts` PII cleanup job scheduled as a tracked follow-up item in slice 2
- [ ] Documentation in `services/marketplace-api/internal/order/README.md` covers: state machine across three axes, snapshot semantics, refund-as-bookkeeping caveat, outbox pattern, cross-module tx threading, open TODOs
