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

## Why `slaDeclared` is `false` despite a real SLA

`inbox` is declared as:

```json
"inbox": { "slaDeclared": false }
```

This is deliberate, not an oversight, even though one of mark8ly's five
inbox kinds carries a real, meaningful SLA. `sea_manual_review` reads
`sea_manual_review_queue`, whose migration comment
(`services/marketplace-api/internal/inbox/sea_review.go:12-15`) states that
any row entering the queue immediately pauses the 14-day validation clock on
the associated subscription, under a 5-business-day SLA
(`sla_due_at`, read into `due` at `sea_review.go:72` and assigned to
`DueAt: &due` at `sea_review.go:79`).

But `slaDeclared: true` is a per-queue, not a per-kind, promise:
`design-system/packages/admin-conformance/src/checks.ts:206-220` requires
that when `slaDeclared` is true, **every** item returned by `GET
/admin/inbox` — not just the ones from one kind — carries `due_at`. mark8ly's
queue aggregates five kinds
(`sea_manual_review`, `arbitrage`, `migration_fastpath`, `onboarding`,
`erasure`), and `sea_manual_review` is the *only* one of the five that ever
sets `DueAt`. The other four never do.

Flipping `slaDeclared` to `true` today would therefore fail conformance the
first time the queue holds anything other than a SEA review item — which,
given five active kinds, is the common case, not the edge case. Doing it
correctly requires `due_at` on all five kinds, and the erasure kind is not a
"just add it" gap: `services/marketplace-api/internal/inbox/erasure.go:13`
states the omission is a deliberate refusal — *"GDPR's 30-day window is
real, but the table has no due column and deriving a statutory deadline in a
read endpoint would be inventing policy in the wrong place."* Adding
`due_at` to erasure means either a schema change to carry a real deadline
(a migration, out of scope for this change) or inventing a policy value this
endpoint has no authority to invent. Neither belongs in this fix.

This tension is filed upstream as
[design-system#36](https://github.com/tesserix/design-system/issues/36):
`slaDeclared` is one boolean per product, but SLA reality is per queue kind,
so for a product whose queues differ neither value is honest. `true` fails,
and `false` — what this file declares — understates a real
subscription-clock-pausing deadline on the one queue that has one. Kora
declares `false` with no SLA anywhere, so today the flag renders the two
products identical when they are not. Until that is resolved, `false` is
the accurate-enough answer and this paragraph is the record of why.

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
