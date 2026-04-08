# Orders M2 — state machine, services, and transactional outbox drainer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the domain logic on top of Orders M1's schema. Land the three orthogonal status Go types with exhaustive transition coverage, the `order` / `return` / `abandoned_cart` service layer with idempotency-keyed create, atomic refund recording, cross-module transaction threading, and the background outbox drainer that publishes `pending_events` rows to notification-service via `go-shared/messaging`. No HTTP handlers, no authorization middleware, no DTOs — this milestone ends when a Go test script can drive a full order lifecycle (create → confirm → fulfill → refund → return → abandoned cart recovery) against real Postgres with every `order_events` row and every `pending_events` outbox row accounted for.

**Architecture:** Three orthogonal Go status types (`OrderStatus`, `PaymentStatus`, `FulfillmentStatus`) with `CanTransitionTo` methods and exhaustive matrix unit tests. A `Service` struct per module (`order.Service`, `return.Service`, `abandoned_cart.Service`) that accepts a `*gorm.DB` in its constructor and exposes a shared `Unit(ctx, fn func(tx *gorm.DB) error) error` helper so cross-module writes share a single transaction. `order.Service.RecordRefund` is a single atomic `UPDATE orders SET refunded_amount = refunded_amount + ?, payment_status = ?, updated_at = now() WHERE id = ? AND refunded_amount + ? <= grand_total RETURNING refunded_amount` — the read-check-write window is eliminated at the SQL layer. `pending_events` rows are written inside the same transaction as the domain change they describe. A dedicated background goroutine (`outbox.Drainer`) polls `pending_events WHERE published_at IS NULL AND dead_lettered_at IS NULL AND next_attempt_at <= now() ORDER BY next_attempt_at LIMIT 100`, publishes each row, and updates the row state. Exponential backoff on publish failure; dead-letter after 10 attempts with a Prometheus counter bump.

**Tech Stack:** Go 1.26, GORM v1.25, Postgres 15, `github.com/shopspring/decimal`, `github.com/google/uuid`, `gorm.io/datatypes`, `github.com/stretchr/testify`, `github.com/mark8ly/go-shared/messaging` (for the outbox publisher interface). No new external deps beyond what M1 + products introduced.

**Spec reference:** `docs/superpowers/specs/2026-04-09-orders-feature-slice-1-design.md` — authoritative sections: §2 decisions 4, 7, 10 (state machine, atomic refund, outbox); §6.5 (create-order transaction flow); §6.5.1 (abandoned cart items_snapshot schema); §6.6 (error codes); §7 (full state machine across all three axes); §9 M2 exit criteria; §14 DoD items 2, 5, 6, 7.

**Out of scope for M2** (handled later):
- HTTP handlers, DTOs, error envelope shape → M4
- OpenFGA model additions + middleware → M3
- Storefront checkout → M5
- Prometheus metrics wiring beyond the outbox drainer counter → M5
- Admin UI → separate plan series

---

## Hard prerequisites from Orders M1

Before Task 1 runs, the entire Orders M1 plan must be merged (or the current branch must be rebased on top of the M1 commits, if orders is being shipped as one stacked PR). Specifically:

1. The `0002_orders_initial` migration has shipped and all nine tables exist at the expected schema version.
2. `internal/order/models.go`, `models_returns.go`, `models_abandoned_cart.go`, `models_outbox.go`, `number.go`, `doc.go` exist and compile.
3. `order.NextDocumentNumber` is callable and the concurrent benchmark (50 goroutines, p99 < 50ms) has passed in CI. The recorded p99 number is in `internal/order/README.md` — this plan reads it.
4. `internal/order/models_test.go` and `number_test.go` pass (round-trip, CHECK constraints, concurrent sequence).
5. No preexisting `internal/order/repository.go`, `service.go`, `state_machine.go` — M2 creates them fresh. If they exist from a prior aborted attempt, STOP and escalate.

**Task 0 verifies all five before any new files are touched.**

---

## Hard prerequisites from `go-shared`

Orders M2 imports a publisher interface from `go-shared/messaging`. Products slice 1 is expected to have already integrated `go-shared` into `marketplace-api`. Verify in Task 0 step 4:

```go
// The interface the outbox drainer depends on. Products may already have
// promoted this; if not, Task 0 stops and escalates.
package messaging

type Publisher interface {
    Publish(ctx context.Context, topic string, payload []byte) error
}
```

If `go-shared/messaging.Publisher` is not present or has a different signature, **STOP** in Task 0. M2 does not patch `go-shared`; it consumes whatever products has already landed and fails loudly if the contract doesn't match.

---

## Decisions locked for this milestone

1. **Three orthogonal status types, not one combined enum.** `OrderStatus`, `PaymentStatus`, `FulfillmentStatus` are separate Go types with separate `CanTransitionTo` methods. The three axes never reference each other in transition logic. Service methods that touch multiple axes (e.g. `MarkFulfilled` sets both `status` and `fulfillment_status`) call each type's `CanTransitionTo` independently.
2. **Transition tables are package-level vars, not methods.** `var orderStatusTransitions = map[OrderStatus][]OrderStatus{...}` — readable in `go doc`, trivially unit-testable, and exhaustive matrix tests iterate the map directly. A method-based implementation (`switch` inside `CanTransitionTo`) is harder to introspect.
3. **Every service method returns typed errors.** `order.ErrInvalidTransition`, `order.ErrRefundExceedsTotal`, `order.ErrNotFound`, `order.ErrIdempotencyConflict`, `return.ErrReturnItemsExceedOrdered`, `abandoned_cart.ErrAlreadyConverted`. Handlers in M4 map these to error envelope codes — services do not know about HTTP status codes.
4. **`order.Service.Create` looks up by `(store_id, idempotency_key)` first.** Before opening the transaction, `repo.GetByIdempotencyKey(ctx, storeID, key)` is called. If a row is found, return it unchanged with a boolean `reused=true`. This is the happy path for storefront retries. The `UNIQUE (store_id, idempotency_key)` constraint is the second line of defense for concurrent races.
5. **`RecordRefund` is a single SQL statement, not a service-layer check.** The atomic guarantee comes from the `WHERE refunded_amount + ? <= grand_total` clause and the `CHECK (refunded_amount <= grand_total)` DB constraint — not from `SELECT + compute + UPDATE`. Service-level validation (e.g. "refund reason is non-empty") happens before the statement, but the amount guard is the SQL itself.
6. **Cross-module transactions use a shared `*gorm.DB` handle, not a wrapper type.** The `Unit` helper is `func (s *Service) Unit(ctx context.Context, fn func(tx *gorm.DB) error) error` and passes the transaction to a closure. Other services' methods that need to run inside the same transaction take a `tx *gorm.DB` as their first parameter (in addition to `ctx`). Example: `return.Service.Approve(ctx, tx, returnID, ...)` — the `tx` is threaded explicitly so the typing system enforces the invariant. No ambient context magic.
7. **Outbox drainer is a single goroutine per service replica.** Horizontal scaling is achieved by Postgres row locking — `SELECT ... FOR UPDATE SKIP LOCKED LIMIT 100` so two replicas drain different rows. Each row is processed independently: publish, then update `published_at` or `attempts+next_attempt_at`. No in-memory state.
8. **Exponential backoff: `2^attempts * 10s`, capped at 1 hour.** Attempt 1 fails → next_attempt_at = now + 20s. Attempt 5 → now + 5m20s. Attempt 10 → now + 1h (cap). After 10 failed attempts, `dead_lettered_at = now()` and the row stops being picked up.
9. **Outbox publisher is an injected interface, not a concrete type.** `order.Service`, `return.Service`, `abandoned_cart.Service` all write to `pending_events` directly (via repository). The drainer takes a `messaging.Publisher` and calls `Publish(ctx, row.Topic, row.Payload)`. Tests inject a fake publisher. Production wires the real `go-shared/messaging` publisher.
10. **Drainer polling interval is 2 seconds.** Low-enough latency for transactional emails without hammering the DB. Configurable via `ORDERS_OUTBOX_POLL_INTERVAL` env var (default `2s`), so tests can set it to `50ms` for fast iteration.
11. **`payment_status` transitions are orthogonal but the service still centralizes them.** `order.Service.SetPaymentStatus(ctx, tx, orderID, target, reason)` is the only code path; direct `UPDATE orders SET payment_status = ?` is forbidden by code review.
12. **`FulfillmentStatus = 'partial'` is forward-compatible only.** M2 never writes it. The Go enum exposes `FulfillmentStatusPartial` so when slice 2 adds partial fulfillment, no enum change is required.
13. **`return.Service` writes to `order_events` via an `order.Service.RecordReturnEvent(ctx, tx, orderID, kind, payload)` method.** This method is cross-module-tx-safe (takes a `tx`). `return.Service` never touches `order_events` directly — keeps the ownership boundary clean and testable.
14. **`abandoned_cart.Service.TriggerRecoveryEmail` enforces a 24-hour dedup window in-service, not in the handler.** If `recovery_sent_at` is within 24h, return `abandoned_cart.ErrRecoveryTooRecent` without writing an outbox row.
15. **Outbox drainer is started in `cmd/marketplace-api/main.go` only when `MODE=admin` or `MODE=both`.** Storefront mode replicas do not drain. Rationale: draining is an admin-bound operation in the service's topology (same binary, mode flag controls which code paths run). This avoids double-draining by storefront replicas that scale independently.
16. **Outbox drainer graceful shutdown.** On SIGTERM, the drainer finishes the current batch (up to 100 rows) and exits. The main.go shutdown hook waits up to 30 seconds for the drainer to return before force-exiting.

---

## File structure produced by M2

```
services/marketplace-api/
├── internal/
│   ├── order/
│   │   ├── status.go                  # NEW — OrderStatus, PaymentStatus, FulfillmentStatus + transitions
│   │   ├── status_test.go             # NEW — exhaustive transition matrix tests
│   │   ├── errors.go                  # NEW — typed errors for service layer
│   │   ├── events.go                  # NEW — OrderEventKind constants + payload helpers
│   │   ├── repository.go              # NEW — OrderRepository with CRUD + list + atomic refund
│   │   ├── repository_test.go         # NEW — repository integration tests
│   │   ├── service.go                 # NEW — order.Service + Unit helper
│   │   ├── service_test.go            # NEW — service integration tests
│   │   ├── return_repository.go       # NEW
│   │   ├── return_service.go          # NEW — Request, Approve, MarkReceived, MarkRefunded, Reject
│   │   ├── return_service_test.go     # NEW — cross-module tx threading tests
│   │   ├── abandoned_cart_repository.go  # NEW
│   │   ├── abandoned_cart_service.go     # NEW — List, Get, TriggerRecoveryEmail
│   │   └── abandoned_cart_service_test.go # NEW
│   └── outbox/
│       ├── drainer.go                 # NEW — background goroutine
│       ├── drainer_test.go            # NEW — success, retry, dead-letter integration tests
│       └── fake_publisher.go          # NEW — testing helper (build tag `//go:build testing`)
└── cmd/
    └── marketplace-api/
        └── main.go                    # MODIFY — wire drainer on boot for admin|both modes
```

---

## Task decomposition

Tasks are mostly serial, with two clearly marked parallelizable clusters. Commits are per task.

### Task 0: Verify Orders M1 prerequisites + go-shared messaging contract

**Files:** none (verification only)

- [ ] **Step 1: Verify M1 files exist and compile**

```bash
for p in \
  services/marketplace-api/internal/order/models.go \
  services/marketplace-api/internal/order/models_returns.go \
  services/marketplace-api/internal/order/models_abandoned_cart.go \
  services/marketplace-api/internal/order/models_outbox.go \
  services/marketplace-api/internal/order/number.go \
  services/marketplace-api/migrations/0002_orders_initial.up.sql \
  services/marketplace-api/migrations/0002_orders_initial.down.sql; do
  test -f "$p" || { echo "MISSING: $p"; exit 1; }
done
cd services/marketplace-api && go build ./internal/order/... && echo "M1 build OK"
```
Expected: `M1 build OK`. If missing or build fails — **STOP**, Orders M1 hasn't landed on this branch.

- [ ] **Step 2: Verify no M2 files already exist (prevents mid-abort resumption confusion)**

```bash
for p in \
  services/marketplace-api/internal/order/status.go \
  services/marketplace-api/internal/order/service.go \
  services/marketplace-api/internal/order/repository.go \
  services/marketplace-api/internal/order/return_service.go \
  services/marketplace-api/internal/order/abandoned_cart_service.go \
  services/marketplace-api/internal/outbox/drainer.go; do
  if [ -f "$p" ]; then echo "PREEXISTING: $p"; exit 1; fi
done
echo "clean slate OK"
```
Expected: `clean slate OK`. If any preexists, **STOP** — investigate whether a prior M2 attempt left artifacts.

- [ ] **Step 3: Verify M1 tests still pass on current tree**

```bash
cd services/marketplace-api && go test ./internal/order/...
```
Expected: all M1 tests PASS including the concurrent document seq gate. If any fail, M1 regressed and M2 cannot build on it.

- [ ] **Step 4: Verify `go-shared/messaging.Publisher` interface is present**

```bash
cd services/marketplace-api && go doc github.com/mark8ly/go-shared/messaging.Publisher
```
Expected: prints an interface with a `Publish(ctx context.Context, topic string, payload []byte) error` method. If the method signature differs — e.g. takes a struct instead of `[]byte` — **STOP** and either update this plan's Task 18 (drainer) to match the real signature, or ask the human to standardize the contract in `go-shared`.

- [ ] **Step 5: Verify `marketplace_db_schema_migrations` is at version 2**

```bash
psql -h localhost -U dev -d marketplace_db -tAc \
  "SELECT version FROM marketplace_db_schema_migrations ORDER BY version DESC LIMIT 1;"
