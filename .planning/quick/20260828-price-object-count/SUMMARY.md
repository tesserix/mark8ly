---
slug: price-object-count
issue: tesserix/mark8ly#414
status: complete
completed: 2026-08-28
branch: fix/414-price-object-count
---

# Correct the Stripe Price object count: 8 → 42

## What was wrong

Two places stated how many Stripe Price objects `cmd/billing-bootstrap` creates,
both saying **8**. `init()` produces **42**.

The package doc contradicted itself before it contradicted the code: its own
parenthetical, `Starter+Studio+Pro × monthly+annual`, is 6 — and it omitted the
PPP tier entirely, which was the larger error.

## Counts, measured rather than assumed

Verified twice independently — once by the controller before filing, once by the
executor before writing anything, each via a temporary test calling
`AllDescriptors()` directly and deleted afterwards:

```
total=42  developed=6  ppp=36
```

## Changes

- `services/marketplace-api/internal/billing/pricing/catalog.go` — package doc
  rewritten to describe the real shape (6 developed-market prices carrying
  `currency_options` for 7 currencies, plus 36 PPP prices, one per currency) and
  to point at `AllDescriptors()` and the tests as the source of truth rather than
  restating a figure that can rot.
- `docs/superpowers/plans/2026-04-19-p20-prod-launch-readiness.md` — **two**
  occurrences, not one: B1.8 at line 134 and the go-live checklist at line 543.
  Both now describe verification by counting `mark8ly_`-prefixed lookup keys
  rather than checking against a fixed number.
- `services/marketplace-api/internal/billing/pricing/catalog_test.go` — added
  `TestCatalog_TotalDescriptorCount`.

## On the new test, and a correction made during review

`TestCatalog_DevelopedDescriptorCount` and `TestCatalog_PPPDescriptorCount`
already existed, so the per-tier counts were *already* under test. The doc drifted
anyway — the tests were never the missing piece, checking the doc against them was.

The new test was initially justified in its own comment as catching a descriptor
**swapped** between tiers. That justification is wrong: the two existing tests
already catch a swap (developed would read 5, PPP 37, failing both). The comment
was corrected to state what it actually adds — `len(AllDescriptors()) == 42` plus
`developed + ppp == len(all)`, which together catch a descriptor whose `Tier` is
*neither* value and is therefore counted by neither existing loop while both stay
green.

Worth recording that this was the same defect the issue itself is about — a
comment asserting something the code does not do — reintroduced in the fix for it,
and caught in review.

## Verification

- `go build ./...` clean
- `go vet ./internal/billing/pricing/...` clean
- `go test ./internal/billing/pricing/... -run . -v` — 32 tests pass
- **Discrimination check:** deleted one PPP descriptor (INR/Starter/Monthly); the
  new test failed (`expected: 36, actual: 35`); reverted; re-ran green. The test
  bites.

## Out of scope

No behaviour change. `developedAmounts`, `pppAmounts`, `init()` and every
descriptor are untouched.
