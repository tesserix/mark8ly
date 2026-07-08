# Order Refund → Payment Gateway Wiring

**Date:** 2026-07-09
**Status:** Approved (design), pending spec review
**Service:** `services/marketplace-api`
**Author:** Mahesh + Claude

---

## 1. Problem

Every order-refund path in `marketplace-api` mutates database bookkeeping only — it
never moves money. Concretely:

- Admin order refund (`internal/handlers/admin/orders.go:Refund`) → `order.Service.RecordRefund`
- Return refund (`internal/order/return_service.go:MarkRefunded`) → `order.Service.RecordRefund`

`order.Service.RecordRefund` (`internal/order/service.go:335`) does an atomic
`UPDATE orders SET refunded_amount=…, payment_status='refunded'`, writes
`order_events` + `outbox_events`, and **triggers a refund email** — but never calls
`payment.Service.RefundPayment`, the only code that hits Stripe/Razorpay/PayPal
`/refunds`. `payment.Service.RefundPayment` (`internal/payment/service.go:108`) has
**zero non-test callers** — it is orphaned.

Net effect in production: the customer is emailed "you've been refunded", the merchant
sees `refunded_amount` rise, and the card is **never credited**. There is no
`refund_transactions` ledger row on the order path, so no reconciliation and no
protection against double-refunds.

Secondary defects folded into this work:

- **Paid cancellation strands money.** `order.Service.Cancel` never touches
  `payment_status` and never refunds. A customer who paid then self-cancels lands at
  `status=cancelled / payment_status=paid / refunded_amount=0` with no money back.
- **Amount/status mismatch.** `RefundOrderRequest` accepts both `amount` and a
  `payment_status` target from the client; they can disagree (full amount tagged
  `partially_refunded`, or vice-versa) and the server accepts it.
- **Refund cap is against `grand_total`, not captured amount.** The
  `authorized → refunded` transition is legal, so money that was only authorized
  (never captured) can be "refunded" in bookkeeping.

## 2. Goals / Non-goals

**Goals**
- All three refund triggers move real money through the customer's original gateway,
  via the existing `payment.Gateway` interface (Stripe, Razorpay, PayPal — provider
  resolved per order, not hardcoded).
- A refund is **never lost** (retry sweeper) and **never doubled** (provider
  idempotency key).
- Paid, un-shipped self-cancellation issues an automatic full refund.
- Server-side correctness for amount, status, and cap.
- **No change ships without comprehensive automated tests** (see §9). This service is
  in production.

**Non-goals**
- Refund to store credit / gift-card / wallet (future).
- Split-payout / escrow reversal (not present in this marketplace flow).
- COD / manual / offline refunds — explicitly **blocked** with `422 refund_unavailable`.

## 3. Locked decisions

| # | Decision | Choice |
|---|----------|--------|
| 1 | Paid self-cancel behavior | **Auto-refund to original method** (full) |
| 2 | Orders with no gateway transaction (COD/manual) | **Block** → `422 refund_unavailable` |
| 3 | Partial vs full status | **Derive server-side** from amount vs grand_total |
| 4 | Reliability model | **Idempotency key + ledger-first saga + retry sweeper** |
| 5 | Sweeper hosting | **Standalone Cloud Run Job** (`cmd/refund-sweep-cron`, mirrors `cmd/reconciliation-cron`) |
| 6 | Rollout | **Feature flag** `REFUND_GATEWAY_ENABLED` (ship dark, enable per env) |

## 4. Architecture

### 4.1 New component — `RefundCoordinator` (`internal/orderrefund/`)

A single domain service that all refund triggers call, so money-movement logic lives
in exactly one place. Depends only on well-defined interfaces (all fakeable in tests):

- `order.Service` — existing `RecordRefund`, `Cancel`, `loadForUpdate`
- `payment.Service` — gateway call + ledger persistence
- `PaymentTxnLookup` — `payment_transactions` by `order_id` (provider + intent + captured amount)
- `GatewayConfigResolver` — `payment_gateway_configs` by (store, provider, active) → `payment.NewGateway` (reuse the pattern in `internal/giftcard/gateway_resolver.go`)

```
RefundCoordinator.Refund(ctx, RefundCommand{OrderID, Amount*, Reason, Actor}) (RefundResult, error)
  1. loadForUpdate(order)                              // row lock
  2. txn = PaymentTxnLookup(order_id)
        └─ none OR not captured → ErrRefundUnavailable // COD/manual/authorized-only
  3. capturedTotal = Σ captured payment_transactions
     amount = Amount or (grand_total - refunded_amount) // nil ⇒ full remaining
     validate: 0 < amount ≤ min(grand_total, capturedTotal) - refunded_amount
  4. targetStatus = derive(refunded_amount + amount, grand_total)
        → partially_refunded | refunded
  5. gateway = GatewayConfigResolver(store, txn.provider)
  6. runIdempotentSaga(...)                            // §4.2
```