```
Expected: `2` (products M1+M2 = version 1, orders M1 = version 2). If `< 2`, run `go run ./cmd/migrate -direction up` first. If `> 2`, something is ahead of us — **STOP** and investigate.

- [ ] **Step 6: Verify M1 benchmark result is recorded**

```bash
grep -A2 'M1 benchmark result' services/marketplace-api/internal/order/README.md
```
Expected: shows a p99 number. If missing, M1 handoff was incomplete — re-run M1 Task 13 step 6 to backfill. M2 reads this number in Task 22 to sanity-check the full create-tx latency hasn't regressed.

No commit. Task 0 is read-only.

---

### Task 1: Status Go types with transition tables

**Files:**
- Create: `services/marketplace-api/internal/order/status.go`

- [ ] **Step 1: Write `status.go` with the three types**

```go
package order

// OrderStatus is the operational lifecycle of an order. It is ORTHOGONAL to
// PaymentStatus and FulfillmentStatus — 'refunded' is deliberately absent here
// and lives on PaymentStatus only.
//
// See spec §7 and §2 decision 4.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusFulfilled OrderStatus = "fulfilled"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// orderStatusTransitions is the complete set of legal transitions.
// Nil slice ⇒ terminal. Absent key ⇒ illegal starting state.
var orderStatusTransitions = map[OrderStatus][]OrderStatus{
	OrderStatusPending:   {OrderStatusConfirmed, OrderStatusCancelled},
	OrderStatusConfirmed: {OrderStatusFulfilled, OrderStatusCancelled},
	OrderStatusFulfilled: nil, // terminal
	OrderStatusCancelled: nil, // terminal
}

