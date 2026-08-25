# Design — `POST /admin/billing/trials/{store_id}/extend` (#286)

**Status:** approved
**Issue:** #286 · **Depends on:** #275, #285, **#353** (shipped) · **Umbrella:** #260 · **Date:** 2026-08-25

## What #353 already settled

#286 could not be built as written. A trial end was not stored anywhere — it was recomputed
as `created_at + 90 days` at seven independent sites, so an operator action writing an
"extension" would have changed a number nothing read. #353 shipped the substrate: a nullable
`store_subscriptions.trial_ends_at`, one accessor `trial.EndsAt`, and all seven consumers
taught to read it.

That is why this endpoint **enforces**, and it is checkable rather than hopeful. Writing
`trial_ends_at` now changes:

| consumer | effect |
|---|---|
| `billing/trial/expiry_cron.go` | the trial is not expired on day 90 |
| `billing/trial/subscribe.go` | the value sent to Stripe as `trial_end` |
| `subscription/planchange/planchange.go` | same, on a plan change |
| `handlers/admin/subscription.go` | the merchant's own banner and countdown |
| `billing/trial/expiring.go` | this console's `/admin/billing/trials` list |
| `subscription/dunning/trial_reminders.go` | which day the reminders fire |

A guard test fails if an eighth derivation site appears. This spec therefore does **not**
repeat #287's mistake of specifying an operator action before checking what enforces it —
that check is #353, and it is done.

## The path parameter is a store id, and the issue's shape does not exist

