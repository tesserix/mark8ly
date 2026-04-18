# P6 — Dunning Cron + `payment_action_required` Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Own the post-failure ladder. Implement the §16 dunning schedule (day 0 → past_due, day 5/7 nudge emails, day 8 past_due → expired, day 14 expired → store_closed hand-off to P12, day 90 pending_hard_delete hand-off), wire the §4.7 `payment_action_required` merchant-facing recovery (hosted-invoice URL + T-14/T-7/T-1 reminders), and honor the §16.5 refund-window-dominates rule so merchants inside the first 14 days of their first charge cannot be auto-dunned into `expired`.

**Architecture:** A new `internal/subscription/dunning` package owns four idempotent cron jobs started from `main.go` via `robfig/cron/v3`: `dunning.StepDailyLadder` (day 0 → day 90 transitions), `dunning.SendDunningEmails` (day 5 + day 7 nudges), `dunning.SendPaymentActionReminders` (T-14/T-7/T-1 for SCA challenges), and `dunning.RefundWindowGuard` (metric + early-exit for jobs that would force an `expired` transition inside the 14-day refund window). Every state mutation goes through `statemachine.Transition` from P3 — dunning NEVER writes status directly. Emails go through a minimal `internal/email` facade that wraps either the existing notification-service HTTP client (if present) or a SendGrid client; the facade is `email.Send(ctx, template, data)` and nothing more. The `invoice.payment_action_required` webhook handler in P2 already drops the state; P6 adds the merchant-facing column `hosted_invoice_url` on `store_subscriptions` (via a micro-migration) and a reminder-sent bookkeeping table `payment_action_reminders`. Dunning crons are state-derived: recomputing from `(status, updated_at, first_charge_at, invoice metadata)` alone produces the same transitions, so any retry or late start yields identical outcomes.

**Tech Stack:** Go 1.26, `robfig/cron/v3` v3.0.1 (already in go.mod via audit-service — re-add if absent from `marketplace-api`), Gin/GORM (existing), PostgreSQL 15, existing `internal/audit`, `subscription.WithAdvisoryLock` (P1), `statemachine.Transition` (P3), `webhookevents.StripeWebhookEvent` (P2). Emails via existing `notification-service` HTTP POST or `github.com/sendgrid/sendgrid-go` — pick whichever is already wired; do not add both.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §4.7 (`payment_action_required` fallback + T-14/T-7/T-1 reminder cadence), §16.1–16.5 (trigger, schedule, recovery, tone, dunning-vs-refund).

