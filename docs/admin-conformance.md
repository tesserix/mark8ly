# Admin conformance declaration

This file explains the reasoning behind `admin-conformance.json` — the
declaration `mark8ly`'s nightly conformance run (design-system's
`admin-conformance` suite) checks against production. The JSON file itself
is parsed with `JSON.parse` and cannot carry comments, so every "why" that
isn't self-evident from the key/value pair goes here instead. Issue: #415.

## The declarable vocabulary is closed to nine ids

`admin-conformance.json`'s `endpoints` map may only contain keys drawn from
the Product Admin Integration Contract's fixed id list. That list is defined
once, in the design-system monorepo, and is exactly nine entries:

`design-system/packages/admin-conformance/src/contract.ts:13-126` —
`ENDPOINTS` (and the derived `ENDPOINT_IDS`) define exactly:

1. `kpis`
2. `inbox`
3. `audit-logs`
4. `entities`
5. `health`
6. `billing/subscriptions`
7. `billing/trials`
8. `tenant-lifecycle`
9. `lifecycle/reason-codes`

There is no tenth id, and there is no way to add a private one: the parser
that reads this file, `design-system/packages/admin-conformance/src/declaration.ts`
(`parseEndpoints`), **throws** on any key that is not one of the nine —
`unknown endpoint ${key}; valid ids are: ...` — which fails the *entire*
conformance run, not just the offending entry. A product cannot partially
declare; an unrecognised key takes every other declared endpoint down with
it. This is why the file must stay strictly inside the nine ids and nothing
else, and why the guard in
`services/marketplace-api/internal/handlers/platformadmin/conformance_declaration_test.go`
independently mirrors this same closed set and asserts every key in this
file is a member of it — a typo here should fail in mark8ly's own CI, not
overnight against production.

## The seven mounted-but-undeclarable routes

mark8ly's platform-admin surface (`services/marketplace-api/internal/handlers/platformadmin/routes.go`)
mounts several `/admin/*` reads that the console consumes directly but that
have **no corresponding id in the nine above**. Because the vocabulary is
closed and an unknown key throws (see above), these routes structurally
*cannot* appear in `admin-conformance.json` — not "were not gotten around
to," but "there is nowhere to put them." This is one structural fact, not
seven separate omissions to justify individually:

| route | handler |
|---|---|
| `GET /admin/outbox` | `outbox.go:59` |
| `GET /admin/email-sends` | `email_sends.go:56` |
| `GET /admin/notifications` | `notifications.go:50` |
| `GET /admin/tickets` | `tickets.go:40` |
| `GET /admin/break-glass` | `break_glass.go:65` |
| `GET /admin/conversions` (the issue's "entities/conversions") | `conversions.go:31` |
| `GET /admin/onboarding/funnel`, `GET /admin/onboarding/sessions` (the issue's "onboarding-funnel") | `onboarding.go:41-42` |

These are mark8ly-specific reads the console UI calls directly against this
product's federated base URL — they were never meant to be part of the
cross-product contract the conformance suite checks, and adding them to this
file would either throw at parse time (killing the whole nightly run) or, if
the contract were ever extended to include them, would need to happen
upstream in `design-system/packages/admin-conformance/src/contract.ts` first,
not here.

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
design-system#36 shipped in `@tesserix/admin-conformance` 0.6.0 — and this
file now declares `slaKinds` instead. Declaring `slaKinds` requires suite
`>=0.6.0`; the nightly CronJob resolves `@tesserix/admin-conformance` against
the range `>=0.5.0 <1.0.0`, which is satisfied today but would break this
declaration if ever pinned below 0.6.0.

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
