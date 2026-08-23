# Admin Billing Trials — Design

**Issue:** #285. Part of the platform console integration series (#260), after #274, #275, #276, #277, #279, #282, #283.

**Goal:** Answer "which trials expire this week" for the whole estate — the most-requested platform view in the console's backlog, and unanswerable anywhere today.

**Also:** correct a defect this design uncovered in #282's `trials_expiring` counter, which is already live. See "The correction" below. That fix ships here because both endpoints must agree, and agreeing is only meaningful if they agree on the *right* rule.

## The correction — read this first

`internal/subscription.CountTrialsExpiring`, shipped in #282, selects on `current_period_end`. **That is the wrong column for an expiring trial.**

The authoritative rule lives in `internal/billing/trial/expiry_cron.go`, which is what actually expires trials:

```go
Where("status = ?", subscription.StatusTrialing).
Where("stripe_subscription_id IS NULL").
Where("created_at < ?", now.AddDate(0, 0, -TrialDays))   // TrialDays = 90
```

The same `created_at + 90 days` rule is what the **merchant-facing** endpoint shows a merchant as their trial end (`internal/handlers/admin/subscription.go:197`) and what the reminder emails target (`internal/subscription/dunning/trial_reminders.go`).

A trialing subscription without a card has **no Stripe subscription yet** (`stripe_subscription_id IS NULL`), so `current_period_end` is NULL for exactly the population "expiring trials" means. `CountTrialsExpiring`'s own `current_period_end IS NOT NULL` clause then excludes them all.

**Consequence:** `/admin/kpis` has been returning `trials_expiring: 0` structurally, not factually. Production showed `0` and it was reported as verified; the cross-check used at the time (against another endpoint also reporting `0`) was too weak to distinguish a real zero from a broken one.

Why the tests missed it: every fixture set `current_period_end` explicitly, so they proved the query does what the code says. They could not prove it asks the right question. **A fixture built from the implementation can only confirm the implementation.**

## Definitions

**Expiring trial** — all three, together:

```
status = 'trialing'
AND stripe_subscription_id IS NULL          -- no card: it will expire, not convert
AND created_at + TrialDays  ∈ (asOf, asOf + window]
```

- `TrialDays = 90` (`internal/billing/trial/subscribe.go`). The trial *length*.
- The **window** is how far ahead we look — `days` on the endpoint, defaulting to 7.
- Half-open on the left: an **already-expired** trial is not "expiring". Inclusive on the right: one ending exactly at the window edge **is**.

**Why card-less only.** A trialing subscription *with* a card has a Stripe subscription and will **convert**, not expire. Its renewal date comes from Stripe via `current_period_end`, which is a different question ("what converts this week") and belongs to #284 if anyone wants it. Mixing them puts two expiry rules in one list and forces the console to know which applies per row.

**Trial end on the wire is `created_at + TrialDays`** — the same value the merchant sees. If the console and the merchant ever quote different dates, one of them is lying to someone.

## Where the code lives, and why it moves

`internal/billing/trial` **already owns** `TrialDays` and the expiry cron. `internal/billing/trial` imports `internal/subscription`, so `subscription` **cannot** import it back — an import cycle. `CountTrialsExpiring` therefore cannot reference `TrialDays` from where it currently sits.

So the trial queries move to `internal/billing/trial`, beside the cron whose rule they must match:

| moves from | to |
|---|---|
| `subscription.CountTrialsExpiring` | `trial.CountExpiring` |
| `subscription.TrialExpiryHorizon` | `trial.DefaultExpiryWindow` |

One package owns what a trial is and when it ends. A future change to the rule lands in one file, next to the cron that enforces it, rather than in two packages that cannot see each other.

`internal/subscription` keeps everything else. This is not a refactor of the subscription package.

## `GET /admin/billing/trials`

Query: `days` (default 7, clamped 90 — beyond the trial length the window is meaningless), plus `page` and `limit` (default 50, clamp 500).

