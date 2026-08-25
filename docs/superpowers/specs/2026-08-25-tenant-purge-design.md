# Design — `POST /admin/tenants/{id}/purge` (#288)

**Status:** approved
**Issue:** #288 · **Depends on:** #275, #277, #287 · **Umbrella:** #260 · **Date:** 2026-08-25

The irreversible one, and last by design. Everything else in this milestone can be rolled
back by reverting a deploy. This cannot.

## Three of the issue's premises do not hold

Checked against the schema and against production before designing anything, per the rule
this milestone arrived at the hard way.

### 1. An entry point already exists

#288 says `tenantpurge/purge.go` "exists with no operator-facing entry point." There is an
entry point: `POST /internal/tenants/:tenantID/purge`
(`internal/handlers/internalsvc/tenant_purge.go`), gated by `X-Internal-Auth`, mounted on
both the `mode.Both` and `mode.Admin` engines, and driven by platform-api's outbox drainer
(`account.TenantDeletedOutboxKind`, registered at platform-api `cmd/server/main.go:328`).

It is not *operator*-facing — no identity, no capability, no confirmation, no audit — so the
issue's intent survives. What changes is that this is not building a trigger from nothing;
it is adding a governed path beside an existing ungoverned one, and the two must not
disagree about what a purge is.

### 2. `tenantpurge.Purge` is only the marketplace half

There is a complete tenant-deletion flow, live in production:

```
merchant (owner) → platform-api account teardown
  └─ ONE tx: DELETE tenants (stores + invitations CASCADE) + enqueue tenant.deleted
  └─ post-commit best-effort: owner FGA tuple, store parents, owner GIP identity
outbox drainer → VendorClient.PurgeTenant → marketplace-api /internal/…/purge
  └─ tenantpurge.Purge
```

An operator endpoint that called `tenantpurge.Purge` alone would destroy every product,
order, customer and audit row while leaving the tenant row, its platform-api stores, its FGA
tuples and its GIP identities alive. The console would still list the tenant and the merchant
could still log in. A half-purged tenant is worse than either outcome.

**Ruling: the operator endpoint drives the whole teardown**, through a new operator-initiated
entry point on platform-api that reuses the proven transactional path.

The existing teardown cannot be called as-is: `Service.DeleteAccount` branches on the
*actor's* FGA role for the tenant and requires `RoleOwner`. A platform operator holds no FGA
role, so an operator variant is required regardless of how this is sequenced.

### 3. `tenants.slug` does not exist

#288's confirmation semantics name "slug, and expected store count."

**There is no tenant slug.** Migration `0008_create_stores` dropped it at Phase Q
(`ALTER TABLE tenants DROP COLUMN IF EXISTS slug`, line 78), when a tenant stopped being a
store. Verified against production on 2026-08-25 rather than read from the migration:
`tenants` is `id, name, owner_user_id, owner_email, status, created_at, updated_at`.

Naming what came closest to disconfirming that, because "it does not exist" is not a
checkable sentence: **`stores.slug` exists** and is what `{slug}.mark8ly.com` resolves — a
*store* slug, carried on `tenantdirectory.StoreSummary` and emitted by #277's detail route.
And `tenants.slug` did exist before `0008`, with the same format CHECK, which is why the
issue could name it in good faith.

This is the **fifth** instance in this series of an issue naming an absent field, after
#277's tenant slug, #276's `metadata` shape, #329's assignee and #286's row id.

**`store_count` exists but cannot discriminate.** All four production tenants have exactly one
store (measured 2026-08-25). A confirmation value that is `1` for every tenant in the estate
cannot detect a stale tab pointed at the wrong tenant — which is the entire stated purpose of
the check. Trap 13 arriving in the design rather than in a test.

**Ruling: the confirmation value is the set of store slugs.** Unique across
**platform-api**, whose `stores.slug` carries `stores_slug_key UNIQUE (slug)` (measured, not
read — and named per service, because marketplace-api's local projection spells its own
constraint `stores_slug_unique`; the authoritative one for this check is upstream). One slug
per store, and the set changes when a store is added, removed or renamed — so it subsumes `store_count` with real discriminating power. The console already
holds it from #277's detail row.

## The capability gate cannot be built yet

AC#3 asks that the endpoint "requires the highest-privilege capability the gateway can
assert." This surface checks capability **presence only, never the value**
(`internal/handlers/platformadmin/middleware.go:147-158`, verified). #287 deliberately
declined to invent capability names because the console is the authority and a guess refuses
every real request. That is the same decision blocking #333.