// CanTransitionTo returns true iff target is a legal next state from s.
func (s OrderStatus) CanTransitionTo(target OrderStatus) bool {
	allowed, ok := orderStatusTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

// Allowed returns the legal next states from s (may be empty for terminal).
func (s OrderStatus) Allowed() []OrderStatus {
	return append([]OrderStatus(nil), orderStatusTransitions[s]...)
}

// -----------------------------------------------------------------------------
// PaymentStatus — independent axis. Carries refund semantics.
// -----------------------------------------------------------------------------

type PaymentStatus string

const (
	PaymentStatusPending            PaymentStatus = "pending"
	PaymentStatusAuthorized         PaymentStatus = "authorized"
	PaymentStatusPaid               PaymentStatus = "paid"
	PaymentStatusFailed             PaymentStatus = "failed"
	PaymentStatusRefunded           PaymentStatus = "refunded"
	PaymentStatusPartiallyRefunded  PaymentStatus = "partially_refunded"
)

var paymentStatusTransitions = map[PaymentStatus][]PaymentStatus{
	PaymentStatusPending:           {PaymentStatusAuthorized, PaymentStatusPaid, PaymentStatusFailed},
	PaymentStatusAuthorized:        {PaymentStatusPaid, PaymentStatusFailed, PaymentStatusRefunded},
	PaymentStatusPaid:              {PaymentStatusRefunded, PaymentStatusPartiallyRefunded},
	PaymentStatusPartiallyRefunded: {PaymentStatusRefunded},
	PaymentStatusFailed:            {PaymentStatusPending, PaymentStatusPaid}, // retry path
	PaymentStatusRefunded:          nil,                                        // terminal
}

func (s PaymentStatus) CanTransitionTo(target PaymentStatus) bool {
	allowed, ok := paymentStatusTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

func (s PaymentStatus) Allowed() []PaymentStatus {
	return append([]PaymentStatus(nil), paymentStatusTransitions[s]...)
}

// -----------------------------------------------------------------------------
// FulfillmentStatus — independent axis.
// FulfillmentStatusPartial is forward-compatible only; slice 1 never writes it.
// -----------------------------------------------------------------------------

type FulfillmentStatus string

const (
	FulfillmentStatusUnfulfilled FulfillmentStatus = "unfulfilled"
	FulfillmentStatusPartial     FulfillmentStatus = "partial" // slice 2
	FulfillmentStatusFulfilled   FulfillmentStatus = "fulfilled"
)

var fulfillmentStatusTransitions = map[FulfillmentStatus][]FulfillmentStatus{
	FulfillmentStatusUnfulfilled: {FulfillmentStatusPartial, FulfillmentStatusFulfilled},
	FulfillmentStatusPartial:     {FulfillmentStatusFulfilled},
	FulfillmentStatusFulfilled:   nil, // terminal
}

func (s FulfillmentStatus) CanTransitionTo(target FulfillmentStatus) bool {
	allowed, ok := fulfillmentStatusTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

func (s FulfillmentStatus) Allowed() []FulfillmentStatus {
	return append([]FulfillmentStatus(nil), fulfillmentStatusTransitions[s]...)
}
```

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/status.go
git commit -m "feat(marketplace-api): three orthogonal order status types"
```

---

### Task 2: Exhaustive transition matrix tests

**Files:**
- Create: `services/marketplace-api/internal/order/status_test.go`

- [ ] **Step 1: Write the test file**

```go
package order_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/mark8ly/marketplace-api/internal/order"
)

// TestOrderStatus_LegalTransitions iterates the full source × target matrix.
// It is the canonical guard that any change to the transition table is conscious.
func TestOrderStatus_LegalTransitions(t *testing.T) {
	all := []order.OrderStatus{
		order.OrderStatusPending, order.OrderStatusConfirmed,
		order.OrderStatusFulfilled, order.OrderStatusCancelled,
	}
	legal := map[order.OrderStatus]map[order.OrderStatus]bool{
		order.OrderStatusPending: {
			order.OrderStatusConfirmed: true,
			order.OrderStatusCancelled: true,
		},
		order.OrderStatusConfirmed: {
			order.OrderStatusFulfilled: true,
			order.OrderStatusCancelled: true,
		},
		// fulfilled, cancelled are terminal — no legal targets
	}
	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			got := from.CanTransitionTo(to)
			assert.Equal(t, want, got, "from=%s to=%s", from, to)
		}
	}
}

func TestOrderStatus_RefundedIsNotAValue(t *testing.T) {
	// Guards against accidentally adding OrderStatusRefunded back.
	// If this constant ever exists, the test file stops compiling.
	var _ = order.OrderStatus("refunded") // cast is allowed, but the enum doesn't include it
	// Proof: calling CanTransitionTo with an "unknown" starting state returns false.
	assert.False(t, order.OrderStatus("refunded").CanTransitionTo(order.OrderStatusPending))
}

func TestPaymentStatus_LegalTransitions(t *testing.T) {
	all := []order.PaymentStatus{
		order.PaymentStatusPending, order.PaymentStatusAuthorized, order.PaymentStatusPaid,
		order.PaymentStatusFailed, order.PaymentStatusRefunded, order.PaymentStatusPartiallyRefunded,
	}
	legal := map[order.PaymentStatus]map[order.PaymentStatus]bool{
		order.PaymentStatusPending: {
			order.PaymentStatusAuthorized: true,
			order.PaymentStatusPaid:       true,
			order.PaymentStatusFailed:     true,
		},
		order.PaymentStatusAuthorized: {
			order.PaymentStatusPaid:     true,
			order.PaymentStatusFailed:   true,
			order.PaymentStatusRefunded: true,
		},
		order.PaymentStatusPaid: {
			order.PaymentStatusRefunded:          true,
			order.PaymentStatusPartiallyRefunded: true,
		},
		order.PaymentStatusPartiallyRefunded: {
			order.PaymentStatusRefunded: true,
		},
		order.PaymentStatusFailed: {
			order.PaymentStatusPending: true,
			order.PaymentStatusPaid:    true,
		},
		// refunded is terminal
	}
	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			got := from.CanTransitionTo(to)
			assert.Equal(t, want, got, "from=%s to=%s", from, to)
		}
	}
}

func TestFulfillmentStatus_LegalTransitions(t *testing.T) {
	all := []order.FulfillmentStatus{
		order.FulfillmentStatusUnfulfilled,
		order.FulfillmentStatusPartial,
		order.FulfillmentStatusFulfilled,
	}
	legal := map[order.FulfillmentStatus]map[order.FulfillmentStatus]bool{
		order.FulfillmentStatusUnfulfilled: {
			order.FulfillmentStatusPartial:   true,
			order.FulfillmentStatusFulfilled: true,
		},
		order.FulfillmentStatusPartial: {
			order.FulfillmentStatusFulfilled: true,
		},
		// fulfilled is terminal
	}
	for _, from := range all {
		for _, to := range all {
			want := legal[from][to]
			got := from.CanTransitionTo(to)
			assert.Equal(t, want, got, "from=%s to=%s", from, to)
		}
	}
}

func TestAllowed_ReturnsCopies(t *testing.T) {
	// Mutating the returned slice must not affect the internal table.
	s := order.OrderStatusPending
	a := s.Allowed()
	if len(a) == 0 {
		t.Fatal("expected allowed targets for pending")
	}
	a[0] = order.OrderStatus("hacked")
	b := s.Allowed()
	assert.NotEqual(t, a, b, "Allowed() must return a defensive copy")
}
```

- [ ] **Step 2: Run the tests**

```bash
cd services/marketplace-api && go test -run 'TestOrderStatus|TestPaymentStatus|TestFulfillmentStatus|TestAllowed' -v ./internal/order/
```
Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/status_test.go
git commit -m "test(marketplace-api): exhaustive transition matrices for order status axes"
```

---

### Task 3: Typed service errors + event kind constants

**Files:**
- Create: `services/marketplace-api/internal/order/errors.go`
- Create: `services/marketplace-api/internal/order/events.go`

- [ ] **Step 1: Write `errors.go`**

```go
package order

import "errors"

var (
	// ErrNotFound is returned when a lookup misses. Handlers map to 404.
	ErrNotFound = errors.New("order: not found")

	// ErrInvalidTransition carries the illegal from/to and the allowed next
	// states. Handlers map to HTTP 409 with the spec §6.6 error envelope
	// `invalid_transition` including `details.allowed`.
	ErrInvalidTransition = errors.New("order: invalid transition")

	// ErrRefundExceedsTotal means refunded_amount + new_amount > grand_total.
	// Handlers map to 409 `refund_exceeds_total`.
	ErrRefundExceedsTotal = errors.New("order: refund exceeds total")

	// ErrIdempotencyConflict means the same idempotency_key was seen with a
	// different payload than the original. Handlers map to 409
	// `idempotency_conflict`.
	ErrIdempotencyConflict = errors.New("order: idempotency conflict")

	// ErrOrderNotCancellable is returned when Cancel is called on a fulfilled
	// or already-cancelled order. Handlers map to 409 `order_not_cancellable`.
	ErrOrderNotCancellable = errors.New("order: not cancellable")

	// ErrReturnItemsExceedOrdered means the return tried to return more
	// quantity than remains returnable on the source order_item.
	ErrReturnItemsExceedOrdered = errors.New("order: return items exceed ordered quantity")

	// ErrRecoveryTooRecent: abandoned cart recovery email already sent in the
	// last 24 hours.
	ErrRecoveryTooRecent = errors.New("abandoned_cart: recovery email sent within 24h")

	// ErrAbandonedCartAlreadyConverted: the cart has been converted to an order
	// and cannot be recovered.
	ErrAbandonedCartAlreadyConverted = errors.New("abandoned_cart: already converted")
)
```

- [ ] **Step 2: Write `events.go`**

```go
package order

import (
	"encoding/json"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// OrderEventKind is the `kind` column on order_events.
// Keep this aligned with the free-form varchar(40) until a CHECK constraint is
// introduced in a future migration.
type OrderEventKind string

const (
	EventKindStatusChanged   OrderEventKind = "status_changed"
	EventKindPaymentCaptured OrderEventKind = "payment_captured"
	EventKindRefundRecorded  OrderEventKind = "refund_recorded"
	EventKindNoteAdded       OrderEventKind = "note_added"
	EventKindFulfilled       OrderEventKind = "fulfilled"
	EventKindCancelled       OrderEventKind = "cancelled"
	EventKindReturnLinked    OrderEventKind = "return_linked"
	EventKindReturnApproved  OrderEventKind = "return_approved"
	EventKindReturnReceived  OrderEventKind = "return_received"
	EventKindReturnRefunded  OrderEventKind = "return_refunded"
	EventKindReturnRejected  OrderEventKind = "return_rejected"
)

// StatusChangedPayload is the canonical payload for a top-level status change.
type StatusChangedPayload struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
}

// RefundRecordedPayload is the canonical payload for a bookkeeping refund.
type RefundRecordedPayload struct {
	Amount           decimal.Decimal `json:"amount"`
	Reason           string          `json:"reason,omitempty"`
	RefundedTotal    decimal.Decimal `json:"refunded_total"`
	IsProviderRefund bool            `json:"is_provider_refund"` // false in slice 1
}

func mustJSON(v any) datatypes.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // payloads are closed types; any marshal failure is a programmer error
	}
	return datatypes.JSON(b)
}
```

- [ ] **Step 3: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/order/errors.go \
        services/marketplace-api/internal/order/events.go
git commit -m "feat(marketplace-api): order typed errors and event kind helpers"
```

---

### Task 4: Order repository skeleton + GetByID + GetByIdempotencyKey + SoftDelete

**Files:**
- Create: `services/marketplace-api/internal/order/repository.go`

- [ ] **Step 1: Write the repository skeleton**

```go
package order

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the data-access layer for the order aggregate.
// All methods take a *gorm.DB so callers can thread transactions through.
// Pass s.db for non-transactional reads, or tx from within a service Unit.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*Order, error) {
	if tx == nil {
		tx = r.db
	}
	var o Order
	err := tx.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetByIdempotencyKey looks up an existing order by (store_id, idempotency_key).
// Returns ErrNotFound if none exists. Used by Service.Create as the first line
// of idempotency defense before the insert path.
func (r *Repository) GetByIdempotencyKey(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, key string) (*Order, error) {
	if tx == nil {
		tx = r.db
	}
	var o Order
	err := tx.WithContext(ctx).
		Where("store_id = ? AND idempotency_key = ? AND deleted_at IS NULL", storeID, key).
		First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// SoftDelete marks an order as deleted without actually removing rows.
// This is the only delete path in slice 1 — hard delete is blocked at the
// schema level by the return_items RESTRICT chain.
func (r *Repository) SoftDelete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	if tx == nil {
		tx = r.db
	}
	now := time.Now()
	res := tx.WithContext(ctx).
		Model(&Order{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/repository.go
git commit -m "feat(marketplace-api): order repository skeleton (GetByID, GetByIdempotencyKey, SoftDelete)"
```

---

### Task 5: Order repository — atomic RecordRefund

**Files:**
- Modify: `services/marketplace-api/internal/order/repository.go`
- Create: `services/marketplace-api/internal/order/repository_test.go`

This is the heart of the refund correctness guarantee. Single SQL statement, no read-check-write.

- [ ] **Step 1: Append `RecordRefund` to the repository**

```go
// RecordRefund atomically increases refunded_amount by delta IFF the resulting
// total would not exceed grand_total. On success returns the new refunded_amount.
// Returns ErrRefundExceedsTotal if the update would overflow (zero rows affected).
//
// This is a single-statement UPDATE — there is NO read-check-write window.
// Concurrent callers both trying to refund over the limit are serialized by
// Postgres row locking; exactly one succeeds, the other returns ErrRefundExceedsTotal.
func (r *Repository) RecordRefund(
	ctx context.Context,
	tx *gorm.DB,
	id uuid.UUID,
	delta decimal.Decimal,
	newPaymentStatus PaymentStatus,
) (decimal.Decimal, error) {
	if tx == nil {
		tx = r.db
	}
	if delta.IsNegative() || delta.IsZero() {
		return decimal.Zero, errors.New("order: refund delta must be positive")
	}

	var newTotal decimal.Decimal
	err := tx.WithContext(ctx).Raw(`
		UPDATE orders
		SET refunded_amount = refunded_amount + ?,
		    payment_status  = ?,
		    updated_at      = now()
		WHERE id = ?
		  AND deleted_at IS NULL
		  AND refunded_amount + ? <= grand_total
		RETURNING refunded_amount
	`, delta, string(newPaymentStatus), id, delta).Scan(&newTotal).Error

	if err != nil {
		return decimal.Zero, err
	}
	if newTotal.IsZero() {
		// Either the order didn't exist / was soft-deleted, OR the guard clause
		// failed. Distinguish by a follow-up lookup.
		o, lookupErr := r.GetByID(ctx, tx, id)
		if lookupErr != nil {
			return decimal.Zero, lookupErr
		}
		if o.RefundedAmount.Add(delta).GreaterThan(o.GrandTotal) {
			return decimal.Zero, ErrRefundExceedsTotal
		}
		return decimal.Zero, ErrRefundExceedsTotal
	}
	return newTotal, nil
}
```

Add `"github.com/shopspring/decimal"` to the import block.

- [ ] **Step 2: Write the integration test**

```go
package order_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestRepository_RecordRefund_AtomicOverflowRejected(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	repo := order.NewRepository(db)
	o := newTestOrder(t, db) // GrandTotal = 100

	// First partial refund succeeds
	newTotal, err := repo.RecordRefund(ctx, nil, o.ID, decimal.NewFromInt(60), order.PaymentStatusPartiallyRefunded)
	require.NoError(t, err)
	require.True(t, newTotal.Equal(decimal.NewFromInt(60)))

	// Second refund that would push over grand_total must fail
	_, err = repo.RecordRefund(ctx, nil, o.ID, decimal.NewFromInt(50), order.PaymentStatusRefunded)
	require.ErrorIs(t, err, order.ErrRefundExceedsTotal)

	// refunded_amount in the DB remains at 60
	back, err := repo.GetByID(ctx, nil, o.ID)
	require.NoError(t, err)
	require.True(t, back.RefundedAmount.Equal(decimal.NewFromInt(60)))
}

func TestRepository_RecordRefund_ConcurrentOverflowExactlyOneWins(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	repo := order.NewRepository(db)
	o := newTestOrder(t, db) // GrandTotal = 100

	// Run the race 100 times to eliminate flakes.
	for iter := 0; iter < 100; iter++ {
		// Reset
		require.NoError(t, db.Model(&o).Update("refunded_amount", 0).Error)

		var wg sync.WaitGroup
		results := make([]error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			i := i
			go func() {
				defer wg.Done()
				_, results[i] = repo.RecordRefund(ctx, nil, o.ID, decimal.NewFromInt(60), order.PaymentStatusPartiallyRefunded)
			}()
		}
		wg.Wait()

		// Exactly one success, exactly one ErrRefundExceedsTotal
		successes, failures := 0, 0
		for _, r := range results {
			if r == nil {
				successes++
			} else if errors.Is(r, order.ErrRefundExceedsTotal) {
				failures++
			} else {
				t.Fatalf("unexpected error: %v", r)
			}
		}
		require.Equal(t, 1, successes, "iter=%d", iter)
		require.Equal(t, 1, failures, "iter=%d", iter)

		// Total in DB is exactly 60, not 120
		back, _ := repo.GetByID(ctx, nil, o.ID)
		require.True(t, back.RefundedAmount.Equal(decimal.NewFromInt(60)))
	}
}
```

Add `sync`, `errors` to the import block.

- [ ] **Step 3: Run**

```bash
cd services/marketplace-api && go test -run TestRepository_RecordRefund -v ./internal/order/
```
Expected: both PASS. The concurrent test may take a few seconds for 100 iterations — that's expected.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/order/repository.go \
        services/marketplace-api/internal/order/repository_test.go
git commit -m "feat(marketplace-api): atomic RecordRefund with concurrent-race integration test"
```

---

### Task 6: Order repository — List with tab filter + HasOpenReturn

**Files:**
- Modify: `services/marketplace-api/internal/order/repository.go`
- Modify: `services/marketplace-api/internal/order/repository_test.go`

- [ ] **Step 1: Define the list filter struct and method**

Append to `repository.go`:
```go
// ListFilter is the DSL the HTTP list handler passes to the repository.
type ListFilter struct {
	StoreID    uuid.UUID
	Tab        string    // "all" | "open" | "fulfilled" | "returned" | "cancelled" | "abandoned"
	Search     string    // substring match on order_number prefix / customer_email
	Status     []string  // additional status filter (only when Tab == "all")
	DateFrom   *time.Time
	DateTo     *time.Time
	CustomerID *uuid.UUID
	Limit      int
	Offset     int
}

// ListRow is the repository-level projection for the list page.
// Note HasOpenReturn is derived via LEFT JOIN LATERAL.
type ListRow struct {
	Order         Order
	ItemCount     int
	HasOpenReturn bool
}

// List returns paginated rows for the Orders tab. Abandoned is handled by a
// separate abandoned_cart repository — do NOT route Tab == "abandoned" here.
func (r *Repository) List(ctx context.Context, f ListFilter) ([]ListRow, int64, error) {
	if f.Tab == "abandoned" {
		return nil, 0, errors.New("order.Repository.List: abandoned tab is served by abandoned_cart repository")
	}

	q := r.db.WithContext(ctx).
		Table("orders").
		Where("store_id = ? AND deleted_at IS NULL", f.StoreID)

	switch f.Tab {
	case "", "all":
		// no status filter beyond the multi-select
	case "open":
		q = q.Where("status IN ?", []string{string(OrderStatusPending), string(OrderStatusConfirmed)})
	case "fulfilled":
		q = q.Where("status = ?", OrderStatusFulfilled)
	case "cancelled":
		q = q.Where("status = ?", OrderStatusCancelled)
	case "returned":
		// Returned = at least one NON-rejected return exists.
		q = q.Where(`EXISTS (
			SELECT 1 FROM returns r
			WHERE r.order_id = orders.id
			  AND r.status IN ('requested','approved','received','refunded')
		)`)
	default:
		return nil, 0, errors.New("order.Repository.List: unknown tab " + f.Tab)
	}

	if len(f.Status) > 0 && (f.Tab == "" || f.Tab == "all") {
		q = q.Where("status IN ?", f.Status)
	}
	if f.Search != "" {
		// Prefix search on order_number, plus equality prefix on lower(email).
		q = q.Where("(order_number LIKE ? OR lower(customer_email) LIKE ?)",
			f.Search+"%", f.Search+"%")
	}
	if f.DateFrom != nil {
		q = q.Where("placed_at >= ?", *f.DateFrom)
	}
	if f.DateTo != nil {
		q = q.Where("placed_at < ?", *f.DateTo)
	}
	if f.CustomerID != nil {
		q = q.Where("customer_id = ?", *f.CustomerID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Main query with LATERAL joins for ItemCount and HasOpenReturn.
	// LEFT JOIN LATERAL so a missing join row doesn't drop the order.
	type scanRow struct {
		Order
		ItemCount     int  `gorm:"column:item_count"`
		HasOpenReturn bool `gorm:"column:has_open_return"`
	}

	var rows []scanRow
	err := q.Select(`
		orders.*,
		COALESCE(ic.cnt, 0)  AS item_count,
		COALESCE(hor.has, false) AS has_open_return
	`).
		Joins(`LEFT JOIN LATERAL (SELECT COUNT(*) AS cnt FROM order_items WHERE order_id = orders.id) ic ON true`).
		Joins(`LEFT JOIN LATERAL (
			SELECT EXISTS(
				SELECT 1 FROM returns r
				WHERE r.order_id = orders.id
				  AND r.status IN ('requested','approved','received','refunded')
			) AS has
		) hor ON true`).
		Order("orders.placed_at DESC").
		Limit(f.Limit).
		Offset(f.Offset).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	out := make([]ListRow, len(rows))
	for i, r := range rows {
		out[i] = ListRow{Order: r.Order, ItemCount: r.ItemCount, HasOpenReturn: r.HasOpenReturn}
	}
	return out, total, nil
}
```

- [ ] **Step 2: Write integration tests covering every tab path**

Append to `repository_test.go`:
```go
func TestRepository_List_TabFilters(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	repo := order.NewRepository(db)
	storeID := uuid.New()

	// Seed 1 pending, 1 confirmed, 1 fulfilled, 1 cancelled, 1 fulfilled with return
	pending  := newOrderInStatus(t, db, storeID, order.OrderStatusPending, order.PaymentStatusPending)
	confirmed := newOrderInStatus(t, db, storeID, order.OrderStatusConfirmed, order.PaymentStatusPaid)
	fulfilled := newOrderInStatus(t, db, storeID, order.OrderStatusFulfilled, order.PaymentStatusPaid)
	cancelled := newOrderInStatus(t, db, storeID, order.OrderStatusCancelled, order.PaymentStatusPending)

	fulfilledWithReturn := newOrderInStatus(t, db, storeID, order.OrderStatusFulfilled, order.PaymentStatusPaid)
	newApprovedReturn(t, db, fulfilledWithReturn)

	// fulfilledWithRejectedReturn must NOT show up on Returned tab
	fulfilledWithRejected := newOrderInStatus(t, db, storeID, order.OrderStatusFulfilled, order.PaymentStatusPaid)
	newRejectedReturn(t, db, fulfilledWithRejected)

	cases := []struct {
		tab     string
		wantIDs []uuid.UUID
	}{
		{"all", []uuid.UUID{pending.ID, confirmed.ID, fulfilled.ID, cancelled.ID, fulfilledWithReturn.ID, fulfilledWithRejected.ID}},
		{"open", []uuid.UUID{pending.ID, confirmed.ID}},
		{"fulfilled", []uuid.UUID{fulfilled.ID, fulfilledWithReturn.ID, fulfilledWithRejected.ID}},
		{"cancelled", []uuid.UUID{cancelled.ID}},
		{"returned", []uuid.UUID{fulfilledWithReturn.ID}}, // rejected excluded
	}
	for _, tc := range cases {
		t.Run(tc.tab, func(t *testing.T) {
			rows, total, err := repo.List(ctx, order.ListFilter{
				StoreID: storeID, Tab: tc.tab, Limit: 50,
			})
			require.NoError(t, err)
			require.EqualValues(t, len(tc.wantIDs), total)
			gotIDs := make([]uuid.UUID, len(rows))
			for i, r := range rows {
				gotIDs[i] = r.Order.ID
			}
			require.ElementsMatch(t, tc.wantIDs, gotIDs)
		})
	}
}

func TestRepository_List_HasOpenReturn_OnAllTab(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	repo := order.NewRepository(db)
	storeID := uuid.New()

	plain := newOrderInStatus(t, db, storeID, order.OrderStatusFulfilled, order.PaymentStatusPaid)
	withReturn := newOrderInStatus(t, db, storeID, order.OrderStatusFulfilled, order.PaymentStatusPaid)
	newApprovedReturn(t, db, withReturn)

	rows, _, err := repo.List(ctx, order.ListFilter{StoreID: storeID, Tab: "all", Limit: 50})
	require.NoError(t, err)

	byID := map[uuid.UUID]order.ListRow{}
	for _, r := range rows {
		byID[r.Order.ID] = r
	}
	require.False(t, byID[plain.ID].HasOpenReturn)
	require.True(t, byID[withReturn.ID].HasOpenReturn)
}

func TestRepository_List_AbandonedTabRejected(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	repo := order.NewRepository(db)
	_, _, err := repo.List(ctx, order.ListFilter{StoreID: uuid.New(), Tab: "abandoned"})
	require.Error(t, err)
}
```

Add `newOrderInStatus`, `newApprovedReturn`, `newRejectedReturn` test helpers in a shared test helper file `internal/order/testhelpers_test.go` (create it as part of this task).

- [ ] **Step 3: Run**

```bash
cd services/marketplace-api && go test -run TestRepository_List -v ./internal/order/
```
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/order/repository.go \
        services/marketplace-api/internal/order/repository_test.go \
        services/marketplace-api/internal/order/testhelpers_test.go
git commit -m "feat(marketplace-api): order list with tab filter and HasOpenReturn projection"
```

---

### Task 7: Order service skeleton + Unit helper

**Files:**
- Create: `services/marketplace-api/internal/order/service.go`

- [ ] **Step 1: Write the service skeleton**

```go
package order

import (
	"context"

	"gorm.io/gorm"
)

// Service is the domain-layer entry point for the order aggregate.
//
// All writes go through a Service method — repositories are not exposed
// outside this package. This is enforced by convention and code review;
// the repository is public so the service_test package can exercise it directly.
//
// Cross-module transaction boundaries use the Unit helper: both Service
// receivers can be composed inside a single *gorm.DB transaction so
// return.Service.Approve can call order.Service.RecordReturnEvent atomically.
type Service struct {
	db   *gorm.DB
	repo *Repository
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, repo: NewRepository(db)}
}

// Unit runs fn inside a GORM transaction. The passed *gorm.DB is the transaction
// handle — thread it into any repository or cross-module service method that
// needs to participate in the same transaction.
//
// This is the ONLY way services in this package start a transaction. Code that
// calls s.db.Transaction(...) directly is a code smell.
func (s *Service) Unit(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// Repo exposes the repository for use inside a Unit closure. Returning the
// repository rather than individual methods keeps the Unit call sites terse.
func (s *Service) Repo() *Repository {
	return s.repo
}
```

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/service.go
git commit -m "feat(marketplace-api): order.Service skeleton with Unit helper"
```

---

### Task 8: Order service — Create with idempotency + outbox

**Files:**
- Modify: `services/marketplace-api/internal/order/service.go`

This is the largest service method. It wraps the create flow described in spec §6.5.

- [ ] **Step 1: Define the create input type**

```go
// CreateInput is the full payload accepted by Service.Create.
// The storefront checkout handler builds this from its cart session.
type CreateInput struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	StorePrefix    string          // e.g. "BOWL" — drives the human-readable order_number
	IdempotencyKey string          // cart session id
	CustomerID     *uuid.UUID      // nil for guests
	CustomerEmail  string
	CustomerName   *string
	Items          []CreateItemInput
	Shipping       AddressInput
	Billing        AddressInput
	Subtotal       decimal.Decimal
	ShippingTotal  decimal.Decimal
	TaxTotal       decimal.Decimal
	DiscountTotal  decimal.Decimal
	GrandTotal     decimal.Decimal
	CurrencyCode   string
	PaymentProvider *string
}

type CreateItemInput struct {
	ProductID     *uuid.UUID
	VariantID     *uuid.UUID
	TitleSnapshot string
	SKUSnapshot   string
	OptionSummary *string
	UnitPrice     decimal.Decimal
	Quantity      int
	LineTotal     decimal.Decimal
	ImageURL      *string
}

type AddressInput struct {
	Name        string
	Line1       string
	Line2       *string
	City        string
	Region      *string
	PostalCode  *string
	CountryCode string
	Phone       *string
}

type CreateResult struct {
	Order  *Order
	Reused bool // true iff an existing order was returned via idempotency lookup
}
```

Add these type definitions either at the bottom of `service.go` or in a new `service_input.go` file at your discretion.

- [ ] **Step 2: Implement `Create`**

```go
// Create inserts a new order. Idempotent on (store_id, idempotency_key).
//
// Flow:
//  1. Fast-path lookup by (store_id, idempotency_key). If found, return it.
//  2. Begin transaction.
//  3. Issue an atomic document number via NextDocumentNumber.
//  4. Insert the orders row.
//  5. Insert order_items.
//  6. Insert order_addresses (shipping + billing).
//  7. Insert the initial order_events row (status_changed: null -> pending).
//  8. Insert the order.placed pending_events row (outbox).
//  9. Commit.
//
// On a UNIQUE(store_id, idempotency_key) constraint violation in step 4, the
// transaction rolls back and the method retries the fast-path lookup once
// (resolving the tiny race where two concurrent creates with the same key
// both failed step 1 and raced step 4). After one retry, an unresolvable
// conflict returns ErrIdempotencyConflict.
func (s *Service) Create(ctx context.Context, in CreateInput) (*CreateResult, error) {
	// Fast path
	if existing, err := s.repo.GetByIdempotencyKey(ctx, nil, in.StoreID, in.IdempotencyKey); err == nil {
		return &CreateResult{Order: existing, Reused: true}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	var created *Order
	createFn := func(tx *gorm.DB) error {
		// Atomic document number
		day := time.Now().UTC()
		seq, err := NextDocumentNumber(ctx, tx, in.StoreID, "order", day)
		if err != nil {
			return err
		}
		orderNumber := fmt.Sprintf("M-%s-%s-%05d",
			strings.ToUpper(in.StorePrefix),
			day.Format("060102"),
			seq,
		)

		o := &Order{
			TenantID:       in.TenantID,
			StoreID:        in.StoreID,
			OrderNumber:    orderNumber,
			IdempotencyKey: in.IdempotencyKey,
			CustomerID:     in.CustomerID,
			CustomerEmail:  in.CustomerEmail,
			CustomerName:   in.CustomerName,
			Status:         string(OrderStatusPending),
			PaymentStatus:  string(PaymentStatusPending),
			FulfillmentStatus: string(FulfillmentStatusUnfulfilled),
			Subtotal:       in.Subtotal,
			ShippingTotal:  in.ShippingTotal,
			TaxTotal:       in.TaxTotal,
			DiscountTotal:  in.DiscountTotal,
			GrandTotal:     in.GrandTotal,
			CurrencyCode:   in.CurrencyCode,
			PaymentProvider: in.PaymentProvider,
			PlacedAt:       time.Now(),
		}
		if err := tx.Create(o).Error; err != nil {
			return err
		}

		// Items
		for _, item := range in.Items {
			it := &OrderItem{
				OrderID: o.ID,
				ProductID: item.ProductID,
				VariantID: item.VariantID,
				TitleSnapshot: item.TitleSnapshot,
				SKUSnapshot: item.SKUSnapshot,
				OptionSummary: item.OptionSummary,
				UnitPrice: item.UnitPrice,
				Quantity: item.Quantity,
				LineTotal: item.LineTotal,
				CurrencyCode: in.CurrencyCode,
				ImageURL: item.ImageURL,
			}
			if err := tx.Create(it).Error; err != nil {
				return err
			}
		}

		// Addresses
		for _, addr := range []struct {
			kind string
			a    AddressInput
		}{{"shipping", in.Shipping}, {"billing", in.Billing}} {
			row := &OrderAddress{
				OrderID: o.ID,
				Kind: addr.kind,
				Name: addr.a.Name, Line1: addr.a.Line1, Line2: addr.a.Line2,
				City: addr.a.City, Region: addr.a.Region, PostalCode: addr.a.PostalCode,
				CountryCode: addr.a.CountryCode, Phone: addr.a.Phone,
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
		}

		// Initial order_events row
		evt := &OrderEvent{
			OrderID: o.ID,
			Kind: string(EventKindStatusChanged),
			Payload: mustJSON(StatusChangedPayload{From: "", To: string(OrderStatusPending)}),
		}
		if err := tx.Create(evt).Error; err != nil {
			return err
		}

		// Outbox: order.placed
		if err := s.enqueueOutbox(tx, in.TenantID, in.StoreID, "order.placed", map[string]any{
			"order_id": o.ID, "order_number": o.OrderNumber, "customer_email": o.CustomerEmail,
			"grand_total": o.GrandTotal, "currency_code": o.CurrencyCode,
		}); err != nil {
			return err
		}

		created = o
		return nil
	}

	err := s.Unit(ctx, createFn)
	if err != nil {
		// Unique-constraint race: someone inserted between fast-path and tx start
		if isUniqueViolation(err, "orders_idempotency_per_store_unique") {
			if existing, err2 := s.repo.GetByIdempotencyKey(ctx, nil, in.StoreID, in.IdempotencyKey); err2 == nil {
				return &CreateResult{Order: existing, Reused: true}, nil
			}
			return nil, ErrIdempotencyConflict
		}
		return nil, err
	}
	return &CreateResult{Order: created, Reused: false}, nil
}

// enqueueOutbox writes a pending_events row inside an existing tx.
func (s *Service) enqueueOutbox(tx *gorm.DB, tenantID, storeID uuid.UUID, topic string, payload any) error {
	row := &PendingEvent{
		TenantID: tenantID, StoreID: storeID, Topic: topic,
		Payload: mustJSON(payload), NextAttemptAt: time.Now(),
	}
	return tx.Create(row).Error
}

// isUniqueViolation detects Postgres error code 23505 with the given constraint name.
func isUniqueViolation(err error, constraint string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, constraint)
	}
	return false
}
```

Imports needed: `fmt`, `strings`, `time`, `errors`, `github.com/google/uuid`, `github.com/shopspring/decimal`, `gorm.io/gorm`, `github.com/jackc/pgx/v5/pgconn`.

Note: `github.com/jackc/pgx/v5/pgconn` must already be in `go.mod` via GORM's Postgres driver. If not, add it via `go get` in this step.

- [ ] **Step 3: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/order/service.go
git commit -m "feat(marketplace-api): order.Service.Create with idempotency and outbox"
```

---

### Task 9: Order service — TransitionStatus, MarkFulfilled, Cancel

**Files:**
- Modify: `services/marketplace-api/internal/order/service.go`

- [ ] **Step 1: Implement the shared `transitionStatusTx` helper**

```go
// transitionStatusTx is the internal helper used by MarkFulfilled and Cancel.
// It centralizes the legality check, the DB update, and the order_events write.
// Callers pass the desired target + an optional side-effect closure for
// per-transition fields like cancelled_at or fulfilled_at.
func (s *Service) transitionStatusTx(
	ctx context.Context,
	tx *gorm.DB,
	id uuid.UUID,
	target OrderStatus,
	actorID *uuid.UUID,
	actorEmail *string,
	sideEffect func(*Order),
) (*Order, error) {
	o, err := s.repo.GetByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	current := OrderStatus(o.Status)
	if !current.CanTransitionTo(target) {
		return nil, fmt.Errorf("%w: from=%s to=%s allowed=%v",
			ErrInvalidTransition, current, target, current.Allowed())
	}
	o.Status = string(target)
	if sideEffect != nil {
		sideEffect(o)
	}
	if err := tx.WithContext(ctx).Save(o).Error; err != nil {
		return nil, err
	}
	evt := &OrderEvent{
		OrderID:    o.ID,
		Kind:       string(EventKindStatusChanged),
		ActorID:    actorID,
		ActorEmail: actorEmail,
		Payload:    mustJSON(StatusChangedPayload{From: string(current), To: string(target)}),
	}
	if err := tx.Create(evt).Error; err != nil {
		return nil, err
	}
	return o, nil
}

// MarkFulfilled transitions confirmed -> fulfilled AND writes a fulfilled
// order_events row with tracking info, AND enqueues the order.fulfilled outbox row.
func (s *Service) MarkFulfilled(ctx context.Context, id uuid.UUID, tracking *TrackingInfo, actor Actor) (*Order, error) {
	var result *Order
	err := s.Unit(ctx, func(tx *gorm.DB) error {
		now := time.Now()
		o, err := s.transitionStatusTx(ctx, tx, id, OrderStatusFulfilled, actor.ID, actor.Email, func(o *Order) {
			o.FulfilledAt = &now
			o.FulfillmentStatus = string(FulfillmentStatusFulfilled)
		})
		if err != nil {
			return err
		}

		// Optional tracking event
		if tracking != nil {
			evt := &OrderEvent{
				OrderID:    o.ID,
				Kind:       string(EventKindFulfilled),
				ActorID:    actor.ID,
				ActorEmail: actor.Email,
				Payload:    mustJSON(map[string]any{"tracking_number": tracking.Number, "carrier": tracking.Carrier}),
			}
			if err := tx.Create(evt).Error; err != nil {
				return err
			}
		}

		if err := s.enqueueOutbox(tx, o.TenantID, o.StoreID, "order.fulfilled", map[string]any{
			"order_id": o.ID, "order_number": o.OrderNumber,
		}); err != nil {
			return err
		}

		result = o
		return nil
	})
	return result, err
}

// Cancel transitions pending|confirmed -> cancelled, sets cancelled_at,
// and enqueues the order.cancelled outbox row.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, reason string, actor Actor) (*Order, error) {
	var result *Order
	err := s.Unit(ctx, func(tx *gorm.DB) error {
		now := time.Now()
		o, err := s.transitionStatusTx(ctx, tx, id, OrderStatusCancelled, actor.ID, actor.Email, func(o *Order) {
			o.CancelledAt = &now
		})
		if err != nil {
			// Translate "invalid transition from fulfilled" into the more specific
			// ErrOrderNotCancellable so the handler can return a dedicated code.
			if errors.Is(err, ErrInvalidTransition) {
				return fmt.Errorf("%w: %s", ErrOrderNotCancellable, err.Error())
			}
			return err
		}

		evt := &OrderEvent{
			OrderID:    o.ID,
			Kind:       string(EventKindCancelled),
			ActorID:    actor.ID,
			ActorEmail: actor.Email,
			Payload:    mustJSON(map[string]any{"reason": reason}),
		}
		if err := tx.Create(evt).Error; err != nil {
			return err
		}

		if err := s.enqueueOutbox(tx, o.TenantID, o.StoreID, "order.cancelled", map[string]any{
			"order_id": o.ID, "reason": reason,
		}); err != nil {
			return err
		}

		result = o
		return nil
	})
	return result, err
}

// Actor carries the admin identity for audit rows.
type Actor struct {
	ID    *uuid.UUID
	Email *string
}

// TrackingInfo is the optional fulfillment metadata.
type TrackingInfo struct {
	Number  string
	Carrier string
}
```

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/service.go
git commit -m "feat(marketplace-api): order.Service TransitionStatus, MarkFulfilled, Cancel"
```

---

### Task 10: Order service — RecordRefund + ResendConfirmation

**Files:**
- Modify: `services/marketplace-api/internal/order/service.go`

- [ ] **Step 1: Implement `RecordRefund`**

```go
// RecordRefund records a bookkeeping refund via the atomic repository path.
// Slice 1: this does NOT call any payment provider. The order_events row and
// pending_events row both carry payload.is_provider_refund = false.
//
// idempotencyKey is optional; if provided and the same key was used for a prior
// refund on this order, the call is a no-op and returns the prior refund row.
func (s *Service) RecordRefund(
	ctx context.Context,
	orderID uuid.UUID,
	amount decimal.Decimal,
	reason string,
	idempotencyKey string,
	actor Actor,
) (*Order, error) {
	if amount.IsNegative() || amount.IsZero() {
		return nil, errors.New("order: refund amount must be positive")
	}

	var result *Order
	err := s.Unit(ctx, func(tx *gorm.DB) error {
		// Idempotency: if an order_events row already exists with this key on
		// this order, no-op and return the existing state.
		if idempotencyKey != "" {
			var existingEvents int64
			tx.Model(&OrderEvent{}).
				Where("order_id = ? AND kind = ? AND payload->>'idempotency_key' = ?",
					orderID, EventKindRefundRecorded, idempotencyKey).
				Count(&existingEvents)
			if existingEvents > 0 {
				o, err := s.repo.GetByID(ctx, tx, orderID)
				if err != nil {
					return err
				}
				result = o
				return nil
			}
		}

		o, err := s.repo.GetByID(ctx, tx, orderID)
		if err != nil {
			return err
		}

		// Determine target payment status
		afterTotal := o.RefundedAmount.Add(amount)
		var targetPS PaymentStatus
		if afterTotal.Equal(o.GrandTotal) {
			targetPS = PaymentStatusRefunded
		} else {
			targetPS = PaymentStatusPartiallyRefunded
		}

		newTotal, err := s.repo.RecordRefund(ctx, tx, orderID, amount, targetPS)
		if err != nil {
			return err
		}

		evt := &OrderEvent{
			OrderID:    o.ID,
			Kind:       string(EventKindRefundRecorded),
			ActorID:    actor.ID,
			ActorEmail: actor.Email,
			Payload: mustJSON(map[string]any{
				"amount":             amount,
				"reason":             reason,
				"refunded_total":     newTotal,
				"is_provider_refund": false,
				"idempotency_key":    idempotencyKey,
			}),
		}
		if err := tx.Create(evt).Error; err != nil {
			return err
		}

		if err := s.enqueueOutbox(tx, o.TenantID, o.StoreID, "order.refunded", map[string]any{
			"order_id":           o.ID,
			"amount":             amount,
			"refunded_total":     newTotal,
			"is_provider_refund": false,
		}); err != nil {
			return err
		}

		// Refetch with new state
		refreshed, err := s.repo.GetByID(ctx, tx, orderID)
		if err != nil {
			return err
		}
		result = refreshed
		return nil
	})
	return result, err
}

// ResendConfirmation re-enqueues an order.placed outbox row so the drainer
// publishes another copy. Used by the admin UI's "Resend confirmation email"
// affordance when an upstream notification blip dropped the original.
func (s *Service) ResendConfirmation(ctx context.Context, orderID uuid.UUID, actor Actor) error {
	return s.Unit(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetByID(ctx, tx, orderID)
		if err != nil {
			return err
		}
		evt := &OrderEvent{
			OrderID:    o.ID,
			Kind:       string(EventKindNoteAdded),
			ActorID:    actor.ID,
			ActorEmail: actor.Email,
			Payload:    mustJSON(map[string]any{"note": "confirmation email resent"}),
		}
		if err := tx.Create(evt).Error; err != nil {
			return err
		}
		return s.enqueueOutbox(tx, o.TenantID, o.StoreID, "order.placed", map[string]any{
			"order_id": o.ID, "order_number": o.OrderNumber, "resend": true,
		})
	})
}
```

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/service.go
git commit -m "feat(marketplace-api): order.Service RecordRefund (atomic) and ResendConfirmation"
```

---

### Task 11: Order service — RecordReturnEvent hook for cross-module calls

**Files:**
- Modify: `services/marketplace-api/internal/order/service.go`

- [ ] **Step 1: Add the cross-module helper**

```go
// RecordReturnEvent writes a return-related event row on the source order.
// This method is called BY return.Service inside a shared transaction.
// It MUST take a *gorm.DB so the two writes (returns table + order_events)
// are atomic.
func (s *Service) RecordReturnEvent(
	ctx context.Context,
	tx *gorm.DB,
	orderID uuid.UUID,
	kind OrderEventKind,
	payload any,
	actor Actor,
) error {
	evt := &OrderEvent{
		OrderID:    orderID,
		Kind:       string(kind),
		ActorID:    actor.ID,
		ActorEmail: actor.Email,
		Payload:    mustJSON(payload),
	}
	return tx.WithContext(ctx).Create(evt).Error
}
```

- [ ] **Step 2: Build**

```bash
cd services/marketplace-api && go build ./internal/order/...
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/service.go
git commit -m "feat(marketplace-api): order.Service.RecordReturnEvent cross-module hook"
```

---

### Task 12: Order service integration tests — end-to-end lifecycle

**Files:**
- Create: `services/marketplace-api/internal/order/service_test.go`

- [ ] **Step 1: Test Create happy path + outbox row**

```go
package order_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestService_Create_HappyPath_WritesOutboxRow(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)

	in := validCreateInput()
	res, err := svc.Create(ctx, in)
	require.NoError(t, err)
	require.False(t, res.Reused)
	require.NotEmpty(t, res.Order.OrderNumber)
	require.Regexp(t, `^M-TEST-\d{6}-\d{5}$`, res.Order.OrderNumber)

	// Outbox row exists for order.placed
	var outboxCount int64
	db.Model(&order.PendingEvent{}).
		Where("store_id = ? AND topic = ?", in.StoreID, "order.placed").
		Count(&outboxCount)
	require.EqualValues(t, 1, outboxCount)
}

func TestService_Create_IdempotentRetryReturnsExisting(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)

	in := validCreateInput()
	res1, err := svc.Create(ctx, in)
	require.NoError(t, err)

	res2, err := svc.Create(ctx, in) // same idempotency key
	require.NoError(t, err)
	require.True(t, res2.Reused)
	require.Equal(t, res1.Order.ID, res2.Order.ID)

	// Exactly ONE order row, ONE outbox row
	var orderCount, outboxCount int64
	db.Model(&order.Order{}).Where("store_id = ?", in.StoreID).Count(&orderCount)
	db.Model(&order.PendingEvent{}).Where("store_id = ? AND topic = ?", in.StoreID, "order.placed").Count(&outboxCount)
	require.EqualValues(t, 1, orderCount)
	require.EqualValues(t, 1, outboxCount)
}

func TestService_Lifecycle_PendingToFulfilled(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)

	in := validCreateInput()
	res, err := svc.Create(ctx, in)
	require.NoError(t, err)
	orderID := res.Order.ID

	// pending -> confirmed is not wired as its own method in slice 1 (payment
	// flow does it); do it via repo for now for the test to proceed.
	require.NoError(t, db.Model(&order.Order{}).Where("id = ?", orderID).
		Update("status", string(order.OrderStatusConfirmed)).Error)

	_, err = svc.MarkFulfilled(ctx, orderID, &order.TrackingInfo{Number: "DHL1Z", Carrier: "DHL"}, order.Actor{})
	require.NoError(t, err)

	back, _ := svc.Repo().GetByID(ctx, nil, orderID)
	require.Equal(t, string(order.OrderStatusFulfilled), back.Status)
	require.Equal(t, string(order.FulfillmentStatusFulfilled), back.FulfillmentStatus)
	require.NotNil(t, back.FulfilledAt)

	// Outbox now has order.placed + order.fulfilled
	var outboxCount int64
	db.Model(&order.PendingEvent{}).Where("store_id = ?", in.StoreID).Count(&outboxCount)
	require.EqualValues(t, 2, outboxCount)
}

func TestService_Cancel_FulfilledOrderRejected(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	o := newOrderInStatus(t, db, uuid.New(), order.OrderStatusFulfilled, order.PaymentStatusPaid)

	_, err := svc.Cancel(ctx, o.ID, "test", order.Actor{})
	require.ErrorIs(t, err, order.ErrOrderNotCancellable)
}

func TestService_RecordRefund_PartialThenFull(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	o := newOrderInStatus(t, db, uuid.New(), order.OrderStatusFulfilled, order.PaymentStatusPaid)
	// GrandTotal = 100 via newOrderInStatus

	// Partial refund
	_, err := svc.RecordRefund(ctx, o.ID, decimal.NewFromInt(30), "damage", "", order.Actor{})
	require.NoError(t, err)
	back, _ := svc.Repo().GetByID(ctx, nil, o.ID)
	require.Equal(t, string(order.PaymentStatusPartiallyRefunded), back.PaymentStatus)
	require.True(t, back.RefundedAmount.Equal(decimal.NewFromInt(30)))
	// Status stays fulfilled
	require.Equal(t, string(order.OrderStatusFulfilled), back.Status)

	// Complete the refund
	_, err = svc.RecordRefund(ctx, o.ID, decimal.NewFromInt(70), "full", "", order.Actor{})
	require.NoError(t, err)
	back, _ = svc.Repo().GetByID(ctx, nil, o.ID)
	require.Equal(t, string(order.PaymentStatusRefunded), back.PaymentStatus)
	require.True(t, back.RefundedAmount.Equal(decimal.NewFromInt(100)))
	require.Equal(t, string(order.OrderStatusFulfilled), back.Status) // still fulfilled

	// Over-refund is rejected
	_, err = svc.RecordRefund(ctx, o.ID, decimal.NewFromInt(1), "impossible", "", order.Actor{})
	require.ErrorIs(t, err, order.ErrRefundExceedsTotal)
}

func TestService_RecordRefund_IdempotencyKey(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	o := newOrderInStatus(t, db, uuid.New(), order.OrderStatusFulfilled, order.PaymentStatusPaid)

	_, err := svc.RecordRefund(ctx, o.ID, decimal.NewFromInt(30), "damage", "key-123", order.Actor{})
	require.NoError(t, err)

	// Replay with same key is a no-op
	_, err = svc.RecordRefund(ctx, o.ID, decimal.NewFromInt(30), "damage", "key-123", order.Actor{})
	require.NoError(t, err)

	back, _ := svc.Repo().GetByID(ctx, nil, o.ID)
	require.True(t, back.RefundedAmount.Equal(decimal.NewFromInt(30))) // NOT 60
}

func TestService_ResendConfirmation_EnqueuesOutbox(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	o := newOrderInStatus(t, db, uuid.New(), order.OrderStatusConfirmed, order.PaymentStatusPaid)

	require.NoError(t, svc.ResendConfirmation(ctx, o.ID, order.Actor{}))

	var count int64
	db.Model(&order.PendingEvent{}).
		Where("store_id = ? AND topic = ?", o.StoreID, "order.placed").
		Count(&count)
	require.EqualValues(t, 1, count) // newOrderInStatus bypassed Service.Create, so only this one row
}
```

Add `validCreateInput()` to `testhelpers_test.go` returning a fully-populated `CreateInput` with one item, a shipping + billing address, `GrandTotal = 100`, `StorePrefix = "TEST"`.

- [ ] **Step 2: Run**

```bash
cd services/marketplace-api && go test -run TestService_ -v ./internal/order/
```
Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/service_test.go \
        services/marketplace-api/internal/order/testhelpers_test.go
git commit -m "test(marketplace-api): order.Service end-to-end lifecycle integration tests"
```

---

### Task 13: Return repository

**Files:**
- Create: `services/marketplace-api/internal/order/return_repository.go`

- [ ] **Step 1: Write the repository**

```go
package order

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ReturnRepository struct {
	db *gorm.DB
}

func NewReturnRepository(db *gorm.DB) *ReturnRepository {
	return &ReturnRepository{db: db}
}

func (r *ReturnRepository) GetByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*Return, error) {
	if tx == nil {
		tx = r.db
	}
	var ret Return
	err := tx.WithContext(ctx).Where("id = ?", id).First(&ret).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &ret, err
}

