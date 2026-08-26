# Email send log — design (#348 piece A)

Date: 2026-08-26
Issue: **#348** (email delivery log). Milestone: Platform integration v1.

#348 as filed is four subsystems. This document designs **piece A only** — the send log and the
transport decorator. The decomposition is recorded in §7.

---

## 1. What is true today, measured not assumed

Verified by reading `services/marketplace-api` on 2026-08-26.

**Transactional email is fire-and-forget and unrecorded.** Every product mailer renders an envelope
and hands it to `email.Sender`; nothing writes a row. There is no `sent_email`, `email_log` or
equivalent table in any of the 103 migrations.

**`email.Sender` is a one-method interface** — `Send(ctx context.Context, msg Message) error`
(`internal/email/sender.go:60`) — with **13 call sites across 12 mailer files**. Production wiring is
SendGrid primary → Resend fallback via `FallbackSender`, constructed at a **single** point,
`email.NewFromConfig(...)` in `cmd/marketplace-api/main.go:550`.

**`internal/email` has no database dependency.** It is provider-only.

**Neither adapter captures a provider message id.** SendGrid returns `X-Message-Id` in its response
headers and Resend returns `{"id": …}` in its body; both are discarded — `sendgrid.go:117` and
`resend.go:112` read only the status code. So no correlation key exists today.

**All five mailers that use `email.Sender` already set `kind`.** Exactly five files construct an
`email.Message`: `ticket/mailer.go`, `giftcard/mailer.go`, `orderdoc/mailer.go`,
`shipping/labelmailer.go` and `campaign/email_dispatcher.go`. Every one sets `CustomArgs` with a
`kind`, and most also set `tenant_id` / `store_id`. **Attribution is already complete for everything
that goes through this transport**, so this piece threads nothing through call sites.

> An earlier draft of this document claimed "7 of 12 mailers set no `CustomArgs`, including all three
> dunning mailers and winback". **That was wrong**, and the error is recorded rather than deleted
> because of where it came from: grepping `.Send(ctx`, a method name, which also matches
> `signup/anomaly_cron.go` (Slack, not email), `campaign/send_worker.go` (the campaign dispatcher,
> which delegates to `email_dispatcher.go`) and `billing/dispatch/handlers.go` (a different
> interface entirely). Grepping the type actually constructed — `email.Message{` — found the real
> set of five. Grep a type, not a method name, when the question is "who uses this interface".

**There is a second, entirely separate email path, and it sends nothing.** `email.Client`
(`internal/email/client.go:33`) is a template facade used by dunning, the trial-reminder cadence,
payment-action reminders, winback and the trial-billed confirmation. Its only implementation is
`NoOpClient`, wired at `main.go:1599`, `:1764` and `:1879`. Those emails have never been sent. Filed
as **#381**. Fixing it is not this piece's job, but it bounds what this piece may claim: a send log
over `email.Sender` will correctly show nothing for those templates, because nothing is sent. The log
makes that gap visible; it does not close it.

The existing vocabulary is consistent where it exists: lowercase snake_case single tokens
(`giftcard`, `shipping_label`, `campaign`, typed constants in `orderdoc`), alongside `product` and
optional `tenant_id` / `store_id` / `campaign_id`.

---

## 2. Correlation: our id, not theirs

The decorator mints a uuid per send, writes the log row with it as the primary key, and injects it
into `Message.CustomArgs`. SendGrid echoes `custom_args` on every engagement event and Resend mirrors
them as tags, so the key returns to us verbatim from either provider.

**The row's identity and the correlation key are the same value.** There is no join table and no
second identifier to keep in step.

Chosen over capturing the provider's own message id, which would require changing `Sender.Send` to
return a receipt — touching both adapters, `FallbackSender`, `LogSender` and all 13 call sites just
to compile. It is also provider-agnostic: a third provider works with no change.

**The cost, stated plainly:** we hold no provider-side identifier, so "find this in the SendGrid
dashboard" remains unanswerable from our data. Piece B narrows this, because provider events identify
themselves.

---

## 3. The table

Migration `000104`. New table `email_sends`.

| column | type | notes |
|---|---|---|
| `id` | uuid PK | **the send_id**, injected into `CustomArgs` |
| `tenant_id` | uuid NULL | nullable: platform-level mail (`signup/anomaly_cron`) has no tenant |
| `store_id` | uuid NULL | nullable for the same reason |
| `recipient` | text NOT NULL | required — "did the merchant get it" is unanswerable without it |
| `kind` | varchar(64) NOT NULL | structured, lowercase snake_case; `unknown` when unattributed |
| `status` | varchar(16) NOT NULL | `sending` \| `sent` \| `failed` |
| `error` | text NULL | populated on failure |
| `created_at` | timestamptz NOT NULL | the attempt |
| `sent_at` | timestamptz NULL | completion |

