# Design — `GET /admin/notifications` (#332)

**Status:** approved
**Issue:** #332 · **Umbrella:** #260 · **Reference endpoints:** #276, #329 · **Date:** 2026-08-24
**Split out:** #348 (email delivery log)

## The issue's premise does not hold

#332 calls `/admin/notifications` "the sent-mail log across tenants" and opens with the
support question *"did the merchant actually get the email?"*. It names the data source
itself: the `notifications` table, migrations `000016` and `000091`.

That table is the **in-app notification bell**, not a mail log. Verified against the
migration and the model (`internal/notification/models.go`):

| #332 asks for | what the table has |
|---|---|
| `recipient` (email) | `recipient_user_id` — the **customer profile id** (`customers.id`, a UUID), and **only on storefront-customer rows**. NULL on every merchant/staff row, where the target is the store, not a person |
| delivery `status` | no such column. `is_read` is a **read** flag |
| `template` | `type` — an event kind (`new_order`, `low_stock`, `order_shipped`, …) |
| "did the email arrive" | nothing in the row was ever emailed |

### No *delivery-outcome* record exists anywhere in the estate

**Corrected 2026-08-25.** This section originally claimed that *no record of outbound
mail exists anywhere in the estate*. That claim was **false**, and the final whole-branch
review caught it. Two partial per-email records do exist — one of them
(`shipments.dispatched_email_sent_at`) appeared in the very grep output this section was
written from, and was read past. The correct, narrower claim is the one that matters:
**nothing records a delivery outcome.** The failure is trap 9's exact shape — a negative
established by a search that was run and then misread, and then written into a comment
confident enough to redirect the next reader.

What the search actually supports, across all four services' migrations and Go sources:

- **Transactional mail is fire-and-forget and unrecorded.** Every product mailer hands a
  finished envelope to `email.Sender` — SendGrid primary, Resend fallback
  (`internal/email/sender.go`); `platform-api/internal/notification` mirrors it. Neither
  persists anything. No `sent_email` / `email_log` table exists in any service.
- **No provider event webhook exists.** `email.Message.CustomArgs`'s doc comment refers to
  "the notification-service Wave 1.5 webhook receiver"; it was never rebuilt here. Open and
  click tracking are enabled on outgoing mail (`internal/email/sendgrid.go:86-90`) and
  nothing receives the resulting events.
- **Two partial handoff records DO exist, and neither is a delivery outcome.**
  `shipments.dispatched_email_sent_at` (migration `000086`) is a per-shipment timestamp
  written at `internal/handlers/admin/shipments.go:287-288` purely as a dedup gate for the
  "your order has shipped" email — it records that a send was *attempted once*, nothing
  about its fate. And `campaign_recipients`, below.
- **That per-recipient campaign table records handoff only, and loses failures.**
  `campaign_recipients` (`000012`) declares `pending`/`sent`/`delivered`/`opened`/
  `clicked`/`bounced`/`unsubscribed` (`internal/campaign/models.go:28-34`). Only `sent`
  is ever written, from the single caller at `internal/campaign/send_worker.go:241`. A
  dispatch failure leaves the row at `pending` and increments a campaign-level counter
  (`send_worker.go:232-239`) — so `pending` conflates "not attempted" with "failed".

**Therefore acceptance criterion 3 — "delivery status is the current one, not just queued
at send time" — is not satisfiable by any data this estate holds, for any endpoint, today.**
The two handoff records above establish *that a send was attempted*, never *what became of
it*, which is the question the criterion asks.
It is not a gap in this endpoint's query; it is a gap in what is recorded. That work is
**#348**, split out with the analysis above.

**Ruling:** #332 ships the cross-tenant in-app notification log — real, already written by
~20 call sites, and currently unreadable outside a single store. It does **not** claim to
answer the email question, and says so on the issue. Same shape as #329's missing
`assignee` and #277's missing tenant slug: the field named in the issue does not exist, so
it is omitted and reported rather than invented.

## Contract

```
GET /api/v1/platform/admin/notifications
```

A read: HMAC signature required; no operator identity or capability, per the enforcement
matrix in the foundation spec.

### Filters

Every one maps to a real column.

| param | behaviour |
|---|---|
| `type` | exact match on `type`. This is the issue's `template`/`type` — there is no template column |
| `tenant_id` | optional **narrowing**, not a required scope |
| `store_id` | optional narrowing |
| `audience` | `store` \| `customer` → `recipient_user_id IS NULL` / `IS NOT NULL`. Any other value is ignored, not an error |
| `recipient_user_id` | exact match — the honest form of the issue's `recipient` filter |
| `read` | `true` \| `false` on `is_read` |
| `since_hours` | `created_at >= now - N hours` |
| `from` / `to` | explicit range; **wins over `since_hours`** when both are supplied, matching #276 and #329 |
| `limit` | default 50, clamp 500; oversized clamps, missing takes the default, never errors |
| `page` | 1-based |

