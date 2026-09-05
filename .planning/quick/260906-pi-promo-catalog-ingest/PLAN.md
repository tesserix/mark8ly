# Ingest the console's promo catalog

Step 2 of #726. Step 1 (#742) made `promo_codes` able to hold what the console
publishes. This fetches it and populates the table.

Decision already taken (#726): the console owns promo definitions; mark8ly
consumes them and never mints. Do not add a local create path.

## Why rows, not just a cache

`promo_redemptions.promo_code_id` is `NOT NULL REFERENCES promo_codes(id)`
(migration 000061). A redemption must point at a real row, so an in-memory
cache of the catalog is not sufficient — the codes have to land in the table.

## Reuse, do not reinvent

`internal/billing/consolecatalog` already does exactly this shape for the plan
catalog: OAuth client-credentials token, fetch, cache with TTL, fail-open to
last-known, compiled fallback. Mirror it. Read `client.go` and `cache.go`
before writing anything.

**Credentials are shared, only the URL differs.** The console gates the route
on the `read-promo-catalog` capability, which rides in the token's roles claim
— the same machine identity can hold both capabilities. So reuse
`CONSOLE_CATALOG_TOKEN_URL` / `CLIENT_ID` / `CLIENT_SECRET` / `SCOPE` / `MODE`
and add only a URL, e.g. `CONSOLE_PROMO_CATALOG_URL`. Do NOT provision a
second OAuth client.

Follow `Config.Configured()`'s precedent: unconfigured is a SUPPORTED state,
not an error. With no URL set, the ingest is skipped and the service starts
normally.

## The contract (from the console route's own doc)

    { source, mode, revision_id, codes: [ {
        code, trial_extension_days,          // null when it extends no trial
        discount: {                          // null for trial-extension-only
          kind: "percent_off" | "amount_off",
          percent_off, amount_off_minor, currency,
          duration: "once"|"repeating"|"forever",
          duration_in_months,                // null unless repeating
          stripe_coupon_id                   // KEY ABSENT when not minted in mode
        },
        valid_from, valid_until, max_redemptions } ] }

## Mapping — put this in a PURE function and test it directly

| console | promo_codes |
|---|---|
| `code` | `code` |
| `trial_extension_days` | `trial_extension_days` |
| `discount.percent_off` (numeric, e.g. 50.00) | `discount_type='percentage'`, `discount_value` in **basis points** (50.00 -> 5000) |
| `discount.amount_off_minor` | `discount_type='amount'`, `discount_value` = minor units |
| `discount.duration` / `duration_in_months` | `max_duration_months`: `forever` -> NULL, `once` -> 1, `repeating` -> `duration_in_months` |
| `discount.stripe_coupon_id` | `stripe_coupon_id`, NULL when the key is absent |
| `valid_from` / `valid_until` / `max_redemptions` | same |
| — | `created_by` = a system marker naming the console as source |

Percent conversion is the easiest thing to get wrong: `numeric(5,2)` -> basis
points is x100, and 33.33 must become 3333 without float drift. Parse as a
decimal string, not a float64.

Leave `max_per_email` (default 1), `min_effective_price_per_currency`,
`allowed_plans` and `annual_only` alone — the console cannot express them and
mark8ly's defaults are the current policy (flagged on #726, not decided).

## Sync semantics

- **Upsert on `code`** (it has a unique index). A re-sync updates in place.
- **A code withdrawn from the catalog is EXPIRED, never deleted.** Set
  `valid_until = now()` where it is currently null or later. `Validate` already
  honours `valid_until` (`validator.go:88`), and `promo_redemptions` FKs to
  these rows, so deleting would break an audit trail and could fail on the FK.
- **A code that fails mapping must not abort the batch.** Log it, count it,
  continue. One malformed row must not block every other code.
- Reject anything the DB would reject anyway (no benefit at all, a partial
  discount) at the mapping step, with a clear reason — a constraint violation
  surfacing as a raw pg error is a worse diagnostic.

## Running it

At boot and on an interval, mirroring how the plan catalog is refreshed. Nil
and unconfigured must both be safe. Failure to reach the console must never
fail startup — the previously-ingested rows remain valid.

Add a metric for codes ingested / skipped / expired, labelled by reason.

## Out of scope

- Redemption itself and the onboarding field (#620).
- Attaching the coupon at redeem time — needs `Coupons: Read` on the Stripe
  key, which is not yet granted.
- Any local promo-code creation.

## Done when

- The mapper is a pure function with tests covering: percent, amount,
  trial-only, absent coupon id, each duration, and each rejection
- A withdrawn code is expired, not deleted
- Unconfigured is a no-op and the service still starts
- `go build ./...`, `go vet` (plain and `-tags integration`), `go test` green

## Constraints

- One atomic commit, single-line conventional message, NO attribution.
- SHARED CHECKOUT: no checkout/switch/stash/reset beyond this branch.
- `TEST_DATABASE_URL` is unset here; integration tests skip silently while
  printing `ok`. Make the mapper unit-testable without a DB and report
  honestly what ran.