**Depends on:**
- **P1** — data model (`StoreSubscription.Status`, `FirstChargeAt`, `WithAdvisoryLock`, `audit.EmitStateTransition`)
- **P2** — webhook dispatcher already routes `invoice.payment_failed` → `past_due` and `invoice.payment_action_required` → `payment_action_required` via `statemachine.Transition`; this plan only observes the lading, never re-races the webhook
- **P3** — `statemachine.Transition` for every mutation; `payment_action_required` already excluded from `ReadOnlyStatuses()` (Council finding #3)

**Related plans (NOT in scope here):**
- **P10** — refund flow itself (the 14-day window policy); P6 only honors the window as a gate
- **P11** — cancellation emails and save-offer flow; P6 sends dunning + SCA reminders only
- **P12** — Cloudflare Worker `closed.html` page; P6 transitions to `store_closed` and emits a signal the Worker reads via subscription-status lookup
- **P16** — admin banner UI for "Complete authentication"; P6 exposes the `hosted_invoice_url` and a `/subscription/complete-action` redirect endpoint; banner components are P16

---

## Scope Check

In scope:

1. `robfig/cron/v3` bootstrap in `main.go` and a single `dunning.Scheduler` that registers four jobs.
2. `dunning.StepDailyLadder` cron — runs once per hour; for every subscription in `past_due`/`expired`/`store_closed`, computes the elapsed days since the status entry timestamp and fires the appropriate `statemachine.Transition`.
3. `dunning.SendDunningEmails` cron — day 5 + day 7 email nudges with Customer Portal link (P2's `portal:*:5min` idempotency-keyed URL) and merchant-friendly copy.
4. `dunning.SendPaymentActionReminders` cron — T-14 / T-7 / T-1 reminders for `payment_action_required` subs, carrying the saved `hosted_invoice_url`.
5. Micro-migration `000047_hosted_invoice_url.up.sql` — adds `hosted_invoice_url TEXT` to `store_subscriptions`.
6. Micro-migration `000048_payment_action_reminders.up.sql` — creates `payment_action_reminders(subscription_id, offset_key, sent_at)` so reminders are idempotent.
7. Webhook-path patch — when P2's dispatcher sees `invoice.payment_action_required`, it now ALSO writes `hosted_invoice_url` into `store_subscriptions`. Small, surgical edit in P2's handler.
8. Webhook-path patch — `invoice.paid` handler (already present in P2) confirms the `payment_action_required → active` transition pre-scaffolded by P3 Task 3 is present; add a regression test.
9. `/admin/stores/:storeId/subscription/complete-action` GET endpoint — 302-redirects to the stored `hosted_invoice_url` (allowlisted in P3's RequireActive config).
10. `email.Send` facade + three templates: `dunning_day_5`, `dunning_day_7`, `payment_action_reminder` (offset-parameterised).
11. Refund-window guard — `dunning.RefundWindowGuard(sub)` returns `Suppressed` if `now - first_charge_at < 14d`, the daily-ladder cron refuses to force `past_due → expired` under that condition, and a counter `subscription.dunning.suppressed_refund_window` is emitted.
12. Metric counters:
    - `subscription.dunning.emails_sent{day}` — one per nudge emitted
    - `subscription.payment_action_required.reminders_sent{offset}` — one per SCA reminder emitted
    - `subscription.dunning.suppressed_refund_window` — one per ladder-step skipped due to the 14-day rule

Out of scope:

- Cloudflare Worker `closed.html` rendering — P12 (day-14 storefront hand-off).
- Admin UI banner markup and CTA styling — P16.
- Refund calculation / Stripe refund API calls — P10.
- Cancellation + save-offer emails — P11.
- Trial expiry emails and day 90 first-charge confirmation — P5.
- Hard-delete executor (day 180 `pending_hard_delete → hard_deleted`) — runs in a separate, security-reviewed plan; P6 only transitions INTO `pending_hard_delete`, never out of it.
- Template HTML/MJML authoring — copy is in-plan; visual design is P16.
- Replacing the notification-service with a new SendGrid wrapper if notification-service is already wired.

---

## File Structure

### Create

- `services/marketplace-api/migrations/000047_hosted_invoice_url.up.sql`
- `services/marketplace-api/migrations/000047_hosted_invoice_url.down.sql`
- `services/marketplace-api/migrations/000048_payment_action_reminders.up.sql`
- `services/marketplace-api/migrations/000048_payment_action_reminders.down.sql`
- `services/marketplace-api/internal/subscription/dunning/scheduler.go` — `Scheduler`, `Start`, `Stop`, job registration via `robfig/cron/v3`
- `services/marketplace-api/internal/subscription/dunning/ladder.go` — `StepDailyLadder` job: computes elapsed days, calls `statemachine.Transition`
- `services/marketplace-api/internal/subscription/dunning/ladder_test.go`
- `services/marketplace-api/internal/subscription/dunning/emails.go` — `SendDunningEmails` + `SendPaymentActionReminders`
- `services/marketplace-api/internal/subscription/dunning/emails_test.go`
- `services/marketplace-api/internal/subscription/dunning/guard.go` — `RefundWindowGuard` pure function + `IsInRefundWindow(sub, now)`
- `services/marketplace-api/internal/subscription/dunning/guard_test.go`
- `services/marketplace-api/internal/subscription/dunning/metrics.go` — Prometheus counters
- `services/marketplace-api/internal/email/client.go` — `email.Client` interface + `email.Send(ctx, template, data)` facade
- `services/marketplace-api/internal/email/notification_service.go` — HTTP adapter to existing `notification-service` (if present)
- `services/marketplace-api/internal/email/sendgrid.go` — thin SendGrid adapter (only if notification-service is not wired)
- `services/marketplace-api/internal/email/templates.go` — template IDs + data types (no HTML; copy lives in template registry or downstream provider)
- `services/marketplace-api/internal/handlers/admin/subscription_complete_action.go` — GET endpoint that 302-redirects to `hosted_invoice_url`
- `services/marketplace-api/internal/handlers/admin/subscription_complete_action_test.go`
- `services/marketplace-api/internal/subscription/dunning/integration_test.go` — time-mocked full ladder (day 0 → day 90)

### Modify

- `services/marketplace-api/cmd/marketplace-api/main.go` — start `dunning.Scheduler` (graceful shutdown wired)
- `services/marketplace-api/internal/billing/dispatch/handlers.go` — in `handleInvoicePaymentActionRequired`, persist `hosted_invoice_url` into `store_subscriptions` alongside the existing status transition (one surgical `UPDATE ... SET hosted_invoice_url` inside the same advisory-locked txn P3 Task 3 opened)
- `services/marketplace-api/internal/billing/dispatch/handlers.go` — in `handleInvoicePaid`, clear `hosted_invoice_url` on success (NULL out)
- `services/marketplace-api/internal/subscription/readonly/allowlist.go` — add `/admin/stores/:storeId/subscription/complete-action` to `DefaultAllowlist` (so an `expired` merchant can still reach their hosted invoice to recover)
- `services/marketplace-api/internal/subscription/models.go` — add `HostedInvoiceURL *string` field to `StoreSubscription`
- `services/marketplace-api/internal/handlers/admin/routes.go` — register GET `/admin/stores/:storeId/subscription/complete-action`
- `services/marketplace-api/go.mod` / `go.sum` — add `github.com/robfig/cron/v3` if not already transitively present

### Delete

- Nothing. P6 is additive.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Migrations 047 + 048 | — |
| 2 | Add `HostedInvoiceURL` to `StoreSubscription` model | 1 |
| 3 | `email.Client` facade + adapter selection (notification-service or SendGrid) | — |
| 4 | `dunning.RefundWindowGuard` (pure) + unit tests | — |
| 5 | `dunning.StepDailyLadder` cron logic + unit tests | 2, 4, P3 |
| 6 | `dunning.SendDunningEmails` cron + unit tests | 3, 5 |
| 7 | `dunning.SendPaymentActionReminders` cron + unit tests | 1, 2, 3 |
| 8 | `dunning.Scheduler` bootstrap + `main.go` wiring | 5, 6, 7 |
| 9 | Webhook patch — persist `hosted_invoice_url` on `payment_action_required`; clear on paid | 1, 2, P2 |
| 10 | `/subscription/complete-action` 302-redirect endpoint + allowlist entry | 2, 9 |
| 11 | Metrics counters exposed via existing `/metrics` endpoint | 5, 6, 7 |
| 12 | Full time-mocked integration test (day 0 → day 90 ladder + refund-window suppression) | 5, 6, 8 |
| 13 | Regression test — SCA recovery: `payment_action_required → active` end-to-end with `invoice.paid` | 9, P2, P3 |
| 14 | Regression test — criterion 38 re-assert: `payment_action_required` merchant keeps full admin AND the dunning ladder skips them | 5, 9, P3 |
| 15 | Copy-tone test — templates contain no threatening language (§16.4) | 3, 6, 7 |

---

## Reusable patterns

**A. Cron handler shape** — every job is `func(ctx context.Context, db *gorm.DB, now time.Time) error`. `now` is injected (not `time.Now()`) so tests can drive the clock. The scheduler wraps the function with a context, a DB handle, and `time.Now().UTC()` at call time.

```go
type Job func(ctx context.Context, db *gorm.DB, now time.Time) error
```

**B. Idempotent ladder step** — `StepDailyLadder` reads every sub in the active-failure set, computes `daysSince(status_entered_at)`, and:

- `past_due` + `daysSince >= 8` AND NOT refund-window → `statemachine.Transition(past_due → expired)`
- `expired` + `daysSince >= 14` (from expiry) → `statemachine.Transition(expired → store_closed)`
- `store_closed` + `daysSince >= 90` (from expiry) → `statemachine.Transition(store_closed → pending_hard_delete)`

CAS conflicts from `statemachine.Transition` are swallowed (P3 pattern) — another cron tick or a webhook already moved the row. Ladder re-entry on the next run still produces the correct answer.

**C. Day-count source** — `daysSince` reads `updated_at` for the current status row PLUS the audit event timestamp for the transition INTO that status (from `subscription.state_transition` events P3 emits). `updated_at` may shift on unrelated writes; the audit event is authoritative. Helper: `dunning.statusEnteredAt(ctx, db, storeID, status)`.

**D. Email idempotency** — `payment_action_reminders` has a PK on `(subscription_id, offset_key)`. `INSERT ... ON CONFLICT DO NOTHING; SELECT FROM payment_action_reminders WHERE ...; if row just inserted, send email.` This makes the reminder cron safe to run hourly — duplicate runs short-circuit. Dunning nudges use a looser mechanism: the daily ladder reads the last `subscription.dunning.email_sent` audit event for the sub and skips if same day.

**E. Refund-window guard** —

```go
func IsInRefundWindow(sub *subscription.StoreSubscription, now time.Time) bool {
    if sub.FirstChargeAt == nil { return false } // trial still — no refund window yet
    return now.Sub(*sub.FirstChargeAt) < 14 * 24 * time.Hour
}
```

Used by `StepDailyLadder` BEFORE any `past_due → expired` transition. If `true`, emit `subscription.dunning.suppressed_refund_window` counter and no-op. `past_due → active` (via `invoice.paid`) is unaffected — the guard only blocks *forward* ladder movement into read-only states.

**F. Email facade** —

```go
package email

type Client interface {
    Send(ctx context.Context, template TemplateID, to string, data map[string]any) error
}

type TemplateID string

const (
    TemplateDunningDay5              TemplateID = "dunning_day_5"
    TemplateDunningDay7              TemplateID = "dunning_day_7"
    TemplatePaymentActionReminder    TemplateID = "payment_action_reminder"
)
```

Two adapter candidates — pick the one already wired into the service. If `notification-service` client exists in `internal/clients/notification.go` (check at plan execution time), wrap it; otherwise, use `github.com/sendgrid/sendgrid-go`. Do NOT add both. Nothing beyond `Send(ctx, template, data)` belongs in this facade.

**G. Scheduler lifecycle** —

```go
c := cron.New(cron.WithLocation(time.UTC))
c.AddFunc("0 * * * *",  wrap(ladder.StepDailyLadder, db, email))          // hourly
c.AddFunc("5 9 * * *",  wrap(emails.SendDunningEmails, db, email))        // 09:05 UTC daily
c.AddFunc("15 9 * * *", wrap(emails.SendPaymentActionReminders, db, email)) // 09:15 UTC daily
c.Start()
// On shutdown: ctx := c.Stop(); <-ctx.Done()
```

No new framework — this is the exact pattern `audit-service` already uses.

---

## Task 1: Migrations 047 + 048

**Files:**

- Create: `services/marketplace-api/migrations/000047_hosted_invoice_url.up.sql`
- Create: `services/marketplace-api/migrations/000047_hosted_invoice_url.down.sql`
- Create: `services/marketplace-api/migrations/000048_payment_action_reminders.up.sql`
- Create: `services/marketplace-api/migrations/000048_payment_action_reminders.down.sql`

- [ ] **Step 1: Write `000047_hosted_invoice_url.up.sql`**

```sql
-- Migration 000047: persist Stripe hosted invoice URL for payment_action_required recovery (§4.7).
ALTER TABLE store_subscriptions
    ADD COLUMN IF NOT EXISTS hosted_invoice_url TEXT;

COMMENT ON COLUMN store_subscriptions.hosted_invoice_url IS
    'Stripe invoice hosted URL captured from invoice.payment_action_required webhook. '
    'NULL when no action-required invoice is outstanding. '
    'Exposed to merchant via /admin/stores/:storeId/subscription/complete-action redirect.';
```

- [ ] **Step 2: Write `000047_hosted_invoice_url.down.sql`**

```sql
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS hosted_invoice_url;
```

- [ ] **Step 3: Write `000048_payment_action_reminders.up.sql`**

```sql
-- Migration 000048: idempotency table for SCA reminder cron (T-14 / T-7 / T-1).
CREATE TABLE IF NOT EXISTS payment_action_reminders (
    subscription_id UUID NOT NULL REFERENCES store_subscriptions(id) ON DELETE CASCADE,
    offset_key      TEXT NOT NULL CHECK (offset_key IN ('t_minus_14','t_minus_7','t_minus_1')),
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    stripe_invoice_id TEXT,
    PRIMARY KEY (subscription_id, offset_key)
);

CREATE INDEX IF NOT EXISTS idx_payment_action_reminders_sent_at
    ON payment_action_reminders(sent_at DESC);

COMMENT ON TABLE payment_action_reminders IS
    'One row per (subscription, reminder offset). PK gives INSERT ON CONFLICT DO NOTHING '
    'idempotency so the cron is safe to run multiple times per day.';
```

- [ ] **Step 4: Write `000048_payment_action_reminders.down.sql`**

```sql
DROP TABLE IF EXISTS payment_action_reminders;
```

- [ ] **Step 5: Run migrations up + down to verify**

```bash
cd services/marketplace-api
make migrate-up
make migrate-down COUNT=2
make migrate-up
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/migrations/000047_*.sql \
        services/marketplace-api/migrations/000048_*.sql
git commit -m "feat(db): hosted_invoice_url column + payment_action_reminders table for P6"
```

---

## Task 2: Add `HostedInvoiceURL` to the Go model

**Files:**

- Modify: `services/marketplace-api/internal/subscription/models.go`

- [ ] **Step 1: Add the field**

In the `StoreSubscription` struct, after the existing billing columns added in P1:

```go
// HostedInvoiceURL is the Stripe-hosted invoice URL captured on
// invoice.payment_action_required. Non-nil only while a challenge is
// outstanding; cleared on invoice.paid. See §4.7.
HostedInvoiceURL *string `gorm:"column:hosted_invoice_url" json:"hosted_invoice_url,omitempty"`
```

- [ ] **Step 2: Write a loader test**

```go
func TestStoreSubscription_HostedInvoiceURL_RoundTrip(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    tenantID, storeID := uuid.New(), uuid.New()
    url := "https://invoice.stripe.com/i/acct_x/test_ABC"
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusPaymentActionRequired,
        HostedInvoiceURL: &url,
    }).Error)

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
    require.NotNil(t, sub.HostedInvoiceURL)
    require.Equal(t, url, *sub.HostedInvoiceURL)
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/models.go \
        services/marketplace-api/internal/subscription/models_test.go
git commit -m "feat(subscription): add HostedInvoiceURL to StoreSubscription"
```

---

## Task 3: `email.Client` facade + adapter selection

**Files:**

- Create: `services/marketplace-api/internal/email/client.go`
- Create: `services/marketplace-api/internal/email/templates.go`
- Create: `services/marketplace-api/internal/email/notification_service.go` **OR** `internal/email/sendgrid.go` — pick ONE
- Create: `services/marketplace-api/internal/email/client_test.go`

**First action — discover which transport is already wired:**

```bash
cd services/marketplace-api
grep -RlnE 'notification[-_ ]?service|sendgrid' internal/ cmd/ | head
```

- If `internal/clients/notification_client.go` or similar exists → adapter is `notification_service.go`, wrap the existing client.
- Otherwise → adapter is `sendgrid.go`, use `github.com/sendgrid/sendgrid-go`.

Document the choice inline in `client.go` as a `// why:` comment.

- [ ] **Step 1: Write the interface in `client.go`**

```go
// Package email provides a minimal Send-only facade used by the dunning and
// payment-action reminder crons (§16, §4.7). Intentionally tiny: one method,
// template-id + data map — rendering lives in the downstream provider.
package email

import "context"

type TemplateID string

const (
    TemplateDunningDay5             TemplateID = "dunning_day_5"
    TemplateDunningDay7             TemplateID = "dunning_day_7"
    TemplatePaymentActionReminder   TemplateID = "payment_action_reminder"
)

type Client interface {
    Send(ctx context.Context, template TemplateID, to string, data map[string]any) error
}

// NoopClient is a test double that records every call.
type NoopClient struct {
    Calls []SentEmail
}
type SentEmail struct {
    Template TemplateID
    To       string
    Data     map[string]any
}
func (c *NoopClient) Send(ctx context.Context, t TemplateID, to string, d map[string]any) error {
    c.Calls = append(c.Calls, SentEmail{t, to, d})
    return nil
}
```

- [ ] **Step 2: Write `templates.go`**

```go
package email

// TemplateData types document what each template expects. The cron code
// constructs these, converts to map[string]any, and hands to Send.
type DunningDayData struct {
    MerchantName       string
    StoreName          string
    Amount             string    // localised, e.g. "₹999"
    CardLast4          string
    CustomerPortalURL  string    // from P2 PortalIdempotencyKey
    SupportEmail       string
    DaysUntilExpiry    int       // 8 - daysSince; used by day_5 / day_7 copy
}

type PaymentActionReminderData struct {
    MerchantName       string
    StoreName          string
    Amount             string
    HostedInvoiceURL   string
    OffsetLabel        string    // "14 days", "7 days", "tomorrow"
    SupportEmail       string
}
```

Copy tone MUST match §16.4 — editorial, calm, never threatening. Copy examples that PASS (to include in the test at Task 15):

- `"We couldn't charge {card_last4}. Visit your Customer Portal when it suits you — your storefront stays live until {date}."`
- `"Your bank needs to confirm this payment. Complete the quick authentication step to keep your store uninterrupted."`

Copy that must FAIL tests (banned phrases from §16.4):

- "URGENT", "IMMEDIATE ACTION REQUIRED", "Your account will be CLOSED", "PAY NOW", "Final warning".

- [ ] **Step 3: Write adapter (one of the two)**

If notification-service is present:

```go
// internal/email/notification_service.go
package email

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

type NotificationServiceClient struct {
    BaseURL string
    HTTP    *http.Client
}

func (c *NotificationServiceClient) Send(ctx context.Context, t TemplateID, to string, d map[string]any) error {
    body := map[string]any{"template": t, "to": to, "data": d}
    buf, _ := json.Marshal(body)
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/send", bytes.NewReader(buf))
    req.Header.Set("Content-Type", "application/json")
    resp, err := c.HTTP.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        return fmt.Errorf("notification-service: %s", resp.Status)
    }
    return nil
}
```

Otherwise (SendGrid):

```go
// internal/email/sendgrid.go
package email

import (
    "context"
    "fmt"

    sg "github.com/sendgrid/sendgrid-go"
    "github.com/sendgrid/sendgrid-go/helpers/mail"
)

type SendGridClient struct {
    APIKey       string
    FromAddress  string
    FromName     string
    TemplateIDs  map[TemplateID]string // map our TemplateID to SendGrid dynamic template IDs
}

func (c *SendGridClient) Send(ctx context.Context, t TemplateID, to string, d map[string]any) error {
    tmpl, ok := c.TemplateIDs[t]
    if !ok { return fmt.Errorf("email: no sendgrid template for %s", t) }
    m := mail.NewV3Mail()
    m.SetFrom(mail.NewEmail(c.FromName, c.FromAddress))
    m.SetTemplateID(tmpl)
    p := mail.NewPersonalization()
    p.AddTos(mail.NewEmail("", to))
    for k, v := range d { p.SetDynamicTemplateData(k, v) }
    m.AddPersonalizations(p)

    resp, err := sg.NewSendClient(c.APIKey).SendWithContext(ctx, m)
    if err != nil { return err }
    if resp.StatusCode >= 300 { return fmt.Errorf("sendgrid: %d %s", resp.StatusCode, resp.Body) }
    return nil
}
```

- [ ] **Step 4: Write a test that the NoopClient records calls**

```go
func TestNoopClient_RecordsCalls(t *testing.T) {
    c := &email.NoopClient{}
    err := c.Send(context.Background(), email.TemplateDunningDay5, "m@example.com", map[string]any{"x": 1})
    require.NoError(t, err)
    require.Len(t, c.Calls, 1)
    require.Equal(t, email.TemplateDunningDay5, c.Calls[0].Template)
}
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/email/
git commit -m "feat(email): Send-only facade for dunning + SCA reminder crons"
```

---

## Task 4: `dunning.RefundWindowGuard` (pure)

**Files:**

- Create: `services/marketplace-api/internal/subscription/dunning/guard.go`
- Create: `services/marketplace-api/internal/subscription/dunning/guard_test.go`

**Spec reference:** §16.5.

- [ ] **Step 1: Failing tests**

```go
package dunning_test

import (
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/dunning"
)

func mustParse(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

func TestIsInRefundWindow_NoFirstCharge_False(t *testing.T) {
    sub := &subscription.StoreSubscription{ID: uuid.New(), FirstChargeAt: nil}
    require.False(t, dunning.IsInRefundWindow(sub, time.Now()))
}

func TestIsInRefundWindow_Day13_True(t *testing.T) {
    firstCharge := mustParse("2026-04-01T00:00:00Z")
    sub := &subscription.StoreSubscription{FirstChargeAt: &firstCharge}
    require.True(t, dunning.IsInRefundWindow(sub, mustParse("2026-04-14T23:00:00Z")))
}

func TestIsInRefundWindow_Day14Exact_False(t *testing.T) {
    firstCharge := mustParse("2026-04-01T00:00:00Z")
    sub := &subscription.StoreSubscription{FirstChargeAt: &firstCharge}
    // Exactly 14d is NOT in the window — window is strictly < 14d.
    require.False(t, dunning.IsInRefundWindow(sub, mustParse("2026-04-15T00:00:00Z")))
}

func TestIsInRefundWindow_Day20_False(t *testing.T) {
    firstCharge := mustParse("2026-04-01T00:00:00Z")
    sub := &subscription.StoreSubscription{FirstChargeAt: &firstCharge}
    require.False(t, dunning.IsInRefundWindow(sub, mustParse("2026-04-21T00:00:00Z")))
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `guard.go`**

```go
// Package dunning implements the §16 failed-payment ladder and the §4.7
// payment_action_required recovery nudges. Every state change is funneled
// through statemachine.Transition (P3); this package never writes status
// directly.
package dunning

import (
    "time"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

// refundWindow is the hard cap — §16.5: first 14 days from the first charge
// are always refund-dominant, so the dunning ladder MUST NOT force a merchant
// from past_due into expired within this window. The guard is a pure function;
// callers combine it with counter emit + no-op.
const refundWindow = 14 * 24 * time.Hour

// IsInRefundWindow returns true iff the sub has a first-charge timestamp AND
// now is strictly less than 14d after it. Subs without first_charge_at (still
// trialing) are NOT in a refund window — the trial-expiry path owns them (P5).
func IsInRefundWindow(sub *subscription.StoreSubscription, now time.Time) bool {
    if sub == nil || sub.FirstChargeAt == nil {
        return false
    }
    return now.Sub(*sub.FirstChargeAt) < refundWindow
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/guard.go \
        services/marketplace-api/internal/subscription/dunning/guard_test.go
git commit -m "feat(dunning): refund-window guard (§16.5)"
```

---

## Task 5: `dunning.StepDailyLadder` cron

**Files:**

- Create: `services/marketplace-api/internal/subscription/dunning/ladder.go`
- Create: `services/marketplace-api/internal/subscription/dunning/ladder_test.go`
- Create: `services/marketplace-api/internal/subscription/dunning/metrics.go`

**Spec reference:** §16.2.

- [ ] **Step 1: Failing test — day 8 past_due → expired fires transition**

```go
//go:build integration

package dunning_test

import (
    "context"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/dunning"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestStepDailyLadder_PastDueDay8_TransitionsToExpired(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)

    // Seed: past_due for 8 days, first charge 60 days ago (well outside refund window).
    firstCharge := time.Now().UTC().Add(-60 * 24 * time.Hour)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusPastDue,
        FirstChargeAt: &firstCharge,
    }).Error)
    // Seed the audit event that marks the status entry timestamp.
    rec.SeedStateTransition(storeID, "active", "past_due",
        time.Now().UTC().Add(-8*24*time.Hour))

    ladder := dunning.NewLadder(em)
    err := ladder.Step(context.Background(), db, time.Now().UTC())
    require.NoError(t, err)

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusExpired, sub.Status)
}
```

- [ ] **Step 2: Failing test — refund window suppresses the transition**

```go
func TestStepDailyLadder_RefundWindow_SuppressesExpired(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    em := audit.NewEmitter(audit.NewRecorderForTesting())

    // First charge 5 days ago — inside 14d window. Past_due for 9 days is
    // impossible in practice (status older than first charge) but the guard
    // must still refuse to move the row.
    firstCharge := time.Now().UTC().Add(-5 * 24 * time.Hour)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusPastDue,
        FirstChargeAt: &firstCharge,
    }).Error)

    ladder := dunning.NewLadder(em)
    err := ladder.Step(context.Background(), db, time.Now().UTC())
    require.NoError(t, err)

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusPastDue, sub.Status, "refund window must dominate")

    require.EqualValues(t, 1, dunning.MetricSuppressedRefundWindow.Value(),
        "suppressed counter must tick")
}
```

- [ ] **Step 3: Failing test — day 14 expired → store_closed**

```go
func TestStepDailyLadder_ExpiredDay14_TransitionsToStoreClosed(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    firstCharge := time.Now().UTC().Add(-120 * 24 * time.Hour)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusExpired,
        FirstChargeAt: &firstCharge,
    }).Error)
    rec.SeedStateTransition(storeID, "past_due", "expired",
        time.Now().UTC().Add(-14*24*time.Hour))

    ladder := dunning.NewLadder(em)
    require.NoError(t, ladder.Step(context.Background(), db, time.Now().UTC()))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusStoreClosed, sub.Status)
}
```

- [ ] **Step 4: Failing test — day 90 store_closed → pending_hard_delete**

```go
func TestStepDailyLadder_StoreClosedDay90_TransitionsToPendingHardDelete(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    firstCharge := time.Now().UTC().Add(-200 * 24 * time.Hour)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusStoreClosed,
        FirstChargeAt: &firstCharge,
    }).Error)
    // Entered store_closed 90 days ago (from expiry day, per §16.2 "day 90 hard-delete path").
    rec.SeedStateTransition(storeID, "expired", "store_closed",
        time.Now().UTC().Add(-90*24*time.Hour))

    ladder := dunning.NewLadder(em)
    require.NoError(t, ladder.Step(context.Background(), db, time.Now().UTC()))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusPendingHardDelete, sub.Status)
}
```

- [ ] **Step 5: Run — expect FAIL**

- [ ] **Step 6: Write `metrics.go`**

```go
package dunning