Unknown query parameters are ignored, as elsewhere on this surface.

### Row projection

```json
{
  "id": "…",
  "tenant_id": "…",
  "store_id": "…",
  "type": "new_order",
  "title": "New order received",
  "audience": "store",
  "recipient_user_id": "…",
  "resource_type": "order",
  "resource_id": "…",
  "is_read": false,
  "created_at": "2026-08-24T10:00:00Z"
}
```

`recipient_user_id` carries the customer profile id (`customers.id`). It, `resource_type`
and `resource_id` are `omitempty`. `audience` is
always present — it is what makes an absent `recipient_user_id` mean "this went to the
store" rather than "the lookup failed".

### Deliberately absent

- **`message`** — the interpolated body. It is the only field in the table carrying
  customer detail (`"New order ORD-1042 placed."`), and keeping it out is the entire point
  of #332's "do not return message bodies". Same reasoning that keeps `description` out of
  #329 and `payload` out of #331.
- **`status`** — no delivery status exists. Emitting `is_read` under the name `status`
  would put a governance label on a metric that answers a different question — #282's
  structurally-wrong counter, with more consequence because an operator would act on it.
- **`recipient` (email)** — no email column exists. `recipient_user_id` is the customer
  profile id (`customers.id`) and is set only on customer rows.

All three are reported on #332 rather than worked around.

### On `title`

`title` is included as the subject-analogue #332 explicitly permits. Every title written
today is a fixed literal — thirteen distinct values across the codebase, and the one
computed-looking site (`internal/handlers/storefront/tickets.go:604`) assigns a constant to
a local first. `message` is the only interpolating field.

**That observation is not the safety mechanism.** `message` must be absent because
`toNotificationRow` never reads it — a property of the projection, not of what today's
titles happen to contain. **Project, do not pass through**: a column added to
`notification.Notification` tomorrow must not be able to reach the console automatically.

## Components

### `internal/notification` — a new cross-store method

`ListByStore` hardcodes `store_id = ?` (`repository.go:72-74`) and `ListByCustomer` adds
`recipient_user_id`. With a zero UUID either matches **nothing**, which is the safe failure
for a merchant-facing query.

**Add `ListPlatform(ctx, db, PlatformListFilter)` with its own filter type. Do NOT make
`ListFilter`'s `StoreID` optional.** Widening a zero `StoreID` to mean "all stores" would
invert a fail-safe into fail-open on the merchant path — one forgotten field away from a
merchant reading another store's notifications. This is #329's ruling for
`ticket.ListFilter`, unchanged, and for the same reason.

```go
// PlatformListFilter is the CROSS-STORE filter. Deliberately a separate type
// from ListFilter, which requires a store and matches nothing without one.
type PlatformListFilter struct {
    TenantID        *uuid.UUID // optional NARROWING, not a scope
    StoreID         *uuid.UUID // optional NARROWING, not a scope
    Type            string
    Audience        string  // "store" | "customer" | "" (any)
    RecipientUserID string
    Read            *bool
    From, To        *time.Time
    Page, Limit     int
}

const MaxPlatformPageSize = 500
const DefaultPlatformPageSize = 50
```

The two constants mirror `ticket` and `audit` so every cross-tenant read on this surface
clamps alike.

### `internal/handlers/platformadmin/notifications.go`

Handler, narrow `NotificationLister` interface, and `Register` mounting `GET
/admin/notifications` — copying `tickets.go` rather than inventing a fourth shape.
`Deps.Notifications` is nil-guarded: nil leaves the route unmounted, matching every other
optional client-backed route. This is a read, so no `Emitter` and no `EmitOperatorAction`.

### Migration `000102` — a cross-store index

Both existing indexes are store-scoped: `notif_store_unread_idx (store_id, is_read,
created_at DESC)` and `notif_store_recent_idx (store_id, created_at DESC)`. A cross-store
`ORDER BY created_at DESC` has nothing to use.

```sql
CREATE INDEX IF NOT EXISTS notif_created_at_idx ON notifications (created_at DESC);
```

`IF NOT EXISTS` is load-bearing, not decoration. This section tells the operator to check
the table's size before running the migration — which invites pre-creating the index by
hand, or `CONCURRENTLY`, to avoid the `ACCESS EXCLUSIVE` window. A bare `CREATE INDEX`
would then error, golang-migrate would mark the version dirty, and `AssertVersion` would
refuse startup and crashloop every pod: the #287 failure this spec cites, triggered by
following this spec's own advice. Migration `000101` already uses the guard.

