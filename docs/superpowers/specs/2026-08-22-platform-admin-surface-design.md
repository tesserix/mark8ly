# Platform admin surface — foundation and first slice

**Date:** 2026-08-22
**Issues:** #274 (front door), #275 (operator identity), #276 (audit-logs)
**Series:** #274–#290 — the platform console's migration off direct database access

## Scope

This spec covers the foundation of mark8ly's `/admin/*` HTTP surface plus one
endpoint. It does not cover the remaining fourteen issues in the series; each
later batch gets its own spec, written after this one has run in production.

**In scope**

- The front-door decision (#274) and the package structure that follows from it
- Operator-identity middleware, signature verification and replay defence (#275)
- The audit-log schema changes that operator attribution requires
- `GET /admin/audit-logs` (#276), against the contract pinned in its comments

**Out of scope**

- Every other endpoint in the series (#277–#289)
- `@tesserix/admin-conformance` (#290) — blocked on the console publishing it
- Any change to the merchant-facing admin API

## Decisions

### `marketplace-api` fronts `/admin/*` (#274)

One base URL, served by `marketplace-api`, which calls `platform-api` for
tenant, user and onboarding data.

Most of the platform-relevant data already lives in `marketplace-api`, along
with the existing `/admin/` route tree, the `internalsvc` trust boundary and
the audit emitter.

The counter-argument recorded in #274 — that the `marketplace-api →
platform-api` client direction is less established than the reverse — does not
survive contact with the repository. Clients exist in both directions:
`marketplace-api/internal/teamproxy/client.go` already calls `platform-api`'s
internal endpoints and unwraps its `{data: ...}` envelope, and
`platform-api/internal/marketplaceapi/vendor_client.go` is the reverse. #277,
#278 and #283 extend `teamproxy` rather than establishing a new direction.

### The surface is a new package, not new routes on the existing tree

`marketplace-api/internal/handlers/platformadmin/`.

`internal/handlers/admin/routes.go` is 914 lines and serves a different
audience through a different auth chain with a different response envelope.
The two surfaces share the domain packages beneath them — `audit`, `seaqueue`,
`subscription`, `tenantpurge` — and share nothing at the HTTP layer.

This separation is load-bearing rather than stylistic. The merchant audit-logs
endpoint serves `{data, meta: {page, page_size, total, total_pages}}` and is
consumed by `apps/admin/lib/api/settings-tier2-api.ts`. The contract requires
`{data, pagination: {page, limit, total}}`. The shapes cannot be reconciled, so
they are not reconciled: two presenters over one repository.

### This surface fails closed

`internalsvc.RequireInternalAuth` no-ops when its secret is empty
(`internal/handlers/internalsvc/audit_ingest.go:182`; `platform-api` carries an
identical branch). That is correct for its purpose — ship the binary, populate
the secret afterwards.

It is wrong here. `/admin/*` serves cross-tenant tenant, billing and audit
data, and an unconfigured deploy under that rule would serve it unauthenticated.

The `platformadmin` middleware returns `503` when its secret is unset.
Existing `internalsvc` routes are not touched, and their permissive branch
stays as it is.

## Architecture

```
console ──signed HTTP──> marketplace-api
                          └── internal/handlers/platformadmin/
                              ├── middleware.go   signature, operator, capability, replay
                              ├── audit_logs.go   GET /admin/audit-logs
                              └── routes.go       registration
                                    │
                                    ├── internal/audit/          (shared)
                                    └── internal/teamproxy/  ──> platform-api
```

`platformadmin` owns HTTP concerns only: authentication, request parsing,
response shaping. Domain logic and data access stay in the packages that
already own them. A reader should be able to understand what an endpoint
returns without reading a repository, and a repository change should not
require a handler change unless the wire shape changed.

## Component: operator identity middleware (#275)

### Wire format

Every platform call carries:

```
X-Platform-Operator:   <operator id, opaque string>
X-Platform-Capability: <capability being exercised>
X-Platform-Timestamp:  <unix seconds>
X-Platform-Nonce:      <uuid v4>
X-Platform-Signature:  <hex hmac-sha256>
```

The signature covers, joined by `\n`:

```
<method>
<path>
<canonicalised query>
<sha256(body), hex; sha256 of the empty string when there is no body>
<timestamp>
<nonce>
<operator>
<capability>
```

Query canonicalisation: parameters sorted by key, then by value within a
repeated key, percent-encoded, joined by `&`.

The body hash is signed so a captured signature cannot be lifted onto a
different payload. Operator and capability are signed so neither can be
substituted after signing — they are the entire point of the issue, and an
unsigned header carrying them would be an attribution claim anyone on the
network path could forge.

HMAC rather than asymmetric signing because both sides already share secrets
through Secret Manager, and neither service has key-distribution machinery.

**This scheme is defined by mark8ly, not received from the console.** #275 asks
mark8ly to verify a gateway signature whose format is specified nowhere in this
repository. It is published on #275 for the console to implement against or
replace. If the console replaces it, the golden vectors below are what changes;
the enforcement model does not.

### Replay defence

Two layers:

1. A ±300 second timestamp window.
2. A `platform_request_nonces` table with a unique constraint on the nonce.
   The insert failing on duplicate *is* the replay check. Rows are swept on
   the existing nightly cleanup schedule.

The window alone is insufficient. An in-memory nonce cache would also be
insufficient: mark8ly runs on Knative at 0–5 replicas, so a replay routed to a
different pod would not see the original. The database is the only shared
state available on this path.

### Enforcement

| | read paths | write paths |
|---|---|---|
| valid signature | required | required |
| operator identity | optional | required — `401` |
| capability | optional | required — `401` |
| secret unset | `503` | `503` |

Capability is never inferred from the route. Authority is asserted upstream by
the console and the gateway; mark8ly records it and refuses its absence.

### Errors

Machine-readable and stable, per the contract:

| condition | status | `error` |
|---|---|---|
| secret not configured | 503 | `not_configured` |
| signature absent or malformed | 401 | `unauthenticated` |
| signature mismatch | 401 | `unauthenticated` |
| timestamp outside window | 401 | `unauthenticated` |
| nonce replayed | 401 | `unauthenticated` |
| write without operator | 401 | `operator_required` |
| write without capability | 401 | `capability_required` |

Signature, timestamp and nonce failures deliberately share one code and one
message. Distinguishing them tells an attacker which half of the check they
passed.

## Component: audit attribution

Two defects block "every audit row names the operator", and both must be fixed
before any write endpoint in this series can satisfy its acceptance criteria.

### `store_id` is `NOT NULL` and platform writes are store-less

`internal/audit/models.go:83` declares `store_id` non-null.
`resolveScope` (`internal/audit/emitter.go:238`) returns `ok = false` when
either the tenant or the store is missing, and `buildEntry` then returns `nil`
— **the event is dropped with no error**.

Tenant suspend (#287), trial extension (#286) and purge (#288) are
tenant-scoped and have no store. Under the current schema they would produce
no audit row at all, and nothing would report the failure.

**Fix:** a migration making `store_id` nullable; `Entry.StoreID` becomes
`*uuid.UUID`; `resolveScope` requires a tenant and treats the store as
optional.

A sentinel store UUID was considered and rejected: it would appear in every
store-scoped query in the service as a store that does not exist.

### Operator ids do not fit the actor columns

`buildEntry` derives its actor from `c.GetString("user_id")` parsed as a UUID.
An opaque console operator id either fails to parse — dropping the attribution
— or, if it happens to be a UUID, is written to `actor_user_id` where it
denotes a mark8ly user that does not exist.

**Fix:** a fourth `ActorType` value, `operator`, alongside `user`, `system` and
`api`; plus `actor_operator_id` and `capability` columns, both indexed.

Dedicated columns rather than the existing `metadata` jsonb, because of the
requirement in #275 that the console's `console_audit_log` and mark8ly's rows
be *"joinable on operator and timestamp"*. A join predicate belongs in an
indexed column.

### Migration

```sql
ALTER TABLE audit_logs ALTER COLUMN store_id DROP NOT NULL;
ALTER TABLE audit_logs ADD COLUMN actor_operator_id text;
ALTER TABLE audit_logs ADD COLUMN capability text;
CREATE INDEX idx_audit_logs_actor_operator_id ON audit_logs (actor_operator_id)
  WHERE actor_operator_id IS NOT NULL;

CREATE TABLE platform_request_nonces (
  nonce      uuid PRIMARY KEY,
  seen_at    timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL
);
CREATE INDEX idx_platform_request_nonces_expires_at
  ON platform_request_nonces (expires_at);
```

Dropping `NOT NULL` is a catalogue-only change in Postgres: no table rewrite,
no significant lock. It is forward-compatible — every existing writer supplies
a store and continues to work unchanged.

The down migration fails loudly if any row has a null `store_id`. It does not
delete rows to make itself possible. Losing audit rows to a rollback is worse
than a rollback that stops and asks.

## Component: `GET /admin/audit-logs` (#276)

Implemented against the contract pinned in the issue's comments, which
supersedes the field names in the issue body.

### Request

```
GET /admin/audit-logs?limit=<int>&since_hours=<int>
                     &action=&actor=&resource_type=&from=&to=&store_id=
```

- `limit` — the console sends `200`. **Clamped to 500, never refused.**
- `since_hours` — the console sends `720`.
- Both are always sent by the console; a missing one falls back to our default
  and is never an error.
- `store_id` is an optional narrowing filter, not a required scope.

The 500 ceiling is set by the platform's 1 MiB response read limit, past which
a body truncates mid-JSON and surfaces to operators as "invalid response"
rather than "too large".

### Response

```json
{
  "data": [
    {
      "id": "9f2c1e",
      "actor": "merchant@example.com",
      "action": "product.deleted",
      "timestamp": "2026-08-22T10:00:00Z",
      "target": "prod_123",
      "metadata": "..."
    }
  ],
  "pagination": { "page": 1, "limit": 200, "total": 320 }
}
```

Mapping from `audit.Entry`:

| field | source | required |
|---|---|---|
| `id` | `Entry.ID`, **bare** — no `mark8ly:` prefix | yes |
| `actor` | `ActorEmail`, else `actor_operator_id`, else `"system"` | yes |
| `action` | `Entry.Action` | yes |
| `timestamp` | `CreatedAt`, RFC3339 UTC with offset | yes |
| `target` | `ResourceType` + `ResourceID` collapsed to one string, `omitempty` | no |
| `metadata` | see open question | no |

Ids are sent bare because the platform API namespaces every row as
`<slug>:<id>` on arrival. Namespacing on our side produces
`mark8ly:mark8ly:9f2`.

No `source` field is sent. The platform API stamps it from the slug it
requested and overwrites anything in the body.

`status`, `severity`, `ip_address`, `user_agent` and `actor_type` are held in
our schema, are not in the contract, and are **not sent**. Adding fields
unilaterally is what the pinned contract exists to prevent.

### Empty results

`200` with `[]`. Never `null`, never `{}`.

Guaranteed by allocating `make([]row, 0, n)` before appending — a Go `nil`
slice marshals to `{}`, which defeats a caller's `?? []` and has already
crashed a page in this estate precisely when it had no data.

### Query path

A new cross-store method on `audit.Repository`. Not a loosening of the
existing `List`, which returns early unless both tenant and store are set
(`internal/audit/repository.go:60-63`) and is depended on by the merchant
admin UI and its tests.

## Open question

**Is `metadata` a JSON object or a string?**

The pinned contract's example shows a string (`"metadata": "optional free
text"`). Mark8ly's column is `jsonb`, modelled as `map[string]any`.

Asked on #276 rather than guessed, because independent guessing at field
shapes is the defect that comment was written to close.

**Until answered, the field is omitted.** It is optional and specified as
"omit when absent", so omission is the only choice that cannot be wrong on the
wire. The cost is that audit rows reach the console without detail, so this
should not stay open long.

## Testing

### The test this repo is missing

The #276 comment diagnoses the near-miss exactly: *"the Go tests never
marshalled against the console's parser, and the console tests mocked the
response."* Both sides were green and both were wrong.

So the load-bearing test is a **golden-file contract test**: marshal real
handler output and byte-compare it against a fixture encoding the pinned
contract — field names, `omitempty` behaviour, bare ids, absent `source`,
`pagination` rather than `meta`.

When the console publishes `@tesserix/admin-conformance` (#290), that fixture
is what it replaces. Until then it is the only check standing where the last
defect got through.

### Order (TDD, red first)

1. **Signature canonicalisation** — golden vectors, fixed inputs to fixed hex.
   Written before the middleware, and published on #275 as the console's
   reference implementation.
2. **Replay** — same nonce twice, second refused; outside the window, refused;
   concurrent duplicates across two connections, exactly one wins.
3. **Enforcement matrix** — one test per cell of the table above, including
   secret-unset → `503`.
4. **Attribution** — a platform write produces a row with `actor_operator_id`,
   `capability`, and `actor_type = operator`. *This test fails before the
   migration*, and its failure mode is the point: the event is dropped by
   `resolveScope`, so the assertion to write first is that a row exists at all.
5. **`/admin/audit-logs`** — cross-store rows, envelope shape, empty → `200
   []`, clamped `limit`, `since_hours`, ISO-8601 offsets.

### Regression guard

An explicit assertion that the merchant endpoint still emits
`{data, meta: {page, page_size, total, total_pages}}`.

Two presenters over one repository is safe until someone consolidates them.
This test is what makes that consolidation fail loudly.

### Coverage

80% minimum on new packages, per project standard. Unit tests throughout;
integration tests against real Postgres for the migration, the nonce
uniqueness constraint and the cross-store query.

## Rollout

Each step is independently revertible. Steps 1–3 are unobservable from outside
the cluster.

1. **Migration** — nullable `store_id`, new columns, nonce table. Inert;
   nothing reads them.
2. **Middleware and handler merged, route not mounted.** Dead code, live tests.
3. **Populate the secret** in Secret Manager, and the console's copy.
4. **Mount the route.** The first moment behaviour changes in production. One
   line, one revert.
5. **Verify against a demo tenant**, then report live on #276.

Mark8ly is in production with demo accounts only. That makes the blast radius
small, and it also means production will not surface a contract defect on its
own — which is why the discipline above sits in the contract test rather than
in staged traffic.

## Consequences

- #277, #278 and #283 are unblocked and build in `marketplace-api`.
- Every write endpoint in the series (#281, #286, #287, #288) depends on the
  audit schema change specified here. None of them can satisfy their
  attribution criteria before it lands.
- `platformadmin` becomes the home for the remaining endpoints in the series.
- The signature scheme is mark8ly's proposal until the console confirms it.
  A replacement changes the golden vectors, not the enforcement model.