import "github.com/prometheus/client_golang/prometheus"

var (
    MetricEmailsSent = prometheus.NewCounterVec(prometheus.CounterOpts{
        Namespace: "subscription",
        Subsystem: "dunning",
        Name:      "emails_sent_total",
        Help:      "Dunning nudge emails sent, labelled by day offset (5, 7).",
    }, []string{"day"})

    MetricRemindersSent = prometheus.NewCounterVec(prometheus.CounterOpts{
        Namespace: "subscription",
        Subsystem: "payment_action_required",
        Name:      "reminders_sent_total",
        Help:      "SCA challenge reminders sent, labelled by offset (t_minus_14/7/1).",
    }, []string{"offset"})

    MetricSuppressedRefundWindow = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "subscription",
        Subsystem: "dunning",
        Name:      "suppressed_refund_window_total",
        Help:      "Ladder steps skipped because merchant is within first 14 days (§16.5).",
    })
)

func init() {
    prometheus.MustRegister(MetricEmailsSent, MetricRemindersSent, MetricSuppressedRefundWindow)
}
```

- [ ] **Step 7: Write `ladder.go`**

```go
package dunning

import (
    "context"
    "errors"
    "time"

    "github.com/sirupsen/logrus"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/statemachine"
)

// Ladder runs the §16.2 day-counter transitions. It is idempotent — running
// it twice in the same minute produces exactly the same end-state. Row-level
// concurrency is serialised inside statemachine.Transition (advisory lock +
// CAS from P3). CAS conflicts are swallowed.
type Ladder struct {
    emitter *audit.Emitter
}

