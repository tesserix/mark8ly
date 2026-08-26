# Outbox failure state — design (#336, then #331)

Date: 2026-08-26
Issues: **#336** (publisher marks dropped events as published; `outbox_events.error` is never
written) and **#331** (`GET /admin/outbox`). Milestone: Platform integration v1.
Also touches **#289** (`/admin/health`), whose `errored` metric was dropped because it could not
have returned a non-zero value.

Two shippable pieces, in this order. #336 is a correctness fix with a live consequence; #331 is
the read that #336 unblocks.

---

## 1. What is true today, measured not assumed

Verified against production on 2026-08-26 (primary selected by role, per the handoff doc):

| measure | value |
|---|---|
| `outbox_events` total | 688 |
| `published_at IS NULL` (pending) | **0** |
| `error IS NOT NULL` | **0** |
| `payload->>'store_id' IS NULL` | **0** |
| distinct stores in payloads | 1 |
| `store_watermarks` rows | 1 |
| oldest / newest event | 2026-04-30 / 2026-07-30 |

**#336 has never fired.** No event has ever been dropped in production, and the one watermark row
is consistent with the one store that produced all 688 events. This is a latent defect being fixed
ahead of need — the same class as #360 and #361 — not incident response. It matters because it is
unrecoverable and silent when it does fire.

Confirmed in code at `origin/main`:

- `internal/outbox/publisher.go:94` appends `r.ID` to `ids` **before** both `continue` drop paths
  (unparseable payload, `payload.store_id` empty), and `publisher.go:144` then calls
  `MarkPublishedInTx(tx, ids)`. A dropped event is recorded as a successful publish.
- `outbox_events.error` is declared (`migrations/000001_products_initial.up.sql:257`), present on
  the model (`internal/outbox/models.go:26`), selected by the poll (`repository.go:44`), and
  **written by nothing**. The only write to the table is
  `UPDATE outbox_events SET published_at = now()` (`repository.go:65`).
- The publisher runs on a 2s ticker in `admin` and `both` modes (`cmd/marketplace-api/main.go:1548-1560`).

Both drop causes are **deterministic properties of the row**: a payload that will not
`json.Unmarshal` never will, and a payload without `store_id` never grows one. Nothing about
retrying either case can succeed. Failures of the watermark upsert are a different thing — they
return an error, roll the whole transaction back, and are retried on the next tick. That stays
unchanged.

---

## 2. The state model

Three states, all **derived from existing columns**. No migration.

| state | predicate | meaning |
|---|---|---|
| `pending` | `published_at IS NULL AND error IS NULL` | waiting to be published |
| `failed` | `published_at IS NULL AND error IS NOT NULL` | **terminal** — the publisher gave up |
| `published` | `published_at IS NOT NULL` | watermark bumped |

`failed` is exactly the predicate #331 already specifies, so the endpoint's contract with the
console needs no renegotiation.

**`failed` is terminal by design.** A failed row is never retried. Requeueing is an operator
action — clearing `error` re-enters the row into the poll on the next tick, but re-entry is not
recovery. `store_watermarks` is upserted with `GREATEST` over the row's *original* `created_at`, so
a row that sat failed while later events published for the same store will publish without moving
the watermark — leaving consumers unaware and clearing the health alarm at the same time. Recovering
a stale row means enqueuing a fresh event, or bumping `created_at` alongside clearing `error`. This
is the deliberate answer to the "retry, dead-letter or alert" question #336 leaves open: with both
known causes deterministic, bounded retries would burn work to fail identically, and a
`attempts`/backoff column would be machinery for a transient-failure mode that does not exist here.
If a transient drop cause is ever introduced, that is the point to revisit this, not before.

### Why the poll must exclude failed rows

The poll is `WHERE published_at IS NULL ORDER BY tenant_id, created_at LIMIT ? FOR UPDATE SKIP
LOCKED`. Leaving a permanently-failing row at `published_at IS NULL` without narrowing the
predicate makes it a **poison pill**: it sorts at the head of that ordering and is re-selected every
2s forever. Enough of them consume the whole batch window and starve real events. Narrowing the
predicate to `AND error IS NULL` is what makes "terminal" true rather than aspirational.

---

## 3. #336 — the fix

### `internal/outbox/repository.go`

- Poll predicate becomes `WHERE published_at IS NULL AND error IS NULL`.
- New `MarkFailedInTx(tx *gorm.DB, failures []Failure) error`, a sibling of `MarkPublishedInTx`,
  writing `error` **inside the same transaction** as the watermark bumps and the publish marks.
  Either the batch's whole outcome commits or none of it does.
- With a closed vocabulary of two reasons, this is at most two
  `UPDATE outbox_events SET error = ? WHERE id IN ?` statements per tick.

### `internal/outbox/publisher.go`

- `ids` is built **only from rows that contributed a watermark bucket**. This inversion at
  `publisher.go:94` is the whole of #336 part 2.
- Dropped rows are collected as `(id, reason)` and passed to `MarkFailedInTx`.
- Existing `Warn` logging is kept. The log was never the problem; the absence of a durable record
  was.

### The failure reason is a closed vocabulary, never a raw error

