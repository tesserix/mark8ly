# Let promo_codes hold what the console publishes

Step 1 of #726 (decision: the console owns promo definitions; mark8ly ingests).
This is the SCHEMA half only — no client, no ingest. Those follow once the
table can hold the data.

## Why the table cannot hold it today

mark8ly's `promo_codes` (migration 000060) and the console's (tesserix-home
0046) were designed independently. Four mismatches, one of them fatal:

1. **Trial-extension-only codes cannot exist here.** The console allows
   `trial_extension_days` set with `discount` null. mark8ly has
   `discount_type NOT NULL`, `discount_value NOT NULL CHECK (> 0)`, and **no
   trial-extension column at all**. #620 — "redeem promo codes at onboarding
   to extend the trial" — is entirely about these codes.
2. `stripe_coupon_id VARCHAR(100) NOT NULL` here; on the console it lives in a
   child table keyed on `(promo_code_id, mode)` and the API **omits the key**
   when the coupon is not minted in that mode.
3. `CHECK (char_length(code) >= 12)` here; the console has no length
   constraint and its own example is `LAUNCH50` (8 chars).
4. Percent is basis points here, `numeric(5,2)` there. A conversion, not a
   blocker — noted so the ingest does not get it wrong.

**Loosening costs nothing today:** `promo.Repository.Create` has zero
production callers, no migration seeds a row, and no endpoint creates one. The
table is empty, so no existing data has invariants to weaken.

## Task — migration 000131

Write `migrations/000131_promo_codes_console_sourced.{up,down}.sql`:

- `ADD COLUMN trial_extension_days INTEGER`, with
  `CHECK (trial_extension_days IS NULL OR trial_extension_days > 0)` matching
  the console's own constraint.
- `DROP NOT NULL` on `discount_type`, `discount_value` and `stripe_coupon_id`.
  Note the existing `CHECK (discount_value > 0)` already tolerates NULL, since
  a CHECK passes when it evaluates to NULL — verify rather than assume.
- Replace `promo_codes_code_length`. The `>= 12` rule cannot hold for
  console-defined codes. Do NOT drop length validation entirely — an empty or
  one-character code is a bug either way. Pick a floor that admits the
  console's vocabulary and say why in a comment.
- **Add the invariant that replaces what was lost.** A row must carry a
  discount OR a trial extension — a code that does neither is meaningless:
  `CHECK (trial_extension_days IS NOT NULL OR discount_type IS NOT NULL)`.
- **Add the pairing invariant**: `discount_type` and `discount_value` are both
  set or both null. Individually nullable columns otherwise permit a type with
  no value.
- The `promo_codes_stripe_idx` index on `stripe_coupon_id` should become a
  partial index (`WHERE stripe_coupon_id IS NOT NULL`) now that the column is
  nullable — the rows without one are not worth indexing.

Write a real `down` that reverses each step. It must be honest: reverting
means rows that violate the old NOT NULLs cannot exist, so the down migration
should fail loudly if such rows are present rather than deleting them.

**Bump `ExpectedSchemaVersion` to 131 in `migrations.go`.** The service refuses
to start when it mismatches, so forgetting this breaks boot rather than
degrading quietly.

## Task — the Go model

`internal/promo/model.go`'s `PromoCode`: make `StripeCouponID`, `DiscountType`
and `DiscountValue` nullable (pointers), and add `TrialExtensionDays *int`.
Update the struct's doc comment — it currently says "One row per Stripe Coupon
we issue" and "≥12 chars", both of which stop being true.

Then fix the compile fallout. `internal/promo/service.go` and `validator.go`
read these fields; they must handle a nil discount rather than assuming one.
**A trial-extension-only code must not be treated as a zero discount** — that
would silently apply "0% off" instead of extending a trial.

## Out of scope

- The catalog client and the ingest itself (step 2).
- Any change to `promo_redemptions`.
- `max_per_email`: mark8ly defaults it to 1 as an abuse control (§7.3) and the
  console contract cannot express it. Leaving the default is the current
  policy; flagged on #726, not decided here.

## Done when

- A trial-extension-only row (no discount, no coupon id) INSERTs successfully
- A row with neither a discount nor a trial extension is REJECTED
- A row with `discount_type` set and `discount_value` null is REJECTED
- Existing promo validation still behaves for a discount-bearing code
- `go build ./...`, `go vet`, `go test ./internal/promo/...` green
- Migration applies and reverts against a real database

## Constraints

- One atomic commit, single-line conventional message, NO attribution.
- SHARED CHECKOUT: no checkout/switch/stash/reset beyond this branch.