`Amount` is a pointer: `nil` ⇒ full remaining refund (used by auto-cancel and
"refund everything" return path).

### 4.2 Idempotent refund saga

A gateway refund is a non-rollback-able side effect, so it cannot sit inside a DB
transaction. Ledger-first + idempotency-key saga:

```
key = deterministic(order_id, refund_scope_id)   // stable across retries

// refund_scope_id identifies ONE logical refund so retries share a key but
// distinct refunds don't collide:
//   - return path : return_id
//   - cancel path : "cancel:" + order_id            (one auto-refund per order)
//   - admin path  : caller-supplied refund_request_id (client generates a UUID;
//                   required so two intentional partials get distinct keys while
//                   a double-submit of the same partial is deduped)

DB tx #1: INSERT refund_transactions
            (order_id, provider, provider_payment_id, amount, status='pending',
             idempotency_key=key, reason)
          ON CONFLICT (idempotency_key) DO NOTHING   // safe re-entry

gateway.RefundPayment(RefundInput{..., IdempotencyKey: key})   // real money

DB tx #2 (atomic):
   UPDATE refund_transactions SET status='succeeded', provider_refund_id=…
   order.Service.RecordRefund(order_id, amount, targetStatus, reason)  // refunded_amount, events, outbox
   (return path only) return.repo.StampRefunded + UpdateStatus(refunded)
```

- **Idempotency-Key** is added to `payment.RefundInput` and sent by every gateway
  (Stripe `Idempotency-Key` header; Razorpay/PayPal request-id equivalents). Re-issuing
  with the same key never double-refunds at the provider. This is the concrete
  improvement over the Home-Chef `cancellation_execute.go` original-method path, which
  lacks a provider idempotency key and can double-refund in the crash window.
- If tx #2 fails after the gateway succeeds, the ledger row stays `pending` and the
  sweeper (§4.3) re-runs the gateway call with the **same key** (a no-op at the
  provider) then completes tx #2.

### 4.3 Retry sweeper — `cmd/refund-sweep-cron`

Standalone Cloud Run Job (Cloud Scheduler trigger), modeled on `cmd/reconciliation-cron`.

```
every N minutes:
  SELECT * FROM refund_transactions
   WHERE status='pending' AND created_at < now() - interval '5 minutes'
   LIMIT 200
  for each: RefundCoordinator.resumeSaga(row)  // same idem key → provider no-op if already done
```

The existing `refund.succeeded` webhook (`internal/handlers/storefront/webhooks.go:711`)
already flips ledger rows to `succeeded`; it doubles as async confirmation and reduces
sweeper work.

### 4.4 Trigger wiring

| Trigger | File | Change |
|---------|------|--------|
| Admin order refund | `internal/handlers/admin/orders.go:Refund` | call `RefundCoordinator.Refund` instead of `svc.RecordRefund` |
| Return refund | `internal/order/return_service.go:MarkRefunded` | call coordinator inside its tx-orchestration; keep return-state stamping atomic with `RecordRefund` in tx #2 |
| Admin return refund handler | `internal/handlers/admin/returns.go:MarkRefunded` | unchanged surface; downstream now moves money |
| Paid self-cancel | `internal/handlers/storefront/order_detail.go:Cancel` | after cancel, if `payment_status==paid` → `RefundCoordinator.Refund(nil)` (full) |
| Admin cancel | `internal/handlers/admin/orders.go:Cancel` | same paid-order auto-refund hook (behind flag) |

Cancellation and refund are **separate steps**: a gateway blip leaves the order
cancelled + a `pending` ledger row for the sweeper; the customer is never blocked.

## 5. Data model changes

`refund_transactions` (add columns; migration via golang-migrate):

| Column | Type | Note |
|--------|------|------|
| `order_id` | `uuid NOT NULL` | today absent — needed for sweeper + reconciliation |
| `status` | `varchar(30) NOT NULL` | `pending` \| `succeeded` \| `failed` (already partially used by webhook) |
| `idempotency_key` | `varchar(255) NOT NULL` | **UNIQUE** — the saga re-entry guard |

Backfill: none required (no real refunds have moved money yet). New unique index on
`idempotency_key`; index on `(status, created_at)` for the sweeper query.

## 6. API / wire changes

