# The trials list can see tenants stuck at signup

An operator with **three tenants** in mark8ly opened the console's trials
surface and saw nothing — at a 7-day window, at a year, and with
Stripe-managed rows opted in. The list was answering correctly. None of those
tenants is `trialing`.

## What the code says

`subscription.Service.Bootstrap` (`internal/subscription/service.go:120-126`)
creates every new subscription as:

```go
Plan:   PlanTrial,      // on a trial by plan
Status: StatusSignup,   // and NOT `trialing` by status
```

`trialingInWindowScope` (`internal/billing/trial/expiring.go:78`) requires
`status = 'trialing'`. So a tenant on a trial plan is invisible to this list
under **every** window and every existing option.

**`signup → trialing` has exactly one writer in the service:** a Stripe
`checkout.session.completed` webhook (`internal/billing/dispatch/handlers.go:213-222`,
actor `system:webhook:stripe`). A tenant that signs up and never completes
checkout never leaves `signup`.

## And nothing else is watching them either

`expiry_cron` selects `status = 'trialing'` (`expiry_cron.go:90`). A `signup`
row is therefore **never aged out**: not trialing, not expired, not on the
queue. It sits there indefinitely with nothing observing it.

That is the real finding. The console surfacing it is the smaller half.

## The shape: an explicit option, list-only

`ListOptions`'s own comment states the rule this must respect:

> *The zero value is the contract #285 already ships, so an omitted option can
> never widen a live result set by accident.*

So this is **not** a default change. Add `IncludeSignup`, mirroring
`IncludeStripeManaged` exactly — same file, same shape, same discipline.

**`CountExpiring` must not move.** It keeps the narrower "will expire" meaning
for #282's KPI, which `/admin/kpis` shares with the console *"so the two cannot
report different numbers for the same word."* `IncludeStripeManaged` is already
excluded from it "entirely"; `IncludeSignup` follows.

### The window still means something for a signup row

`EndsAt` falls back to `created_at + TrialDays` when `trial_ends_at` is null
(`endsat.go:22-26`), and `EndsBetweenScope`'s two-branch predicate already
covers the null case. So a `signup` row has a well-defined effective end and
sorts correctly against `trialing` rows. Nothing new is needed for the window.

**But say plainly what that date is:** for a `signup` row it is a *notional*
end — nothing will act on it, because `expiry_cron` will not touch the row.
A comment must not let a reader think this list's presence implies the expiry
machinery is watching.

## Tasks

Each is one atomic commit. Tests first.

### T1 — `ListOptions.IncludeSignup`

`internal/billing/trial/expiring.go`.

- Add the field with a doc comment in `IncludeStripeManaged`'s register: what it
  admits, why it is off by default, and that `CountExpiring` ignores it.
- Widen `trialingInWindowScope` to `status IN (trialing, signup)` **only** when
  set. Keep the existing scope byte-identical when it is not.
- The two options are independent and compose: `IncludeSignup` alone must not
  drag in card-backed rows, and vice versa. Four combinations, all meaningful.
- **Do not touch `expiringScope`'s use by `CountExpiring`.**

### T2 — the handler accepts it

`internal/handlers/platformadmin/billing_trials.go`. Mirror how
`include_stripe_managed` is read and validated. Update the route doc comment,
which enumerates accepted params.

### T3 — the row says which population it is from

An operator cannot act on a `signup` row the way they act on a `trialing` one —
the first needs chasing to complete checkout, the second is about to lose
access. The response already carries `Status` on `ExpiringRow`; confirm it
reaches the wire shape, and add it if not. Without it the two are
indistinguishable in the console, which would trade one invisible scope for
another.

### T4 — tests

Follow the package's conventions. Cover: the default excludes `signup`
(the shipped contract, unchanged); `IncludeSignup` admits it; the two options
compose in all four combinations; a `signup` row sorts by
`created_at + TrialDays` against a `trialing` row's explicit `trial_ends_at`;
and **`CountExpiring` is unmoved by the flag**.

That last one is the assertion that matters most — it is what stops a view
change quietly redefining a KPI.

Integration tests here now actually run — they were dead until today
(mark8ly#776), so a new one is worth writing rather than assuming coverage.

## Out of scope, and worth its own issue

**Nothing ages out a `signup` row.** Whether a tenant that never completes
checkout should eventually be expired, chased, or left alone is a product
decision with billing consequences, and it is not a console filter's job to
decide. This plan makes the population *visible*; deciding what happens to it
comes after someone can see it.

## Global constraints

- **Comment accuracy** — this estate's documented recurring defect. Run the
  command before writing the sentence; count anything you assert a count about.
- Do not weaken an existing assertion.
- `go build ./...`, `go vet ./...`, `gofmt -l`, `go test ./...`, and the
  integration suite with `-p 1`, all clean.
