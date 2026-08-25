# Design — pushing a changed `trial_end` to Stripe for card-backed trials (#358)

Split out of #286, which refuses every subscription carrying a
`stripe_subscription_id` with `409 stripe_managed`. This design replaces that
refusal with a working path, and narrows — rather than deletes — the refusal
itself.

Everything below was measured on 2026-08-26 against the code at `main`
(`68ac8bfa`) and the stripe-go SDK in `go.mod`. Where this document contradicts
#358's issue body, the issue body is the thing that was wrong; those places are
called out explicitly.

## What is actually true before any code is written

- `sdk.SubscriptionUpdateParams.TrialEnd *int64` exists — `stripe-go/v82@v82.5.1`,
  `subscription.go:1341`, inside `type SubscriptionUpdateParams struct`
  (declared at `:1272`). The root `CLAUDE.md` says Stripe v76; that line is about
  `marketplace-payment-service`, a different repo, and does not describe this one.
- Our own `UpdateSubscriptionParams` (`internal/billing/stripe/update.go:24`)
  carries `SubscriptionID, PriceID, ProrationBehavior, IdempotencyKey, Metadata`
  and **no** `TrialEnd`.
- `UpdateSubscription` unconditionally populates `Items` with `in.PriceID`
  (`update.go:47-56`). Calling it today to extend a trial would **silently
  re-price the subscription**. This is the whole reason #358 is not a two-line
  change.
- `UpdateSubscription` has exactly two callers, both plan-change paths:
  `internal/subscription/planchange/cron.go:196` and `planchange.go:304`. Both
  pass a `PriceID`. Neither is affected by adding an optional `TrialEnd`.
- Our mapped `stripe.Subscription` (`internal/billing/stripe/subscription.go:14`)
  exposes **neither** `TrialEnd` **nor** `BillingCycleAnchor`. Today we cannot
  read back what Stripe holds for a trial, which both the two-year validation and
  the acceptance criterion require.
- `trial.Extend` (`internal/billing/trial/extend.go:45`) is a free function with
  two wiring sites, `cmd/marketplace-api/main.go:2021` and `:2145`, both via
  `platformadmin.TrialExtenderFunc`.
- `billingStripeClient` is constructed at `main.go:821`, inside the
  `m == mode.Admin || m == mode.Both` block opened at `:566`, and is non-nil only
  when `STRIPE_BILLING_SECRET_KEY` is set. It is in scope at both
  `platformadmin.Register` call sites.

### Two SDK semantics that shape the design

Straight from the `SubscriptionUpdateParams.TrialEnd` doc comment, quoted rather
than paraphrased because both change what the endpoint must do:

> The `billing_cycle_anchor` will be updated to the `trial_end` value.

Setting `trial_end` is **not** a metadata edit. It moves the date the merchant is
billed on thereafter. An operator who extends a trial by three days moves the
merchant's billing anniversary by three days, permanently.

> Can be at most two years from `billing_cycle_anchor`.

A hard bound Stripe enforces. And, from the adjacent `TrialFromPlan` comment:
setting `TrialFromPlan` together with `TrialEnd` "is not allowed" — so the extend
path must never set it.

### The state of the estate's Stripe credentials

`prod-mark8ly-stripe-billing-secret-key` and
`prod-mark8ly-uat-stripe-billing-secret-key` in GCP Secret Manager both hold a
`sk_test_…` key, and the External-Secrets-synced k8s secret
`mark8ly-stripe-billing` agrees. **Production billing runs against Stripe test
mode.** Consistent with `store_subscriptions` holding 0 rows.

This is what makes #358's acceptance verifiable at all — see "Verification"
below — and it is filed separately as **#366**, because the convenience
disappears the moment the key is swapped and every path here starts moving real
money with no code change to mark the transition.

## The decision the issue required: failure ordering

Decided deliberately, before implementation, as #358 asks.

**Stripe is called first and is the source of truth for card-backed trials.**
A failure after Stripe has moved leaves Stripe *ahead* of the local row — the
merchant is charged **later** than the console shows, never earlier. The reverse
ordering (local write, then Stripe) fails in the unsafe direction: the console
shows a date the merchant has already been billed past.

The third option in #358 — enqueue the Stripe update — is rejected. It requires
the outbox, and #336 records that the outbox publisher marks dropped events as
published. An extension that vanishes while being recorded as delivered is worse
than one that fails loudly.

### The race window, and why the row lock is held across the network call