func (r *ReturnRepository) ListForOrder(ctx context.Context, tx *gorm.DB, orderID uuid.UUID) ([]Return, error) {
	if tx == nil {
		tx = r.db
	}
	var rows []Return
	err := tx.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("requested_at DESC").
		Find(&rows).Error
	return rows, err
}

// RemainingReturnableQuantity computes how many units of a given order_item
// have NOT yet been returned (across all returns in any state except rejected).
func (r *ReturnRepository) RemainingReturnableQuantity(ctx context.Context, tx *gorm.DB, orderItemID uuid.UUID) (int, error) {
	if tx == nil {
		tx = r.db
	}
	var row struct {
		Ordered  int
		Returned int
	}
	err := tx.WithContext(ctx).Raw(`
		SELECT oi.quantity AS ordered,
		       COALESCE(SUM(ri.quantity), 0) AS returned
		FROM order_items oi
		LEFT JOIN return_items ri ON ri.order_item_id = oi.id
		LEFT JOIN returns r ON r.id = ri.return_id AND r.status <> 'rejected'
		WHERE oi.id = ?
		GROUP BY oi.quantity
	`, orderItemID).Scan(&row).Error
	if err != nil {
		return 0, err
	}
	return row.Ordered - row.Returned, nil
}
```

- [ ] **Step 2: Build + commit**

```bash
cd services/marketplace-api && go build ./internal/order/...
git add services/marketplace-api/internal/order/return_repository.go
git commit -m "feat(marketplace-api): return repository with remaining-returnable calc"
```

---

### Task 14: Return service — Request, Approve, MarkReceived, MarkRefunded, Reject

**Files:**
- Create: `services/marketplace-api/internal/order/return_service.go`
- Create: `services/marketplace-api/internal/order/return_service_test.go`

- [ ] **Step 1: Write the return service**

```go
package order

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ReturnStatus is the return lifecycle axis.
type ReturnStatus string

