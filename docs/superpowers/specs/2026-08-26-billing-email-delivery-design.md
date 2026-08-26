# Billing email delivery — design (#381, piece A)

Status: **approved**, implementation plan not yet written.
Scope: piece **A** only — make the mail actually arrive. Piece **B** (a template
catalog endpoint for the new console) is sketched at the end and files as its own
issue.

## The problem

No merchant has ever received a dunning notice, a trial-expiry reminder, a
payment-action reminder, a win-back promo, or a trial-billed confirmation from
`marketplace-api`.

`email.Client` (`internal/email/client.go:33`) has exactly one implementation —
`NoOpClient`, which logs and returns `nil`. It is wired at three places in
`cmd/marketplace-api/main.go` (`:1599`, `:1764`, `:1879`), which between them
cover every template the interface declares. A merchant whose card fails gets no
day-5 notice, no day-7 notice and no payment-action reminder, and proceeds
toward suspension in silence.

This is not an inert "adapter not wired yet" gap. Three things make it worse.

## What the issue got right, and one thing it got wrong

**Right:** the recipient is unusable. `dunning_emails.go:115-120` passes
`r.StoreID` — a UUID — as the `to` address, awaiting a
`StoreSubscription.email` column. Swapping in a real transport without fixing
that would attempt delivery to a UUID.

**Right:** the metrics overstate. `DunningEmailsSentTotal` and its siblings
increment on every eligible row, because the no-op always returns `nil`. Same
family as the dead outbox gauge from #375, but worse: that one read zero, this
one reads a plausible non-zero.

**Wrong:** #381 attributes the metric lie to the increment *placement* —
"increments after the no-op returns". The placement is correct.
Increment-after-`Send`-returns-`nil` is exactly right. What was fake was the
implementation behind it. **There is no counter to move.** The fix that makes
the counter honest is the rule in §3: the adapter must never return `nil` for
mail it did not send.

## What the issue did not find

**1. The `to` parameter carries two different identifier kinds.** Four callers
pass a store UUID; `lifecycle/winback.go:76` passes a **Stripe customer ID**.
One `string` parameter, two incompatible meanings, no compiler help.

**2. Some Stripe customer emails are undeliverable by construction.**
`subscription/service.go:104-109` mints `billing+<store_id>@mark8ly.local`
whenever `Bootstrap` is called without an email, and the real value is merely
`user_email` from whichever admin session first hit the bootstrap endpoint.
`.local` is an unroutable TLD; sending there hard-bounces and costs sender
reputation.

**3. Two of the four crons can double-send.** Trial reminders and payment-action
reminders claim an idempotency slot before sending. Dunning and win-back do not
— they re-derive eligibility on every run (dunning from `audit_logs` date
arithmetic, win-back from an `updated_at` window), so a second run on the same
day re-sends. `mark8ly-marketplace-api-admin` runs 1 replica, so the window is a
rolling deploy overlapping 09:05 / 10:00, or a restart re-firing the schedule.
Harmless behind a no-op; **duplicate billing mail once the sender is real.** This
defect is introduced by the fix, so the fix closes it.

**4. There is no renderer.** `internal/email/templates.go` is a three-line stub
("lands when real adapter arrives"), and neither provider adapter supports
provider-side dynamic templates — `Message` carries a finished
`Subject`/`HTMLBody`. All 11 templates must render in Go.

## Decisions

### 1. `to` means an email address, and callers resolve it

The interface signature is unchanged; its `to` parameter finally means what it
says. Callers pass `row.Email`.

Callers resolve, not the adapter, because every caller already runs a SQL scan
over `store_subscriptions` — they select one more column instead of forcing an
N+1 lookup inside the send loop. This keeps the adapter a pure render-and-send
unit with no DB handle, testable in isolation.

`winback.go` stops passing `StripeCustomerID`. The two-meanings-in-one-parameter
hazard disappears because there is now exactly one meaning.

Templates also need a store name. It is already in the local `stores`
projection — the callers join it. No column required, and this closes the
`trial_reminders.go:160-164` TODO asking for an "email/store_name pair".

### 2. `store_subscriptions.email`, written by the webhook that already fires

New nullable `citext` column. Two writers:

- **`handleCustomerUpdated`** (`billing/dispatch/handlers.go:413`) already parses
  the Stripe payload and `UPDATE`s this table for `has_default_payment_method`.
  It gains one JSON field and one column **in the same statement** — no new
  webhook subscription, no new query, no new failure mode.
- **`cmd/backfill-email`**, modeled directly on `cmd/backfill-has-pm`, which
  exists for precisely this shape: read Stripe per row, write the column,
  idempotent, throttled, `--dry-run`. Covers rows predating the column, since
  `customer.updated` only fires on change.

### 3. The adapter never returns `nil` for mail it did not send

`email.templateClient` (`internal/email/template_client.go`), the only
implementation of `email.Client`. It renders a key through the existing
`emailtemplates.Loader`, builds a `Message`, and hands it to the existing
`Sender` chain — so billing mail inherits the SendGrid→Resend failover the five
working mailers already use, and #348's send-log decorator will pick it up for
free.

It rejects a recipient that is empty, has no `@`, or ends in `.local` /
`.invalid`, returning a sentinel **`email.ErrUndeliverable`**.

**This sentinel is what makes the counters honest**, and it is the whole of the
metric fix. Callers distinguish it from transport failure: never increment
`sent`, increment a new `skipped_total{reason}` instead, and log at Warn with
the `store_id`. An undeliverable row becomes visible rather than merely
uncounted.