Stripe-first implies read → external call → write, which is a window in which the
row can change: the subscription can convert to `active`, or a second operator can
extend it. If that window is unguarded, Stripe can end up holding a `trial_end`
for a subscription that is, locally, no longer trialing.

**The transaction takes `SELECT … FOR UPDATE` on the `store_subscriptions` row
and holds it across the Stripe call.** An external call inside a transaction is
an anti-pattern in general; here it is bounded and deliberate:

- one row, not a table, and no other transaction contends for it in the normal case
- the Stripe call runs under an explicit bounded context (10s), so the lock's
  lifetime has a ceiling that does not depend on Stripe's behaviour
- the traffic is human operators on a governance surface, not merchant traffic
- the alternative — re-check after the fact — does not remove the divergence, it
  only reports it, and leaves a real (if rare) state where Stripe holds a
  `trial_end` the local row never received

The connection pool is 5 per service (shared Cloud SQL); a 10s ceiling on one
held connection is acceptable at operator rates and would not be at merchant
rates. If this endpoint ever becomes automated, revisit this paragraph first.

## Where the Stripe call lives

`trial.Extend` gains the Stripe dependency, rather than the handler
orchestrating around it.

```go
type StripeTrialUpdater interface {
    GetSubscription(ctx context.Context, id string) (*stripe.Subscription, error)
    UpdateTrialEnd(ctx context.Context, in stripe.UpdateTrialEndParams) (*stripe.Subscription, error)
}

type Extender struct{ Stripe StripeTrialUpdater }

func NewExtender(su StripeTrialUpdater) *Extender
func (e *Extender) Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (ExtendResult, error)
```

The method signature is **identical** to today's free function, so
`platformadmin.TrialExtender`, `TrialExtenderFunc` and the entire handler are
untouched. Only the two `main.go` sites change.

Rejected alternatives, and why:

- **Handler orchestrates.** It would have to re-derive the state checks
  `Extend` already owns in order to know whether the trial is card-backed —
  a second derivation site, which is precisely what #353 existed to eliminate.
- **A separate `ExtendStripeManaged`.** The caller cannot know which path applies
  until it has read the row, so a third place would still have to check.

### The typed-nil hazard, stated because it has already cost a Critical

`main.go` must assign the updater **only** when `billingStripeClient != nil`:

```go
var trialStripe trial.StripeTrialUpdater
if billingStripeClient != nil {
    trialStripe = &trialStripeAdapter{c: billingStripeClient}
}
```

Assigning a nil `*billingstripe.Client` into the interface unconditionally makes
`e.Stripe != nil` **true** and the first method call panics — inside the
transaction, after the row lock has been taken. This is #288's second Critical,
verbatim, in a new location; the same shape already has a written precedent in
`main.go`'s `tenantTeardownClient` comment.

A comment at the call site is not enough, because a call site can be copied
wrongly and a comment cannot fail a test. **`NewExtender` normalises a typed nil
to a true nil** via a `reflect` check, and two tests pin it: one asserts
`NewExtender(typedNil).Stripe == nil`, the other asserts a card-backed extension
on such a build returns `ErrStripeManaged` rather than panicking, and writes
nothing.

## What the endpoint does

The handler (`internal/handlers/platformadmin/billing_trial_extend.go`) is
unchanged: idempotency reservation, reason-code validation, RFC3339 parsing,
`EmitOperatorAction`. All of it already works and none of it is specific to
whether Stripe is involved.

Inside one transaction:

1. `SELECT … FOR UPDATE` the `store_subscriptions` row for `store_id`.
2. Today's refusal checks, **unchanged and in the same order**:
   `StatusActive → ErrAlreadyConverted`; not `trialing`/`signup` →
   `ErrNotTrialing`; `EndsAt(sub)` not after `now` → `ErrNotTrialing`.
   (`newEnd.After(now)` stays *before* the transaction, as today.)
3. **Not card-backed** (`stripe_subscription_id` null or empty): write
   `trial_ends_at`, delete the `trial_reminders` rows, commit. Byte-for-byte
   today's behaviour. This is the common support case and it must not regress.
4. **Card-backed with `e.Stripe == nil`**: `ErrStripeManaged`. An unconfigured
   pod behaves exactly as it does today, and a silent local-only extension of a
   Stripe-managed trial is not reachable by any configuration.
