# Pricing: one source of truth, generated (#413, #393) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A public, search-indexed page advertises Starter at $29/mo against a billing system that charges $19. Delete the table that lies, and make the remaining two structurally incapable of drifting apart again.

**Architecture:** `catalog.go` stays the single source of truth for every price billing owns. A generator emits `packages/ui/src/subscription/pricing-data.ts` from it, plus a small explicit declaration of the display-only values billing does not own. A CI check regenerates and fails on any diff. The admin `/pricing` page drops its private table and consumes the shared one.

**Issues:** [#413](https://github.com/tesserix/mark8ly/issues/413), [#393](https://github.com/tesserix/mark8ly/issues/393)

---

## Findings that shape this plan

Verified against the code, not assumed. Several contradict the obvious reading.

**1. The admin page's whole table is wrong, not one row.** `apps/admin/app/pricing/page.tsx:49` `STATIC_PRICING_CATALOGUE`, public and search-indexed (`robots: {index: true}`, `:19`). Against `catalog.go` Starter monthly: USD 1900→quoted 2900, GBP 1500→2300, EUR 1700→2700, CAD 2500→3900, AUD 2900→4500. Starter INR quotes the **Studio** price. Only Pro USD agrees.

**2. `packages/ui/src/subscription/pricing-data.ts` already agrees with billing** on the rows it covers (`:66` Starter USD 1900/18200 = `catalog.go:78,88`). It already feeds onboarding. It is the table to keep.

**3. #393 is comment-only — the code is correct.** `internal/billing/stripe/price.go:17-31` holds `zeroDecimalCurrencies` and `stripeUnitAmount`, which divides by 100 at the Stripe boundary. The catalog's ×100 storage is a deliberate internal convention, so VND reaches Stripe correctly. `catalog.go:159`'s comment claiming IDR is zero-decimal is wrong and misleading — IDR is an ordinary two-decimal currency — but nothing computes from that comment. **Do not "fix" any amount.**

**4. The two tables cover different sets, so a naive generator would silently change prices.** This is the central design constraint:

| piece | in `catalog.go`? | consequence |
|---|---|---|
| starter/studio/pro × 7 developed + INR | yes | the genuine overlap — generate these |
| MYR, THB, PHP, IDR, VND (PPP) | yes | **absent from the TS table**; do NOT add them |
| AED, JPY | no | TS-only, marked "USD fallback, deferred", out of v2.3 launch scope |
| `proApp` add-on | no | USD-only in billing; the TS table repeats 19900 across currencies for display |
| `annualMonthlyEquivalent` | no | derived: `round(annual/12)`, verified (18200/12→1517; 238800/12→19900) |

**5. `/pricing` is already dynamically rendered** — it calls `cookies()` for currency, which opts out of static rendering. So the value of generated data is not static-vs-dynamic; it is that prices remain a **build-time constant rather than a request-time fetch**, with no latency added to an indexed page and no failure mode where a console outage blanks public pricing. Preserve that property.

**6. There is a generator convention in this service** — `internal/handlers/platformadmin/cmd/genvectors`. Follow it rather than inventing a location.

---

## Global Constraints

- **No price may change.** The generator's first output must be **byte-identical** to the committed `pricing-data.ts` for everything it covers. That equality is the proof the generator is faithful, and it is the acceptance gate for Task 2. If output differs, the generator is wrong — not the committed file.
- **Do not add PPP currencies (MYR, THB, PHP, IDR, VND) to the TS table.** They are absent today; adding them silently widens what marketing surfaces can display. Out of scope.
- **Do not change any amount in `catalog.go`.** #393 is a comment fix. Touching an amount is a defect.
- **`/pricing` keeps its current six currencies** (USD, GBP, EUR, INR, AUD, CAD). Widening is a marketing decision, not a defect fix, and would risk quoting currencies where signup is blocked.
- No migration. Any DDL means this plan is wrong.
- Go: run from the service root, `cd services/marketplace-api && go test ./... -count=1`, never path-scoped. `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` must pass.
- TS/Next: `npm` at the repo root (there is a `package-lock.json`); do not introduce `pnpm` here.
- Conventional single-line commit messages, no signature, no `Co-Authored-By` trailer.
- **Pre-existing failures — not yours to fix:** `internal/billing/trial/subscribe_integration_test.go` (19 tests, #317), `internal/subscription/planchange` integration (9 FAIL), `internal/whitelabel` integration (nil-pointer panic).

---

## File Structure

| file | responsibility |
|---|---|
| `services/marketplace-api/internal/billing/pricing/catalog.go` (modify) | #393: replace the misleading zero-decimal comment. Amounts untouched. |
| `services/marketplace-api/internal/billing/pricing/cmd/genpricing/main.go` (create) | the generator: `catalog.go` + display-extras declaration → `pricing-data.ts` |
| `services/marketplace-api/internal/billing/pricing/display_extras.go` (create) | the display-only values billing does not own — AED/JPY fallback, `proApp`, and the currency allowlist for the TS table |
| `packages/ui/src/subscription/pricing-data.ts` (regenerate) | now generated; header marks it so and names the command |
| `services/marketplace-api/internal/billing/pricing/genpricing_test.go` (create) | regenerates in-memory and fails when the committed file is stale |
| `apps/admin/app/pricing/page.tsx` (modify) | delete `STATIC_PRICING_CATALOGUE`, consume `SHARED_PRICING_CATALOGUE` |
| `apps/admin/app/pricing/PricingClient.tsx` (modify if needed) | GST disclosure for AUD |

---

## Tasks

### Task 1 — #393: the comment, and nothing else

- [ ] Replace `catalog.go:159`'s zero-decimal note with the wording the issue supplies: VND is zero-decimal and Stripe receives `unit_amount` unmultiplied; this catalog stores ×100 internally and `stripeUnitAmount()` divides at the boundary for currencies in `zeroDecimalCurrencies`; **IDR is not one of them** and ×100 is simply correct there.
- [ ] Verify by reading `internal/billing/stripe/price.go:17-31` that the described behaviour is what the code does, and cite it in the comment.
- [ ] **Change no amount.** `git diff` must show comment lines only.

**Verify:** `go test ./... -count=1` from the service root; `git diff --stat` shows one file, comment-only.

### Task 2 — The generator, proven faithful

- [ ] `display_extras.go`: declare, with a comment justifying each, the values billing does not own — the AED and JPY fallback rows (and that they are out of launch scope, retained for display), the `proApp` add-on prices, and the ordered currency allowlist the TS table carries. Deriving these from `catalog.go` is not possible; inventing them in the generator would hide them.
- [ ] `cmd/genpricing/main.go`: emit `pricing-data.ts` from `pricing.developedAmounts`/`pppAmounts` plus `display_extras.go`. Compute `annualMonthlyEquivalent` as `round(annual/12)`. Preserve the existing file's type declarations, doc comments, currency order and formatting.
- [ ] Add a generated-file header naming the exact command to regenerate and stating the file must not be hand-edited.
- [ ] **The acceptance gate:** run the generator and diff against the committed `pricing-data.ts`. It must be **byte-identical apart from the new header**. Paste the diff in your report. A non-empty diff means the generator is wrong — do not "fix" the committed file to match the generator.
- [ ] `genpricing_test.go`: regenerate in memory, compare to the committed file, fail with a message naming the regenerate command. Must fail loudly if it reads nothing — a guard that silently compares empty to empty is worse than none.
- [ ] Prove it fails: perturb one amount in the committed TS file, run the test, capture the failure, restore.

**Verify:** `go test ./... -count=1`; the byte-identity diff; the captured failure and restore.

### Task 3 — Retire AED and JPY, through the generator

Context: we do not serve Arab or SEA markets. `country-map.ts` nonetheless maps `AE→AED`,
`JP→JPY`, `SA→SAR` and the five SEA countries to their own currencies, and the shared table
carries AED/JPY rows whose values are the **USD numbers**. So a UAE visitor is quoted
`AED 19.00` where the real price is ~AED 70 — roughly 27% of it, on an indexed page.
Removing those rows makes AE/JP fall through to the USD row, which Task 4 then renders
*labelled USD*. That is the honest answer for a market we do not sell in.

- [ ] Remove the AED and JPY entries from `display_extras.go` — they exist only because the
      committed file had them.
- [ ] Regenerate `pricing-data.ts`. The diff must contain **exactly** the AED and JPY rows,
      nothing else. Paste it in your report; any other changed line means the generator is
      not faithful and this task stops.
- [ ] Update the file's doc comment, which currently explains AED/JPY as retained fallbacks.
- [ ] Confirm no consumer indexes `AED`/`JPY` directly (`country-map.ts` may still map to
      them — that is Task 4's fallback path, not a compile-time reference).

This is deliberately the generator's first real change: it demonstrates that a pricing edit
now flows through `catalog.go` + `display_extras.go` and arrives as a reviewable diff.

**Verify:** `go test ./... -count=1`; the AED/JPY-only diff; the staleness guard still green.

### Task 4 — Point `/pricing` at the shared table, fix the fallback, disclose GST

- [ ] Delete `STATIC_PRICING_CATALOGUE` from `apps/admin/app/pricing/page.tsx` and consume `SHARED_PRICING_CATALOGUE` from `@repo/ui` (already exported from the barrel; already used by onboarding).
- [ ] Reconcile the types: the page's `PricingCatalogue` uses `Record<string, PlanPrice>`, the shared one `Partial<Record<Currency, PlanPrice>>`. Adapt at the boundary; do not weaken the shared type.
- [ ] Keep the rendered currency set at today's six. A currency the shared table lacks must not render as blank or zero — handle absence explicitly.
- [ ] Add the "Plus GST" disclosure wherever AUD is shown. §19.4 requires it and `catalog.go` marks AUD `TaxBehavior: "exclusive"`. Mirror the existing wording at `apps/onboarding/components/marketing/Pricing.tsx:90-94` rather than writing a second phrasing.
- [ ] Verify the rendered page now shows Starter USD $19/mo and $182/yr, matching billing.

**Verify:** build the admin app; typecheck; confirm the six currencies render and AUD carries the disclosure.

- [ ] **The fallback must carry its own currency.** `PricingClient.tsx:39` returns
      `plan.prices[currency] ?? plan.prices['USD']` and line 213 renders it with
      `currency={currency}` — so a Thai visitor sees the USD *number* labelled `฿`, about 3%
      of the real price. Change the resolver to return the price **and the currency it
      actually resolved to**, and render that. `apps/onboarding/components/marketing/Pricing.tsx:253`
      already does this correctly for the USD-only add-on (`currency="USD"` hardcoded) —
      follow that precedent rather than inventing a second approach.
- [ ] Apply the same fix to `apps/onboarding/components/marketing/Pricing.tsx` (:219, :229),
      which shares the shape.
- [ ] Verify concretely: a visitor whose currency has no row sees **$19 / USD**, never `฿19`
      or `AED 19`.

### Task 5 — Make an unpriceable currency unreachable

Root cause of the mislabelling: **three lists disagree.** 12 countries can onboard
(`apps/onboarding/app/onboarding/page.tsx:18-27`, gated on a tested shipping carrier), the
pricing table has 10 currencies, and `SUPPORTED_CURRENCIES`
(`packages/ui/src/subscription/country-map.ts:12`) has **24** — including THB, VND, IDR,
MYR, PHP, SAR, KRW, HKD, BRL, MXN, ZAR, NGN, KES, none of which has a price row.
`normalizeCurrency` passes all 24 through, so an unpriceable currency reaches a renderer.

**Do NOT add a fourth hand-maintained list.** Derive the priceable set from the pricing
table itself, so it cannot drift from the prices by construction.

- [ ] Export the set of currencies that actually have rows, derived from
      `SHARED_PRICING_CATALOGUE` (every plan must have the currency, not merely one plan).
- [ ] `normalizeCurrency` returns `USD` for any currency without a price row. Keep
      `COUNTRY_TO_CURRENCY` complete — geo knowing that TH uses THB is correct and useful;
      what must not happen is *displaying* a price in a currency we cannot price. Narrowing
      the `Currency` type instead would ripple through that map and is not the goal.
- [ ] Defence in depth: `getPlanPrice` / `getAddOnPrice` (`pricing-data.ts:142-150`) return
      the price **and the currency actually resolved**, mirroring `resolvePricedPlan`
      (`apps/onboarding/app/page.tsx:104-118`), whose comment already names this exact bug:
      *"would otherwise let us label a USD amount with the visitor's currency code — a wrong
      price in the schema."* Update `Pricing.tsx:219,229` to render the resolved currency.
- [ ] Add `NZD` to `/pricing`'s allowlist (`apps/admin/app/pricing/page.tsx:103`). NZ is a
      supported onboarding country (ShipEngine) and the shared table has NZD rows, so a New
      Zealand merchant is currently shown USD for no reason. Better: derive that allowlist
      from the same exported set rather than restating it.
- [ ] Tests: a visitor currency with no row renders **USD amounts labelled USD** on both
      surfaces; NZD renders NZD; the derived set matches the table's actual coverage.

**Verify:** typecheck and build both apps; the new tests; confirm no consumer of
`COUNTRY_TO_CURRENCY` broke.

### Task 6 — Close out

- [ ] Comment on #413 with what shipped and the drift guard; comment on #393 confirming code was correct and only the comment changed.
- [ ] Note in #305's thread that the generator exists and that its **input** is the only thing #305 needs to swap — the emitted file, the staleness check and the deploy path are already in place.

---

## Out of scope, deliberately

- **The console as the source.** That is mark8ly#305 / tesserix-home#329, and it is console-blocked. This plan's generator is designed so that arc changes only the generator's input.
- **The `repository_dispatch` automation.** It needs the console side to exist to fire. When built, it should open a **PR rather than push to main**: a mistaken console price edit should not publish public prices with no human in the loop. `base-image-refresh.yml` is the in-repo precedent for the trigger.
- **Widening `/pricing`'s currency set**, and **adding PPP currencies to the TS table.** Both are marketing decisions.
- **Any amount change.** If a price looks wrong, that is a separate issue with a separate decision.