**Ruling: ship with the value gate explicitly deferred.** Presence is required, the value is
recorded on the audit row, and both the spec and a code comment say the value is not checked
and why. A single named hook holds the deferral so that settling #333's vocabulary switches
the gate on for `suspend`, `unsuspend`, `extend` and `purge` in one place rather than four.

This is a weaker guarantee than the AC asks for on the one endpoint where it matters most.
Recorded here rather than glossed, so nobody later reads a green checkbox as a gate.

## Contract

### Preview — non-destructive

```
GET /api/v1/platform/admin/tenants/{id}/purge/preview
```

A **read** under the foundation's enforcement matrix: signature required, operator and
capability optional. It exposes nothing an operator cannot already assemble from #277 and
#276, and special-casing one route against the matrix is the inconsistency this surface has
repeatedly been punished for.

```json
{ "data": {
    "tenant_id": "…", "tenant_name": "The Bondi Store", "status": "active",
    "store_slugs": ["the-bondi-store"],
    "tables": [ {"table": "orders", "rows": 412}, {"table": "products", "rows": 78} ],
    "total_rows": 1904 } }
```

`tables` lists **every table in the purge plan**, including those with zero rows — an
omitted zero and an unenumerated table are indistinguishable to a reader, and the whole point
of this route is to show what the plan reaches. The purge response's `tables` follows the same
rule, for the same reason.

### Purge — irreversible

```
POST /api/v1/platform/admin/tenants/{id}/purge

{ "store_slugs": ["the-bondi-store"],
  "reason_code": "erasure_request",
  "reason": "GDPR art.17 request, ticket #4412" }
```

A **write**: signature *and* operator *and* capability, per the enforcement matrix.

`{id}` is the tenant UUID. That noun does exist.

#### Reason codes

Matching #287 and #286: a required closed code plus optional free text, both stored on the
audit row. Deliberately a different set again — the reasons for destroying a tenant are not
the reasons for suspending one.

```go
// PurgeReasonCodes is the closed set of reasons a tenant may be purged for.
var PurgeReasonCodes = []string{
    "merchant_request", // the merchant asked for their account and data to be deleted
    "erasure_request",  // a statutory erasure demand (GDPR art.17) — see #259's clock
    "fraud",            // confirmed fraudulent tenant, removed after investigation
    "abandoned",        // onboarding never completed; a dormant tenant reclaimed
    "legal",            // a legal or regulatory demand other than erasure
    "operator_error",   // a tenant created in error, or a test tenant
}
```

`merchant_request` and `erasure_request` are kept distinct because only the second carries a
statutory clock, and an audit trail that cannot tell them apart cannot answer the question a
regulator asks.

`reason` is optional free text, trimmed, capped at 500 characters. **Capped by runes, not
bytes** — #286's review found a byte truncation producing invalid UTF-8, which Postgres
rejects on the jsonb write, which fails the audit emit. On this endpoint that would mean an
irreversible destruction recorded nowhere.

#### Refusals

| condition | status | `error` |
|---|---|---|
| supplied slug set differs from actual, in either direction | 409 | `confirmation_mismatch` |
| tenant not found upstream (including already purged) | 404 | `tenant_not_found` |
| `reason_code` absent or not in the set | 400 | `invalid_reason_code` |
| `store_slugs` absent | 400 | `invalid_request` |
| body absent or unparseable | 400 | `invalid_request` |
| `id` not a UUID | 400 | `invalid_tenant_id` |
| platform-api unreachable | 503 | `upstream_unavailable` |

`confirmation_mismatch` returns the **actual** set as `expected`, so the console can refresh
without a second round trip. That is safe to disclose: the operator is already authenticated
and already holds the tenant's detail row.

An **absent** `store_slugs` is `invalid_request`, never an implicit "no stores" match — a
client that drops the field must fail, not purge. An **empty array** is a different thing and
is legal: it asserts "this tenant has no stores", and it succeeds only against a tenant that
actually has none. The two are distinguished by presence, not by length. The field is decoded as a pointer
(`*[]string`) to state that in the type — **not** because a plain slice would collapse them.
Corrected after measuring: `encoding/json` already leaves a plain `[]string` nil for `{}` and
allocates a non-nil empty slice for `[]`, so a plain field distinguishes absent from empty on
its own. The pointer's real value is that it depends only on whether the key was present,
rather than on a JSON library's nil-vs-empty convention for slices.