```
ReasonPayloadUnparseable   = "payload_unparseable"
ReasonPayloadMissingStoreID = "payload_missing_store_id"
ReasonStoreNotFound        = "store_not_found"
```

`ReasonStoreNotFound` (#374) covers a third row shape: a `store_id` that is well-formed and present
but has no matching row in `stores`. Before this fix that row reached the watermark upsert and its
FK violation (`store_watermarks.store_id REFERENCES stores(id)`) aborted the whole transaction,
taking the batch's good rows and failure marks down with it. It is terminal for the same reason the
other two are — a missing store is a permanent property of the row, not a transient condition to
retry.

**`err.Error()` must not be persisted.** `encoding/json`'s unmarshal errors quote the offending
input, so storing the raw error would copy fragments of an arbitrary customer-data JSONB payload
into a column that #331 then serves cross-tenant to the console. That defeats the same reasoning
that keeps `payload` out of the #331 response, and it would do so through a field nobody would
think to audit.

The closed vocabulary is also what makes `error` safe to return on #331 at all. The two decisions
hold each other up.

### The package doc comment

`publisher.go:16-19` currently documents the present behaviour as a deliberate choice: *"Rows
without it are logged and marked published without a watermark bump — losing the signal is
preferable to blocking the publisher on a producer bug."*

The first half stays true — a bad row still never blocks the publisher. The second half becomes
false. The comment is rewritten to describe the terminal-failed state, not deleted; a reader who
finds only silence there will re-derive the old behaviour as intentional.

---

## 4. #289 `/admin/health` — the composition consequence

This is the part no task-scoped review would surface, and it is the reason the fix is not a
one-line change.

`health_checks.go:38-45` derives `pending` and `oldest_pending_age_seconds` from `published_at IS
NULL`, and `health.go:155` degrades the `outbox` dependency once the oldest pending row is
`OutboxPendingThreshold` (5m) old. **Left alone, the first terminally-failed row would put the
estate-wide health surface into a degraded state that never clears** — a false alarm shipped as a
bug fix.

### Query

Keep the outer `WHERE published_at IS NULL` — this preserves the partial-index reasoning the
existing comment spells out (`outbox_unpublished_idx`, migration `000001`, chosen so a
never-pruned table is not scanned on a shared db-f1-micro) — and split with `FILTER`:

```sql
SELECT
    COUNT(*) FILTER (WHERE error IS NULL)                          AS pending,
    COALESCE(EXTRACT(EPOCH FROM (
        ? - MIN(created_at) FILTER (WHERE error IS NULL)))::bigint, 0)
                                                                   AS oldest_pending_age_seconds,
    COUNT(*) FILTER (WHERE error IS NOT NULL)                      AS errored
FROM outbox_events
WHERE published_at IS NULL
```

### Contract

- `OutboxHealth` gains `Errored int64`.
- The doc comment at `health.go:62-66` — *"There is deliberately no `errored` metric. Nothing in
  this service ever writes `outbox_events.error` … such a metric could never return a non-zero
  value"* — is now false and is replaced with what makes it true, citing #336.
- Degrade condition becomes `errored > 0 || oldest_pending_age_seconds >= OutboxPendingThreshold`.
- `errored` is an **additive** key in the dependency's `metrics` map. Existing console consumers of
  #289 are unaffected.

**`errored > 0` degrades deliberately, and the alarm does not clear by draining** — only by an
operator resolving the row. That is the correct shape for a condition that requires a human, and it
is not a new pattern on this handler: `csv_import_jobs` already degrades on
`RunningStaleHeartbeat > 0` (`health.go:169`).

---

## 5. #331 — `GET /admin/outbox`

A **read**. It needs no entry in `RequiredWriteCapabilities` and no capability-value decision, so
the vocabulary question blocking #333 does not touch it.

Modelled field-for-field on `notifications.go` (#332), the nearest sibling — whose own doc comment
already cites #331's `payload` exclusion.

- **Query and filter live in `internal/outbox`** as `ListPlatform` + `PlatformListFilter`. The
  handler (`internal/handlers/platformadmin/outbox.go`) holds a one-method `OutboxLister`
  interface, matching `NotificationLister` / `TicketLister` / `EstateCounts`.
- **Row shape:** `id`, `tenant_id`, `aggregate`, `aggregate_id`, `event_type`, `status`,
  `created_at`, `age_seconds`, `published_at` (nullable), `error` (nullable).
- **`payload` is excluded by construction** — a field-by-field projection like `toNotificationRow`,
  not by which columns the query happens to select, so a column added to the model tomorrow cannot
  leak.
- **`status` is derived server-side in SQL**, per the issue's explicit requirement that the console
  not reimplement the null-check.
- **`age_seconds` is server-computed** from the same instant `older_than_minutes` filters on.
  Deriving it client-side lets the console display an age that disagrees with the filter that
  selected the row.
- **Filters:** `status` (`pending` / `failed` / `published`), `event_type`, `older_than_minutes`,
  `since_hours`, `limit` (clamped, never refused), `page`, and `tenant_id` to narrow. Unknown
  values narrow nothing rather than erroring — the established contract across this surface.
- **Empty is `200` with `[]`**, from a pre-allocated slice: a nil slice marshals to `null` and
  defeats a caller's `?? []` exactly when there is no data.
- **Default sort `created_at DESC`**, consistent with every sibling list on this surface;
  `older_than_minutes` is the tool for the "what is stuck" question.
- Standard envelope: `{ data, pagination { page, limit, total } }`, RFC3339 UTC timestamps, bare
  ids — the #260 conventions.

### `error` is opaque outside the service, not a three-way switch

`outbox_events.error` is `text` with no `CHECK` constraint (migration
`000001_products_initial.up.sql:256`). The closed vocabulary described in §3 is enforced in Go by
`sanitizeReason`, which only covers writes going through `MarkFailedInTx`. Both the production
verification exercise in §7 and the operator requeue path described in §2 are manual `UPDATE`s
against this column, and neither goes through `sanitizeReason`. So the two known reason codes are
what the *service* can produce, not what a *consumer* can observe. #331's console must treat `error`
as an opaque string with an unknown-value fallback, never a three-way switch.

### Known cost, accepted

`total` for `status=published` counts across a table that is never pruned, on a shared db-f1-micro.
At 688 rows this is immaterial, and it is the same shape every sibling endpoint already has. Worth
revisiting if the table reaches a size where the count dominates — not now.

---

## 6. Sequencing

**Two branches, two PRs, #336 first.** #336 is a correctness fix with a live consequence and should
not wait behind an endpoint. #331 then builds on a table whose state model is already true.

Considered and **rejected**: provoking one synthetic row after #336 and leaving it in place as a
fixture until #331 ships, so one row proves both. It would leave `/admin/health` degraded on the
estate surface for however many days separate the two PRs. Two short provoke-and-clean exercises
instead.

---

## 7. Verification

The takeover doc's standing lesson from #288: *a green test suite plus a mounted route is not
delivery evidence.* Production has zero malformed rows and will produce none on its own — the
outbox is idle — so this fix's behaviour cannot be observed by waiting. It must be provoked or it
stays unproven.

### Integration tests (real Postgres)

- **The mixed batch.** One good row and one malformed row in the *same* tick. Asserts the good row
  published **and** its watermark bumped, **and** the bad row errored **and** left unpublished.
  This is the Trap-15 case: #288's Criticals all lived in a composition no single test constructed,
  and here the composition is the entire point of the fix.
- **The poison-pill proof.** Tick twice; the errored row must not be re-selected on the second
  tick. Nothing else tests the narrowed poll predicate, which is the reason it exists.
- **Exact error codes asserted as strings**, never `error IS NOT NULL`. #358's lesson: a stub
  returns the zero value for a field nobody set.
- **The health split.** `pending` excludes errored rows, `oldest_pending_age_seconds` ignores them,
  `errored` counts them, and `errored > 0` degrades.
- **#331 status derivation** across all three states, and `payload` absent from every response.

### Production exercise (#336, after deploy)

With explicit go-ahead at the moment of the write, not merely by this document:

1. Insert **one** synthetic `outbox_events` row with an unparseable payload.
2. Observe the publisher set `error` to the expected code and leave `published_at` NULL.
3. Confirm it is **not** re-selected on subsequent ticks.
4. Confirm `/admin/health` reports `errored: 1`, `pending: 0`, and status `degraded`.
5. `DELETE` the row and confirm health returns to healthy.

Blast radius is one row we created ourselves in an idle queue, reversible by `DELETE`. Step 5 is
part of the evidence, not cleanup after it — recovery is the half that a provoke-only exercise
leaves unproven.

Repeated for #331 after it deploys, to see the row as `status=failed`.

### Discipline

- `internal/outbox` is recorded at **2 FAIL** on `origin/main`. Diff the failing set against a
  throwaway worktree of `origin/main` before calling anything pre-existing — the discipline the
  handoff doc demands, and the way to find out whether a third was added.
- Run with `-tags=integration` and `TEST_DATABASE_URL` set, from the service root, `-p 1`.
  `go test ./...` without the tag never compiles build-tagged files (Trap 8).
- `set -o pipefail` or `${PIPESTATUS[0]}` whenever an exit code is reported as evidence (Trap 8's
  corollary).
- Read back every `gh` write (Trap 16).

---

## 8. Found while reading, deliberately not in scope

**Done.** `metrics.OutboxEventsPending` and `metrics.OutboxEventsPublishedTotal` were declared and
registered but written by nothing — the same family of dead declaration as `outbox_events.error`
itself, and as #322 and #323. This was filed as its own issue, #375, and closed on this branch.

The gauge (`OutboxEventsPending`) was deleted rather than wired up: it is redundant with
`/admin/health`, and a per-replica gauge would be reported identically by every replica running in
`admin` or `both` mode, so any dashboard summing it across replicas would multiply the true value.
`outbox_events_published_total` is now written by the publisher, and `outbox_events_failed_total`
was added alongside it.

Also out of scope: producer-side validation that would stop a malformed row being enqueued at all.
Worth considering, but it is a different defence at a different layer, and it cannot help the rows
already in the table.
