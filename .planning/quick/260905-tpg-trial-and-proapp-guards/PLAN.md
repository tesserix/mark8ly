# Trial email honesty + Pro+App teardown guard

Three small, independent fixes found while triaging the billing backlog
(#702, #703). Each one closes a place where the code claims something it
does not do. None of them requires Stripe, the console, or a working
subscription — that is why they were selected.

## Background that shaped the scope

#703 was filed claiming merchants are charged after a 90-day trial with no
warning. That is **false**: `internal/subscription/dunning/trial_reminders.go`
sends T-15/T-10/T-7/T-3/T-1 reminders, payment-method-aware, and is scheduled
at `cmd/marketplace-api/main.go:2138` (09:30 UTC daily, deduped by the
`trial_reminders` table). The issue has been corrected.

What remains is narrower, and one part of the original scope is now a
deletion rather than an addition.

---

## Task 1 — remove the banner cron's phantom email ladder

`internal/billing/trial/banner_cron.go` carries `bannerTarget.template` with
values `trial_day_60` / `trial_day_75` / `trial_day_85`. **No such TemplateID
has ever existed.** On a 90-day trial those land at T-30, T-15 and T-5, so
wiring them would duplicate `no_pm_t_minus_15` and fire a third mail between
the existing T-7 and T-3.

The banner cron's real job — advancing `trial_banner_state` for the in-app
banner — works and is untouched here.

- Delete the `template` field from `bannerTarget` and its three values.
- Replace the "email deferred / when StoreSubscription gets email+store_name
  columns" comments with a short note that trial email is owned by
  `dunning/trial_reminders.go`, pointing at it.
- Keep the `logger.Info` on banner advance, minus the `template` key.

**Done when:** no reference to a non-existent template remains, the package
builds, and existing banner-cron tests still pass unchanged. No behaviour
change — this is the commit that stops the next reader rebuilding a duplicate
ladder.

## Task 2 — send a trial-expired notice

`internal/billing/trial/expiry_cron.go` transitions a cardless trial to
`expired` and logs "email deferred". Nothing tells the merchant it happened.
T-1 warned them it would; nothing confirms it did or says what state their
store is in now.

- Add `TemplateTrialExpired TemplateID = "trial_expired"` to
  `internal/email/client.go`, append it to `billingTemplateKeys`
  (`internal/email/templates.go`), and add subject/HTML/text entries to
  `templates_content.go`. Use only variables the lint test supplies —
  `store_name` at minimum.
- Send it from `expiry_cron.go` on a successful transition, following
  `dunning/trial_reminders.go`'s pattern: recipient from `row.Email`,
  store name via `subscription.StoreNameFor`, `email.ValidateRecipient`
  semantics, skip counter on `ErrUndeliverable`, sent counter on success.
- The cron must keep working when the email client is nil (mirror how other
  crons treat an unconfigured client) — do not make email a hard dependency
  of trial expiry.
- Dedup: a store must not receive two expiry notices. The transition itself
  is CAS-guarded (`ErrCASConflict` path), so send only on `err == nil`.

**Done when:** the template lints, a unit test proves the mail is sent once
on a successful expiry and not at all on a CAS conflict, and expiry still
succeeds with a nil email client.

## Task 3 — refuse to seed a Pro+App teardown with no app identifiers

`internal/whitelabel/lifecycle/pro_app_cancelled_consumer.go` accepts an
event and inserts a `white_label_app_state` row. The Advancer then skips
`pullApps` when `AppleAppID` is empty (`advancer.go:186`) and
`archiveFirebase` when `FirebaseProjectID` is empty (`:201`).

So an event with blank identifiers walks the state machine to
`credentials_purged`, pulls nothing, and **reports the teardown complete
while the app stays live**. Worse than today's honest no-op.

Nothing in the estate currently produces those identifiers (see #702) — so
this task does not wire the consumer. It makes the hole loud.

- Extend the existing `TenantID`/`StoreID` validation in `Handle` to reject
  an event carrying none of `AppleAppID`, `GooglePackage`,
  `FirebaseProjectID`, with a distinct sentinel error.
- Reject when all three are empty. Do not require all three — a store may
  legitimately have Apple but no Firebase.

**Done when:** a unit test proves an all-empty event returns the sentinel and
writes no row, and an event with any one identifier still inserts as before.

---

## Constraints

- TDD: test first, watch it fail, then implement.
- One atomic commit per task, single-line conventional-commit message.
- `go build ./...` and `go test ./...` green in `services/marketplace-api`
  before each commit.
- No behaviour change beyond what each task states. In particular Task 1 is
  pure deletion and Task 3 adds a guard, not a wiring.