Same reason #276 added `idx_audit_logs_created_at`. No claim is made here about the lock
window at production scale — the table is written by ~20 call sites per store and its
current size should be checked before the migration runs, not assumed from the foundation
spec's `audit_logs` measurements, which were taken on a different table.

The down migration drops the index.

### `ExpectedSchemaVersion` 101 → 102

`migrations.go` in the **root package**. `AssertVersion` (`pkg/migrate/migrate.go:110-122`)
requires **exact equality** — a mismatch refuses startup and crashloops every pod on
rollout. This is what #287's `0015` hit in platform-api.

`TestExpectedSchemaVersionMatchesHighestMigration` guards the bump, and it lives in the
root package, so **any path-scoped command (`go test ./internal/...`) excludes it.** The
verification set must run `go test ./...` from the service root.

## Testing

- **Cross-tenant is the point.** Seed notifications under two stores in two different
  tenants and assert both appear in one unfiltered response. A single-store fixture cannot
  distinguish this endpoint from the store-scoped one it replaces.
- **`tenant_id` and `store_id` narrow rather than scope** — both directions asserted: with
  the filter, only those rows; without it, all of them.
- **`audience` is tested on both values, with both kinds of row seeded** — a customer row
  (`recipient_user_id` set) and a store row (NULL). Seeding only one kind makes the filter
  untestable in the direction that matters; that is the enum case from trap 6's last row.
- **`read` is tested with both a read and an unread row seeded**, same reasoning.
- **The projection excludes what it must, asserted on raw JSON** — no `message` key, no
  `status` key. Asserted on the bytes, not on an unmarshalled struct, which cannot
  distinguish an absent key from an empty one.
- **`from`/`to` beats `since_hours`** when both are sent, with the fixture placed where the
  two candidate implementations disagree — not merely "historical". An offset that looks
  old can still sit inside the window being measured.
- **Fail-safe preserved:** `ListByStore` still returns nothing for a zero `StoreID`. If a
  future change makes zero mean "all stores", that test fails. That is its purpose.
- **Golden fixture** `testdata/notifications_response.json`, proved by mutation to catch a
  field **rename** and a field **addition**. A fixture that only catches omissions is
  theatre.
- **Empty is `200` + `[]`**, allocated `make([]notificationRow, 0, n)` — a nil slice
  marshals to `null` and defeats the caller's `?? []`.
- Integration tests: `-p 1`, the LAN IP DSN (never `localhost`), `//go:build integration`,
  external test package (`package platformadmin_test`), matching every existing test in that package (`internal/notification`'s own tests are internal — follow the package you are writing in, not a service-wide rule).

### Verification set

- `go build ./...`
- `go vet ./...`
- `go vet -tags=integration ./...` — the only command that compiles build-tagged files
- `go test -count=1 ./...` **from the service root** — path-scoped runs exclude the root
  package, and with it the schema-version guard
- Integration runs confirmed from **verbose output that the tests ran**; `--- SKIP` and
  `--- PASS` are one character apart, and `TEST_DATABASE_URL` is the variable this repo
  sets

## Rollout

Migration `000102`, the version bump, the repository method, the handler and the route
mount ship in one branch. `AssertVersion`'s exact-equality check means the migration must
be applied at the moment the new binary starts — the same sequencing #287 used for
platform-api `0015`.

The surface stays inert without `MARKETPLACE_PLATFORM_ADMIN_SECRET`; that is already true
of every route on this group and needs nothing new here.

**Verification in production is limited by the data, and the report must say so.**
Data-independent: the route is mounted, an unsigned request gets `401`, a bogus path under
the same prefix gets `404` (the second is what makes the first mean anything), and the
clamp and default behaviours. Data-dependent and **not** provable against the four live
demo tenants: whether the cross-tenant fan-out actually spans tenants. An empty `200` is
not a passing integration check.

## Consequences

- The console's Governance → **Notification log** entry gets a real data source, answering
  "what was this store told, and when" — and explicitly **not** "did the email arrive".
- **#348** carries the email delivery log: a send record written by the shared transport, a
  signature-verified provider event webhook, and the `campaign_recipients` failure-recording
  fix. Until it lands, no surface in this estate can report a delivery outcome. Its scope
  is wider than first written — it must reconcile with the two existing partial records
  (`shipments.dispatched_email_sent_at` and `campaign_recipients`) rather than build beside
  them.
- `internal/campaign`'s unwritten recipient states join #322's list of declared-but-never-
  written enums.