func NewLadder(em *audit.Emitter) *Ladder { return &Ladder{emitter: em} }

// Step scans every subscription in past_due / expired / store_closed and moves
// the row forward if the elapsed time in the current status meets the §16.2
// threshold. `now` is injected so tests can drive the clock.
func (l *Ladder) Step(ctx context.Context, db *gorm.DB, now time.Time) error {
    var candidates []subscription.StoreSubscription
    err := db.WithContext(ctx).Where("status IN ?", []subscription.SubscriptionStatus{
        subscription.StatusPastDue,
        subscription.StatusExpired,
        subscription.StatusStoreClosed,
    }).Find(&candidates).Error
    if err != nil { return err }

    for i := range candidates {
        sub := &candidates[i]
        if err := l.stepOne(ctx, db, sub, now); err != nil {
            // Log and continue — one bad row must not halt the ladder.
            logrus.WithError(err).WithField("store_id", sub.StoreID).
                Warn("dunning: stepOne failed")
        }
    }
    return nil
}

func (l *Ladder) stepOne(ctx context.Context, db *gorm.DB, sub *subscription.StoreSubscription, now time.Time) error {
    enteredAt, err := statusEnteredAt(ctx, db, sub.StoreID, sub.Status)
    if err != nil { return err }
    days := int(now.Sub(enteredAt).Hours() / 24)

    switch sub.Status {
    case subscription.StatusPastDue:
        if days < 8 { return nil }
        if IsInRefundWindow(sub, now) {
            MetricSuppressedRefundWindow.Inc()
            logrus.WithField("store_id", sub.StoreID).
                Info("dunning: past_due→expired suppressed by refund window (§16.5)")
            return nil
        }
        return l.transition(ctx, db, sub, subscription.StatusExpired,
            "system:cron:dunning_day_8", "past_due_final_fail")

    case subscription.StatusExpired:
        if days < 14 { return nil }
        return l.transition(ctx, db, sub, subscription.StatusStoreClosed,
            "system:cron:dunning_day_14", "expired_storefront_close")

    case subscription.StatusStoreClosed:
        // "day 90 hard-delete path" — §16.2. 90 days in store_closed.
        if days < 90 { return nil }
        return l.transition(ctx, db, sub, subscription.StatusPendingHardDelete,
            "system:cron:dunning_day_90", "store_closed_hard_delete_scheduled")
    }
    return nil
}

func (l *Ladder) transition(
    ctx context.Context, db *gorm.DB,
    sub *subscription.StoreSubscription, to subscription.SubscriptionStatus,
    actor, reason string,
) error {
    err := statemachine.Transition(ctx, statemachine.TransitionInput{
        DB: db, Emitter: l.emitter,
        TenantID: sub.TenantID, StoreID: sub.StoreID,
        From: sub.Status, To: to,
        Actor: actor, Reason: reason,
    })
    if errors.Is(err, statemachine.ErrCASConflict) {
        // Another ladder tick or a webhook moved the row — not our problem.
        return nil
    }
    return err
}
```

- [ ] **Step 8: Write `statusEnteredAt` helper in `ladder.go`**

```go
// statusEnteredAt returns the timestamp at which this sub entered its current
// status, derived from the most recent matching subscription.state_transition
// audit event. Falls back to updated_at if no audit event is found — the
// fallback keeps legacy rows (migrated from pre-P3) moving.
func statusEnteredAt(ctx context.Context, db *gorm.DB, storeID uuid.UUID, status subscription.SubscriptionStatus) (time.Time, error) {
    var eventAt time.Time
    err := db.WithContext(ctx).Raw(`
        SELECT created_at FROM audit_events
        WHERE event_type = 'subscription.state_transition'
          AND metadata->>'store_id' = ?
          AND metadata->>'to_status' = ?
        ORDER BY created_at DESC
        LIMIT 1`,
        storeID.String(), string(status),
    ).Scan(&eventAt).Error
    if err == nil && !eventAt.IsZero() {
        return eventAt, nil
    }
    // Fallback: updated_at on the sub row.
    var updatedAt time.Time
    err = db.WithContext(ctx).Raw(`SELECT updated_at FROM store_subscriptions WHERE store_id = ?`, storeID).Scan(&updatedAt).Error
    return updatedAt, err
}
```

- [ ] **Step 9: Run tests — expect PASS**

- [ ] **Step 10: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/ladder.go \
        services/marketplace-api/internal/subscription/dunning/ladder_test.go \
        services/marketplace-api/internal/subscription/dunning/metrics.go
git commit -m "feat(dunning): daily ladder — past_due→expired→store_closed→pending_hard_delete"
```