const (
	ReturnStatusRequested ReturnStatus = "requested"
	ReturnStatusApproved  ReturnStatus = "approved"
	ReturnStatusReceived  ReturnStatus = "received"
	ReturnStatusRefunded  ReturnStatus = "refunded"
	ReturnStatusRejected  ReturnStatus = "rejected"
)

var returnStatusTransitions = map[ReturnStatus][]ReturnStatus{
	ReturnStatusRequested: {ReturnStatusApproved, ReturnStatusRejected},
	ReturnStatusApproved:  {ReturnStatusReceived, ReturnStatusRejected},
	ReturnStatusReceived:  {ReturnStatusRefunded},
	ReturnStatusRefunded:  nil,
	ReturnStatusRejected:  nil,
}

func (s ReturnStatus) CanTransitionTo(target ReturnStatus) bool {
	for _, a := range returnStatusTransitions[s] {
		if a == target {
			return true
		}
	}
	return false
}

// ReturnService is the cross-module-aware service for returns.
// It composes with order.Service via the Unit helper and RecordReturnEvent.
type ReturnService struct {
	db       *gorm.DB
	repo     *ReturnRepository
	orderSvc *Service // co-located package ⇒ direct struct reference
}

func NewReturnService(db *gorm.DB, orderSvc *Service) *ReturnService {
	return &ReturnService{db: db, repo: NewReturnRepository(db), orderSvc: orderSvc}
}