5. **Card-backed with an updater**:
   a. `GetSubscription` — read Stripe's current `status`, `trial_end`, and
      `billing_cycle_anchor`.
   b. Refuse if Stripe's status is not `trialing` (`ErrStripeStateConflict`) —
      local and Stripe disagree, and reconciling silently is not this endpoint's
      job.
   c. Refuse if `newEnd` is more than two years after Stripe's
      `billing_cycle_anchor` (`ErrTrialEndTooFar`), stating the actual bound.
   d. `UpdateTrialEnd` with the exact Unix second, `proration_behavior=none`,
      **no** `Items`, **no** `TrialFromPlan`, and an idempotency key derived from
      the handler's already-scoped key so a retry cannot move the date twice.
   e. Write `trial_ends_at`, delete the reminder rows, commit.

Any failure in 5a–5d rolls back before the local row is touched. Only a failure
of the commit itself leaves Stripe ahead, which is the direction chosen above.

## Stripe client changes

`UpdateSubscriptionParams` gains `TrialEnd *int64`, and `Items` is populated
**only when `PriceID != ""`**:

- A `TrialEnd`-only call therefore cannot re-price. This is asserted by a test on
  the **outgoing form values**, not on the params struct — a struct assertion
  proves what we built, not what Stripe receives.
- A guard rejects a call with neither `PriceID` nor `TrialEnd` set, so the
  optional price cannot silently become a no-op update.
- `TrialFromPlan` is never set anywhere on this path.
- The two `planchange` callers pass `PriceID` and leave `TrialEnd` nil: their
  behaviour is unchanged, and a test pins that the plan-change call still sends
  `items[0][price]`.

A narrow `UpdateTrialEnd` wrapper expresses the extend call, so the extend path
cannot accidentally acquire a `PriceID` later.

`stripe.Subscription` gains `TrialEnd int64` and `BillingCycleAnchor int64`,
mapped field by field from the SDK object as the existing mapper does — a
projection, never a passthrough.

## Making the new path reachable: `GET /admin/billing/trials`

`trial.ListExpiring` filters `stripe_subscription_id IS NULL`
(`internal/billing/trial/expiring.go:57`), and that query backs #285's
`GET /admin/billing/trials`. **Card-backed trials are invisible in the console's
trial list.** Shipping #358 without addressing that delivers an endpoint whose
only entry point is a store id the console has no way to obtain from this
surface — a route that works and that nobody can reach.

So the list is widened, in this branch, in the narrowest way that does not
disturb what is already live:

- `trial.ListExpiring` gains a trailing `ListOptions{IncludeStripeManaged bool}`.
  The **zero value is today's behaviour**, so an omitted option can never widen a
  live result set by accident.
- The handler reads `?include_stripe_managed=true`, defaulting to false. #285's
  shipped contract is unchanged for every existing caller.
- Every row gains `stripe_managed` (a bool, **not** `omitempty` — it is a fact
  about every row, and an absent `false` is indistinguishable from an older
  server).
- **`CountExpiring` does not change.** It backs #282's `trials_expiring` KPI,
  whose meaning is "trials that will EXPIRE". A card-backed trial converts rather
  than expiring; counting it there would silently move a delivered metric. The
  list's `total` is therefore computed over the list's own scope, not by calling
  `CountExpiring`.

One caveat worth stating rather than discovering later: for a card-backed trial
that has never been extended, `trial_ends_at` is NULL and `EndsAt` derives
`created_at + 90d`. That agrees with Stripe **because `subscribe.go:132` sends
`EndsAt` as Stripe's `trial_end` at creation** — not by coincidence. If the two
ever diverge through a Stripe-side change made outside this system, the list
shows our value while Stripe holds the authoritative one. The `stripe_managed`
flag is what tells the operator which rows carry that caveat.

## Refusals

| condition | sentinel | HTTP | code |
|---|---|---|---|
| Stripe not configured, trial is card-backed | `ErrStripeManaged` (existing) | 409 | `stripe_managed` |
| Stripe says the subscription is not trialing | `ErrStripeStateConflict` | 409 | `stripe_state_conflict` |
| `newEnd` > Stripe anchor + 2 years | `ErrTrialEndTooFar` | 400 | `trial_end_too_far` |
| Stripe call failed or timed out | wrapped, not a sentinel | 502 | `stripe_unavailable` |