- `RefundOrderRequest.payment_status` → **optional / ignored**; server derives it.
  `amount` optional (omit ⇒ full remaining). Keep the field for backward compat but
  document as deprecated.
- `RefundOrderRequest` gains optional `refund_request_id` (UUID). When present it is the
  idempotency scope (see §4.2) so a double-submit of the same partial is deduped; when
  absent the server generates one (single-shot, no client retry-safety).
- New error code `refund_unavailable` (422) when no captured gateway transaction exists.
- Response includes `provider_refund_id` and final `payment_status`.

## 7. Payment gateway interface changes

- `payment.RefundInput` gains `IdempotencyKey string`.
- `StripeGateway.RefundPayment` sends `Idempotency-Key` header.
- `RazorpayGateway.RefundPayment` sends its request-id / idempotency header and
  `notes{order_id,reason,scope}` (mirrors Home-Chef `RefundRequest.Notes`).
- `PayPalGateway.RefundPayment` sends `PayPal-Request-Id`.
- `payment.Service.RefundPayment` persists the ledger row within the coordinator's tx
  boundary rather than owning its own connection (moves ledger write into tx #1/#2).

## 8. Correctness / validation rules

- `amount > 0`.
- `refunded_amount + amount ≤ min(grand_total, capturedTotal)`.
- `targetStatus` derived, never client-trusted.
- Only orders with a **captured** payment transaction are refundable via gateway;
  `authorized`-only orders → `refund_unavailable`.
- All attempts (success + failure) emit an audit event (extend the existing admin-path
  audit to the cancel + return paths).

## 9. Testing strategy (blocking — prod service)

No task in the implementation plan is "done" until its tests are written and green.
Target ≥ 80% coverage on all new/changed packages.

### 9.1 Unit — `internal/orderrefund` (fake `Gateway`, fake repos)
- Partial refund → `partially_refunded`, correct `refunded_amount`.
- Full refund (explicit amount == remaining) → `refunded`.
- Full refund (`amount=nil`) → refunds `grand_total - refunded_amount`.
- Over-cap (`refunded_amount + amount > grand_total`) → rejected, no gateway call.
- No `payment_transactions` row → `ErrRefundUnavailable`, no gateway call.
- Authorized-but-not-captured → `ErrRefundUnavailable`.
- Status derivation table test (amounts around the grand_total boundary).
- Idempotency key is **stable** for the same (order, scope) across calls.
- Gateway returns error → ledger row stays `pending`, no `refunded_amount` change,
  error surfaced.

### 9.2 Gateway unit tests (httptest server per provider)
- Stripe/Razorpay/PayPal `RefundPayment` sends the idempotency header.
- Re-call with same key is treated as safe (assert header present + request shape).
- Non-200 provider response → wrapped error, no ledger mutation.

### 9.3 Integration (real DB, fake gateway) — `*_integration_test.go`
- **Return happy path**: request → approve → receive → refund asserts a
  `refund_transactions` row (`succeeded`, correct `order_id`, `idempotency_key`) **and**
  `orders.refunded_amount` bump **and** `order_events`/`outbox_events` rows, all
  consistent in one logical operation.
- **Paid-cancel auto-refund**: paid + un-shipped order → customer cancel → order
  cancelled, full refund ledger row, `payment_status=refunded`, refund email dispatched.
- **Un-paid cancel**: pending/COD order → cancelled, **no** refund row.
- **Shipment guard**: label cut → self-cancel still `409`, no refund.
- **Sweeper**: a `pending` row older than threshold is resumed and completes **without
  a second provider charge** (assert the fake gateway received the same idempotency key
  and short-circuited).
- **Double-submit**: two concurrent admin refunds for the same order/scope →
  exactly one gateway call, one ledger row (unique key), correct final amount.
- **Over-cap via two partials**: 60 + 60 on a 100 order → second rejected.

### 9.4 Regression
- Existing `order` / `return` / `payment` suites stay green.
- `go test -race ./...` on touched packages.

## 10. Rollout & safety

- `REFUND_GATEWAY_ENABLED` flag gates the gateway call + auto-cancel-refund. Off ⇒
  current behavior (bookkeeping only) so the deploy is inert until flipped, per the
  escrow-flag pattern.
- Ship the migration + code dark, enable in a non-prod env, run the integration suite
  against a Stripe/Razorpay **test-mode** key, then enable in prod.
- Sweeper Job deployed but scheduled conservatively (e.g. every 5 min) after the flag is on.
- Audit + structured logs on every refund attempt for observability.

## 11. Open questions

_None blocking. Store-credit refund destination and a merchant "manual refund recorded"
path (for COD) are noted as future follow-ups._