---

## Task 6: `dunning.SendDunningEmails` cron

**Files:**

- Create: `services/marketplace-api/internal/subscription/dunning/emails.go`
- Create: `services/marketplace-api/internal/subscription/dunning/emails_test.go`

**Spec reference:** §16.2 (day 5 + day 7 nudges).

- [ ] **Step 1: Failing test — day 5 email sent with portal URL**

```go
//go:build integration

func TestSendDunningEmails_Day5_SendsOnce(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    mail := &email.NoopClient{}

    firstCharge := time.Now().UTC().Add(-30 * 24 * time.Hour)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusPastDue,
        FirstChargeAt: &firstCharge, ContactEmail: "m@example.com",
    }).Error)
    rec.SeedStateTransition(storeID, "active", "past_due",
        time.Now().UTC().Add(-5*24*time.Hour))

    sender := dunning.NewEmailJob(em, mail, stubPortal("https://billing.stripe.com/p/session/abc"))
    require.NoError(t, sender.SendDunningEmails(context.Background(), db, time.Now().UTC()))

    require.Len(t, mail.Calls, 1)
    require.Equal(t, email.TemplateDunningDay5, mail.Calls[0].Template)
    require.Equal(t, "https://billing.stripe.com/p/session/abc", mail.Calls[0].Data["CustomerPortalURL"])

    // Second run the same day is a no-op (idempotent).
    require.NoError(t, sender.SendDunningEmails(context.Background(), db, time.Now().UTC()))
    require.Len(t, mail.Calls, 1, "same-day re-run must not re-send")
}

func stubPortal(url string) dunning.PortalURLResolver {
    return func(_ context.Context, _ uuid.UUID) (string, error) { return url, nil }
}
```

- [ ] **Step 2: Write `emails.go`**

```go
package dunning

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/sirupsen/logrus"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/email"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

// PortalURLResolver returns a Stripe Customer Portal session URL for the sub,
// idempotent-keyed per P2 (portal:<customer>:<5min-bucket>). The caller is
// expected to wire this to the existing portal session factory.
type PortalURLResolver func(ctx context.Context, storeID uuid.UUID) (string, error)

type EmailJob struct {
    emitter *audit.Emitter
    mail    email.Client
    portal  PortalURLResolver
}

func NewEmailJob(em *audit.Emitter, mail email.Client, portal PortalURLResolver) *EmailJob {
    return &EmailJob{emitter: em, mail: mail, portal: portal}
}

// SendDunningEmails sends day-5 and day-7 nudges to past_due subscriptions.
// Idempotent: uses the subscription.dunning.email_sent audit event as the
// de-dupe key. Same-day re-runs are no-ops.
func (j *EmailJob) SendDunningEmails(ctx context.Context, db *gorm.DB, now time.Time) error {
    var past []subscription.StoreSubscription
    if err := db.WithContext(ctx).
        Where("status = ?", subscription.StatusPastDue).
        Find(&past).Error; err != nil {
        return err
    }

    for i := range past {
        sub := &past[i]
        enteredAt, err := statusEnteredAt(ctx, db, sub.StoreID, sub.Status)
        if err != nil {
            logrus.WithError(err).Warn("dunning: statusEnteredAt failed")
            continue
        }
        days := int(now.Sub(enteredAt).Hours() / 24)
        var tmpl email.TemplateID
        var dayLabel string
        switch days {
        case 5:
            tmpl, dayLabel = email.TemplateDunningDay5, "5"
        case 7:
            tmpl, dayLabel = email.TemplateDunningDay7, "7"
        default:
            continue
        }

        if alreadySent(ctx, db, sub.StoreID, dayLabel, now) {
            continue
        }

        portalURL, err := j.portal(ctx, sub.StoreID)
        if err != nil {
            logrus.WithError(err).Warn("dunning: portal resolver failed")
            continue
        }

        data := map[string]any{
            "StoreName":         sub.StoreName,
            "Amount":            formatAmount(sub),
            "CustomerPortalURL": portalURL,
            "SupportEmail":      "hello@mark8ly.com",
            "DaysUntilExpiry":   8 - days,
        }
        if err := j.mail.Send(ctx, tmpl, sub.ContactEmail, data); err != nil {
            logrus.WithError(err).Warn("dunning: email send failed")
            continue
        }
        MetricEmailsSent.WithLabelValues(dayLabel).Inc()
        j.emitter.EmitEmailSent(sub.StoreID, "subscription.dunning", string(tmpl), now)
    }
    return nil
}

func alreadySent(ctx context.Context, db *gorm.DB, storeID uuid.UUID, dayLabel string, now time.Time) bool {
    var count int64
    _ = db.WithContext(ctx).Raw(`
        SELECT COUNT(*) FROM audit_events
        WHERE event_type = 'subscription.dunning.email_sent'
          AND metadata->>'store_id' = ?
          AND metadata->>'day' = ?
          AND created_at >= ?`,
        storeID.String(), dayLabel, now.Truncate(24*time.Hour),
    ).Scan(&count).Error
    return count > 0
}

func formatAmount(sub *subscription.StoreSubscription) string {
    if sub.BillingCurrency == nil {
        return fmt.Sprintf("%d", sub.BillingAmount)
    }
    return fmt.Sprintf("%s %d", *sub.BillingCurrency, sub.BillingAmount)
}
```

- [ ] **Step 3: Write `SendPaymentActionReminders` in the same file**

See Task 7 — kept separate for clarity.

- [ ] **Step 4: Add `EmitEmailSent` to `audit.Emitter` (one-line wrapper)**

```go
// internal/audit/emitter.go — add:
func (e *Emitter) EmitEmailSent(storeID uuid.UUID, kind, template string, at time.Time) {
    e.Emit(Event{
        Type: "subscription.dunning.email_sent",
        Metadata: map[string]any{
            "store_id": storeID.String(),
            "kind":     kind,
            "template": template,
            "day":      extractDay(template), // "5" or "7"
        },
        CreatedAt: at,
    })
}
```

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/emails.go \
        services/marketplace-api/internal/subscription/dunning/emails_test.go \
        services/marketplace-api/internal/audit/emitter.go