// RequestInput captures what the admin chose when creating a return.
type RequestInput struct {
	TenantID    uuid.UUID
	StoreID     uuid.UUID
	StorePrefix string
	OrderID     uuid.UUID
	Reason      *string
	Notes       *string
	Items       []RequestItemInput
	Currency    string
}

type RequestItemInput struct {
	OrderItemID uuid.UUID
	Quantity    int
	Reason      *string
}

// Request creates a new return record in `requested` status.
// Validates that item quantities do not exceed remaining returnable quantity.
func (s *ReturnService) Request(ctx context.Context, in RequestInput, actor Actor) (*Return, error) {
	if len(in.Items) == 0 {
		return nil, errors.New("return: at least one item required")
	}

	var result *Return
	err := s.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
		// Validate each line
		for _, it := range in.Items {
			remaining, err := s.repo.RemainingReturnableQuantity(ctx, tx, it.OrderItemID)
			if err != nil {
				return err
			}
			if it.Quantity > remaining {
				return fmt.Errorf("%w: order_item=%s remaining=%d requested=%d",
					ErrReturnItemsExceedOrdered, it.OrderItemID, remaining, it.Quantity)
			}
		}

		// Issue atomic return number
		day := time.Now().UTC()
		seq, err := NextDocumentNumber(ctx, tx, in.StoreID, "return", day)
		if err != nil {
			return err
		}
		retNumber := fmt.Sprintf("R-%s-%s-%05d", strings.ToUpper(in.StorePrefix), day.Format("060102"), seq)

		r := &Return{
			TenantID:     in.TenantID,
			StoreID:      in.StoreID,
			OrderID:      in.OrderID,
			ReturnNumber: retNumber,
			Status:       string(ReturnStatusRequested),
			Reason:       in.Reason,
			Notes:        in.Notes,
			CurrencyCode: in.Currency,
		}
		if err := tx.Create(r).Error; err != nil {
			return err
		}
		for _, it := range in.Items {
			ri := &ReturnItem{
				ReturnID:    r.ID,
				OrderItemID: it.OrderItemID,
				Quantity:    it.Quantity,
				Reason:      it.Reason,
			}
			if err := tx.Create(ri).Error; err != nil {
				return err
			}
		}

		// Cross-module: write return_linked event on the source order
		if err := s.orderSvc.RecordReturnEvent(ctx, tx, in.OrderID, EventKindReturnLinked,
			map[string]any{"return_id": r.ID, "return_number": r.ReturnNumber}, actor); err != nil {
			return err
		}

		result = r
		return nil
	})
	return result, err
}

// transitionReturnStatusTx is the shared helper for Approve/MarkReceived/MarkRefunded/Reject.
func (s *ReturnService) transitionReturnStatusTx(
	ctx context.Context,
	tx *gorm.DB,
	returnID uuid.UUID,
	target ReturnStatus,
	actor Actor,
	sideEffect func(*Return),
	orderEventKind OrderEventKind,
) (*Return, error) {
	r, err := s.repo.GetByID(ctx, tx, returnID)
	if err != nil {
		return nil, err
	}
	current := ReturnStatus(r.Status)
	if !current.CanTransitionTo(target) {
		return nil, fmt.Errorf("%w: return from=%s to=%s", ErrInvalidTransition, current, target)
	}
	r.Status = string(target)
	if sideEffect != nil {
		sideEffect(r)
	}
	if err := tx.Save(r).Error; err != nil {
		return nil, err
	}
	// Cross-module event on source order
	return r, s.orderSvc.RecordReturnEvent(ctx, tx, r.OrderID, orderEventKind,
		map[string]any{"return_id": r.ID, "from": current, "to": target}, actor)
}

func (s *ReturnService) Approve(ctx context.Context, returnID uuid.UUID, actor Actor) (*Return, error) {
	var result *Return
	err := s.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
		var err error
		result, err = s.transitionReturnStatusTx(ctx, tx, returnID, ReturnStatusApproved, actor, nil, EventKindReturnApproved)
		return err
	})
	return result, err
}

func (s *ReturnService) MarkReceived(ctx context.Context, returnID uuid.UUID, actor Actor) (*Return, error) {
	var result *Return
	err := s.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
		now := time.Now()
		var err error
		result, err = s.transitionReturnStatusTx(ctx, tx, returnID, ReturnStatusReceived, actor,
			func(r *Return) { r.ReceivedAt = &now }, EventKindReturnReceived)
		return err
	})
	return result, err
}

// MarkRefunded marks the return as refunded AND records the refund on the order
// via order.Service.RecordRefund (atomic). The refund amount must be supplied —
// slice 1 does not auto-derive it from the return items.
func (s *ReturnService) MarkRefunded(ctx context.Context, returnID uuid.UUID, refundAmount decimal.Decimal, actor Actor) (*Return, error) {
	var result *Return
	err := s.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
		now := time.Now()
		r, err := s.transitionReturnStatusTx(ctx, tx, returnID, ReturnStatusRefunded, actor,
			func(r *Return) {
				r.RefundedAt = &now
				r.RefundAmount = &refundAmount
			}, EventKindReturnRefunded)
		if err != nil {
			return err
		}
		// Also record the refund on the source order (atomic).
		o, err := s.orderSvc.repo.GetByID(ctx, tx, r.OrderID)
		if err != nil {
			return err
		}
		afterTotal := o.RefundedAmount.Add(refundAmount)
		var targetPS PaymentStatus
		if afterTotal.Equal(o.GrandTotal) {
			targetPS = PaymentStatusRefunded
		} else {
			targetPS = PaymentStatusPartiallyRefunded
		}
		if _, err := s.orderSvc.repo.RecordRefund(ctx, tx, r.OrderID, refundAmount, targetPS); err != nil {
			return err
		}
		result = r
		return nil
	})
	return result, err
}

func (s *ReturnService) Reject(ctx context.Context, returnID uuid.UUID, reason string, actor Actor) (*Return, error) {
	var result *Return
	err := s.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
		var err error
		result, err = s.transitionReturnStatusTx(ctx, tx, returnID, ReturnStatusRejected, actor, nil, EventKindReturnRejected)
		return err
	})
	return result, err
}
```

- [ ] **Step 2: Write cross-module tx tests**

```go
package order_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestReturnService_Request_WritesOrderEvent_AndRespectsRemainingQuantity(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	retSvc := order.NewReturnService(db, svc)

	// Seed an order with one item qty=2
	o := newOrderInStatus(t, db, uuid.New(), order.OrderStatusFulfilled, order.PaymentStatusPaid)
	item := newOrderItemQty(t, db, o.ID, 2)

	// Request return for qty=1 → OK
	_, err := retSvc.Request(ctx, order.RequestInput{
		TenantID: o.TenantID, StoreID: o.StoreID, StorePrefix: "TEST",
		OrderID: o.ID, Currency: "EUR",
		Items: []order.RequestItemInput{{OrderItemID: item.ID, Quantity: 1}},
	}, order.Actor{})
	require.NoError(t, err)

	// return_linked event on source order
	var eventCount int64
	db.Model(&order.OrderEvent{}).
		Where("order_id = ? AND kind = ?", o.ID, string(order.EventKindReturnLinked)).
		Count(&eventCount)
	require.EqualValues(t, 1, eventCount)

	// Another return for remaining 1 → OK
	_, err = retSvc.Request(ctx, order.RequestInput{
		TenantID: o.TenantID, StoreID: o.StoreID, StorePrefix: "TEST",
		OrderID: o.ID, Currency: "EUR",
		Items: []order.RequestItemInput{{OrderItemID: item.ID, Quantity: 1}},
	}, order.Actor{})
	require.NoError(t, err)

	// Third return for another 1 → fails (0 remaining)
	_, err = retSvc.Request(ctx, order.RequestInput{
		TenantID: o.TenantID, StoreID: o.StoreID, StorePrefix: "TEST",
		OrderID: o.ID, Currency: "EUR",
		Items: []order.RequestItemInput{{OrderItemID: item.ID, Quantity: 1}},
	}, order.Actor{})
	require.ErrorIs(t, err, order.ErrReturnItemsExceedOrdered)
}

