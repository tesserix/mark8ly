# Deliver the trial extension a promo code promises (#620)

`promo_codes.trial_extension_days` is ingested from the console (#745), stored,
and documented on the model as *"the number of days added to the store trial on
redemption (#620)"*. `promo.Service.ApplyPromo` never reads it.

So a merchant who redeems a trial-extension code today gets a `200`, a ledger
row, and **no extra days**. Nothing errors and nothing logs, because the
validator already handles a discount-less row correctly — it leaves the price
untouched and accepts, on the stated understanding that "the benefit is
delivered elsewhere". Elsewhere does not exist.

`validator.go` warns about exactly this failure mode one layer down:

> both are silently correct arithmetic for the wrong code, and both would hide
> a failed ingest.

## Tasks

### 1. One definition of "can this trial move"

`trial.Extend` refuses a converted, non-trialing or already-lapsed trial inside
its row lock. A caller that wants to refuse *before* writing anything needs the
same rule, and a second copy of it would drift.

- [ ] Extract `trial.Extendable(sub, now) error` from `Extend`'s refusal
      switch, returning the same sentinels. `Extend` calls it under the lock;
      nothing else changes about `Extend`.
- **Done when** `Extend`'s behaviour is byte-identical and the switch exists once.

### 2. Apply the extension on redemption

- [ ] `promo.TrialExtender` — the narrow interface satisfied by `*trial.Extender`.
- [ ] `ApplyInput.Sub *subscription.StoreSubscription` — the row the extension
      is computed from. All three callers already hold it.
- [ ] Pre-flight, before any write: a trial-extension code on a subscription
      that fails `trial.Extendable` is rejected with a new reason, so the
      merchant gets a 422 and **their redemption is not burned**.
- [ ] Extend AFTER the ledger row is written, with
      `callerIdemKey = "promo_redeem:<redemption id>"`, per #620.
- [ ] Base date is `trial.EndsAt(sub) + N days` — never `created_at + TrialDays`.
      #620 names this specifically: `Extend` takes an absolute end, so computing
      from the signup date silently overwrites an operator-granted extension.
- **Done when** a 14-day code on a trial ending in 10 days sets the end 24 days
  out, and a code applied to an operator-extended trial adds to the stored end.

### 3. Fail honestly, in both directions

- [ ] Extension fails → roll back the ledger row and the Stripe coupon, so a
      retry works rather than reporting the code spent.
- [ ] **Except** `trial.ErrStripeAppliedLocalWriteFailed`: Stripe has already
      moved the merchant's billing date. Rolling back would hide it. Keep the
      row and surface the error.
- [ ] A trial-extension code with no extender wired, or no subscription passed,
      is a **server** fault and must 500 — never a 422 telling the merchant a
      valid code is invalid.
- **Done when** each path has a test that fails if the branch is removed.

### 4. Say what was granted

- [ ] `ApplyOutput.TrialExtensionDays` / `TrialEndsAt`, set through `terms()` so
      `ValidateCode` describes the same row identically — the rule
      `PercentOffBps` already follows for the win-back email (#727).
- [ ] Surface both on the apply-promo response.
- **Done when** the confirmation can state "+14 days, now ending 3 Nov" from
  the response alone.

### 5. Wire it

- [ ] `promo.NewService` gains the extender at both `main.go` construction
      sites, sharing the `trial.NewExtender(trialStripe)` already built there.

## Out of scope

The onboarding field (#620's last checkbox) — it needs the validate endpoint to
be reachable unauthenticated, which is a separate surface with its own
rate-limiting question. This task makes the benefit real for the redemption
paths that already exist.

## Decision recorded: a lapsed trial cannot be extended

#620 open question 1. `extend.go` already settled it — *"Reinstating an
already-expired trial is out of scope"* — and this change adopts that answer
rather than inventing a second one: the code is refused with
`trial_not_extendable`, and the merchant keeps their redemption.