git commit -m "feat(dunning): day-5 + day-7 email nudges with Customer Portal URL"
```

---

## Task 7: `dunning.SendPaymentActionReminders` cron

**Files:**

- Modify: `services/marketplace-api/internal/subscription/dunning/emails.go` (append)
- Modify: `services/marketplace-api/internal/subscription/dunning/emails_test.go`

**Spec reference:** §4.7 — "Reminder emails T-14, T-7, T-1" from the renewal/charge date.

- [ ] **Step 1: Failing test — T-14 reminder fires exactly once**

```go
func TestSendPaymentActionReminders_T14_SendsOnceWithHostedURL(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    mail := &email.NoopClient{}

    hostedURL := "https://invoice.stripe.com/i/acct_x/test_ABC"
    renewAt := time.Now().UTC().Add(14 * 24 * time.Hour)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusPaymentActionRequired,
        HostedInvoiceURL: &hostedURL,
        CurrentPeriodEnd: &renewAt,
        ContactEmail: "m@example.com",
    }).Error)

    sender := dunning.NewEmailJob(em, mail, nil /* portal not needed */)
    require.NoError(t, sender.SendPaymentActionReminders(context.Background(), db, time.Now().UTC()))

    require.Len(t, mail.Calls, 1)
    require.Equal(t, email.TemplatePaymentActionReminder, mail.Calls[0].Template)
    require.Equal(t, hostedURL, mail.Calls[0].Data["HostedInvoiceURL"])
    require.Equal(t, "14 days", mail.Calls[0].Data["OffsetLabel"])

    // Row exists in payment_action_reminders — second run is a no-op.
    require.NoError(t, sender.SendPaymentActionReminders(context.Background(), db, time.Now().UTC()))
    require.Len(t, mail.Calls, 1)
}
```

- [ ] **Step 2: Append `SendPaymentActionReminders` to `emails.go`**

```go
// SendPaymentActionReminders sends T-14 / T-7 / T-1 reminders to subs in
// payment_action_required. Idempotency comes from the payment_action_reminders
// table: PK on (subscription_id, offset_key) + INSERT ... ON CONFLICT DO NOTHING.
func (j *EmailJob) SendPaymentActionReminders(ctx context.Context, db *gorm.DB, now time.Time) error {
    var subs []subscription.StoreSubscription
    if err := db.WithContext(ctx).
        Where("status = ? AND hosted_invoice_url IS NOT NULL AND current_period_end IS NOT NULL",
            subscription.StatusPaymentActionRequired).
        Find(&subs).Error; err != nil {
        return err
    }

    for i := range subs {
        sub := &subs[i]
        daysUntilRenew := int(sub.CurrentPeriodEnd.Sub(now).Hours() / 24)

        var offsetKey, offsetLabel string
        switch {
        case daysUntilRenew == 14:
            offsetKey, offsetLabel = "t_minus_14", "14 days"
        case daysUntilRenew == 7:
            offsetKey, offsetLabel = "t_minus_7",  "7 days"
        case daysUntilRenew == 1:
            offsetKey, offsetLabel = "t_minus_1",  "tomorrow"
        default:
            continue
        }

        // Idempotency: insert-on-conflict. Returns rows=1 only the first time.
        res := db.WithContext(ctx).Exec(`
            INSERT INTO payment_action_reminders (subscription_id, offset_key, stripe_invoice_id)
            VALUES (?, ?, ?)
            ON CONFLICT (subscription_id, offset_key) DO NOTHING`,
            sub.ID, offsetKey, sub.HostedInvoiceURL,
        )
        if res.Error != nil {
            logrus.WithError(res.Error).Warn("dunning: reminder idempotency insert failed")
            continue
        }
        if res.RowsAffected == 0 {
            continue // already sent
        }

        data := map[string]any{
            "StoreName":        sub.StoreName,
            "Amount":           formatAmount(sub),
            "HostedInvoiceURL": *sub.HostedInvoiceURL,
            "OffsetLabel":      offsetLabel,
            "SupportEmail":     "hello@mark8ly.com",
        }
        if err := j.mail.Send(ctx, email.TemplatePaymentActionReminder, sub.ContactEmail, data); err != nil {
            logrus.WithError(err).Warn("dunning: payment-action reminder send failed")
            // Delete the idempotency row so a retry re-sends.
            _ = db.WithContext(ctx).Exec(`
                DELETE FROM payment_action_reminders
                WHERE subscription_id = ? AND offset_key = ?`, sub.ID, offsetKey).Error
            continue
        }
        MetricRemindersSent.WithLabelValues(offsetKey).Inc()
    }
    return nil
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/emails.go \
        services/marketplace-api/internal/subscription/dunning/emails_test.go
git commit -m "feat(dunning): SCA reminders T-14/T-7/T-1 with idempotency table (§4.7)"
```

---

## Task 8: `dunning.Scheduler` bootstrap + `main.go` wiring

**Files:**

- Create: `services/marketplace-api/internal/subscription/dunning/scheduler.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Write `scheduler.go`**

```go
package dunning

import (
    "context"
    "time"

    "github.com/robfig/cron/v3"
    "github.com/sirupsen/logrus"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/email"
)

type Scheduler struct {
    cron   *cron.Cron
    db     *gorm.DB
    ladder *Ladder
    mail   *EmailJob
}

func NewScheduler(db *gorm.DB, em *audit.Emitter, mailClient email.Client, portal PortalURLResolver) *Scheduler {
    return &Scheduler{
        cron:   cron.New(cron.WithLocation(time.UTC)),
        db:     db,
        ladder: NewLadder(em),
        mail:   NewEmailJob(em, mailClient, portal),
    }
}

func (s *Scheduler) Start() error {
    // Ladder every hour on the hour — idempotent, so over-running is fine.
    if _, err := s.cron.AddFunc("0 * * * *", s.runLadder); err != nil { return err }
    // Dunning emails 09:05 UTC daily.
    if _, err := s.cron.AddFunc("5 9 * * *", s.runDunningEmails); err != nil { return err }
    // SCA reminders 09:15 UTC daily.
    if _, err := s.cron.AddFunc("15 9 * * *", s.runPaymentActionReminders); err != nil { return err }
    s.cron.Start()
    logrus.Info("dunning: scheduler started (ladder hourly, emails 09:05, reminders 09:15 UTC)")
    return nil
}

func (s *Scheduler) Stop(ctx context.Context) error {
    stopCtx := s.cron.Stop()
    select {
    case <-stopCtx.Done():
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s *Scheduler) runLadder() {
    if err := s.ladder.Step(context.Background(), s.db, time.Now().UTC()); err != nil {
        logrus.WithError(err).Error("dunning: ladder step failed")
    }
}
func (s *Scheduler) runDunningEmails() {
    if err := s.mail.SendDunningEmails(context.Background(), s.db, time.Now().UTC()); err != nil {
        logrus.WithError(err).Error("dunning: SendDunningEmails failed")
    }
}
func (s *Scheduler) runPaymentActionReminders() {
    if err := s.mail.SendPaymentActionReminders(context.Background(), s.db, time.Now().UTC()); err != nil {
        logrus.WithError(err).Error("dunning: SendPaymentActionReminders failed")
    }
}
```

- [ ] **Step 2: Wire in `main.go`**

```go
// after DB + audit emitter + email client are constructed:
portalResolver := billing.NewPortalURLResolver(stripeClient) // from P2
dunningScheduler := dunning.NewScheduler(db, auditEmitter, emailClient, portalResolver)
if err := dunningScheduler.Start(); err != nil {
    logrus.WithError(err).Fatal("dunning: scheduler failed to start")
}

// graceful shutdown — extend the existing shutdown sequence:
<-shutdownCtx.Done()
stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := dunningScheduler.Stop(stopCtx); err != nil {
    logrus.WithError(err).Warn("dunning: scheduler shutdown timeout")
}
```

- [ ] **Step 3: Build**

```bash
cd services/marketplace-api
go build ./...
```

- [ ] **Step 4: Smoke test — scheduler starts + stops cleanly**

```bash
go test ./internal/subscription/dunning/... -run TestScheduler -v
```

(Add a small unit test that calls `Start()` then `Stop(ctx)` with a fake DB; the goal is coverage of the bootstrap path.)

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/scheduler.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(dunning): robfig/cron scheduler bootstrap + main.go graceful shutdown"
```

---

## Task 9: Webhook patch — persist `hosted_invoice_url`

**Files:**

- Modify: `services/marketplace-api/internal/billing/dispatch/handlers.go`
- Modify: `services/marketplace-api/internal/billing/dispatch/handlers_test.go`

**Objective:** When `invoice.payment_action_required` lands, extract `data.object.hosted_invoice_url` from the event payload and UPDATE `store_subscriptions.hosted_invoice_url` inside the same advisory-locked transaction that P3 Task 3 opened. When `invoice.paid` lands, NULL the column out.

- [ ] **Step 1: Failing test — payment_action_required webhook persists URL**

```go
func TestHandleInvoicePaymentActionRequired_PersistsHostedURL(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
    }).Error)

    raw := []byte(`{
        "id":"evt_2","type":"invoice.payment_action_required",
        "data":{"object":{
            "id":"in_x","customer":"cus_x","subscription":"sub_x",
            "hosted_invoice_url":"https://invoice.stripe.com/i/acct_x/test_ABC"
        }}
    }`)
    d := dispatch.NewWithStateMachine(em)
    require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
        EventID: "evt_2", EventType: "invoice.payment_action_required", Payload: raw,
    }))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusPaymentActionRequired, sub.Status)
    require.NotNil(t, sub.HostedInvoiceURL)
    require.Equal(t, "https://invoice.stripe.com/i/acct_x/test_ABC", *sub.HostedInvoiceURL)
}
```

- [ ] **Step 2: Failing test — invoice.paid clears the URL**

```go
func TestHandleInvoicePaid_ClearsHostedURL_OnPaymentActionRequiredRecovery(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    em := audit.NewEmitter(audit.NewRecorderForTesting())

    hosted := "https://invoice.stripe.com/i/acct_x/test_ABC"
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusPaymentActionRequired,
        HostedInvoiceURL: &hosted,
    }).Error)

    raw := []byte(`{"id":"evt_3","type":"invoice.paid","data":{"object":{"customer":"cus_x"}}}`)
    d := dispatch.NewWithStateMachine(em)
    require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
        EventID: "evt_3", EventType: "invoice.paid", Payload: raw,
    }))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusActive, sub.Status, "P3 transition must complete")
    require.Nil(t, sub.HostedInvoiceURL, "hosted URL must clear on recovery")
}
```

- [ ] **Step 3: Patch `handleInvoicePaymentActionRequired`**

In `internal/billing/dispatch/handlers.go`, after the existing `statemachine.Transition` call for `active → payment_action_required`, persist the URL:

```go
var e struct {
    Data struct {
        Object struct {
            Customer          string `json:"customer"`
            HostedInvoiceURL  string `json:"hosted_invoice_url"`
        } `json:"object"`
    } `json:"data"`
}
if err := json.Unmarshal(raw, &e); err != nil { return err }
// ...existing lookup of sub by customer...
// ...existing statemachine.Transition call...