Set comparison is order-insensitive and exact in both directions — a supplied subset is a
mismatch, not a partial match.

`upstream_unavailable` must never be conflated with "nothing to do" — the failure mode
`tenantlifecycle`'s own package doc was written to guard against.

#### Response

```json
{ "data": {
    "tenant_id": "…", "tenant_name": "The Bondi Store",
    "store_ids": ["…"], "store_slugs": ["the-bondi-store"],
    "reason_code": "erasure_request", "reason": "…",
    "tables": [ {"table": "orders", "rows": 412} ],
    "total_rows": 1904,
    "purged_at": "2026-08-25T09:00:00Z" } }
```

Timestamps RFC3339 UTC with offset; ids bare; no `source` field.

## What it does

```
POST /admin/tenants/{id}/purge
  ├─ validate: uuid, reason_code ∈ set, store_slugs present
  ├─ platform-api: POST /internal/tenants/{id}/teardown  {store_slugs}
  │     └─ ONE tx: compare slug set → DELETE tenants WHERE id (RowsAffected==1)
  │                → enqueue tenant.deleted
  │     └─ post-commit best-effort: owner FGA tuple, store parents, owner GIP
  │     └─ returns: tenant name, store ids, store slugs
  ├─ tenantpurge.Purge(tenantID, storeIDs) — inline, synchronous → per-table counts
  ├─ TenantGateInvalidator.Invalidate(tenantID)
  └─ audit row, written SYNCHRONOUSLY, after the purge transaction commits
```

### The confirmation check runs upstream, inside the teardown transaction

Comparing slugs in marketplace-api and then asking platform-api to delete is the same stale
read the check exists to prevent, with a shorter window. The comparison and the `DELETE FROM
tenants` are one transaction or the check is theatre.

This is why the new upstream endpoint takes `store_slugs` rather than marketplace-api
validating against `tenantdirectory.Get` and then calling a plain teardown.

### The outbox is the backstop, not the path

`teardownTenantTx` deletes the tenant row and enqueues `tenant.deleted` in one transaction,
so once it commits the marketplace purge is guaranteed to happen eventually whatever this
request does next. The inline `tenantpurge.Purge` exists so the operator gets a real
destruction report in the response rather than a `202` and a promise. If it fails or the pod
dies mid-request, the drainer retries it and `Purge` is idempotent, so nothing is destroyed
twice and nothing is left behind.

A purge that finds zero rows because the drainer got there first is reported honestly as
zero rows, not as a failure.

### No `Idempotency-Key`

#286 required one. This endpoint does not, and the reason is that a stronger guarantee is
already present: `DeleteInTx` runs `DELETE FROM tenants WHERE id = ?` inside a transaction
and fails on `RowsAffected == 0` (`internal/tenant/repository.go:214-218`). Two concurrent
purges of the same tenant therefore have exactly one winner at the database, and the loser
gets `404`. Layering #286's reserve-then-work machinery on top would add a weaker guarantee
over a stronger one, plus a new failure mode — #286's own fix wave shipped a Critical where a
reserved key was never released.

A replay after a successful purge answers `404 tenant_not_found`. That is the honest answer:
the tenant is gone. AC#4's "no-op, not a partial re-run" is satisfied where it already lives,
in `Purge`'s `WHERE` clauses, and is what protects the drainer's retry.

### The audit row would otherwise be destroyed by the thing it records

`purgePlan` contains `DELETE FROM audit_logs WHERE tenant_id = ?`. `Emitter.Emit` is
asynchronous, buffered, and drops events with a warning when the queue is full
(`internal/audit/emitter.go:46, 85, 98`). So an `EmitOperatorAction` on this path races its
own DELETE: the row may land before the purge and be destroyed, after it and survive, or
never be written at all. AC#2 is not satisfiable through the existing helper.

**Add `func (e *Emitter) EmitSync(c *gin.Context, ev Event) error`** — the existing
unexported `buildEntry` followed by `repo.Create` on the caller's goroutine. Reusing
`buildEntry` rather than assembling an `Entry` a second time is deliberate: a second
derivation of actor type, operator id, capability and scope is precisely the shape trap 7
keeps catching, and it would drift the moment either path gained a field.

Called **after** the purge transaction commits, so the insert follows the DELETE that would
have removed it. A failure to audit is returned and surfaced, never logged and swallowed — an
irreversible destruction recorded nowhere is the exact gap this series exists to close.

