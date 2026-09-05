# Emit an audit event on subscription period transitions

Half of #705. The refund half of that issue is deliberately NOT in scope —
`refund_audit` turned out to be a fraud-control table with "one refund per
card, ever" semantics enforced at `internal/refund/service.go:128`, so writing
to it from a webhook would change refund policy rather than add observability.
That is recorded on the issue with three options; this plan covers only the
clean half.

## The gap

`internal/billing/dispatch/handlers.go:242` `handleSubscriptionUpdated` mirrors
`current_period_start`, `current_period_end` and `cancel_at_period_end` from
Stripe into `store_subscriptions`, and emits nothing. It is a free function
`func(ctx, tx, raw) error`, so it has no access to `d.emitter` — that is the
structural reason the original `TODO(P3)` was never done.

Consequence: there is no record of when a billing period rolled, or when
`cancel_at_period_end` was set or cleared. "This merchant scheduled a
cancellation and then reversed it" cannot be reconstructed — exactly the
question a billing dispute produces, and it interacts with #701's save-offer
path which reverses a scheduled cancellation.

## Design constraint discovered while scoping

The existing `UPDATE` is **blind**: it writes new values without reading the
old ones. Stripe emits `customer.subscription.updated` for many reasons, so
emitting unconditionally would produce an event per webhook rather than per
transition — noise that would make the audit trail less useful, not more.

A transition signal therefore requires reading the prior row inside the same
transaction, comparing, and emitting only on an actual change.

## Task

1. Convert `handleSubscriptionUpdated` to a method on `*Dispatcher` and change
   its registration at `dispatcher.go:65` to `d.handleSubscriptionUpdated`.
   `Handler` is `func(ctx, tx, raw) error` and the dispatcher already binds
   methods this way (`dispatcher.go:78-81`), so this is a one-line change.
2. Inside the handler, SELECT the current row for `stripe_customer_id` (id,
   tenant_id, store_id, current_period_start, cancel_at_period_end) in the same
   `tx` BEFORE the UPDATE. If no row matches, keep today's behaviour — the
   UPDATE affects zero rows and the handler returns nil.
3. After a successful UPDATE, emit via `d.emitter.Emit(nil, audit.Event{...})`
   ONLY when something actually changed:
   - `cancel_at_period_end` flipped false -> true, or true -> false. These are
     distinct actions; a merchant scheduling a cancellation and a merchant
     reversing one are different events and must not share one action string.
   - `current_period_start` moved to a later value (a period rolled).
3. Add the action constants alongside the existing
   `ActionProAppCancelled = "subscription.pro_app_cancelled"` precedent
   (`internal/subscription/lifecycle/finalize.go:30`), named in the same
   `subscription.*` namespace.
4. `d.emitter` may be nil — `Emitter.Emit` is nil-safe (`emitter.go:126`), but
   the handler must not panic building the event either.

## Out of scope

- Anything touching `refund_audit`, `handleChargeRefunded`, or refund policy.
- Backfilling past transitions.

## Done when

- Emits exactly one event when `cancel_at_period_end` flips, with distinct
  actions for set vs clear
- Emits when the period rolls forward
- Emits NOTHING when the webhook carries identical values (the common case)
- Existing dispatcher tests pass unchanged
- Unit tests, not integration — `TEST_DATABASE_URL` is unset locally and
  integration tests skip silently while still printing `ok`

## Constraints

- TDD: test first, watch it fail.
- One atomic commit, single-line conventional message, NO attribution trailers.
- `go build ./...`, `go vet`, `go test` green before committing.
