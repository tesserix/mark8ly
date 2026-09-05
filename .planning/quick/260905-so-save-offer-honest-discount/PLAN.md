# Make the save offer apply a discount, or stop claiming one

Half of #701. `internal/subscription/cancel/service.go:126-155` reverses a
scheduled cancellation and returns:

    SaveOfferMsg: "Save offer accepted. A 20% discount applies to your next billing cycle."

while the discount itself is only a log line. The merchant takes an action they
would not otherwise have taken — un-cancelling — in reliance on a statement
returned to them as settled fact, and is then billed full price.

## The blocker, established before planning

**`promo_codes` is never written in production.** `promo.Repository.Create`
exists with zero production callers; no migration seeds a code, and no admin or
platform endpoint creates one. Both `ApplyPromo` and `ValidateForSaveOffer`
begin with `repo.GetByCode`, so today they can only return not-found.

That is not a reason to defer this. The user's decision (#701) is:
**reverse the cancellation, but never claim a discount that was not applied.**
With no code present the merchant gets an honest message; when promo
provisioning lands, the same code path starts applying the discount with no
further change.

## Task

In `internal/subscription/cancel`:

1. Give `Service` an optional promo dependency — narrow interface, not the
   concrete `*promo.Service`, so tests can stub it. It must be **nil-safe**:
   a Service constructed without it behaves as today minus the false claim.
   Check for an import cycle before choosing where the interface lives
   (`promo` must not import `cancel`).
2. In `acceptSaveOffer`, after the `cancel_scheduled -> active` transition
   succeeds, attempt the discount: `ValidateForSaveOffer` then `ApplyPromo`.
3. **The message must follow what actually happened.** Only return the "a 20%
   discount applies" wording when `ApplyPromo` succeeded. Otherwise return a
   message that states the reversal alone and claims no discount. Do not
   invent a "pending" state — either it applied or it did not.
4. **A failed or impossible discount must NOT fail the reversal.** The merchant
   asked to un-cancel; that succeeded and is the more important half. Log the
   reason and carry on.
5. The promo code the save offer uses: a documented package constant, since
   nothing today can create it. `promo_codes` has
   `CHECK (char_length(code) >= 12)`, so the constant must satisfy that.
   State in its comment that provisioning a code with this exact string is what
   switches the discount on, and that until then the path is a no-op by design.

## Out of scope, deliberately

- **The win-back email** (`lifecycle/winback.go`). Its "20% off six months" is
  **baked into the template subject and body** (`email/templates_content.go:57,111,219`),
  not driven by the `promo` data key that gets passed. Fixing it means editing
  marketing copy or attaching a real code — a product decision, not the same
  mechanical change. Report it; do not touch it here.
- **Creating promo codes.** No provisioning path exists (see blocker). Out of
  scope and tracked separately.
- Anything in `internal/promo` itself. `ValidateForSaveOffer` and `ApplyPromo`
  are already correct; they have never been called.

## Done when

- Discount applied  -> reversal happens AND the message says a discount applies
- Discount fails / no promo dep / no code -> reversal STILL happens, message
  claims no discount, no error surfaced to the merchant
- A promo failure never leaves the subscription cancel_scheduled
- Tests genuinely execute — check whether this package's existing tests are
  integration-tagged or gate on TEST_DATABASE_URL (unset locally, they skip
  silently while printing `ok`). Factor the message decision into a pure
  helper if that is what it takes to get real coverage.

## Constraints

- TDD: test first, watch it fail.
- One atomic commit, single-line conventional message, NO attribution trailers.
- SHARED CHECKOUT: never run checkout/switch/stash/reset. Only add + commit.
- `go build ./...`, `go vet`, `go test` green before committing.