if e.Data.Object.HostedInvoiceURL != "" {
    if err := tx.Exec(`
        UPDATE store_subscriptions
        SET hosted_invoice_url = ?, updated_at = now()
        WHERE id = ?`,
        e.Data.Object.HostedInvoiceURL, sub.ID,
    ).Error; err != nil {
        return fmt.Errorf("dispatch: persist hosted_invoice_url: %w", err)
    }
}
```

- [ ] **Step 4: Patch `handleInvoicePaid`**

After the existing `statemachine.Transition(payment_action_required → active)` OR `past_due → active` branch succeeds, clear the URL:

```go
if err := tx.Exec(`
    UPDATE store_subscriptions
    SET hosted_invoice_url = NULL, updated_at = now()
    WHERE id = ? AND hosted_invoice_url IS NOT NULL`, sub.ID,
).Error; err != nil {
    logrus.WithError(err).Warn("dispatch: failed to clear hosted_invoice_url; continuing")
}
```

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/billing/dispatch/
git commit -m "feat(billing): persist/clear hosted_invoice_url on payment_action_required webhooks"
```

---

## Task 10: `/subscription/complete-action` redirect endpoint

**Files:**

- Create: `services/marketplace-api/internal/handlers/admin/subscription_complete_action.go`
- Create: `services/marketplace-api/internal/handlers/admin/subscription_complete_action_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`
- Modify: `services/marketplace-api/internal/subscription/readonly/allowlist.go`

- [ ] **Step 1: Failing test — 302 with Location**

```go
//go:build integration

func TestCompleteAction_RedirectsToHostedInvoiceURL(t *testing.T) {
    suite := inttest.NewSuite(t)
    hosted := "https://invoice.stripe.com/i/acct_x/test_ABC"
    tenantID, storeID := suite.SeedStoreWithHostedURL(
        subscription.StatusPaymentActionRequired,
        subscription.PlanStarter,
        hosted,
    )
    resp := suite.AdminGET(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/subscription/complete-action")
    require.Equal(t, 302, resp.Code)
    require.Equal(t, hosted, resp.Header().Get("Location"))
}

func TestCompleteAction_404_WhenNoHostedURL(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanStarter)
    resp := suite.AdminGET(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/subscription/complete-action")
    require.Equal(t, 404, resp.Code)
}
```

- [ ] **Step 2: Write the handler**

```go
package admin

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

type CompleteActionHandler struct {
    Repo subscription.Repository
}

func (h *CompleteActionHandler) Handle(c *gin.Context) {
    tenantID := c.MustGet("tenant_id").(string)
    storeIDStr := c.Param("storeId")
    storeID, err := uuid.Parse(storeIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"}); return
    }
    sub, err := h.Repo.GetByStoreID(c.Request.Context(), tenantID, storeID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "subscription_not_found"}); return
    }
    if sub.HostedInvoiceURL == nil || *sub.HostedInvoiceURL == "" {
        c.JSON(http.StatusNotFound, gin.H{"error": "no_outstanding_action"}); return
    }
    c.Redirect(http.StatusFound, *sub.HostedInvoiceURL)
}
```

- [ ] **Step 3: Register the route**

In `routes.go`, in the store-scoped admin group (after `RequireActive`):

```go
completeAction := &CompleteActionHandler{Repo: deps.SubscriptionRepo}
storeRoute.GET("/subscription/complete-action", completeAction.Handle)
```

- [ ] **Step 4: Add to the read-only allowlist**

In `internal/subscription/readonly/allowlist.go`:

```go
var DefaultAllowlist = []AllowedRoute{
    // ...existing entries...
    {http.MethodGet, "/admin/stores/:storeId/subscription/complete-action"},
}
```

(An `expired` merchant who has `hosted_invoice_url` populated from an earlier SCA event — edge case — should still reach recovery.)

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/subscription_complete_action*.go \
        services/marketplace-api/internal/handlers/admin/routes.go \
        services/marketplace-api/internal/subscription/readonly/allowlist.go
git commit -m "feat(admin): /subscription/complete-action 302 redirect to hosted invoice"
```

---

## Task 11: Metrics exposure

**Files:**

- Verify: `services/marketplace-api/cmd/marketplace-api/main.go` already mounts Prometheus `/metrics`

- [ ] **Step 1: Verify `/metrics` is mounted**

```bash
grep -n 'promhttp.Handler\|/metrics' services/marketplace-api/cmd/marketplace-api/main.go
```

If absent, add:

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"
// ...
router.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

- [ ] **Step 2: Smoke test — counters appear**

```bash
go test ./internal/subscription/dunning/... -run Metrics -v
```

Add a test that triggers one ladder step + one email send and asserts both counters increment by 1.

- [ ] **Step 3: Commit (if any change)**

```bash
git add -u
git commit --allow-empty -m "chore(metrics): verify dunning counters exposed via /metrics"
```

---

## Task 12: Full time-mocked integration — day 0 → day 90

**Files:**

- Create: `services/marketplace-api/internal/subscription/dunning/integration_test.go`

**Purpose:** End-to-end proof that a single subscription, walked through a simulated 90-day clock, ends in `pending_hard_delete` with the correct emails sent and no refund-window suppression (first_charge well outside the window).

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestIntegration_FullLadder_Day0ToDay90(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "payment_action_reminders", "audit_events")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    mail := &email.NoopClient{}

    firstCharge := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
        FirstChargeAt: &firstCharge, ContactEmail: "m@example.com",
    }).Error)

    ladder := dunning.NewLadder(em)
    emails := dunning.NewEmailJob(em, mail, stubPortal("https://billing.stripe.com/p/abc"))

    // Day 15 — invoice.payment_failed → past_due.
    day15 := firstCharge.AddDate(0, 0, 15)
    require.NoError(t, statemachine.Transition(ctx, statemachine.TransitionInput{
        DB: db, Emitter: em, TenantID: tenantID, StoreID: storeID,
        From: subscription.StatusActive, To: subscription.StatusPastDue,
        Actor: "system:webhook:stripe", Reason: "invoice.payment_failed",
    }))
    rec.SeedStateTransitionAt(storeID, "active", "past_due", day15)

    // Day 15+5 — day 5 nudge.
    day20 := day15.AddDate(0, 0, 5)
    require.NoError(t, emails.SendDunningEmails(ctx, db, day20))
    requireTemplateSent(t, mail, email.TemplateDunningDay5)

    // Day 15+7 — day 7 nudge.
    day22 := day15.AddDate(0, 0, 7)
    require.NoError(t, emails.SendDunningEmails(ctx, db, day22))
    requireTemplateSent(t, mail, email.TemplateDunningDay7)

    // Day 15+8 — past_due → expired.
    day23 := day15.AddDate(0, 0, 8)
    require.NoError(t, ladder.Step(ctx, db, day23))
    requireStatus(t, db, storeID, subscription.StatusExpired)

    // Day 23+14 = day 37 — expired → store_closed.
    day37 := day23.AddDate(0, 0, 14)
    rec.SeedStateTransitionAt(storeID, "past_due", "expired", day23)
    require.NoError(t, ladder.Step(ctx, db, day37))
    requireStatus(t, db, storeID, subscription.StatusStoreClosed)

    // Day 37+90 = day 127 — store_closed → pending_hard_delete.
    day127 := day37.AddDate(0, 0, 90)
    rec.SeedStateTransitionAt(storeID, "expired", "store_closed", day37)
    require.NoError(t, ladder.Step(ctx, db, day127))
    requireStatus(t, db, storeID, subscription.StatusPendingHardDelete)
}

func TestIntegration_RefundWindow_BlocksExpiry(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    em := audit.NewEmitter(audit.NewRecorderForTesting())

    // First charge 3 days ago — deep in refund window.
    firstCharge := time.Now().UTC().Add(-3 * 24 * time.Hour)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusPastDue,
        FirstChargeAt: &firstCharge,
    }).Error)
    // Pretend it's been past_due for 9 days (clock mismatch — we assert the guard still fires).

    ladder := dunning.NewLadder(em)
    before := dunning.MetricSuppressedRefundWindow.Value()
    require.NoError(t, ladder.Step(context.Background(), db,
        firstCharge.AddDate(0, 0, 13))) // day 13 — still in window
    after := dunning.MetricSuppressedRefundWindow.Value()

    require.Greater(t, after, before, "suppression counter must tick")
    requireStatus(t, db, storeID, subscription.StatusPastDue)
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/integration_test.go
git commit -m "test(dunning): full day-0→day-90 ladder + refund-window suppression"
```

---

## Task 13: Regression — SCA recovery end-to-end

**Files:**

- Create: `services/marketplace-api/internal/subscription/dunning/sca_recovery_test.go`

**Purpose:** Exercises the full happy path for §4.7 recovery:
1. Webhook `invoice.payment_action_required` lands → `active → payment_action_required`, `hosted_invoice_url` persisted.
2. `SendPaymentActionReminders` fires at T-14 / T-7 / T-1 (three separate invocations with different mocked `now`).
3. Webhook `invoice.paid` lands → `payment_action_required → active`, `hosted_invoice_url` cleared.
4. No future reminder fires (status no longer matches filter).

