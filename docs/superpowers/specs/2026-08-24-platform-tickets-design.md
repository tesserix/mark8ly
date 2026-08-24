# Design — `GET /admin/tickets` (#329)

**Status:** approved
**Issue:** #329 · **Umbrella:** #260 · **Reference endpoint:** #276 · **Date:** 2026-08-24

## Why this one is urgent

Every other remaining endpoint feeds a console surface that is visibly `pending`. This one feeds
`platform.tickets`, which is **already live and silently partial**: it renders the platform's own
tickets and shows **no mark8ly tickets at all**, because mark8ly's ticket surface is store-scoped
(`/admin/stores/:storeId/tickets`) and the cross-store fan-out was never rebuilt.

An operator working that queue sees a complete-looking list that is missing every merchant ticket.
A surface that is visibly pending is honest; one that is live and quietly incomplete is worse.

## Three findings that shape the build

### 1. There is no `assignee`

#329 asks for an `assignee` filter. Searching `internal/ticket/` and the ticket migrations finds no
`assignee`, `assigned_to` or owner column anywhere. Tickets have a **submitter**
(`submitted_by_name` / `submitted_by_email`), not an assignee.

**Ruling: omit the filter and report it on the issue.** This is the third time in this series an issue
has named a field that does not exist — #277 asked to match on a tenant slug that tenants do not have,
#276 named a `metadata` shape that was wrong. The umbrella's own guidance is to check the model before
implementing a field name from the issue text. Inventing ticket assignment to satisfy a filter would
add a product concept the merchant UI has no way to set.

### 2. There are two ticket tables, and picking the wrong one is easy

`Ticket.TableName()` is **`support_tickets`**, and the model carries an explicit warning:

> The bare `tickets` table in the same schema belongs to a different platform-engineering ticket
> system and is intentionally NOT touched here.

That other table is almost certainly what the console reads today, which is exactly why its queue
looks complete while containing no merchant tickets. **Query through the `ticket` package's model so
`TableName()` decides**, never a hand-written table name.

### 3. The existing `List` is fail-safe, and must stay that way

`internal/ticket/repository.go:76` hardcodes `WHERE store_id = ? AND tenant_id = ?`. With zero UUIDs
it matches **nothing**.

**Ruling: add a new, explicitly cross-store repository method. Do NOT make `ListFilter`'s scope
optional.** Making a zero `StoreID` mean "all stores" would invert a fail-safe into fail-open on the
merchant-facing path — one forgotten field away from a merchant seeing another store's tickets. The
current behaviour (empty result) is the safe failure, and it is worth keeping precisely because the
same filter type is used by store-scoped callers.

## Contract

```
GET /api/v1/platform/admin/tickets
```

A read: HMAC signature required; no operator identity or capability, per the enforcement matrix.

**Filters** — each maps to a real column:

| param | behaviour |
|---|---|
| `status` | exact match on `status` |
| `priority` | exact match on `priority` |
| `store_id` | optional **narrowing**, not a required scope |
| `since_hours` | `created_at >= now - N hours` |
| `from` / `to` | explicit range; wins over `since_hours` when both are supplied, matching #276 |
| `limit` | default and clamp as #276 (oversized clamps, missing takes the default, never errors) |
| `page` | 1-based |

Unknown query parameters are ignored, as elsewhere on this surface.

**Row projection** — enough to triage without a second call:

```json
{
  "id": "…", "ticket_number": "T-1042",
  "tenant_id": "…", "store_id": "…",
  "subject": "…",
  "status": "open", "priority": "high",
  "requester_name": "…", "requester_email": "…",
  "conversation_id": "…",
  "created_at": "…", "updated_at": "…", "resolved_at": null
}
```

`updated_at` is the "last activity" the issue asks for.

### Deliberately absent

- **`description`** — customer-written free text. #331 refuses to return `payload` and #332 refuses
  message bodies for the same reason: a cross-tenant governance surface must not become a way to read
  every merchant's customer correspondence. Subject, status and requester answer the triage question.
  A body view, if ever wanted, needs its own endpoint, its own capability and its own justification.
- **`replies`** — same argument, more so; the association is not preloaded on this path.
- **`assignee`** — no such column (see finding 1).

**Project, do not pass through.** Map `ticket.Ticket` field by field into a row type, so a column added
to the model tomorrow cannot leak to the console automatically. `Description` and `Replies` being
absent must be a property of the projection, not of what the query happened to select.

## Conventions inherited

- Envelope exactly `{"data": [...], "pagination": {"page","limit","total"}}`
- Empty is `200` + `[]`, allocated `make([]ticketRow, 0, n)` — a nil slice marshals to `null`
- `pagination.limit` reports the **effective** (clamped) limit
- Timestamps RFC3339 UTC with offset; ids **bare**
- Golden fixture, proved by mutation to catch a field **rename** and a field **addition**

## Testing

- **Cross-store is the point:** seed tickets under two different stores in two different tenants and
  assert both appear in one unfiltered response. A single-store fixture cannot distinguish this
  endpoint from the store-scoped one it replaces.
- **`store_id` narrows rather than scopes:** with the filter set, only that store's rows return; without
  it, all do. Both directions asserted.
- **The projection excludes what it must:** assert on the raw JSON that no `description` or `replies`
  key appears — not on an unmarshalled struct, which cannot distinguish an absent key from an empty one.
- **Fail-safe preserved:** a test that the existing store-scoped `List` still returns nothing for a zero
  `StoreID`. If a future change makes zero mean "all", that test fails — which is the point.
- Integration tests use `-p 1`, the LAN IP DSN, and `go vet -tags=integration ./...` is part of
  verification (build-tagged files are invisible to the default toolchain).

## Out of scope

- **Replying to a ticket from the console** is a write needing operator attribution; #329 says it belongs
  with #281's inbox action work.
- **The Otto transcript** behind `conversation_id` — #330 is the open decision about how the console
  reaches Otto. The id ships as an identifier; resolving it does not.
