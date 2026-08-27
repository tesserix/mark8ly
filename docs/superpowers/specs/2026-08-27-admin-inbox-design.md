# `GET /admin/inbox` — design

Implements #280. Constrains #281.

## Why this endpoint exists

Five queues in mark8ly are waiting on a human, and none of them has an interface anywhere. The
strongest case is `sea_manual_review_queue`, whose own migration
(`000065_sea_manual_review_queue.up.sql`) states that any ID entering it **immediately pauses the
14-day validation clock on the associated subscription**, under a 5-business-day SLA. Nothing reads
that table. A queue that silently pauses billing and that nobody can see is the problem this endpoint
ends.

The console's front door must need no per-product knowledge, so all five kinds share one item shape.

## Sources

| kind | source | locality |
|---|---|---|
| `sea_manual_review` | `sea_manual_review_queue`, status `pending`/`in_review` | local |
| `migration_fast_path` | `internal/billing/migration`, `Review.Status = pending` | local |
| `erasure_request` | `customer_erasure_requests`, unprocessed | local |
| `arbitrage_appeal` | `internal/arbitrage`, `Resolution = ongoing` | local |
| `onboarding_stalled` | platform-api onboarding sessions, idle beyond threshold | **remote** |

`onboarding_stalled` is reached through the existing `OnboardingFunnel` client
(`internal/handlers/platformadmin/onboarding.go`), which already exposes `ListSessions` and already
carries the fail-soft behaviour. This design does not add a second cross-service path.

## Architecture

New package `internal/inbox`, one provider per kind behind a narrow interface:

```go
type Provider interface {
	Kind() string
	List(ctx context.Context, f Filter) ([]Item, error)
	Count(ctx context.Context, f Filter) (int64, error)
}
```

Files: `models.go`, `aggregator.go`, and one per provider — `sea_review.go`,
`migration_fastpath.go`, `erasure.go`, `arbitrage.go`, `onboarding.go`. The handler at
`internal/handlers/platformadmin/inbox.go` parses, calls the aggregator, and renders the house
envelope; it holds no per-kind knowledge.

Adding a sixth kind is one file plus one registration, with no aggregator change. That property is
the reason for the interface, and it should survive review.

## Item shape

```json
{
  "id": "uuid",
  "kind": "sea_manual_review",
  "title": "Acme Pte Ltd",
  "subtitle": "MY tax ID pending review",
  "waiting_since": "2026-08-12T09:31:00Z",
  "due_at": "2026-08-19T09:31:00Z",
  "severity": "normal",
  "href": "...",
  "actions": [{ "id": "approve", "label": "Approve", "destructive": false }]
}
```

`due_at` comes from `sla_due_at` for `sea_manual_review`, and is absent elsewhere.

### `severity` is derived, never stored

- `critical` — past `due_at`
- `warning` — within 24h of `due_at`
- `normal` — otherwise, or no `due_at`

**Erasure requests deliberately get no derived `due_at`.** GDPR's 30-day window is real and an
unprocessed erasure request is exactly #259's complaint, but `customer_erasure_requests` has no due
column, and deriving a statutory deadline inside a read endpoint invents policy in the wrong place.
Track it separately; do not smuggle it in here.

### `actions` are derived from item state, not from capability

#280's acceptance asks that `actions` list "only what a platform operator may actually invoke", which
reads as capability filtering. **That is not expressible today.**
`internal/handlers/platformadmin/middleware.go:36-51` sets `CapabilityValueChecked = false` and
explains why: the highest-privilege capability "is not expressible until the console's capability
vocabulary is settled — the same blocker as #333, and the reason #287 declined to invent capability
names."

So actions are derived from the item's own status: a `pending` SEA review declares `approve` and
`reject`; a resolved one is absent from the list entirely. Each action carries `destructive: bool`,
which #281 keys its idempotency requirement off.

Following #287's precedent, this design invents no capability names. The derivation site carries a
comment pointing at `CapabilityValueChecked`, which the middleware describes as "a SWITCH, not a
marker" — filling in `RequiredWriteCapabilities` and flipping it turns value enforcement on with no
other edit. This keeps the vocabulary blocker costing one issue (#333) rather than two.

## Ordering and pagination

Order by `due_at` ascending with nulls last, then `waiting_since` ascending: overdue first, then
longest-waiting.

Aggregation fans out, merges in memory, sorts, and slices. To serve page *N* at limit *L* the
aggregator needs the first *N×L* merged items, so cost grows with depth. Therefore:

- **Hard cap of 500 merged items in aggregate mode.** A request whose `page × limit` exceeds it
  returns `400` naming the cap and suggesting a filter. It does **not** return a silently truncated
  page — a truncated result that looks complete is worse than an error.
- **`?kind=<one kind>` delegates pagination to that provider**, which pages natively with real
  `LIMIT`/`OFFSET` and has no cap. Aggregate view is broad and shallow; single-kind view is deep.

Filters: `kind`, `tenant_id`, `status`.

`pagination.total` is the sum of each provider's `Count`.

Rejected: materialising an `inbox_items` table. It gives the best read shape and real pagination, but
introduces a sync problem across five writers, and the remote source cannot write into it. Revisit if
inbox volume ever stops being a human work queue — the provider interface does not preclude it.

## Failure semantics

Any single source failing is **not** a request failure. That kind is omitted, its name appears in a
`degraded` array on the envelope, and the response is `200`.

```json
{ "data": [...], "pagination": {...}, "degraded": ["onboarding_stalled"] }
```

If **every** source fails, return `500` — nothing useful can be rendered and that is a real outage.

This applies uniformly, not only to the remote source. A local database error hiding four healthy
queues is the same failure as a remote one, and a rule that depends on which source broke is harder
for the console to render than one that does not.

Silently omitting a failed source is rejected outright: an operator cannot distinguish "no stalled
onboardings" from "we could not ask", which reintroduces the invisible-queue problem this endpoint
exists to end.

## Testing

- **Per-provider integration tests** against the isolated-container pattern, using
  `testdb.SeedStore`/`SeedVendor`.
- **Aggregator tests with fake providers** — this is what the interface buys. Cover ordering with
  mixed `due_at` nulls, the page-cap boundary on both sides, single-kind delegation bypassing the
  cap, and each degraded permutation including all-fail.
- **Handler tests** for the envelope and that `degraded` surfaces.

The `sea_manual_review` provider carries the most weight. Its test must assert the real behaviour —
that a pending row with a breached SLA comes back `critical` and sorts overdue-first — not merely
that a row round-trips.

## Consequences for #281

- Its route becomes `POST /admin/inbox/{kind}/{id}/actions/{actionId}`. The kind is explicit, `id`
  stays opaque to the console, and no five-way lookup is needed to find the item again. This diverges
  from the path written in #281's body; changing it here is cheaper than discovering it there.
- It validates the requested action against the item's own declared `actions`, re-derived from the
  same provider. An action absent from that array is rejected even if mark8ly implements it
  elsewhere — that is what makes the array a contract rather than documentation.
- Idempotency keys are required on actions carrying `destructive: true`.
- **Part (b) of #281 is already done.** The CSM fast-path review route is mounted at
  `cmd/marketplace-api/main.go:2148` via `migration.Handler.RegisterInternalRoutes`, on both engines
  so they cannot drift. Only part (a) remains.

## Out of scope

Capability filtering of `actions` (blocked, see above). A derived GDPR due date for erasure requests.
A materialised inbox table. Any console-side rendering.