- [ ] **Step 1: Write the test** (structure as in Task 12; compose the three existing test helpers).

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/sca_recovery_test.go
git commit -m "test(dunning): SCA recovery end-to-end — reminders + invoice.paid clears state"
```

---

## Task 14: Criterion 38 re-assert under dunning

**Files:**

- Create: `services/marketplace-api/internal/subscription/dunning/criterion_38_test.go`

**Purpose:** P3 already proved `payment_action_required` merchants keep full admin (criterion 38). P6 must additionally prove:

1. The dunning ladder does NOT fire `past_due` → `expired` for a `payment_action_required` sub (different branch; should never be touched).
2. The SCA reminder cron DOES fire for them.
3. The dunning email cron does NOT fire for them (they're not in `past_due`).

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestCriterion38_DunningSkipsPaymentActionRequired(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "payment_action_reminders")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    mail := &email.NoopClient{}

    hosted := "https://invoice.stripe.com/i/acct_x/test_ABC"
    renewAt := time.Now().UTC().Add(14 * 24 * time.Hour)
    firstCharge := time.Now().UTC().Add(-30 * 24 * time.Hour)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusPaymentActionRequired,
        HostedInvoiceURL: &hosted, CurrentPeriodEnd: &renewAt,
        FirstChargeAt: &firstCharge, ContactEmail: "m@example.com",
    }).Error)

    ladder := dunning.NewLadder(em)
    emails := dunning.NewEmailJob(em, mail, stubPortal("unused"))

    require.NoError(t, ladder.Step(context.Background(), db, time.Now().UTC()))
    requireStatus(t, db, storeID, subscription.StatusPaymentActionRequired)

    require.NoError(t, emails.SendDunningEmails(context.Background(), db, time.Now().UTC()))
    require.Empty(t, mail.Calls, "dunning nudges must not fire for payment_action_required")

    require.NoError(t, emails.SendPaymentActionReminders(context.Background(), db, time.Now().UTC()))
    require.Len(t, mail.Calls, 1, "SCA T-14 reminder must fire")
    require.Equal(t, email.TemplatePaymentActionReminder, mail.Calls[0].Template)
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/criterion_38_test.go
git commit -m "test(dunning): criterion 38 — dunning leaves payment_action_required alone"
```

---

## Task 15: Copy-tone regression

**Files:**

- Create: `services/marketplace-api/internal/email/copy_tone_test.go`
- Modify: `services/marketplace-api/internal/email/templates.go` (expose a test registry of the rendered copy)

**Purpose:** §16.4 — "editorial/calm, not threatening". The three dunning/SCA templates must not contain banned phrases, regardless of which downstream provider renders them.

- [ ] **Step 1: Add a test-only registry in `templates.go`**

```go
// TestingCopyRegistry exposes the canonical body of each template for
// compliance testing. In production the actual render happens in the
// notification-service / SendGrid provider; this registry exists so the
// subscription team can guarantee tone in CI without depending on the
// provider.
var TestingCopyRegistry = map[TemplateID]string{
    TemplateDunningDay5: `Hi {MerchantName},

We couldn't process your most recent payment for {StoreName}. No rush — your
storefront stays live. Update your card in the Customer Portal when it suits you:
{CustomerPortalURL}

We're here if you'd like help: {SupportEmail}.`,
    TemplateDunningDay7: `Hi {MerchantName},

A quick note about {StoreName}. If we're unable to collect payment in the next
day or so, your admin will move into read-only mode while we sort things out.
Your storefront remains live. The easiest fix is to update your card:
{CustomerPortalURL}

Any questions, just reply.`,
    TemplatePaymentActionReminder: `Hi {MerchantName},

Your bank needs to confirm this payment for {StoreName} — it'll take a minute.
Amount: {Amount}. Due in {OffsetLabel}. Complete the quick authentication step:
{HostedInvoiceURL}

If you need anything: {SupportEmail}.`,
}
```

- [ ] **Step 2: Write the tone test**

```go
package email_test

import (
    "strings"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/email"
)

var bannedPhrases = []string{
    "URGENT",
    "IMMEDIATE ACTION REQUIRED",
    "PAY NOW",
    "FINAL WARNING",
    "YOUR ACCOUNT WILL BE CLOSED",
    "LAST CHANCE",
    "ACT NOW",
    "!!", // multiple exclamation marks
}

func TestEmailCopyTone_NoThreateningLanguage(t *testing.T) {
    for tmpl, body := range email.TestingCopyRegistry {
        upper := strings.ToUpper(body)
        for _, banned := range bannedPhrases {
            require.NotContains(t, upper, banned,
                "template %s contains banned phrase %q (§16.4 editorial tone)", tmpl, banned)
        }
    }
}

func TestEmailCopyTone_MentionsCustomerPortalOrHostedURL(t *testing.T) {
    require.Contains(t, email.TestingCopyRegistry[email.TemplateDunningDay5],   "{CustomerPortalURL}")
    require.Contains(t, email.TestingCopyRegistry[email.TemplateDunningDay7],   "{CustomerPortalURL}")
    require.Contains(t, email.TestingCopyRegistry[email.TemplatePaymentActionReminder], "{HostedInvoiceURL}")
}

func TestEmailCopyTone_SupportEmailPresent(t *testing.T) {
    for tmpl, body := range email.TestingCopyRegistry {
        require.Contains(t, body, "{SupportEmail}",
            "template %s should surface a support address (§16.4)", tmpl)
    }
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/email/copy_tone_test.go \
        services/marketplace-api/internal/email/templates.go
git commit -m "test(email): enforce §16.4 editorial/calm tone in dunning + SCA templates"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./internal/subscription/dunning/... ./internal/email/... ./internal/billing/dispatch/... -count=1` all green.
- [ ] Migrations 047 + 048 up and down run cleanly on a fresh DB.
- [ ] `dunning.Scheduler.Start()` + graceful `Stop(ctx)` exercised by unit test.
- [ ] `MetricSuppressedRefundWindow` ticks exactly once per suppressed ladder step; `MetricEmailsSent{day="5"}` and `{day="7"}` tick once per nudge; `MetricRemindersSent{offset="t_minus_14"}` etc. tick once per SCA reminder.
- [ ] Every state mutation in the dunning package flows through `statemachine.Transition` — grep confirms no `UPDATE store_subscriptions ... SET status` outside the dispatcher and statemachine packages:

```bash
cd services/marketplace-api
grep -RnE 'UPDATE\s+store_subscriptions\s+SET\s+status' internal/subscription/dunning/ || echo "clean"
```

Expected: `clean`.

- [ ] Integration test: day-0 → day-127 ladder ends in `pending_hard_delete` with the expected emails sent.
- [ ] Integration test: refund-window merchant (first_charge 3d ago) stays in `past_due`, suppression counter ticks.
- [ ] Integration test: `payment_action_required` merchant receives T-14 reminder, does NOT receive a dunning day-5/7 nudge, does NOT transition to `expired` via the ladder.
- [ ] Integration test: `invoice.paid` webhook after `payment_action_required` transitions to `active` AND clears `hosted_invoice_url`.
- [ ] Integration test: `/admin/stores/:id/subscription/complete-action` returns 302 with `Location: <hosted_invoice_url>`; returns 404 when no outstanding action.
- [ ] Copy-tone test: zero banned phrases across three templates; every template references the appropriate recovery URL + support email.
- [ ] Allowlist: `/subscription/complete-action` reachable for `expired` merchant (returns 302 if URL present, 404 otherwise — never 402).

---

## What's now unlocked

- **P10** (refund flow) can read `IsInRefundWindow` to decide whether a refund request hits the fast path; the counter `subscription.dunning.suppressed_refund_window` becomes the cross-check that P6 is honoring the rule.
- **P11** (cancellation emails) reuses `email.Client` facade, adds its own templates (`cancel_scheduled`, `save_offer_expired`), and the `audit.EmitEmailSent` helper added here.
- **P12** (Cloudflare Worker `closed.html`) consumes the `store_closed` status set by `dunning.StepDailyLadder` on day 14 — the Worker polls subscription status via an existing lookup endpoint; no new API surface from P6.
- **P16** (admin banner) reads `subscription.HostedInvoiceURL` from the per-store subscription endpoint and renders a CTA that POSTs to `/subscription/complete-action` — the endpoint and field are ready for the banner to consume.
- **P17** (observability) ingests the three new counters into the subscription health dashboard alongside P3's state-transition gauge.
- **Hard-delete executor** (a later security-reviewed plan) picks up rows in `pending_hard_delete` placed there by P6's ladder and completes the `pending_hard_delete → hard_deleted` transition — which P6 deliberately does NOT perform.

---

## Execution handoff

Plan complete. P6 depends on P1 (data model + advisory lock + audit scaffold), P2 (webhook dispatcher + hosted invoice URL extraction), and P3 (state machine + `payment_action_required` not-read-only decision). Do not execute P6 before those three are merged and their integration tests are green.

Execute this plan with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**. Once green, the dunning + SCA recovery surface is complete; the only remaining subscription-lifecycle work is cancellation/save-offer (P11), refunds (P10), storefront closed-page (P12), admin banner (P16), and hard-delete executor (separate security plan).
