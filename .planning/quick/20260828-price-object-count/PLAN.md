---
slug: price-object-count
issue: tesserix/mark8ly#414
created: 2026-08-28
---

# Correct the Stripe Price object count: 8 → 42

Two places state how many Stripe Price objects `billing-bootstrap` creates. Both
say **8**. The real number is **42**, measured by calling `AllDescriptors()`
directly on 2026-08-28:

```
total=42  developed=6  ppp=36
```

## Why it matters beyond a stale comment

`docs/superpowers/plans/2026-04-19-p20-prod-launch-readiness.md:134` is item
**B1.8**, a live launch-readiness checkbox owned by "Platform lead", whose
acceptance is a `cmd/billing-bootstrap` run against **prod** Stripe.

An operator verifying that run against "8 Price objects" sees roughly eight
developed-market prices, ticks the box, and stops. The 34 PPP prices that did
not get created fail **only** for INR, MYR, THB, PHP, IDR and VND merchants —
the six markets least likely to appear in a smoke test, and the ones where
`PriceIDFor` (`internal/billing/stripe/update.go:123`) returns a Stripe
`ErrNotFound` that surfaces as a generic error rather than "catalog drift".

This becomes materially more likely to bite now that tesserix-home#396 has
shipped and the console can write the catalog to Stripe.

## Tasks

1. **`internal/billing/pricing/catalog.go:3`** — the package doc says
   "8 developed-market Price objects (Starter+Studio+Pro × monthly+annual)".
   It contradicts itself before it contradicts the code: that parenthetical is
   **6**, not 8. It also omits the PPP tier entirely, which is the larger error.
   State the real shape: 6 developed-market Price objects (3 plans × 2 periods),
   each carrying `currency_options` for 7 currencies, plus 36 PPP Price objects
   (3 plans × 2 periods × 6 currencies) = 42 total.

2. **`docs/superpowers/plans/2026-04-19-p20-prod-launch-readiness.md:134`** —
   B1.8 says "create all 8 Price objects + products". Same correction, and it
   should say what a correct verification looks like rather than just a number.

3. **A test asserting the count by tier**, so the next divergence fails CI
   instead of a production checklist. `internal/plangate/matrix_test.go` already
   uses this pattern for `allFeatures` — follow it.

## Verification

- `go test ./internal/billing/pricing/...` passes, including the new test
- `go build ./...`
- `go vet ./internal/billing/pricing/...`
- The new test FAILS if a descriptor is added or removed without updating it

## Out of scope

Anything about what the catalog contains or how it is published. This is a
counting and documentation correction only — no behaviour change.