### What is deliberately absent

**No `subject`.** #348 asks for it; this design declines, and the reasoning is worth recording
because it contradicts the issue. Subject lines are interpolated customer content — *"Your order
#1234 from Acme Ltd"*. Three prior endpoints on this surface deliberately excluded exactly that:
`message` from #332, `description` from #329, `payload` from #331. A send log served cross-tenant to
the platform console would be the first to carry it. `kind` answers "which email was this" at least
as well, and is strictly more queryable than free text; two sends of the same kind are separated by
timestamp.

**No rendered body**, for the same reason and more so.

**No `provider`.** This one is a direct consequence of §2 and was discovered while designing rather
than assumed. `FallbackSender` tries SendGrid then Resend, but the decorator wraps the **whole
chain** and observes one `Send` returning one `error` — it cannot know which provider accepted the
mail without the interface change §2 rejects. Recording the configured primary would be actively
misleading in precisely the situation where the answer matters: a fallback during an outage. Better
an absent column than one that lies. Piece B supplies it, since provider events identify themselves.

### Indexes

`(tenant_id, created_at)` for the platform read, and a partial index on `status` where
`status = 'sending'` so stuck rows are cheap to find on a shared db-f1-micro.

---

## 4. The decorator

**A new package, `internal/emaillog`**, exposing a type that implements `email.Sender` and wraps
another `email.Sender`.

It does **not** live in `internal/email`: that package is provider-only with no database dependency
(§1), and putting a write in it would invert that. Wired once, at `main.go:550`, by wrapping the
result of `email.NewFromConfig(...)`.

### Per send

1. Insert a row with `status = 'sending'`.
2. Call the wrapped `Sender`.
3. Update to `sent` (with `sent_at`) or `failed` (with `error`).

**Write-before-send is deliberate.** A process death mid-send leaves a row at `sending` — which is
*distinguishable* from "never attempted" and therefore actionable. The alternative, writing once
after the send, loses the record entirely on a crash, which is the exact silent gap this issue exists
to close, merely narrowed to a smaller window. This mirrors the outbox `pending` state shipped in
#336: a stuck row that can be seen is worth more than a clean absence.

**A failing log write never blocks the send.** If either write errors, log loudly and proceed with
delivery. An observability feature that can take down transactional mail is worse than no
observability feature. The consequence — a failed pre-write means an unlogged send — is accepted.

**`CustomArgs` is copy-on-write.** The decorator builds a new map and never mutates the caller's,
which may be shared or reused.

---

## 5. Attribution

The decorator reads `kind`, `tenant_id` and `store_id` **from `CustomArgs`**. All five mailers that
use `email.Sender` already set them (§1), so this piece changes no mailer and touches no call site.
Coverage is complete because the decorator wraps the transport; attribution is complete because the
mailers already supply it.

**`kind` still falls back to `unknown`** when absent, mirroring `sanitizeReason`'s `ReasonUnknown`
from #336. Nothing needs it today. It exists so a mailer added later without attribution appears in
the log as unattributed and queryable, rather than writing an empty string nobody notices.

## 6. Testing

- **Every send writes a row**, including one whose delivery fails.
- **A failing log write does not block delivery.** The most important test here: the email must still
  go out when the database is unavailable. Getting it backwards means an observability feature that
  takes down transactional mail.
- **`sending` → `sent`** and **`sending` → `failed`**, with the error recorded on the latter.
- **No subject and no body reach the row**, asserted against stored values rather than against the
  struct definition.
- **`CustomArgs` is not mutated**, and the `send_id` reaches the wrapped sender.
- **`kind` falls back to `unknown`** when a mailer supplies none.

---

## 7. Decomposition of #348, and what this piece is not

| | piece | state |
|---|---|---|
| **A** | send log + transport decorator | **this document** |
| **B** | provider event webhook | needs A for something to correlate to; a NEW public trust boundary — model it on `internal/webhookevents` and `billing/stripe/signature.go:29`, not from scratch |
| **C** | `campaign_recipients` terminal state | independent bug — `send_worker.go:241` writes `RecipientSent` on success while the failure path at `:233` only logs, leaving the row at `pending` and conflating "not attempted" with "permanently failed". Shippable on its own, today. |
| **D** | the platform read | trivial once A and B exist; same shape as #331 |

Order **A → B → D**, with **C** independent.

Also out of scope for A:

- **A sweeper for rows stuck at `sending`.** They are visible and queryable, which is the point.
  Reconciling them is a separate decision, exactly as #336 left requeue to an operator.
- **Retention and pruning.** `email_sends` will grow faster than anything else added to this
  database. It deserves its own decision once real volume exists — recorded here so it is not a
  surprise later. Note the audit work (#365) established a seven-year retention precedent for
  operator rows; email is a different class and should not inherit that by default.