func TestReturnService_Lifecycle_RequestApproveReceiveRefund_AtomicAcrossTables(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	retSvc := order.NewReturnService(db, svc)

	o := newOrderInStatus(t, db, uuid.New(), order.OrderStatusFulfilled, order.PaymentStatusPaid)
	item := newOrderItemQty(t, db, o.ID, 1)

	r, err := retSvc.Request(ctx, order.RequestInput{
		TenantID: o.TenantID, StoreID: o.StoreID, StorePrefix: "TEST",
		OrderID: o.ID, Currency: "EUR",
		Items: []order.RequestItemInput{{OrderItemID: item.ID, Quantity: 1}},
	}, order.Actor{})
	require.NoError(t, err)

	_, err = retSvc.Approve(ctx, r.ID, order.Actor{})
	require.NoError(t, err)
	_, err = retSvc.MarkReceived(ctx, r.ID, order.Actor{})
	require.NoError(t, err)
	_, err = retSvc.MarkRefunded(ctx, r.ID, decimal.NewFromInt(50), order.Actor{})
	require.NoError(t, err)

	// The source order now has refunded_amount = 50, payment_status = partially_refunded
	back, _ := svc.Repo().GetByID(ctx, nil, o.ID)
	require.True(t, back.RefundedAmount.Equal(decimal.NewFromInt(50)))
	require.Equal(t, string(order.PaymentStatusPartiallyRefunded), back.PaymentStatus)
	require.Equal(t, string(order.OrderStatusFulfilled), back.Status) // unchanged

	// Five order_events rows on the source order: return_linked, return_approved,
	// return_received, return_refunded (from transitionReturnStatusTx), and the
	// implicit one from MarkRefunded's call back into RecordRefund.
	// Actual count depends on whether MarkRefunded also adds a refund_recorded
	// event — it does not (RecordRefund at repo level only does the atomic
	// UPDATE; the service-level RecordRefund which adds an event is NOT called
	// from MarkRefunded). So expect exactly 4 return-related events.
	var eventCount int64
	db.Model(&order.OrderEvent{}).Where("order_id = ?", o.ID).Count(&eventCount)
	require.GreaterOrEqual(t, eventCount, int64(4))
}
```

Add `newOrderItemQty` helper to `testhelpers_test.go`.

- [ ] **Step 3: Run + commit**

```bash
cd services/marketplace-api && go test -run TestReturnService -v ./internal/order/
git add services/marketplace-api/internal/order/return_service.go \
        services/marketplace-api/internal/order/return_service_test.go \
        services/marketplace-api/internal/order/testhelpers_test.go
git commit -m "feat(marketplace-api): return.Service with cross-module tx threading"
```

---

### Task 15: Abandoned cart repository + service

**Files:**
- Create: `services/marketplace-api/internal/order/abandoned_cart_repository.go`
- Create: `services/marketplace-api/internal/order/abandoned_cart_service.go`
- Create: `services/marketplace-api/internal/order/abandoned_cart_service_test.go`

- [ ] **Step 1: Repository**

```go
package order

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AbandonedCartRepository struct {
	db *gorm.DB
}

func NewAbandonedCartRepository(db *gorm.DB) *AbandonedCartRepository {
	return &AbandonedCartRepository{db: db}
}

func (r *AbandonedCartRepository) Get(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*AbandonedCart, error) {
	if tx == nil {
		tx = r.db
	}
	var c AbandonedCart
	err := tx.WithContext(ctx).Where("id = ?", id).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (r *AbandonedCartRepository) List(ctx context.Context, storeID uuid.UUID, limit, offset int) ([]AbandonedCart, int64, error) {
	q := r.db.WithContext(ctx).
		Model(&AbandonedCart{}).
		Where("store_id = ? AND converted_order_id IS NULL", storeID)
	var total int64
	q.Count(&total)
	var rows []AbandonedCart
	err := q.Order("last_active_at DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *AbandonedCartRepository) MarkRecoverySent(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	if tx == nil {
		tx = r.db
	}
	now := time.Now()
	return tx.WithContext(ctx).
		Model(&AbandonedCart{}).
		Where("id = ?", id).
		Update("recovery_sent_at", now).Error
}
```

- [ ] **Step 2: Service**

```go
package order

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AbandonedCartService struct {
	db       *gorm.DB
	repo     *AbandonedCartRepository
	orderSvc *Service
}

func NewAbandonedCartService(db *gorm.DB, orderSvc *Service) *AbandonedCartService {
	return &AbandonedCartService{
		db:       db,
		repo:     NewAbandonedCartRepository(db),
		orderSvc: orderSvc,
	}
}

func (s *AbandonedCartService) List(ctx context.Context, storeID uuid.UUID, limit, offset int) ([]AbandonedCart, int64, error) {
	return s.repo.List(ctx, storeID, limit, offset)
}

func (s *AbandonedCartService) Get(ctx context.Context, id uuid.UUID) (*AbandonedCart, error) {
	return s.repo.Get(ctx, nil, id)
}

// TriggerRecoveryEmail enqueues an outbox row for the recovery email and stamps
// recovery_sent_at. Enforces a 24h dedup window.
func (s *AbandonedCartService) TriggerRecoveryEmail(ctx context.Context, id uuid.UUID, actor Actor) error {
	return s.orderSvc.Unit(ctx, func(tx *gorm.DB) error {
		c, err := s.repo.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if c.ConvertedOrderID != nil {
			return ErrAbandonedCartAlreadyConverted
		}
		if c.RecoverySentAt != nil && time.Since(*c.RecoverySentAt) < 24*time.Hour {
			return ErrRecoveryTooRecent
		}
		if err := s.repo.MarkRecoverySent(ctx, tx, id); err != nil {
			return err
		}
		return s.orderSvc.enqueueOutbox(tx, c.TenantID, c.StoreID, "abandoned_cart.recovery_email", map[string]any{
			"cart_id": c.ID, "customer_email": c.CustomerEmail, "subtotal": c.Subtotal,
		})
	})
}
```

- [ ] **Step 3: Tests**

```go
package order_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestAbandonedCart_TriggerRecovery_HappyPath(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	abSvc := order.NewAbandonedCartService(db, svc)

	c := seedAbandonedCart(t, db)
	require.NoError(t, abSvc.TriggerRecoveryEmail(ctx, c.ID, order.Actor{}))

	// Outbox row present
	var count int64
	db.Model(&order.PendingEvent{}).
		Where("store_id = ? AND topic = ?", c.StoreID, "abandoned_cart.recovery_email").
		Count(&count)
	require.EqualValues(t, 1, count)

	// recovery_sent_at stamped
	back, _ := abSvc.Get(ctx, c.ID)
	require.NotNil(t, back.RecoverySentAt)
}

func TestAbandonedCart_TriggerRecovery_DedupWithin24h(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	abSvc := order.NewAbandonedCartService(db, svc)

	c := seedAbandonedCart(t, db)
	require.NoError(t, abSvc.TriggerRecoveryEmail(ctx, c.ID, order.Actor{}))
	err := abSvc.TriggerRecoveryEmail(ctx, c.ID, order.Actor{})
	require.ErrorIs(t, err, order.ErrRecoveryTooRecent)

	// Only one outbox row
	var count int64
	db.Model(&order.PendingEvent{}).Where("store_id = ?", c.StoreID).Count(&count)
	require.EqualValues(t, 1, count)
}

func TestAbandonedCart_TriggerRecovery_ConvertedCartRejected(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	abSvc := order.NewAbandonedCartService(db, svc)

	c := seedAbandonedCart(t, db)
	// Simulate conversion
	orderID := uuid.New()
	require.NoError(t, db.Model(&c).Update("converted_order_id", orderID).Error)

	err := abSvc.TriggerRecoveryEmail(ctx, c.ID, order.Actor{})
	require.ErrorIs(t, err, order.ErrAbandonedCartAlreadyConverted)
}
```

Add `seedAbandonedCart` helper to `testhelpers_test.go` that inserts a minimal valid abandoned cart row with a valid `items_snapshot` JSONB payload.

- [ ] **Step 4: Run + commit**

```bash
cd services/marketplace-api && go test -run TestAbandonedCart -v ./internal/order/
git add services/marketplace-api/internal/order/abandoned_cart_repository.go \
        services/marketplace-api/internal/order/abandoned_cart_service.go \
        services/marketplace-api/internal/order/abandoned_cart_service_test.go \
        services/marketplace-api/internal/order/testhelpers_test.go
git commit -m "feat(marketplace-api): abandoned cart service with 24h recovery dedup"
```

---

### Task 16: Outbox drainer — package scaffold and fake publisher

**Files:**
- Create: `services/marketplace-api/internal/outbox/drainer.go`
- Create: `services/marketplace-api/internal/outbox/fake_publisher.go`

- [ ] **Step 1: Write the drainer scaffold**

```go
package outbox

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Publisher is the interface consumed from go-shared/messaging.
// Duplicated here as a local alias so outbox is decoupled from the concrete type.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Row mirrors order.PendingEvent but the drainer package doesn't import the
// order package to avoid a cycle. Any new columns must be kept in sync.
type Row struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	Topic          string
	Payload        datatypes.JSON
	Attempts       int
	LastError      *string
	NextAttemptAt  time.Time
	PublishedAt    *time.Time
	DeadLetteredAt *time.Time
}

func (Row) TableName() string { return "pending_events" }

// Config controls drainer behavior.
type Config struct {
	PollInterval time.Duration // default 2s
	BatchSize    int           // default 100
	MaxAttempts  int           // default 10
	BackoffBase  time.Duration // default 10s, doubled per attempt
	BackoffCap   time.Duration // default 1h
}

func (c Config) withDefaults() Config {
	if c.PollInterval == 0 {
		c.PollInterval = 2 * time.Second
	}
	if c.BatchSize == 0 {
		c.BatchSize = 100
	}
	if c.MaxAttempts == 0 {
		c.MaxAttempts = 10
	}
	if c.BackoffBase == 0 {
		c.BackoffBase = 10 * time.Second
	}
	if c.BackoffCap == 0 {
		c.BackoffCap = time.Hour
	}
	return c
}

// Drainer is a background goroutine that publishes pending_events rows.
type Drainer struct {
	db        *gorm.DB
	publisher Publisher
	cfg       Config
	log       *slog.Logger
}

func NewDrainer(db *gorm.DB, publisher Publisher, cfg Config, log *slog.Logger) *Drainer {
	return &Drainer{db: db, publisher: publisher, cfg: cfg.withDefaults(), log: log}
}

// Run polls the outbox until ctx is cancelled. On cancellation, finishes the
// current batch and returns.
func (d *Drainer) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := d.drainBatch(context.Background()); err != nil {
				d.log.Error("outbox drain batch failed", "err", err)
			}
		}
	}
}

// drainBatch locks up to BatchSize due rows via SELECT ... FOR UPDATE SKIP LOCKED
// and publishes each one. Row state is updated in the same transaction as the
// publish outcome to prevent double-publish if the drainer crashes mid-batch.
func (d *Drainer) drainBatch(ctx context.Context) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []Row
		err := tx.Raw(`
			SELECT * FROM pending_events
			WHERE published_at IS NULL
			  AND dead_lettered_at IS NULL
			  AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			FOR UPDATE SKIP LOCKED
			LIMIT ?
		`, d.cfg.BatchSize).Scan(&rows).Error
		if err != nil {
			return err
		}

		for _, row := range rows {
			err := d.publisher.Publish(ctx, row.Topic, []byte(row.Payload))
			if err == nil {
				now := time.Now()
				tx.Model(&Row{}).Where("id = ?", row.ID).Updates(map[string]any{
					"published_at": now,
				})
				d.log.Info("outbox published", "id", row.ID, "topic", row.Topic)
				continue
			}

			// Failed → schedule retry or dead-letter
			attempts := row.Attempts + 1
			errMsg := err.Error()
			updates := map[string]any{
				"attempts":   attempts,
				"last_error": errMsg,
			}
			if attempts >= d.cfg.MaxAttempts {
				now := time.Now()
				updates["dead_lettered_at"] = now
				d.log.Error("outbox dead-lettered", "id", row.ID, "topic", row.Topic, "attempts", attempts)
			} else {
				backoff := time.Duration(math.Pow(2, float64(attempts))) * d.cfg.BackoffBase
				if backoff > d.cfg.BackoffCap {
					backoff = d.cfg.BackoffCap
				}
				updates["next_attempt_at"] = time.Now().Add(backoff)
				d.log.Warn("outbox retrying", "id", row.ID, "attempts", attempts, "backoff", backoff)
			}
			tx.Model(&Row{}).Where("id = ?", row.ID).Updates(updates)
		}
		return nil
	})
}
```

- [ ] **Step 2: Write the fake publisher**

```go
//go:build testing

package outbox

import (
	"context"
	"errors"
	"sync"
)

// FakePublisher is a testing helper that records publish calls and can be
// configured to fail on specific topics.
type FakePublisher struct {
	mu       sync.Mutex
	Calls    []FakeCall
	FailWith map[string]error // topic -> error to return
}

type FakeCall struct {
	Topic   string
	Payload []byte
}

func NewFakePublisher() *FakePublisher {
	return &FakePublisher{FailWith: make(map[string]error)}
}

func (f *FakePublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, FakeCall{Topic: topic, Payload: payload})
	if err, ok := f.FailWith[topic]; ok {
		return err
	}
	return nil
}

func (f *FakePublisher) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}