Envelope: `From: cfg.EmailFrom`, `FromName: "Mark8ly Billing"`, and
`CustomArgs{product, kind, tenant_id}` — the attribution shape the working
mailers already emit, which the notification-service webhook receiver consumes.

### 4. Templates register on the existing loader; no seed rows

All 11 keys (10 in `client.go` plus `win_back_day30` from `lifecycle`) register
against `emailtemplates.Loader` with embedded `.html`/`.txt` fallbacks — the
`orderdoc`/`giftcard` pattern, added alongside `main.go:278-279`. Keys are the
existing `TemplateID` values, so there is one identifier rather than two.

**No seed migration.** The current console lists templates by querying
`email_templates` rows and has no create flow, so a key is invisible until a row
exists — which is why one might reach for seeding. But piece B replaces
rows-as-discovery with an explicit catalog, so seeding now would build the thing
B removes. A key with no row simply renders from its embedded default.

### 5. Claim-first markers for dunning and win-back

New `billing_email_sends(subscription_id, template_key, period_key)` with a
unique constraint, claimed `INSERT … ON CONFLICT DO NOTHING` **before** the
send, mirroring `payment_action_reminders`.

One generic table rather than two more bespoke ones: four near-identical marker
tables is three too many, and this is a natural home to later subsume
`trial_reminders` and `payment_action_reminders`. That consolidation is **not**
in this piece.

`period_key` disambiguates repeats of the same template for the same
subscription: for dunning it is the target date in `YYYY-MM-DD` form (the day
the merchant entered `past_due` plus the offset), and for win-back it is the
window-start date. `template_key` alone would suppress a legitimate day-7 notice
after a day-5 one; `period_key` alone would collide across templates.

The trial-billed confirmation (`billing/dispatch/handlers.go:285`) gets **no**
marker. It fires from the `invoice.paid` webhook, which is already deduplicated
upstream: `handlers/webhooks/stripe.go:98-111` does an `InsertIfNew` keyed on
`event_id` and returns `duplicate` before dispatch ever runs. A second marker
would guard nothing.

(That dedup inserts *before* dispatching, so a dispatch failure makes the retry
look like a duplicate and the confirmation is dropped rather than doubled — the
opposite failure mode, in the same family as #336. Out of scope here.)

### 6. At-most-once is kept, and it self-heals

`processOne` claims the slot *before* sending and deliberately does not release
it on failure — so a transient transport error permanently costs that merchant
that one reminder. Verified in code, not taken from the comment.

This is kept. A dropped reminder is better than a duplicate, and the failure is
now visible through the Warn log and the `skipped`/failure counters rather than
silent.

**No cleanup migration is needed for the no-op era.** Marker rows are only
inserted when an offset's day arrives, so future offsets are unclaimed. A
merchant mid-trial loses at most the offset that passed while the no-op was live
and still receives the rest of the cadence. The gap closes itself.

## Components

| unit | file | depends on |
|---|---|---|
| `templateClient` | `internal/email/template_client.go` | `emailtemplates.Loader`, `email.Sender` |
| embedded templates | `internal/email/templates/*.{html,txt}` | — |
| address writer | `billing/dispatch/handlers.go` | Stripe payload |
| backfill | `cmd/backfill-email/main.go` | Stripe API, DB |
| claim markers | `internal/subscription/dunning`, `.../lifecycle` | DB |

## Testing

**Unit** — adapter envelope construction; every `ErrUndeliverable` case (empty,
no `@`, `.local`, `.invalid`); the webhook handler's new column write; golden
output for all 11 embedded templates.

**Integration** (`-tags=integration`, `-p 1`) — backfill idempotency across
re-runs; a dunning-cron test asserting `sent` does **not** increment for a
`.local` row; a double-run test asserting the claim marker suppresses the second
send for dunning and win-back.

Per the handoff's recurring theme — documentation promising more than the code
enforces — every property asserted in a comment here gets a test that makes it
true. Specifically: the `.local` rejection, the never-`nil`-without-delivery
rule, and the claim-first suppression.

## Out of scope

- Bounce and complaint handling (belongs with #348 piece B, which receives
  provider delivery events).
- Consolidating `trial_reminders` / `payment_action_reminders` into
  `billing_email_sends`.
- Retrying reminders whose slot was burned by a transient failure.
- The undeliverable `billing+<uuid>@mark8ly.local` addresses themselves — this
  design refuses to send to them and makes them countable; sourcing a better
  address for those stores is separate.

## Piece B — template catalog endpoint (files separately)

`internal/emailtemplates` exposes `POST /internal/templates/refresh` and
`POST /internal/templates/:key/test`, but **no way to list the catalog**. So the
console reads `email_templates` directly over the cross-DB grant on
`mark8ly_platform_admin` and can only display keys that already have a row —
even though the service knows the full set, because every key is
`loader.Register`ed at boot.

B adds `GET /internal/templates`: registered key, declared variables, whether a
DB override exists, and its status/version. The new console then lists
everything the service can actually send, needs no cross-DB read to enumerate,
and seed rows stop being necessary. This fixes the same invisibility for
`orderdoc`'s 5 keys and `giftcard`'s 1, not only the 11 added here.

Deliberately not designed further: the response shape should answer to the new
console's needs, and that console is not specified yet.
