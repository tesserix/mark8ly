# Record Stripe Radar early fraud warnings

Half of #704. `internal/billing/dispatch/handlers.go`'s `handleFraudWarning`
is `return nil`. `radar.early_fraud_warning` is subscribed
(`dispatcher.go:75`) and in the allowed-event list (`pkg/config/config.go:135`),
so Stripe delivers these and we acknowledge them — and discard them. Its
comment claims "audit-only in P2"; no audit row is written either.

Stripe telling us a charge is likely fraudulent is about the strongest signal
we get, and it currently reaches nothing.

## The arbitrage half is NOT in scope — deliberately

#704 also covers `arbitrage_flag` being unreachable (the only `recordArbitrage`
caller passes `IPCountry: ""`, and `Evaluate` returns `ReasonIPUnknown` without
one). Threading the real IP country through is tractable — capture via
`arbitrage.CFIPCountryFromGin` at checkout-session creation, carry it in the
Stripe session metadata, read it back in `handleCheckoutSessionCompleted`.

**We are not doing it yet, and the reason is not technical.** Enabling
flagging would mark merchant accounts while:

- **tesserix-home#144 ("Subscription arbitrage appeals queue") is UNBUILT** —
  there is no operator queue to review a flag
- `arbitrage_flag` is already exposed on the merchant's own subscription
  response (`internal/handlers/admin/subscription.go:98`), so a merchant can
  see they are flagged
- the quarterly cron only clears flags for stores on an allowlist

Flagging people who can see it, with no review queue and no appeals path, is
worse than not flagging. That half waits for tesserix-home#144.

## Task

1. Convert `handleFraudWarning` to a `*Dispatcher` method (as a free function
   it has no `d.emitter` — the same structural reason the other P2 handlers
   were never finished) and update its registration.
2. Parse the `radar.early_fraud_warning` payload: `id`, `charge`,
   `payment_intent`, `fraud_type`, `actionable`.
3. **Attribution needs a Stripe lookup.** The payload carries a charge id, not
   a customer, and `audit.Event` requires a `TenantID`. There is no local
   charge -> store mapping (`refund_audit.stripe_charge_id` exists only for
   refunds). Add a narrow, **nil-safe** charge-getter dependency to the
   Dispatcher — mirroring how `refunds` was added — backed by the existing
   `billingstripe.GetCharge` (`internal/billing/stripe/refund.go:71`). From the
   charge's customer, resolve `store_subscriptions` in the same `tx`, exactly
   as `handleChargeRefunded` does.
4. Emit an audit event with a new `subscription.*` action, Severity **Warning**
   (Stripe suspects fraud — this must be findable), metadata carrying the
   warning id, charge id, `fraud_type` and `actionable`.
5. **The signal must survive failed attribution.** If the getter is nil, the
   Stripe lookup fails, or no subscription matches, do NOT silently return nil:
   log at Error with the warning and charge ids, and still increment the
   counter below. An unattributable fraud warning is still a fraud warning.
6. Add a Prometheus counter (follow `metrics/registry.go`'s existing
   `CounterVec` pattern and register it) so this is alertable. #704's own
   argument is that a warning arriving is an **event**, better served by
   alerting than by a boolean column — the counter is what makes that possible.
   Label it so attributed and unattributed warnings are distinguishable.

## Do NOT

- touch `internal/arbitrage`, `recordArbitrage`, or `IPCountry` — see above
- write `arbitrage_flag`
- block or fail the webhook on a fraud warning: this is observational

## Done when

- An attributable warning emits exactly one Warning-severity audit event and
  counts
- A warning we cannot attribute still logs at Error and counts, and returns nil
- A nil charge-getter does not panic
- Tests genuinely execute. Existing tests in `internal/billing/dispatch` are
  ALL `//go:build integration`, so an untagged run compiles none of them and
  `TEST_DATABASE_URL` is unset locally — they would skip while printing `ok`.
  Follow `charge_refunded_audit.go`'s shape: put the decision in a pure helper
  and unit-test that.

## Constraints

- TDD: test first, watch it fail.
- One atomic commit, single-line conventional message, NO attribution trailers.
- SHARED CHECKOUT: never checkout/switch/stash/reset. Only add + commit.
- `go build ./...`, `go vet` (including `-tags integration`), `go test` green.