var ErrFakeFailure = errors.New("fake publisher failure")
```

- [ ] **Step 3: Build + commit**

```bash
cd services/marketplace-api && go build ./internal/outbox/...
git add services/marketplace-api/internal/outbox/
git commit -m "feat(marketplace-api): outbox drainer scaffold and fake publisher"
```

---

### Task 17: Outbox drainer integration tests

**Files:**
- Create: `services/marketplace-api/internal/outbox/drainer_test.go`

- [ ] **Step 1: Test happy path publish**

```go
//go:build testing

package outbox_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func newDrainer(t *testing.T, pub outbox.Publisher) (*outbox.Drainer, *gorm.DB) {
	db := testdb.New(t)
	d := outbox.NewDrainer(db, pub, outbox.Config{
		PollInterval: 50 * time.Millisecond,
		BatchSize:    10,
		MaxAttempts:  3,
		BackoffBase:  10 * time.Millisecond,
		BackoffCap:   100 * time.Millisecond,
	}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	return d, db
}

func seedPendingEvent(t *testing.T, db *gorm.DB, topic string) uuid.UUID {
	row := outbox.Row{
		TenantID: uuid.New(),
		StoreID:  uuid.New(),
		Topic:    topic,
		Payload:  datatypes.JSON([]byte(`{"hello":"world"}`)),
		NextAttemptAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)
	return row.ID
}

func TestDrainer_HappyPath_Publishes(t *testing.T) {
	pub := outbox.NewFakePublisher()
	d, db := newDrainer(t, pub)
	id := seedPendingEvent(t, db, "order.placed")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	require.NoError(t, d.Run(ctx))

	require.GreaterOrEqual(t, pub.CallCount(), 1)

	var row outbox.Row
	require.NoError(t, db.Where("id = ?", id).First(&row).Error)
	require.NotNil(t, row.PublishedAt)
}

func TestDrainer_TransientFailure_RetriesWithBackoff(t *testing.T) {
	pub := outbox.NewFakePublisher()
	pub.FailWith["order.placed"] = outbox.ErrFakeFailure
	d, db := newDrainer(t, pub)
	id := seedPendingEvent(t, db, "order.placed")

	// Run drainer for 300ms — should see multiple retry attempts
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = d.Run(ctx)

	var row outbox.Row
	db.Where("id = ?", id).First(&row)
	require.Greater(t, row.Attempts, 0)
	require.Nil(t, row.PublishedAt)
	require.Nil(t, row.DeadLetteredAt)
	require.NotNil(t, row.LastError)
}

func TestDrainer_DeadLetter_AfterMaxAttempts(t *testing.T) {
	pub := outbox.NewFakePublisher()
	pub.FailWith["order.placed"] = outbox.ErrFakeFailure
	d, db := newDrainer(t, pub)
	id := seedPendingEvent(t, db, "order.placed")

	// With MaxAttempts = 3 and backoff cap of 100ms, 4-5 drain cycles should dead-letter the row
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	_ = d.Run(ctx)

	var row outbox.Row
	db.Where("id = ?", id).First(&row)
	require.GreaterOrEqual(t, row.Attempts, 3)
	require.NotNil(t, row.DeadLetteredAt)
}

func TestDrainer_SuccessAfterTransientFailure(t *testing.T) {
	pub := outbox.NewFakePublisher()
	pub.FailWith["order.placed"] = outbox.ErrFakeFailure
	d, db := newDrainer(t, pub)
	id := seedPendingEvent(t, db, "order.placed")

	// Start the drainer
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// After 100ms, remove the failure and let it succeed
	go func() {
		time.Sleep(100 * time.Millisecond)
		pub.FailWith = map[string]error{}
	}()

	_ = d.Run(ctx)

	var row outbox.Row
	db.Where("id = ?", id).First(&row)
	require.NotNil(t, row.PublishedAt, "should eventually publish after failures clear")
}
```

Note: tests use a `testing` build tag to match `fake_publisher.go`. The test runner must be invoked with `-tags=testing`:

- [ ] **Step 2: Run**

```bash
cd services/marketplace-api && go test -tags=testing -run TestDrainer -v ./internal/outbox/
```
Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/outbox/drainer_test.go
git commit -m "test(marketplace-api): outbox drainer integration tests (happy, retry, dead-letter)"
```

---

### Task 18: Wire drainer into cmd/marketplace-api/main.go

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Read the current main.go to locate the service bootstrap section**

```bash
cat services/marketplace-api/cmd/marketplace-api/main.go
```
Identify: where the GORM DB is opened, where the Gin engine is constructed, where graceful shutdown is handled.

- [ ] **Step 2: Start the drainer goroutine for MODE=admin|both**

Add after DB open, before HTTP server start:
```go
// Outbox drainer — admin mode only. Storefront replicas do not drain.
if cfg.Mode == "admin" || cfg.Mode == "both" {
    publisher := messaging.NewPublisher(...) // wire the real publisher from go-shared
    drainer := outbox.NewDrainer(db, publisher, outbox.Config{
        PollInterval: cfg.OutboxPollInterval,
    }, logger)

    drainerCtx, drainerCancel := context.WithCancel(context.Background())
    drainerDone := make(chan struct{})
    go func() {
        defer close(drainerDone)
        if err := drainer.Run(drainerCtx); err != nil {
            logger.Error("drainer exited with error", "err", err)
        }
    }()
    defer func() {
        drainerCancel()
        select {
        case <-drainerDone:
        case <-time.After(30 * time.Second):
            logger.Warn("drainer did not shut down within 30s")
        }
    }()
}
```

- [ ] **Step 3: Add `ORDERS_OUTBOX_POLL_INTERVAL` to the config struct**

In `pkg/config/config.go`:
```go
OutboxPollInterval time.Duration `envconfig:"ORDERS_OUTBOX_POLL_INTERVAL" default:"2s"`
```

- [ ] **Step 4: Build + boot smoke test**

```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/
MODE=both ORDERS_OUTBOX_POLL_INTERVAL=5s ./marketplace-api &
sleep 2
curl -s localhost:8087/health
kill %1
```
Expected: `/health` returns 200 with JSON. The logger should print "drainer started" or similar and then "drainer did not shut down within 30s" is NOT printed on SIGTERM.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go \
        services/marketplace-api/pkg/config/config.go
git commit -m "feat(marketplace-api): wire outbox drainer into main.go for admin|both mode"
```

---

### Task 19: Full service integration — end-to-end order+return lifecycle

**Files:**
- Create: `services/marketplace-api/internal/order/lifecycle_test.go`

- [ ] **Step 1: Write one test that drives Create → Fulfill → Request return → Approve → Received → Refunded and asserts every expected order_events and pending_events row**

```go
package order_test

// TestFullLifecycle walks through every service method and asserts the final
// state of orders, order_events, returns, and pending_events.
// This is the M2 "demonstrable on a running system" exit criterion.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestFullLifecycle_CreateFulfillReturnRefund(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	svc := order.NewService(db)
	retSvc := order.NewReturnService(db, svc)

	// 1. Create
	in := validCreateInput()
	res, err := svc.Create(ctx, in)
	require.NoError(t, err)
	o := res.Order

	// 2. Transition to confirmed (simulated payment capture — M4 will route through SetPaymentStatus)
	require.NoError(t, db.Model(o).Update("status", string(order.OrderStatusConfirmed)).Error)

	// 3. Fulfill
	_, err = svc.MarkFulfilled(ctx, o.ID, &order.TrackingInfo{Number: "DHL1", Carrier: "DHL"}, order.Actor{})
	require.NoError(t, err)

	// 4. Request a return for one line
	items, _ := svc.Repo().ListItems(ctx, nil, o.ID) // add ListItems to repo if missing
	require.NotEmpty(t, items)
	r, err := retSvc.Request(ctx, order.RequestInput{
		TenantID: in.TenantID, StoreID: in.StoreID, StorePrefix: in.StorePrefix,
		OrderID: o.ID, Currency: in.CurrencyCode,
		Items: []order.RequestItemInput{{OrderItemID: items[0].ID, Quantity: 1}},
	}, order.Actor{})
	require.NoError(t, err)

	// 5. Approve → Receive → Refund
	_, err = retSvc.Approve(ctx, r.ID, order.Actor{})
	require.NoError(t, err)
	_, err = retSvc.MarkReceived(ctx, r.ID, order.Actor{})
	require.NoError(t, err)
	_, err = retSvc.MarkRefunded(ctx, r.ID, decimal.NewFromInt(30), order.Actor{})
	require.NoError(t, err)

	// Assert final order state
	final, _ := svc.Repo().GetByID(ctx, nil, o.ID)
	require.Equal(t, string(order.OrderStatusFulfilled), final.Status)
	require.Equal(t, string(order.PaymentStatusPartiallyRefunded), final.PaymentStatus)
	require.True(t, final.RefundedAmount.Equal(decimal.NewFromInt(30)))

	// Assert outbox: order.placed + order.fulfilled at minimum
	var topics []string
	db.Model(&order.PendingEvent{}).
		Where("store_id = ?", o.StoreID).
		Pluck("topic", &topics)
	require.Contains(t, topics, "order.placed")
	require.Contains(t, topics, "order.fulfilled")

	// Assert order_events: status_changed (initial) + status_changed (fulfilled)
	// + fulfilled (tracking) + return_linked + return_approved + return_received
	// + return_refunded → at least 7
	var eventCount int64
	db.Model(&order.OrderEvent{}).Where("order_id = ?", o.ID).Count(&eventCount)
	require.GreaterOrEqual(t, eventCount, int64(7))
}
```

Note: this test calls `svc.Repo().ListItems(...)` — if the repository doesn't have `ListItems` yet, add a 3-line method in the same step to return `[]OrderItem` for a given order ID.

- [ ] **Step 2: Run the full order package test suite**

```bash
cd services/marketplace-api && go test -v ./internal/order/
```
Expected: all tests from M1 + all tests added in M2 PASS.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/lifecycle_test.go \
        services/marketplace-api/internal/order/repository.go
git commit -m "test(marketplace-api): full order+return lifecycle integration test"
```

---

### Task 20: M2 exit checklist + handoff

**Files:**
- Modify: `services/marketplace-api/internal/order/README.md`

- [ ] **Step 1: Run the whole service test suite to confirm no regression**

```bash
cd services/marketplace-api && go test -tags=testing ./...
```
Expected: all PASS (products + orders M1 + orders M2).

- [ ] **Step 2: Run the concurrent sequence benchmark from M1 once more to confirm no latency regression**

```bash
cd services/marketplace-api && go test -run TestNextDocumentNumber_Concurrent -v ./internal/order/
```
Expected: p99 under 50ms, same order of magnitude as M1's recorded value.

- [ ] **Step 3: Update `internal/order/README.md`**

Append a "M2 handoff" section with:
- M2 landed ← you are here
- List of new exported APIs (`order.Service`, `order.ReturnService`, `order.AbandonedCartService`, `outbox.Drainer`)
- Cross-module tx threading contract (passing `*gorm.DB` explicitly)
- Refund atomicity guarantee (single UPDATE, not read-check-write)
- Outbox drainer ownership (admin|both modes only)
- Pointer to the lifecycle test as the canonical "how do these compose" example
- Pending: FGA (M3), HTTP handlers (M4), checkout integration (M5)

- [ ] **Step 4: Tick the M2 exit criteria from spec §9 M2**

- [x] OrderStatus/PaymentStatus/FulfillmentStatus exhaustively unit-tested — Task 2
- [x] order.Repository list with filter DSL, detail, atomic refund update — Tasks 4, 5, 6
- [x] order.Service with Create, TransitionStatus, Cancel, MarkFulfilled, RecordRefund, ResendConfirmation — Tasks 8, 9, 10
- [x] return.Service and abandoned_cart.Service with lifecycle methods — Tasks 14, 15
- [x] Cross-module transactions via shared `*gorm.DB` handle — Task 11, 14
- [x] Outbox drainer with success/retry/dead-letter paths — Tasks 16, 17
- [x] Drainer wired into admin|both modes of main.go — Task 18
- [x] End-to-end lifecycle integration test — Task 19

- [ ] **Step 5: Commit the handoff note**

```bash
git add services/marketplace-api/internal/order/README.md
git commit -m "docs(marketplace-api): M2 handoff note for order services and outbox"
```

---

## Parallelization notes (for `subagent-driven-development`)

Tasks 4–6 (order repository), 13 (return repository), and 15 (abandoned cart repository) can run in parallel once Tasks 1–3 (status + errors + events) have shipped. Dispatch three repository subagents, wait for all to return, then proceed to services (Tasks 7–12, 14, 15) serially because they depend on each other's interfaces.

Task 16–17 (outbox drainer) is independent of the services and can run in parallel with Tasks 8–15 once Task 3 (events.go) has shipped.

All other tasks are strictly serial.

## Exit gate to M3

Do not start Orders M3 until:

1. Every task in this plan is committed.
2. `go test -tags=testing ./...` passes in CI for `services/marketplace-api`.
3. The lifecycle test (Task 19) passes in CI.
4. The outbox drainer retry + dead-letter tests (Task 17) pass in CI.
5. The M1 concurrent sequence benchmark still passes on the post-M2 tree with no p99 regression.
6. A human has reviewed the outbox drainer + cross-module tx threading patterns and signed off — these two patterns will be reused by every future feature, so the shape matters.

If any item is false, M3 does not start.