Standard envelope. Empty is `200` with `[]`. **Ordered soonest-expiry first.**

```json
{ "data": [
    { "tenant_id": "<bare uuid>",
      "tenant_name": "Acme Trading",
      "store_id": "<bare uuid>",
      "trial_ends_at": "2026-08-27T09:14:02Z",
      "days_remaining": 3,
      "plan": "trial",
      "period": "monthly",
      "billing_currency": "GBP",
      "payment_method_on_file": false,
      "status": "trialing" }
  ],
  "pagination": {"page": 1, "limit": 50, "total": 12} }
```

### `payment_method_on_file`, not `dunning_state`

The issue asks for "current dunning state" on every row. **A trial cannot be in dunning.** The dunning ladder selects `WHERE status IN (past_due, expired, store_closed)`; `status` is a single column, so `trialing` and any dunning state are mutually exclusive by construction.

Shipping a `dunning_state` field that reads `not_applicable` on every row forever would repeat #283's dead-status mistake deliberately.

`has_default_payment_method` is the signal that actually predicts the outcome, and it is what `trial_reminders.go` already branches on to decide which warning email to send. A row reading `payment_method_on_file: false` with `days_remaining: 3` is the view's whole purpose.

**Note the redundancy is deliberate:** the selection already requires `stripe_subscription_id IS NULL`, so every row in this list has no card. `payment_method_on_file` is therefore `false` on every current row — but it is the column the console renders, it stays correct if the selection is ever widened, and it is read from the subscription rather than inferred from the filter.

### No `amount`

The issue asks for money in minor units. **mark8ly does not hold prices.** `PriceIDFor` returns a *Stripe price ID*; there is no plan→amount table anywhere in the workspace. An amount would require a Stripe call per row — an external dependency, latency, and a rate-limit surface on a console read that is otherwise one query.

So: no `amount` key at all. Not `null`, not `0`. `billing_currency`, `plan` and `period` are carried, which is enough to render "Studio · monthly · GBP".

`billing_currency` is nullable in the DB and is **omitted** when absent rather than shipped empty.

This follows #282's precedent: refuse to invent a figure we cannot compute.

### Tenant names

Subscriptions live in marketplace-api; tenant names live in platform-api. Rather than N+1 lookups, the existing tenant directory gains an `ids` filter — additive to `tenantdirectory`, one batch call per page, no fourth client.

If platform-api is unreachable the endpoint returns **`503`**, consistent with every other endpoint on this surface. One unreachable dependency makes the answer untrustworthy, and introducing a second rule here ("names are optional enrichment") would make the surface inconsistent for a marginal gain.

## The cross-endpoint invariant

**`/admin/kpis` `trials_expiring` must equal this endpoint's `pagination.total`** at the default window.

Both read `trial.CountExpiring` / the same predicate, so they agree by construction — and a test asserts it, because "by construction" is exactly the claim that was wrong last time.

## Testing

- **Window edges pinned on the boundary**, not near it: a fixture whose `created_at + TrialDays` falls *exactly* at `asOf + window` (counts), and one exactly at `asOf` (does not — half-open left).
- **A trialing subscription WITH `stripe_subscription_id` is excluded**, however close its trial end. This is the assertion that would have caught the original defect, inverted.
- **A fixture with `current_period_end` NULL still appears** — the population the old query silently dropped.
- Ordering: three rows inserted in an order different from their expiry order, asserted soonest-first.
- `days` respected and clamped; missing `days` takes the default.
- Empty window is `200` with `[]`.
- KPI-vs-listing agreement over the same window.
- Golden fixture proven by mutation against a field rename and a field addition.
- `trial_ends_at` equals `created_at + TrialDays` — the same value the merchant-facing endpoint computes.

## Out of scope

- **Converting trials** (card present, renewal from Stripe). Different question, belongs with #284.
- Amounts. Needs a price source that does not exist.
- Changing `TrialDays` or the expiry cron's behaviour. This design reads the rule; it does not alter it.