`store_id` is null: a purge is tenant-scoped and has no store. That is what #274's migration
made possible.

The row carries the operator, the capability, the confirmed slug set, the reason code and
free text, and the per-table row counts.

### Cache invalidation

`TenantGateInvalidator.Invalidate(tenantID)` after the purge, for the same reason #287 added
it: without it the admin gate serves a cached status for up to its TTL for a tenant that no
longer exists. Best-effort and nil-safe, matching #287 — its absence is degraded lag, not a
failure worth turning a completed destruction into an error response.

## Changes to existing code

### `tenantpurge.Purge` returns a report

Today it discards `RowsAffected` on every step. It returns `Report{Tables []TableResult}`
instead. Both existing call sites in `cmd/marketplace-api/main.go` (the `mode.Both` and
`mode.Admin` internal mounts) adapt. This is what makes AC#2's "what was destroyed" a
measurement rather than a claim.

### One plan, two verbs

`purgePlan` gains a sibling that emits `SELECT count(*)` over the identical table list and
`WHERE` clauses. Preview and purge derive from one enumeration, so they cannot drift into
disagreeing about which tables a purge reaches — which is the failure the second enumeration
below already demonstrates.

### The table list is correct, and one of its stated reasons is not

`purgePlan`'s coverage was verified against **production**, not against the migrations it
cites: 97 base tables, the FK graph read from `pg_constraint`, and cascade closure computed
from it. The 53 explicit steps plus 29 tables reached by cascade cover every tenant-scoped
table except exactly the four documented exclusions. Nothing it names is missing. That list
is sound.

Its **rationale** is not. The package doc and `purge_test.go`'s `protectedTables` both assert
that deleting `business_entity_attestations`, `app_contract_attestations`,
`subscription_plan_change_audit` or `billing_archive` "would error (DB role has DELETE
revoked)."

Measured: the service connects as `marketplace_api` (from the `mark8ly-postgres-marketplace-api`
secret), and that role **owns all four tables with full `arwdDxt`**. `REVOKE … FROM PUBLIC`
never applied to the owner. Only `break_glass_lockouts` genuinely errors, because it is owned
by `postgres` — and that one comment is correct.

The exclusion decision stands unchanged; the reason is replaced with the true one. It matters
because the false version says the database is a second line of defence, and it is not: the
7-year retention record is protected by a Go slice and a unit test, and by nothing else. Two
comments agreeing was, as usual, one comment copied. Trap 12.

### Capability deferral hook

A single named constant or predicate holding "capability values are recorded, not checked",
referenced by all four write endpoints, so #333's decision lands in one place.

## The new upstream endpoint

`POST /internal/tenants/{id}/teardown` on platform-api's `strictInternal` group
(`cmd/server/main.go:353`), mirroring #287's `/internal/tenants/:id/suspend`
(`internal/tenant/handler.go:292`).

Body `{store_slugs: []string}`. Inside one transaction: read the tenant's current store
slugs, compare as a set, and either return a mismatch sentinel carrying the actual set or
proceed into the existing `teardownTenantTx`. Response carries the tenant name, store ids and
store slugs — marketplace-api needs the ids to run `Purge` and echoes the rest back to the
operator.

Post-commit best-effort cleanup mirrors `deleteOwnerAccount`, with the owner taken from
`tenants.owner_user_id` read before the delete rather than from an actor that does not exist
on this path.

## Testing

The organising rule stays: a test must fail if the property it names is deleted. And the
narrower one this milestone paid a Critical for — **a test for a property that discriminates
between two values must contain both values.**

- **Slug confirmation needs two tenants.** A fixture with tenant A and tenant B, each with
  distinct store slugs, and a purge of B supplied with A's slugs. One tenant cannot prove the
  check does anything at all. Then the three drift cases against the same tenant — a store
  added, a store removed, a store renamed — each `409`, with the matching set succeeding in
  the same fixture so the test cannot pass by the check always firing.
- **Two stores, not one.** A tenant with two stores, purged with only one slug supplied, must
  `409`. A set comparison implemented as "every supplied slug exists" passes a one-store
  fixture and silently accepts a subset.
- **Concurrency.** Two simultaneous purges of the same tenant against real Postgres: exactly
  one `200`, exactly one `404`, and exactly one audit row. One request cannot prove a mutex.
