# Emit an audit event for externally-initiated refunds

The remaining half of #705. `handleChargeRefunded` (`internal/billing/dispatch/handlers.go:841`)
is `return nil`, so a refund issued from the Stripe Dashboard leaves **no trace
at all** — no audit row, no log. Refunds issued through our own admin API are
fully recorded; the ones least likely to be recorded are the ones most likely
to need explaining later.

## The chosen option, and why not the other two

#705 proposed writing a `refund_audit` row. **We are not doing that.**
`refund_audit` is a fraud-control table, not a log: migration 000062 puts a
UNIQUE index on `card_fingerprint`, and `internal/refund/service.go:128-137`
reads it before issuing a refund and REJECTS one when that card already has a
row. Writing from the webhook would make a Dashboard refund silently consume
that card's only allowed refund, causing the next legitimate admin refund to be
rejected as fraud. The issue asked for observability; that would have changed
refund policy.

The user chose: **emit an audit event only.** No `refund_audit` write, no
coupling to the fraud guard.

## Design points already established — do not re-derive

- **Redelivery is already deduped.** `internal/handlers/webhooks/stripe.go:94`
  does an idempotent insert keyed on the Stripe `event_id`. The handler needs
  no redelivery guard of its own.
- **Our own refunds also fire this webhook.** `refund.Service` calls Stripe's
  `Refunds.Create`, so Stripe sends `charge.refunded` back for refunds we
  issued and already audited via `EmitRefundIssued`
  (`internal/audit/refund_events.go:29`, action `subscription.refund_issued`).
  Without a discriminator this handler would double-audit every admin refund.
- **The discriminator is `refund_audit.stripe_refund_id`.** Our refunds are
  recorded there with their Stripe refund id; a Dashboard refund is not. A
  READ against that table is safe — it is the WRITE that carries fraud-guard
  meaning.
- `handleChargeRefunded` is a free function today, so it has no `d.emitter`.
  It must become a `*Dispatcher` method, exactly as `handleSubscriptionUpdated`
  did; registration at `dispatcher.go` becomes `d.handleChargeRefunded`.

## Task

1. Add `ExistsByStripeRefundID(ctx, db, refundID) (bool, error)` to
   `internal/refund`'s repository interface and its gorm implementation,
   alongside the existing `ExistsByCardFingerprint` / `ExistsByStore`.
   Read-only.
2. Convert `handleChargeRefunded` to a `*Dispatcher` method and register it as
   such.
3. Parse the `charge.refunded` payload for: charge id, customer,
   `amount_refunded`, `currency`, and the nested `refunds.data[].id`.
4. Resolve `customer` -> `store_subscriptions` (tenant_id, store_id) in the
   same `tx`, mirroring `handleSubscriptionUpdated`'s pre-state SELECT. A
   missing row is NOT an error — return nil and emit nothing, matching
   today's behaviour for a customer we never provisioned.
5. If ANY of the payload's refund ids already exists in `refund_audit`, this
   refund is ours and is already audited: return nil, emit nothing.
6. Otherwise emit via `d.emitter.Emit(nil, audit.Event{...})` with a NEW action
   in the `subscription.*` namespace distinct from `subscription.refund_issued`
   — this is an externally-initiated refund, and conflating the two would
   destroy the only signal that says "this did not come through our controls".
   Follow the field shape used at `lifecycle/finalize.go:88-99`. Metadata
   should carry the charge id, refund id(s), amount and currency.
7. `d.emitter` may be nil (`emitter.go:126` is nil-safe) — do not panic
   building the event either.

## Worth stating in the code

`refund/service.go:80` refuses to refund a subscription holding the Pro+App
add-on (`ErrProAppNotRefundable`). That guard lives ONLY in the admin path, so
a Dashboard refund bypasses it entirely. This event is the only thing that
would ever reveal that having happened. Say so where the action is defined.

## Out of scope

- Any write to `refund_audit`, and any change to the fraud guard.
- Any change to `refund.Service` or the admin refund path.

## Done when

- A Dashboard-initiated refund emits exactly one event with the new action
- A refund we issued (its id present in `refund_audit`) emits NOTHING
- An unknown customer emits nothing and returns nil
- A nil emitter does not panic
- Unit tests, genuinely executing — the existing dispatch tests are
  `//go:build integration` and `TEST_DATABASE_URL` is unset locally, so they
  compile out entirely. Factor the decision into a pure helper if needed.

## Constraints

- TDD: test first, watch it fail.
- One atomic commit, single-line conventional message, NO attribution trailers.
- `go build ./...`, `go vet`, `go test` green before committing.