`stripe_unavailable` is deliberately **502**, distinct from the handler's
existing `503 unavailable` (which means "our own idempotency store is
unreachable"). An operator seeing 502 knows the refusal came from the dependency,
not from us, and that no local write happened.

`409 stripe_managed` therefore survives with a narrower meaning. #358's
acceptance says it must be "replaced by a working path, not merely removed";
keeping it as the unconfigured-pod refusal is the honest reading — the path
works wherever Stripe is configured, and where it is not, the old refusal is
still the correct answer.

## Response and audit

The response gains three fields, present only on the card-backed path:

- `stripe_subscription_id`
- `stripe_trial_end` — RFC3339, echoed from **Stripe's reply**, not from our
  request. What we asked for and what Stripe stored are different claims.
- `billing_anchor_moved: true` — the operator learns the billing date moved from
  the same call that moved it, rather than from a support ticket later.

The audit metadata additionally records `stripe_trial_end_unix` (the exact
integer sent), the previous Stripe `trial_end`, and the previous
`billing_cycle_anchor`. `EmitOperatorAction` is used unchanged (trap 3).

## Testing

The acceptance criterion is explicit that asserting a call *happened* is not
enough, because a stub returns the zero value for a field nobody set. So:

- **Exact integer on the wire.** Assert `seenForm.Get("trial_end")` equals the
  expected Unix second, using the stub-transport pattern already established at
  `internal/billing/stripe/subscription_create_test.go:51`.
- **The price cannot change.** Assert the extend call sends **no**
  `items[0][price]` key at all. Prove by mutation: delete the `PriceID != ""`
  guard and this test must fail.
- **Both branches in one suite** (trap 6). A card-backed *and* a card-less
  subscription. A suite containing only one cannot prove the branch exists.
- **Two stores** (trap 13). #286 shipped a cross-tenant Critical because its
  integration test used one store for every call.
- **The two-year bound at the boundary instant**, not near it — anchor + exactly
  2y passes, anchor + 2y + 1s refuses. "Close to the edge" is not the edge.
- **Stripe-state divergence**: local `trialing`, Stripe `active` → 409, and
  **no** local write. Assert the row is unchanged, not merely the status code.
- **Ordering**: a stub whose `UpdateTrialEnd` returns an error must leave
  `trial_ends_at` and the `trial_reminders` rows untouched. This is the failure
  ordering decision, expressed as a test.
- **Unconfigured build**: nil updater → `ErrStripeManaged`, not a panic; and a
  **typed**-nil updater likewise, asserted on `NewExtender` directly.
- **The widened list, both ways**: default excludes card-backed rows (today's
  contract), the flag includes them, each row is labelled, and `CountExpiring`
  is unmoved by a card-backed row. Prove the default by mutation — force the flag
  true and the default test must fail.

Verification set: `go vet -tags=integration ./...` (the only thing that compiles
build-tagged files), `go test ./...` from the service root, `-p 1`,
`TEST_DATABASE_URL` (not `TEST_DB_DSN`, which makes `internal/billing/trial` skip
19 tests silently), `-count=1`, and `set -o pipefail` wherever an exit code is
going to be reported as evidence.

**Before writing code**, diff the `internal/subscription/planchange` failing set
against `origin/main` in a throwaway worktree and record the exact test names.
That suite is a documented 9 FAIL, and it is this change's blast radius: the
safety net that would catch a broken plan change is already red, so "pre-existing"
has to be a measurement rather than an inheritance. A tenth failure must be
visible as a tenth.

## Verification against Stripe

Production holds 0 `store_subscriptions` rows, so it cannot exercise any of this
— the deploy will prove the route is mounted and refuses unsigned callers, and
nothing more. That class of evidence is what #288 showed to be nearly empty.

Verification therefore runs against **Stripe test mode**, using the estate's own
`sk_test_…` key read from GCP Secret Manager (never from inside a pod, never
written to a file, a commit, a log line or an issue comment):

1. Create a test-mode customer and a trialing subscription with a known
   `trial_end`.
2. Seed a local `store_subscriptions` row carrying that `stripe_subscription_id`.
3. Call the endpoint with a signed request and a new `trial_ends_at`.
4. Read `trial_end` **and** `billing_cycle_anchor` back from Stripe and assert
   both equal the new value — the anchor move is a claim in this document and
   must be confirmed, not assumed.
5. Confirm the local row, the reminder rows and the audit row all agree with it.

## Out of scope

- Reinstating an already-expired trial, local or Stripe-side. `ErrNotTrialing`
  continues to cover it, as #286 settled.
- Reconciling a subscription where local and Stripe already disagree. This
  endpoint refuses (`stripe_state_conflict`); repair is a different tool.
- Changing plan or price during an extension. Structurally prevented, not merely
  unimplemented.
- Ending a trial early via Stripe's `trial_end=now`. The endpoint validates
  `newEnd` is in the future, as today.
- Fixing the `planchange` 9 FAIL, the `internal/billing/trial` env-var split
  (#317), or the outbox (#336).
