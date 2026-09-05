# Admin conformance declaration

This file explains the reasoning behind `admin-conformance.json` — the
declaration `mark8ly`'s nightly conformance run (design-system's
`admin-conformance` suite) checks against production. The JSON file itself
is parsed with `JSON.parse` and cannot carry comments, so every "why" that
isn't self-evident from the key/value pair goes here instead. Issue: #415.

## The declarable vocabulary is closed to seventeen ids

`admin-conformance.json`'s `endpoints` map may only contain keys drawn from
the Product Admin Integration Contract's fixed id list. That list is defined
once, in the design-system monorepo, and is exactly seventeen entries:

`design-system/packages/admin-conformance/src/contract.ts:35-311` —
`ENDPOINTS` (and the derived `ENDPOINT_IDS`) define exactly:

Contract v2's nine:

1. `kpis`
2. `inbox`
3. `audit-logs`
4. `entities`
5. `health`
6. `billing/subscriptions`
7. `billing/trials`
8. `tenant-lifecycle`
9. `lifecycle/reason-codes`

Contract v3's eight, added in `@tesserix/admin-conformance` 0.8.0
(design-system#40) — six of them are routes this document previously listed
as structurally undeclarable:

10. `outbox`
11. `email-sends`
12. `notifications`
13. `break-glass`
14. `conversions`
15. `onboarding/funnel`
16. `onboarding/sessions`
17. `tenant-purge`

`conversions` and `tenant-purge` are declared with `probe: false` upstream and
are deliberately never called: a run that purged a real tenant is
unrecoverable, and one that looked a person up by email is a scheduled PII
read. They are declared so the suite knows they exist, not so it exercises
them.

Declaring these requires the suite at **0.8.1 or newer**, not merely 0.8.0.
0.8.0 introduced the ids but inferred `onboarding/funnel`'s envelope as
`data-flat-map`; 0.8.1 (design-system#43) corrected it to `free`. Against
0.8.0 this product's correct, unchanged handler fails two §4.1 checks. The
chart's `adminConformanceCron.suite.version` floor carries that constraint.

There is no eighteenth id, and there is no way to add a private one: the parser
that reads this file, `design-system/packages/admin-conformance/src/declaration.ts`
(`parseEndpoints`), **throws** on any key that is not one of the seventeen —
`unknown endpoint ${key}; valid ids are: ...` — which fails the *entire*
conformance run, not just the offending entry. A product cannot partially
declare; an unrecognised key takes every other declared endpoint down with
it. This is why the file must stay strictly inside the seventeen ids and
nothing else, and why the guard in
`services/marketplace-api/internal/handlers/platformadmin/conformance_declaration_test.go`
independently mirrors this same closed set and asserts every key in this
file is a member of it — a typo here should fail in mark8ly's own CI, not
overnight against production.

## The mounted-but-undeclarable routes

Contract v3 (0.8.0) closed most of this gap. Six of the seven routes this
section used to list — `outbox`, `email-sends`, `notifications`,
`break-glass`, `conversions` and the `onboarding/*` pair — now have ids and
are declared in `admin-conformance.json`. `tenant-purge`, a write, gained one
too.

What remains genuinely undeclarable is smaller, and it is worth keeping the
reasoning rather than deleting the section, because the *shape* of the
argument is what recurs:

| route | handler | why |
|---|---|---|
| `GET /admin/tickets` | `tickets.go:40` | mark8ly-specific support read; no id in the seventeen |
| `POST /admin/inbox/:kind/:id/actions/:actionId` | `inbox_actions.go` | the inbox **write**; `inbox` declares the read only |
| `GET/PUT /admin/email-templates[/:key]`, `POST .../test-send` | `email_templates.go` | the transactional email template registry (tesserix-home#588); no id in the seventeen |

Because the vocabulary is closed and an unknown key throws (see above), these
structurally *cannot* appear in `admin-conformance.json` — not "were not
gotten around to," but "there is nowhere to put them."

The history is the useful part. This document previously argued that the
seven were "never meant to be part of the cross-product contract" and that
extending it "would need to happen upstream in
`design-system/packages/admin-conformance/src/contract.ts` first, not here."
The second half was right and is exactly what happened: design-system#40 added
the ids upstream, mark8ly#455 declared them, and tesserix-k8s#699 updated the
chart copy the nightly CronJob actually reads. The first half was wrong — they
*were* contract material, and describing a closed vocabulary as a design
decision rather than a current limit is how "structurally impossible" outlives
the structure.

`conformance_declaration_test.go` mirrors the seventeen ids and asserts every
key here is a member. That mirror is cross-repo and cross-language, so nothing
but the comment in that file links it to `contract.ts` — an eighteenth id
upstream requires updating the slice in the same change, or the guard silently
stops covering it.

Its companion, `TestConformanceDeclarationMatchesChartCopy`, compares this file
against the chart copy the CronJob actually reads. **It resolves that chart by
filesystem path**, relative to its own source file, expecting `mark8ly` and
`tesserix-k8s` as siblings — so it skips whenever that layout does not hold.
Two consequences worth knowing before trusting a green run:

- It skips in mark8ly's own CI, which never checks out `tesserix-k8s`. The
  test says so in its skip message.
- It also skips in a **git worktree**, because the relative walk lands inside
  `.claude/worktrees/` rather than the workspace root. A run from the main
  checkout and a run from a worktree therefore disagree, and only the former
  is checking anything.
- Conversely, when it does run it reads the sibling's **working tree**, not
  its default branch. A checkout sitting on an unrelated feature branch will
  report drift that does not exist on `main` — verified the hard way on
  2026-08-29, where a stale sibling made a correctly-updated chart look
  eight endpoints behind.

## Why `inbox` declares `slaKinds`, not `slaDeclared`

`inbox` is declared as:

```json
"inbox": { "slaKinds": ["sea_manual_review"] }
```

`sea_manual_review` is the only one of mark8ly's five inbox kinds that carries
a real SLA. It reads `sea_manual_review_queue`, whose type comment
(`services/marketplace-api/internal/inbox/sea_review.go:10-15`) states that
any row entering the queue immediately pauses the 14-day validation clock on
the associated subscription, under a 5-business-day SLA (`SLADueAt`, read
into `due` at `sea_review.go:72` and assigned to `DueAt: &due` at
`sea_review.go:79`). The other four kinds — `arbitrage_appeal`,
`migration_fast_path`, `onboarding_stalled`, and `erasure_request`
(`services/marketplace-api/internal/inbox/provider.go:6-11`) — never set
`DueAt`.

`slaKinds` and `slaDeclared` are two different promises and the parser
enforces that a product picks exactly one: declaring both throws
`endpoints["inbox"] declares both slaDeclared and slaKinds`
(`design-system/packages/admin-conformance/src/declaration.ts:215`).
`slaDeclared` is a per-queue promise — the conformance suite's `checkDueAt`
requires `due_at` on *every* item `GET /admin/inbox` returns when it is true
(`design-system/packages/admin-conformance/src/checks.ts:280-296`).
`slaKinds` is a per-kind promise: `due_at` is required only of items whose
`kind` is in the declared list, and the check is skipped rather than passed
on any page where no item of a declared kind appears, so it never claims
coverage a run did not actually exercise
(`design-system/packages/admin-conformance/src/checks.ts:242-277`).

Declaring `slaDeclared: true` would force every item on the queue to carry
`due_at`, including `erasure_request`, whose provider deliberately sets none:
`services/marketplace-api/internal/inbox/erasure.go:12-15` states the
omission is a refusal, not a gap — *"GDPR's 30-day window is real, but the
table has no due column and deriving a statutory deadline in a read endpoint
would be inventing policy in the wrong place."* `slaDeclared: false` would
have understated a real, subscription-clock-pausing SLA on the one kind that
has one. Neither boolean was honest for a queue that merges five kinds with
only one of them time-bound — a per-kind declaration is what the queue
actually needed.

That option did not exist when this endpoint first shipped, so this file
carried `slaDeclared: false` as a documented, unhappy compromise and filed
the gap upstream as
[design-system#36](https://github.com/tesserix/design-system/issues/36),
arguing for exactly the `erasure_request` reasoning above: a product whose
queue mixes SLA-bearing and non-SLA-bearing kinds needs a way to say so per
kind, not one boolean for the whole queue. That argument won upstream —
design-system#36 shipped in `@tesserix/admin-conformance` **0.7.0** — and
this file now declares `slaKinds` instead.

Declaring `slaKinds` requires suite **`>=0.7.0`**, and the version matters
more than it looks. The feature commit (design-system `41be1f7`) left the
package at 0.6.0 in source; the release that carries it is the following
`chore: version packages` (`2be4dfc`), published as 0.7.0. Verified against
the published tarballs rather than the repo: 0.6.0 contains **no** occurrence
of `slaKinds`, 0.7.0 contains it throughout. So a suite pinned to 0.6.0 would
not merely ignore this key — `assertKnownOptions`
(`design-system/packages/admin-conformance/src/declaration.ts`) throws on an
unrecognised option, and a throw at declaration-parse time fails the ENTIRE
run, every endpoint with it.

The nightly CronJob resolves a range from
`tesserix-k8s/charts/apps/mark8ly-marketplace-api-admin/values.yaml`, key
`adminConformanceCron.version`, which picks the newest published release and
so satisfies this today.

That floor is load-bearing twice over now, and for the same reason each time:
a version below it does not degrade gracefully, it throws at parse time and
takes the whole run down. `slaKinds` needs ≥0.7.0 (above); contract v3's ids
need ≥0.8.1 (see the vocabulary section — 0.8.0 has the ids but mis-declares
`onboarding/funnel`'s envelope). The chart's range was still `>=0.5.0 <1.0.0`
when contract v3 landed, which resolves correctly today only because npm picks
the newest match; raising the floor to `>=0.8.1` is tracked separately in
tesserix-k8s. Prefer citing the constraint over the literal range here — the
range moves, the reason it exists does not.

## Implemented ≠ declared ≠ wired

The issue (#415) names three distinct states, and this file — and the guard
test alongside it — speaks only to the middle one:

- **Implemented**: the route exists in code and answers requests. (Read the
  handler.)
- **Declared**: the route's contract id appears in `admin-conformance.json`
  and the nightly conformance suite checks it against production. (This
  file, and `conformance_declaration_test.go`.)
- **Wired**: the platform console actually knows to call this product's
  base URL for this endpoint at all — i.e. mark8ly is present in
  platform-api's federation registry. Declaring `inbox` here does not, by
  itself, make the console call it; that is a separate, upstream piece of
  work tracked in `tesserix-home#407`, which covers the federation-registry
  half for the estate. Until that lands, the queue can be correctly
  declared here and still be invisible to an operator looking at the
  console, because nothing told the console mark8ly's endpoint exists.

This file, and the test that mirrors it, guarantee only that "implemented"
and "declared" cannot silently drift apart again — that was #415's actual
bug: `inbox` shipped, was never added to this file, and the nightly suite
had nothing to say about it because it never knew to look.
