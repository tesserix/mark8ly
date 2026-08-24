# Design — trial end must be stored, not derived (#353)

**Status:** approved
**Issue:** #353 · **Unblocks:** #286 · **Closes:** #326 · **Umbrella:** #260 · **Date:** 2026-08-25

## Why this exists as its own issue

#286 asks for `POST /admin/billing/trials/{id}/extend`. **A trial end cannot be extended
today because it is not stored anywhere.** It is recomputed from
`store_subscriptions.created_at + 90 days`, independently, at seven sites.

Implemented literally — write a column, serve the endpoint — #286 would ship an operator
action that changes a number nothing reads. The cron would still expire the trial on day
90, Stripe would still bill on the original date, and the merchant would still be shown
the old one. That is #287's defect class exactly (an operator action that enforces
nothing), except here the operator is making a **billing** promise to a customer.

So the substrate ships first, alone, and is verifiable alone: set `trial_ends_at` on a row
and observe that six behaviours change.

## What was verified

`store_subscriptions` (migration `000015`, extended by `000040`, `000049`, `000067`,
`000087`) has **no trial-end column**. `created_at` is the only anchor;
`trial.TrialDays = 90` (`internal/billing/trial/subscribe.go:21`) is the only length.

Every site that answers "when does this trial end", found by searching every `TrialDays`
reference and then separately for the hardcoded constant:

