# Start the trial at signup (#827)

`store_subscriptions` rows are created in exactly one place — `Service.Bootstrap`
— reached from exactly one place: a CTA on the admin Billing page. `trial.EndsAt`
derives the trial end from that row's `created_at`, so the 90-day clock starts on
the button press. A merchant who never opens Billing has no row, no trial, and is
invisible to every billing cron.

Bootstrap's own doc calls itself a back-fill for "stores created before the v2.3
signup pipeline". That pipeline does not exist.

Full evidence on #827. This plan is the fix.

## The shape of it

A free trial must not depend on a payment provider being reachable. That
dependency is what turns a Stripe outage — or a mis-scoped key, which #696 says
we currently have — into a merchant with no trial at all.

An empty `stripe_customer_id` is already a state this codebase expects and
handles: five call sites guard it, and the paid path (`planchange.go:257`)
refuses a customer-less row outright with "run bootstrap first". `NOT NULL` is
satisfied by `''`, so no migration is needed.

## Tasks

### 1. Separate "the row exists" from "Stripe knows about it"

- [ ] `Bootstrap` no longer calls Stripe. It ensures the row: plan=trial,
      status=signup, and now `billing_currency` when the caller knows it.
- [ ] New `EnsureStripeCustomer(ctx, tenantID, storeID)` — idempotent, attaches
      a customer to a row that has none.
- **Not** best-effort-with-a-warning inside Bootstrap. Swallowing a Stripe error
  would hide exactly the broken-key failure #696 describes, and #827 exists
  because a billing precondition failed silently for months.
- **Done when** Bootstrap succeeds with a nil Stripe client, and the two
  responsibilities fail independently.

### 2. Keep the existing paths whole

- [ ] Admin's bootstrap CTA calls both, surfacing the Stripe error — but the row
      is created first, so a retry is cheap and the trial has started either way.
- [ ] The card-add path ensures a customer before `trial.Subscribe`, which
      otherwise refuses with `ErrMissingStripeCustomer` for every merchant whose
      row was created at signup.
- **Done when** no path that worked before now fails, and the CTA still reports
  a Stripe problem rather than hiding it.

### 3. Create the row at onboarding completion

- [ ] `POST /internal/stores/:storeID/ensure-subscription` on marketplace-api,
      guarded by `InternalAuthSecret` — the migration handler's precedent, not
      the unguarded one next to it. This is a write with a billing consequence.
- [ ] platform-api calls it from `onboarding.Complete`, after `EnsureSelfStore`
      (the store row must exist — `store_subscriptions.store_id` is an FK) and
      under the same best-effort policy: a failure is logged and does not fail
      onboarding.
- **Done when** completing onboarding leaves a `trialing`-eligible row whose
  `created_at` is the signup moment.

### 4. Backfill, written but NOT run

- [ ] `cmd/subscription-backfill` — one row per store that has none, with
      `trial_ends_at` set explicitly from the STORE's `created_at + 90d` rather
      than from the moment of the backfill.
- [ ] `--dry-run` default. Running it is a data decision with a merchant-visible
      outcome; for a store older than 90 days it writes an already-expired
      trial, which is truthful but should be seen before it is written.
- **Done when** a dry run prints what it would write and changes nothing.

## Out of scope

The onboarding promo field (#620). It is what surfaced this, and it becomes
buildable once a row exists at signup — but it is a separate change.

Whether `stripe_customer_id` should become nullable rather than `''`. The empty
string is what every existing guard already tests for; a migration would be a
second representation of the same state.
