# Tell the merchant when their migration fast-path request is decided

The last item on #703. Everything else the issue asked for shipped in #711;
this is the one site left:

```go
// internal/billing/migration/handler.go:212-214
// TODO: migration fast-path approval/rejection emails land when the email
// package and StoreSubscription.email column exist (deferred from P5 scope).
h.logger.Info("migration fast-path decided; email deferred", ...)
```

A merchant submits evidence that they have been trading elsewhere, a CSM
approves or rejects it, and the merchant is never told either way. The
decision is written and audited; only the notification is missing.

## The stated blocker is gone twice over

The comment defers to "the email package and StoreSubscription.email column".
Both exist. `internal/email/` has the client, twelve registered templates,
`ValidateRecipient` and `SkipReason`; `store_subscriptions.email` landed in
migration 000104 and `expiry_cron.go` reads it. Nothing is blocked.

## Shape: copy expiry_cron.go, do not invent

`ExpiryCron.notifyExpired` is the reference and this follows it exactly:
chainable `WithEmail(...)` so existing callers compile untouched, a nil mailer
means send nothing, `ValidateRecipient` before the provider sees the address,
and skip/sent counters on `BillingEmailsSkippedTotal` / `BillingEmailsSentTotal`.

Best-effort is the whole point: the review row is already committed and the
audit event already emitted when we get here, so a send failure is logged and
counted, never returned. The CSM's write must not fail because email is down.

## Two decisions worth recording

**The CSM's notes are NOT sent to the merchant.** `reviewRequest.Notes` is
required, 3–2000 chars, and written by a CSM for internal review. Putting it
in merchant-facing mail would publish internal commentary about that merchant
on the strength of a field nobody wrote with an external reader in mind. The
rejection email says the decision and how to follow up; if a merchant-facing
reason is wanted later it needs its own field, deliberately authored.

**Recipient lookup enters as a function, not a DB handle.** `Handler` holds a
narrow `reviewStore`, not gorm, and that is worth keeping. A
`RecipientLookup func(ctx, storeID) (email, storeName string)` is supplied by
main.go as a closure over `conn`. The handler stays testable without a
database, matching how `reviewStore` already works.

## Tasks

1. `internal/subscription/billing_email.go` — `BillingEmailFor`, the
   symmetric partner to the existing `StoreNameFor`. Returns "" when there is
   no row, so `ValidateRecipient` produces the `no_address` skip reason
   rather than the caller inventing one.
2. `internal/email/` — two templates, `migration_fast_path_approved` and
   `migration_fast_path_rejected`: ids in `client.go`, entries in
   `billingTemplateKeys`, subject + HTML + text in `templates_content.go`.
   Registering them makes them console-overridable (#717's mechanism).
3. `internal/billing/migration/handler.go` — `WithEmail(...)`, a
   `notifyDecision` mirroring `notifyExpired`, replacing the TODO and its log.
4. `cmd/marketplace-api/main.go` — wire client, lookup closure and counters.

## Done when

- A merchant with a valid billing email gets the right one of two emails on
  approve and on reject.
- A handler with no mailer, a store with no email row, and a placeholder
  `@mark8ly.local` address each send nothing, return 200, and count a skip.
- A provider failure is logged and counted, and the response is still 200.
- Existing `NewHandler` callers and tests compile untouched.
