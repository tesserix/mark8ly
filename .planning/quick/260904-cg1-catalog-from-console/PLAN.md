---
id: 260904-cg1
slug: catalog-from-console
date: 2026-09-04
issue: tesserix-home#329 (option B), tesserix/mark8ly#305
kind: quick
---

# Generate the catalog's data from the console

## Why this, and not the issue as filed

tesserix-home#329 asks for the marketing table to build from a console snapshot.
Measured today, that change would buy **nothing**: drift is already impossible via
two composed guards.

| link | guard | state today |
|---|---|---|
| committed `pricing-data.ts` == generated from `catalog.go` | `TestGenerateTSMatchesCommittedFile`, byte comparison with a minimum-size check | passes |
| `catalog.go` == console catalog | `consolecatalog` parity monitor, every 15m in production | `differences=0`, `source=fresh`, `prices=78` |

The duplication that *does* cost something is one #329 would not close. **`catalog.go`
is hand-maintained Go**, and its header still calls itself *"the Go source of truth
for all Stripe pricing"* — which #328's cutover made false. Every amount a running
service serves now comes from the console.

So today a price change is three acts: publish in the console; **hand-edit
`catalog.go`** so the fail-open fallback stays correct, or parity fires; regenerate
the TS. The middle one is the duplication, and it is where drift would enter.

## The safety property this plan turns on

`catalog.go` is #328's **fail-open fallback** — the last thing standing between a
console outage and a wrong price. #631 pinned it as complete (78 rows, 42 lookup
keys) precisely because "a fallback that is present but missing currencies would be
worse than today's behaviour and every other test would still pass".

So the gate is absolute:

> **The first generated output must be byte-identical to the current hand-written
> data.** Not equivalent, not close — identical. Any diff on the first generation is
> either a generator bug or a real console/catalog divergence, and both must be
> understood before the file is committed.

That is also what makes this change its own evidence: if the console can reproduce
`catalog.go` exactly, then the console and the fallback provably agree at that
moment, independently of the parity monitor.

## What is data and what is logic

Verified against `origin/main`:

| region | disposition |
|---|---|
| `developedAmounts` map | **generated** |
| `pppAmounts` map | **generated** |
| `Plan`/`Period`/`Tier`/`Amount`/`PriceDescriptor` types | hand-written, unchanged |
| `developedLookupKey` / `pppLookupKey` | hand-written — key derivation is logic |
| `init()` building `allDescriptors` | hand-written, unchanged |
| `LookupBaseline` / `LookupPPPOption` / `DevelopedCurrencyOptions` / `MustGet` / `AllDescriptors` / `LookupKeyFor` / `MustGetDescriptor` | hand-written, unchanged |
| `display_extras.go` | **hand-maintained, untouched** — display order, fallback rows, and the Pro+App add-on, which has no Stripe Price and cannot come from the catalog |

Only the two maps move. Nothing that decides a lookup key changes: `lookupKeyFor` is
the one path tesserix-home#328 called "the highest-risk code path" and it is out of
scope here.

## Tasks

### T1 — Split the data out, with no behaviour change at all

Move `developedAmounts` and `pppAmounts` into a new `catalog_data.go`, emitted by a
new `cmd/gencatalog` whose input for this task is the **existing literals**. No
network, no console. A pure refactor.

- `go test ./...` passes unchanged, including `TestCatalog_DevelopedDescriptorCount`,
  `TestCatalog_PPPDescriptorCount`, `TestCatalog_TotalDescriptorCount` — those tests
  are already the shape guard and must not be edited to accommodate this.
- `genpricing`'s output must be **byte-identical** before and after, so
  `TestGenerateTSMatchesCommittedFile` passes with the committed `pricing-data.ts`
  untouched. If that file changes, the refactor was not neutral.
- The `consolecatalog` compiled-fallback completeness test must still pass.
- Correct `catalog.go`'s header: it is no longer the source of truth, it is the
  fail-open fallback that mirrors it. Say which, and say what regenerates it.

### T2 — Teach the generator to read the console

Add `-source=console` to `cmd/gencatalog`, reusing `consolecatalog`'s existing client
rather than writing a second HTTP path. Default stays the offline literal source, so
CI and an offline build are unaffected.

- Map the console's served shape onto the two maps. The console serves
  `lookup_key, plan, period, tier, currency, unit_amount_minor, tax_behavior`; the
  developed map is keyed `(plan, period, currency)` and the PPP map by
  `(plan, period, currency)` under `tier: "ppp"`. Derive nothing that the payload
  states.
- **The proof this task exists for:** running `-source=console` against the live
  console must reproduce `catalog_data.go` byte-for-byte as T1 committed it. Record
  that output in the PR. A diff is a finding, not a nuisance.
- Unit-test the mapping against a **fixture** payload, so the test is offline and
  deterministic. Assert the request too, not just the outcome — the estate has been
  bitten once by a writer whose payload nothing asserted.
- A console that is unreachable must fail the **generation** loudly. It must never
  emit a partial or empty catalog: that would silently gut the fail-open fallback,
  and every other test would still pass. Test the empty and partial cases explicitly.

## Out of scope, deliberately

- `lookupKeyFor` / key derivation — highest-risk path, no reason to touch it here.
- The Pro+App add-on and the $2,000 setup fee. They have no Stripe Price and are
  absent from the console catalog; that gap wants its own issue, filed separately.
- Making CI fetch the console. The parity monitor is already the runtime drift
  detector and it works; a credentialled network call in CI would trade a working
  guard for a flakier one.

## Verification

```
cd services/marketplace-api && go build ./... && go test -race -count=1 ./...
```

Plus, for T1 specifically: `git diff --exit-code packages/ui/src/subscription/pricing-data.ts`
must show **no change**.