| site | today | consequence of missing it |
|---|---|---|
| `internal/billing/trial/expiry_cron.go:48` | `created_at < now - TrialDays` → transitions to `expired` | **an extended trial still expires on day 90** |
| `internal/billing/trial/subscribe.go:131` | `created_at + TrialDays` → **Stripe `trial_end`** | Stripe bills on a date the console does not show |
| `internal/handlers/admin/subscription.go:197` | `created_at + TrialDays`, shown to the merchant | the merchant sees a date nobody honours |
| `internal/billing/trial/expiring.go:56`, `:87` | `created_at > asOf - TrialDays` — backs `CountExpiring`/`ListExpiring` | the console's own `/admin/billing/trials` (#285) shows the old date |
| `internal/subscription/dunning/trial_reminders.go:108` | day-buckets on `created_at`, offset back from a fixed length | reminders fire on the original schedule, and never before the new end |
| `internal/subscription/planchange/planchange.go:225` | hardcoded `90 * 24 * time.Hour` → Stripe `trial_end` | same, and it does not reference `TrialDays` at all (#326) |

**The handoff doc records five sites. There are six.** `expiry_cron.go` is missing from its
list, and it is the one whose omission means an extension does not extend anything. The
list above was built by searching and then reading the output — trap 10.

All six anchor on `store_subscriptions.created_at`: `expiry_cron` queries
`subscription.StoreSubscription` despite a comment that says "stores created", so there is
a single anchor today and no pre-existing split.

## Decision

```sql
ALTER TABLE store_subscriptions ADD COLUMN trial_ends_at TIMESTAMPTZ;
```

Effective end = `COALESCE(trial_ends_at, created_at + 90 days)`.

**NULL means never extended.** That is the load-bearing property: "this trial was
extended" becomes a fact that can be read directly, by the console, by an audit row and by
a test, rather than inferred by recomputing `created_at + 90d` and comparing. It also means
**no backfill**, so the migration cannot corrupt a billing table — the riskiest thing this
change could do, not done.

Rejected: a backfilled `NOT NULL` column (simplest reads, but a data migration on billing
data, and "extended" stops being distinguishable from "default"); and
`trial_extension_days` (every consumer keeps doing date arithmetic, which is the thing
being removed, and a future change to `TrialDays` would silently move every
previously-extended trial).

### One accessor

```go
// internal/billing/trial

// EndsAt returns when this subscription's trial ends: the stored
// trial_ends_at when an operator has extended it, otherwise
// created_at + TrialDays. This is the ONLY definition of trial end.
func EndsAt(sub subscription.StoreSubscription) time.Time
```

`internal/billing/trial` already owns "what a trial is and when it ends" (it holds
`TrialDays`, `DefaultExpiryWindow`, `CountExpiring`, `ListExpiring`), so the accessor goes
where the knowledge already lives rather than into a new package.

The property worth testing is not that `EndsAt` is correct — it is that **nothing else
computes a trial end.** A grep-shaped test over the six sites is what makes the seventh
one, added next year, fail loudly.

## The indexed-query problem

`expiringScope` (`expiring.go:51-58`) carries this comment:

> Note the algebra: `created_at + TrialDays > asOf` is `created_at > asOf - TrialDays`.
> Doing it this way keeps the comparison on a plain indexed column instead of an
> expression — do not "simplify" it back to comparing against an expression on
> `created_at`.

**That claim was checked and is true.** Migration `000087` created
`(status, created_at)`, and the scope filters `status = 'trialing'` then compares
`created_at`. A naive `COALESCE(trial_ends_at, created_at + interval '90 days')` predicate
would defeat it.

So the predicate splits in two, and both branches keep an index:

```sql
(
  (trial_ends_at IS NULL     AND created_at    >  :asOf - 90d
                             AND created_at    <= :asOf - 90d + :window)
  OR
  (trial_ends_at IS NOT NULL AND trial_ends_at >  :asOf
                             AND trial_ends_at <= :asOf + :window)
)
```

- The **NULL branch** is today's predicate unchanged, still served by `(status, created_at)`.
- The **NOT NULL branch** is served by a new partial index, which stays small precisely
  because extensions are rare:

```sql
CREATE INDEX IF NOT EXISTS ss_trial_ends_at_idx
    ON store_subscriptions (trial_ends_at)
    WHERE trial_ends_at IS NOT NULL;
```

Both branches live in **one shared scope builder** in `trial`, used by `expiringScope` and
by `expiry_cron`, so the two cannot drift. The duplication of window logic across two
branches is the risk this creates, and it is why the boundary tests below must exercise
both.

## The site that is not mechanical

`trial_reminders.go` does not compute a date. It computes an **offset**:

```go
dayOffset := trial.TrialDays - t.DaysBefore
// ...then day-buckets on created_at
Where("created_at >= ? AND created_at < ?", dayStart, dayEnd)
```

It works backwards from a fixed trial length, so an extended trial keeps receiving
reminders on the original schedule and receives none before the new end. The merchant is
emailed "your trial ends in 3 days" a month early, then nothing.

It changes to bucketing on the **effective end** directly — `effective_end` inside the day
that is `DaysBefore` days from now — which is both correct for extended trials and simpler
than the offset arithmetic it replaces.

### A consequence this issue deliberately does not resolve

`trial_reminders`' primary key is `(subscription_id, offset_key)` (migration `000088`), so
a reminder already sent can never be re-sent. A merchant extended past their T-15 warning
therefore gets **no** warning before the new end — the extension silently costs them a
notification.

This spec makes reminders fire off the effective end and stops there. Whether granting an
extension should delete already-sent reminder rows is a product decision about customer
email, it belongs to the action that grants the extension, and it is recorded here so
**#286's spec inherits it rather than rediscovering it.**

## Migration

`000103_store_subscriptions_trial_ends_at`, plus `ExpectedSchemaVersion` 102 → 103 in the
root-package `migrations.go`. `AssertVersion` requires exact equality; the guard test lives
in the root package, which every path-scoped test command excludes.

```sql
ALTER TABLE store_subscriptions ADD COLUMN IF NOT EXISTS trial_ends_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS ss_trial_ends_at_idx
    ON store_subscriptions (trial_ends_at) WHERE trial_ends_at IS NOT NULL;
```

`IF NOT EXISTS` on the index is not decoration: an operator who pre-creates it by hand or
`CONCURRENTLY` would otherwise make the migration error, the version go dirty, and every
pod refuse to start. Trap 11.

Down drops the index and the column. Dropping the column loses extension grants, so the
down migration is destructive by nature — that is stated in the file rather than made
silently reversible.

## Testing

The organising rule: **a test must fail if the property it names is deleted.**

- **Per site, both branches.** For each of the six consumers, one test with
  `trial_ends_at IS NULL` and one with it set to a value that is *not* `created_at + 90d`
  and not merely a few hours away — far enough that a consumer still using the old
  derivation gives a visibly different answer. The distances matter: an offset that looks
  historical can still sit inside the window being measured.
- **`expiry_cron` is the sharpest test.** Seed a subscription created 100 days ago with
  `trial_ends_at` 30 days in the future. Today it is expired. After the change it must
  survive. Delete the `trial_ends_at` handling and this test fails — which is the point.
- **The Stripe values.** `subscribe.go` and `planchange.go` both send `trial_end` to
  Stripe. Assert the value handed to the Stripe client, not just that a call happened.
  Asserting presence when the value is what matters is how a fabricated zero passes.
- **Boundary, on both branches.** The window is half-open left, inclusive right. Place a
  fixture on the exact boundary instant for the NULL branch *and* for the NOT NULL branch.
  A boundary test that only covers unextended rows passes against a broken extension path.
- **The index still serves the common case.** An `EXPLAIN` assertion is brittle; instead
  the test asserts the shape that matters — that the NULL branch's predicate still compares
  a bare `created_at`, so a future "simplification" to a `COALESCE` expression fails
  visibly rather than silently costing the index.
- **Nothing else computes a trial end.** A test that scans the service for date arithmetic
  on `TrialDays` or a bare `90` outside `internal/billing/trial`, and fails on a new one.
- Integration tests: `//go:build integration`, `-p 1`, LAN IP DSN, `TEST_DATABASE_URL`.
  `go vet -tags=integration ./...` and a root-inclusive `go test ./...` in the verification
  set.

**Pre-existing failures to scope around:** `internal/subscription`'s
`store_subscriptions_store_id_fkey` failures are #317 and unrelated. Scope runs with
`-run`, and do not attempt to fix them here.

## Rollout and what production can prove

Migration, accessor and all six consumers ship together. Splitting the migration from the
consumers would leave a column nothing reads, which is the state this issue exists to end.

**`store_subscriptions` was reported empty in production earlier in this milestone** (no
merchant has entered the billing flow — it requires an explicit call with a Stripe
customer). That claim's freshness has expired and must be re-checked before verifying, not
assumed. If it still holds, then **every behaviour this change touches is unexercised in
production**, and the verification report must say so: the deploy proves the migration
applied and the service starts, and nothing more. An empty `200` is not a passing
integration check.

## Consequences

- **#286 is unblocked** and becomes a small write: set `trial_ends_at`, attribute it, refuse
  a converted subscription, make it idempotent. It inherits the reminder-dedup decision
  recorded above.
- **#326 closes** — `planchange.go`'s hardcoded `90` becomes a call to the accessor.
- A future change to `TrialDays` affects only unextended trials, which is the correct
  behaviour and is currently accidental rather than designed.
- **Out of scope:** the endpoint, operator attribution, reason codes, idempotency, and
  pushing a changed `trial_end` to Stripe.

  **Corrected during spec self-review.** An earlier draft of this section, and of #353,
  claimed "the Stripe client has no modify method". That is **false**:
  `internal/billing/stripe/update.go:37` has `UpdateSubscription`. The claim came from a
  grep shaped `func.*Stripe.*(Update|Modify)`, which requires "Stripe" in the *function
  name* — `UpdateSubscription` does not contain it. Trap 10 again, in the same session that
  documented it: the search ran, and its shape guaranteed the answer.

  What is actually true is narrower and still blocks the Stripe path:
  `UpdateSubscriptionParams` carries only `SubscriptionID`, `PriceID`, `ProrationBehavior`,
  `IdempotencyKey` and `Metadata` — **no `TrialEnd`** — and the function requires a
  `PriceID` because it swaps the subscription's item. It is a plan-change call. Extending a
  trial through it means adding a `TrialEnd` field and making the price swap optional, so
  that extending a trial does not silently re-price the subscription. That is small
  (`sdk.SubscriptionUpdateParams` supports `TrialEnd` natively) but it is a real-money side
  effect, and it belongs to #286 with its own design.
