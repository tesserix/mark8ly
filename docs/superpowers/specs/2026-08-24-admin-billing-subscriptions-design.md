# Admin Billing Subscriptions — Design

**Issue:** #284, folding in #328. Part of the platform console integration series (#260), after #274, #275, #276, #277, #279, #282, #283, #285.

**Goal:** A cross-tenant view of every subscription on the platform, with plan, status and price — and, in the same change, add the price to `/admin/billing/trials`, which shipped without one on a conclusion that turned out to be wrong.

## The issue's flagged risk resolves: no projection is needed

#284 warns that this "may need a **projection** rather than a query", because mark8ly's subscription surface is per-store and plans are Go descriptors rather than DB rows. It asks that the answer be recorded here rather than guessed at.

**It is a query.** `store_subscriptions` carries everything the view needs as ordinary columns: `tenant_id`, `store_id`, `plan`, `status`, `subscription_period`, `billing_currency`, `price_tier`, `current_period_end`, `cancel_at_period_end`, `created_at`.

Plans being Go descriptors affects only **prices**, and `internal/billing/pricing/catalog.go` supplies those, keyed by plan / period / currency with `price_tier` selecting the tier. So the endpoint is a plain paginated query, a per-row catalog lookup, and one batch tenant-name call — the same shape as #285.

No slip. The console can plan on that.

## The correction this folds in (#328)

#285's spec states that mark8ly holds no prices, citing `PriceIDFor` returning a Stripe price ID, and `/admin/billing/trials` therefore ships with **no `amount`**, recording the issue's money criterion as unsatisfiable.

That was wrong. `internal/billing/pricing/catalog.go` holds prices **in minor units**:

```go
type Amount struct {
    Currency        string // lowercase ISO 4217
    UnitAmountMinor int64  // cents, paise, sen, satang…
    TaxBehavior     string
}
```

with `LookupBaseline`, `LookupPPPOption`, `DevelopedCurrencyOptions` and `MustGet`. The earlier conclusion came from finding one plausible location and stopping, rather than searching for the authority — the same mistake that produced #282's broken counter, in a different direction: **concluding that something does not exist also requires a search, not a single lookup.**

Nothing wrong shipped — no fabricated figure — but a trial row carries `plan`, `period` and `billing_currency`, so the conversion price was computable all along.

**One resolver serves both endpoints**, so they cannot disagree about what a plan costs.

## The money resolver

```go
// Resolve returns the catalog price for a subscription's plan, period and
// currency, or ok=false when no price can be determined.
func Resolve(plan, period, currency string, tier PriceTier) (Money, bool)
```

Three rules, each load-bearing:

1. **Never call `pricing.MustGet`.** It panics on a miss (`panic("pricing: no amount for plan=…")`). A console read must not panic on a subscription whose plan/currency combination is absent from the catalog. Use `DevelopedCurrencyOptions` then `LookupPPPOption` — the same pair `MustGet` wraps, minus the panic.
2. **Lowercase to look up, uppercase to serve.** Catalog keys are lowercase ISO 4217; `billing_currency` is `char(3)`; the surface's money contract is uppercase (`"GBP"`). Both conversions are explicit.
3. **Omit, never fake.** When `billing_currency` is NULL, or the catalog has no entry, the row carries **no `amount` key** — not `null`, not `0`, not a guessed currency. This is the same discipline as #282's `501` for GMV and #285's absent amount: a number the system cannot determine is not reported as a number.

`TaxBehavior` (`exclusive` for `aud`) is **not** carried. The console renders a price, not a tax-inclusive total, and shipping a field whose meaning changes the number without the console using it invites misreading. Recorded here so the omission is a decision rather than an oversight.

## `GET /admin/billing/subscriptions`

Query: `status`, `plan`, `page`, `limit` (default 50, clamp 500). Standard envelope; empty is `200` with `[]`. Ordered `created_at DESC`.

```json
{ "data": [
    { "tenant_id": "<bare uuid>",
      "tenant_name": "Acme Trading",
      "store_id": "<bare uuid>",
      "plan": "studio",
      "period": "monthly",
      "status": "active",
      "amount": { "amount": 1500, "currency": "GBP" },
      "current_period_end": "2026-09-24T00:00:00Z",
      "cancel_at_period_end": false }
  ],
  "pagination": {"page": 1, "limit": 50, "total": 1} }
```

### All eight statuses, not four

The issue names `trialing`, `active`, `past_due`, `expired` and says **do not invent a second vocabulary**. `internal/subscription/models.go` defines **eight**: those four plus `signup`, `payment_action_required`, `cancel_scheduled`, `store_closed`.

Filtering to the issue's four would be inventing a second vocabulary — a truncated one — and would hide from the console exactly the subscriptions most needing an operator, `payment_action_required` among them. All eight are accepted as filters and returned as values.

An unrecognised `status` or `plan` value is a `400 validation_error` naming the parameter, not a silently empty list. An empty list must mean "none match", never "you asked for something I did not understand".

### Empty is `[]`, never `501`

The issue is explicit: a product with **no billing concept** returns `501`; an empty list means "no subscriptions", which is a different and real answer. mark8ly has a billing concept, so this endpoint never returns `501`.

Worth knowing: `store_subscriptions` is **empty in production today** (verified read-only against the replica — 4 tenants and 4 stores exist, but subscriptions are created by an explicit merchant-initiated call requiring a Stripe customer, and no merchant has entered that flow). So this endpoint will correctly serve `[]` until the first merchant subscribes. That is the real answer, and it is why `501` would be wrong.

### Tenant names

One batch call through `tenantdirectory`'s `IDs` filter, built in #285 — distinct ids per page, not per row. Unreachable platform-api yields `503`, consistent with the rest of the surface.

## Architecture

| Layer | Responsibility |
|---|---|
| `marketplace-api/internal/billing/pricing` | Already owns the catalog. Gains nothing. |
| `marketplace-api/internal/subscription` | A cross-tenant list query. Estate-wide and unscoped by tenant, so it carries the same warning comment as `CountExpiring` — the platformadmin surface is HMAC-gated with no tenant context, and tenant-facing handlers must not call it. |
| `marketplace-api/internal/handlers/platformadmin` | The money resolver (shared), both handlers' wire shapes. |

The resolver lives beside the handlers rather than in `pricing`, because it encodes a *wire* decision (omit on miss, uppercase on the wire) rather than a pricing fact. `pricing` stays the catalog's owner.

## Testing

- Amount resolved correctly for a **developed**-tier row and a **PPP**-tier row.
- Amount **omitted** when `billing_currency` is NULL, and when the catalog has no entry for the combination — proving `MustGet`'s panic path is unreachable from a request.
- Lowercase lookup / uppercase wire asserted explicitly, with a currency where the difference is visible.
- All eight statuses filterable; an unknown status is `400`, not an empty `200`.
- Empty result is `200` with `[]`.
- One deduplicated tenant lookup per page; no lookup at all for an empty page.
- **Both endpoints agree on the amount** for the same plan/period/currency, asserted by driving both handlers in one test over one fixture — the pattern that caught the KPI/listing divergence in #285.
- Golden fixture per endpoint, each proven by mutation against a field rename and a field addition.

## Out of scope

- Changing what `/admin/billing/trials` already ships beyond adding `amount`.
- `TaxBehavior` on the wire (decided above).
- Any change to the pricing catalog's contents. This design reads it.