#286 names `POST /admin/billing/trials/{id}/extend`. **The console has no subscription id to
put there.** `/admin/billing/trials` (#285) emits `tenant_id`, `tenant_name`, `store_id`,
`trial_ends_at`, `days_remaining`, `plan`, `period`, `billing_currency`, `amount`,
`payment_method_on_file`, `status` — and no row id
(`internal/handlers/platformadmin/billing_trials.go:101-111`; `trial.ExpiringRow` has no ID
field either, `internal/billing/trial/expiring.go:27-37`).

This is the fourth time in this series an issue has named something absent — #277's tenant
slug, #276's `metadata` shape, #329's assignee, and now this.

**Ruling: `{id}` is the `store_id`.** `store_subscriptions` declares `UNIQUE (store_id)`
(migration `000015`), so it identifies exactly one subscription. The console already holds
it. Nothing about a live contract changes. And it matches #287's
`/admin/tenants/{id}/suspend` shape, where the path parameter is likewise the natural
business key rather than a row id.

Rejected: adding `id` to #285's row. It is additive and would work, but it changes a shipped
contract to solve a problem that a `UNIQUE` constraint already solves.

## Contract

```
POST /api/v1/platform/admin/billing/trials/{store_id}/extend
Idempotency-Key: <opaque>

{ "trial_ends_at": "2026-12-01T00:00:00Z",
  "reason": "support escalation — data migration delayed" }
```

A **write**: HMAC signature **and** operator identity **and** capability, per the foundation
spec's enforcement matrix. Absent operator → `401 operator_required`; absent capability →
`401 capability_required`.

### Absolute date, not a relative grant

The body carries the new end, not "+30 days".

`trial_ends_at` is an absolute column, so a relative grant would have to be resolved against
a base — and under a retry, resolving it twice compounds. An absolute value is idempotent by
construction: applying it a second time is a no-op on the state. That property is the
difference between a retry being safe and being a second extension, and it is worth more
than the small convenience of sending a duration.

### Reason: required code plus optional free text, matching #287

```json
{ "reason_code": "support_escalation", "reason": "migration from Shopify slipped two weeks" }
```

**Corrected during planning.** An earlier draft specified free text only, on the grounds that
#287 "declined to invent a vocabulary". That is true of **capability names** — where the
console is the authority and a guess refuses every real request — but it is the wrong
precedent to cite here, and #287's own *reason* handling points the other way:

```go
// tenant_lifecycle.go:27-31
// SuspendReasonCodes is the closed set of reasons a tenant may be suspended
// for. An audit row saying WHAT happened without WHY is the gap this series
// exists to close (#287), so the code is REQUIRED; free text (`reason`) is
// accepted IN ADDITION, never instead.
```

`lifecycleRequest` is `{reason_code, reason}` and is already live on this surface. The
mismatch risk is low in this direction because **mark8ly defines the set and publishes it**;
the console sends it. #287 chose its seven codes unilaterally and they work.

Shipping free text here would leave two operator-action endpoints on one surface with
different reason contracts — an operator must supply `reason_code` to suspend a tenant but
not to extend a trial. Consistency wins.

```go
// ExtendReasonCodes is the closed set of reasons a trial may be extended for.
// Deliberately a different set from SuspendReasonCodes: the reasons for
// granting more trial time are not the reasons for suspending a tenant.
var ExtendReasonCodes = []string{
    "support_escalation", // an open support case needs more time to resolve
    "onboarding_delay",   // setup or migration slipped for reasons outside the merchant's control
    "billing_dispute",    // a billing disagreement is open; the trial should not lapse meanwhile
    "goodwill",           // discretionary grant, no other category applies
    "operator_error",     // correcting a mistaken earlier extension or trial start
}
```

Validation matches #287 exactly, including the error shape, so the console handles both
endpoints the same way:

| condition | status | body |
|---|---|---|
| body absent or unparseable | 400 | `{"error":"invalid_request","message":"request body could not be parsed"}` |
| `reason_code` absent or not in the set | 400 | `{"error":"invalid_reason_code","message":"reason_code is required and must be one of the declared codes","field":"reason_code","allowed":[…]}` |

`reason` stays optional free text, trimmed, capped at 500 characters. Both are stored on the
audit row.

### Refusals

Each has its own code, so the console can tell them apart. **Refused, never silently
ignored** — that is #286's own acceptance criterion.

| condition | status | `error` |
|---|---|---|
| `status = 'active'` | 409 | `already_converted` |
| `stripe_subscription_id IS NOT NULL` | 409 | `stripe_managed` |
| `status` not in (`trialing`, `signup`) | 409 | `not_trialing` |
| `trial_ends_at` not in the future | 400 | `invalid_request` |
| `reason_code` absent or unknown | 400 | `invalid_reason_code` |
| `trial_ends_at` unparseable or absent | 400 | `invalid_request` |
| no subscription for that store | 404 | `not_found` |

Two of these deserve their reasoning recorded.

**`stripe_managed`.** A trial with a Stripe subscription has already had a card added, and
Stripe holds the billing date. Writing `trial_ends_at` locally without telling Stripe would
put the console and Stripe in disagreement about when a real merchant is charged — the exact
split-brain #353 existed to remove. Telling Stripe is not free:
`billingstripe.UpdateSubscriptionParams` carries `SubscriptionID`, `PriceID`,
`ProrationBehavior`, `IdempotencyKey` and `Metadata` and **no `TrialEnd`**, and
`UpdateSubscription` *requires* a `PriceID` because it swaps the subscription's item — it
exists for plan changes. Extending through it means adding a `TrialEnd` field and making the
price swap optional, or an extension silently re-prices the subscription.
`sdk.SubscriptionUpdateParams` supports `TrialEnd` natively, so the work is small, but it is
a real-money side effect and gets its own issue. **v1 refuses, with a code that says why.**

**`invalid_request` on a past date.** Setting an end in the past is indistinguishable from
expiring the trial, which the cron already does on its own schedule. An endpoint named
`extend` should not be the way to expire something.

Note that a date *earlier than the current end but still in the future* is allowed.
`trial.EndsAt` honours a stored value even when it is earlier than the derived one, and an
operator correcting an over-generous grant is legitimate. The endpoint sets an end; it does
not assert a direction.

### Response

```json
{ "store_id": "…", "tenant_id": "…",
  "trial_ends_at": "2026-12-01T00:00:00Z",
  "previous_trial_ends_at": "2026-09-14T10:22:31Z",
  "reason": "…",
  "extended_at": "2026-08-25T09:00:00Z" }
```

`previous_trial_ends_at` is the **effective** end before the write — `trial.EndsAt` of the
row as read, which is the derived date when the trial had never been extended. It is what
makes the audit row and the response legible without a second lookup.

Timestamps RFC3339 UTC with offset; ids bare; no `source` field.

## What it does

One transaction:

1. `UPDATE store_subscriptions SET trial_ends_at = $new WHERE store_id = $id` — after
   re-reading the row inside the transaction, so the refusal checks and the write see the
   same state.
2. `DELETE FROM trial_reminders WHERE subscription_id = $sub` — see below.
3. `platformadmin.EmitOperatorAction(c, emitter, tenantID, ev)` with the reason, the previous
   effective end and the new one.

**`EmitOperatorAction`, never `audit.Emit`.** Nothing on this surface sets `tenant_id` on the
gin context, and `resolveScope` drops an event with no tenant — silently, with no error. The
helper takes the tenant as a required parameter so it cannot be forgotten (trap 3, #310).

### Deleting the reminder rows

`trial_reminders`' primary key is `(subscription_id, offset_key)` (migration `000088`), and
`processOne` inserts `ON CONFLICT DO NOTHING`. So a reminder already sent can never re-send:
a merchant extended past their T-15 warning would get **no** warning before the date they are
actually charged on. The extension would silently cost them the thing that protects them from
a surprise bill.

Deleting the subscription's reminder rows re-arms the cadence against the new end. The cost
is that a merchant may receive a second T-15 email weeks after the first — defensible,
because the first referred to a date that no longer applies.

Rejected: pruning only the reminders that fall after the new end. It avoids the duplicate,
but mapping an `offset_key` back to a date reintroduces exactly the derivation #353 removed,
in a third place.

## Idempotency

`idempotency_keys` exists (migration `000001:264`) with a model at
`internal/idempotency/models.go` and **zero consumers anywhere in the estate** — this is its
first use. Acceptance criterion 4 is therefore net-new machinery on this surface, not a flag.

- The `Idempotency-Key` header is **required** for this endpoint. A write that cannot be
  retried safely is worse than one that refuses to start.
- On a hit, the stored response body is replayed with its original status and **no** second
  audit row is written. That is what "idempotent" has to mean here — the state write is
  naturally idempotent because the value is absolute, but the audit row is not.
- The row's `tenant_id` is `NOT NULL`; the tenant comes from the subscription.
- A **different** key against the same store is a new extension, not a replay. The key
  identifies the request, not the subscription.
- **Nothing prunes this table today, despite two comments saying otherwise.** Migration
  `000001:262` says "cleanup via the nightly sweep job (slice 1)" and
  `internal/idempotency/models.go:3-6` repeats it. Both are false: the only two Go references
  to `idempotency_keys` are `subscription/harddelete/sweeper.go:133` and
  `tenantpurge/purge.go:257`, and both delete **by `tenant_id`** when a tenant is hard-deleted
  or purged. Neither is an expiry sweep, and nothing reads `expires_at`.

  This was found by searching for the sweep rather than trusting the comment — the comment had
  already been copied into an earlier draft of this spec as fact.

  **Therefore this design adds the prune**, to `platformadmin.SweepSpec` — the daily cron
  (`45 9 * * *`) that already sweeps `platform_request_nonces` for this same surface, and is
  already registered only when the platform admin secret is set. One extra `DELETE ... WHERE
  expires_at < now()`. Writing keys into a table nothing prunes would be a slow leak, and
  leaving the two false comments in place would hand the next reader the same wrong answer.
  Correct both comments as part of this work.

## Testing

- **Every refusal seeded on the exact state that triggers it, plus a control that does not.**
  A refusal that always fires and one that never fires both pass a one-sided test.
- **Enforcement, end to end — the assertion #287 lacked.** Extend a subscription created 100
  days ago, run the expiry cron, assert it survives; and assert the unextended control in the
  same fixture *is* expired, so the test cannot pass by the cron doing nothing.
- **Reminder re-arm.** Seed a sent `no_pm_t_minus_15` row, extend to a date 15 days out,
  assert the row is gone and that the reminder cron then fires T-15 at the new end.
- **Idempotency.** Same key twice: identical body, identical status, and exactly **one**
  audit row. Different key: a second extension and a second audit row. Asserting the audit
  row count is what distinguishes real idempotency from a coincidentally-identical response.
- **Attribution.** The audit row carries `actor_operator_id`, `capability`,
  `actor_type = operator`, the tenant, and the reason.
- **Golden fixture**, proved by mutation to catch a field rename and a field addition.
- Integration: `//go:build integration`, `-p 1`, LAN IP DSN, `TEST_DATABASE_URL`. Seed via
  the existing helpers (`seedExpiringRow`, `seedExpiringStore`) — a hand-written
  `INSERT INTO stores` omits `storefront_customer_portal_secret`, which migration `000058`
  declares `CHAR(64) NOT NULL` with its DEFAULT dropped.
- `go vet -tags=integration ./...` and a root-inclusive `go test ./...` in the verification
  set.

## Rollout and what production can prove

No migration: `trial_ends_at` and `idempotency_keys` both already exist. So this ships as
code only, and `ExpectedSchemaVersion` does not move.

**`store_subscriptions` is empty in production** — 0 rows against 4 stores, verified
read-only in-cluster on 2026-08-25 rather than carried forward as a claim. Consequences for
the verification report, which must state both halves:

- **Provable:** the route is mounted, an unsigned request gets `401`, a write without
  operator or capability gets `401`, and a bogus sibling path under the same prefix gets
  `404` — the last is what makes the first mean "this route exists".
- **Not provable:** every refusal, the cascade, the reminder re-arm and the idempotent
  replay. None can be exercised without a scratch tenant that has entered the billing flow,
  which needs an explicit call with a Stripe customer.

An empty `200` is not a passing integration check, and neither is a `401` from a route whose
body has never run.

## Out of scope

- **Pushing a changed `trial_end` to Stripe** — refused with `stripe_managed` and split to its
  own issue, for the reasons under Refusals.
- **Reason codes beyond the five declared** — the set is mark8ly's to extend, and adding one
  is a one-line change plus a note on the issue.
- **Shortening a trial into the past, or reinstating an expired one.** Both are different
  operator actions with different consequences, and neither is what #286 asks for.