- **The audit row survives its own purge.** Assert in one test that the purge's row exists
  *after* the purge with that tenant's id, **and** that the tenant's pre-existing audit rows
  are gone. Only the second half proves the first is not passing on an emitter that wrote
  before the DELETE.
- **Audit failure is surfaced.** Force `Create` to fail and assert the response reflects it.
- **Row counts are values, not presence.** Seed a distinct non-zero row count per table and
  assert the numbers. A report assembled by map lookup returns a fabricated `0` for a missing
  key, and a test asserting the key exists passes on it.
- **Preview and purge enumerate the same tables.** Asserted by comparing the two plans
  directly, so a table added to one and not the other fails loudly.
- **Preview destroys nothing.** Count rows before and after a preview against a seeded tenant.
- **The purge plan still covers the schema.** A test that fails when a new tenant-scoped table
  exists that the plan neither deletes nor reaches by cascade nor names as an exclusion. The
  plan is correct today; nothing currently forces it to stay correct, and this endpoint makes
  that gap operator-triggerable. This is the guard the existing hand-maintained list has never
  had.
- **Attribution.** `actor_operator_id`, `capability`, `actor_type = operator`, the tenant, the
  reason code, the slug set, the counts.
- **Golden fixture** for both routes, proved by mutation to catch a field rename *and* a field
  addition.
- Integration: `//go:build integration`, `-p 1`, LAN IP DSN, `TEST_DATABASE_URL`. Seed stores
  through the existing helpers — a raw `INSERT INTO stores` omits
  `storefront_customer_portal_secret`, which migration `000058` declares `CHAR(64) NOT NULL`
  with its DEFAULT dropped.
- Verification set: `go vet -tags=integration ./...` and a root-inclusive `go test ./...` from
  each service root.

**Pre-existing failures to scope around, not fix:** `internal/billing/trial`'s 19 silently
skipping tests (#317), `internal/subscription/planchange` integration (9 FAIL), and
`internal/whitelabel`'s nil-pointer panic. Each confirmed at `origin/main`.

## Rollout and what production can prove

No migration in either service. `ExpectedSchemaVersion` does not move.

`GET …/purge/preview` is the answer to "there is no scratch tenant, so purging to verify is
not available." It runs the real plan against real production data and destroys nothing — the
first check in this milestone that exercises a handler's body against production instead of
measuring a refusal.

**Data-independent** — these prove the code is mounted and correct regardless of what data
exists: both new paths move `404` → `401` while a bogus sibling under the same prefix stays
`404` and an already-live route stays `401` (exactly one of three moves, per route); the body
says `unauthenticated`, not `not_configured`; signature, timestamp and nonce refusals;
operator and capability refusals on the POST; UUID and reason-code validation.

**Genuinely exercised against real data** — the preview's table enumeration and row counts
across four real tenants. This is the part that has been missing all milestone.

**Not provable in production, and the report must say so** — the purge itself, the
confirmation mismatch, the concurrency mutex, the audit-survives-its-own-purge property, and
the cache invalidation. None can run against a live merchant, and there is no scratch tenant.

An empty `200` is not a passing integration check, and neither is a `401` from a route whose
body has never run.

## Out of scope, filed separately

- **`subscription/harddelete/sweeper.go` is broken, not merely drifted.** It sweeps nine
  tables by a `store_id` column those tables do not have — `review_reactions`,
  `review_replies`, `review_media`, `loyalty_transactions`, `campaign_recipients`,
  `gift_card_transactions`, `coupon_usage`, `product_categories`, `ticket_replies` (verified
  against production `information_schema`). It aborts on the fourth sweep every time, so the
  150-day hard-delete pipeline cannot complete. It is the second enumeration this issue was
  told to check against the first; it does not agree with the first because it does not agree
  with the database. Its own issue.
- **Orphaned FGA tuples and staff GIP identities.** `authz.Client` has no method enumerating a
  tenant's members — `DeleteTuple` requires a `userID` — so teardown removes the owner's tuple
  and the store-parent tuples and leaves staff/admin/viewer tuples pointing at a tenant object
  that no longer exists, plus their GIP identities. The existing merchant-initiated owner path
  has the identical gap. Inherited, not introduced; named and filed rather than silently
  carried into a governance endpoint.
- **Gating on the capability value** — #333's decision, deferred here behind one hook.
- **Purging platform-api's own console-side audit records.** The console owns
  `console_audit_log`; mark8ly does not delete it.
- **Restoring a purged tenant.** There is no such operation and this design does not imply one.
