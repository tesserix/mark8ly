# Tenant directory for the platform console

**Date:** 2026-08-23
**Issue:** #277 — `GET /admin/entities/tenants` and `/admin/entities/tenants/{id}`
**Series:** #260 — the platform console's migration off direct database access
**Builds on:** #292, #302 (the `platformadmin` surface), #274 (front door)

## Scope

Two read endpoints exposing mark8ly's tenant directory to the Tesserix platform
console, replacing the cross-database read that serves its `tenant_names`
enrichment today.

**In scope**

- `GET /internal/tenants` and `/internal/tenants/:id` in `platform-api`
- Client methods in `marketplace-api`
- `GET /admin/entities/tenants` and `/{id}` on the `platformadmin` surface

**Out of scope**

- #278 (user directory) — see "Why #278 is not here"
- Any write path
- Changes to the caller-scoped `listMyTenants`, which the merchant UI depends on

## Why #278 is not in this spec

#278 asks for a searchable directory of staff and operators, with one central
constraint: **no customer rows under any filter.**

That constraint is unenforceable against mark8ly's current data. The only
user-ish table is `user_profiles` (`user_id`, `email`, `display_name`, `phone`,
`avatar_url`) — it carries no role, type or origin. A storefront customer who
ever got a profile row is indistinguishable from a staff member.

The staff/customer split lives in **GIP tenant pools** (`platform`,
`mp-internal`, `mp-customer`), not in Postgres. Satisfying #278 means either
querying GIP through `internal/gipadmin`, or adding a discriminator column that
does not exist — both new capability rather than exposing existing state.

Recorded on #278 for the console team rather than guessed at.

## Decisions

### `q` matches name, owner email, and store slugs

The issue specifies `q` over "name, slug and owner email". **Tenants have no
slug, deliberately.** `platform-api/internal/tenant/models.go:18`:

> No slug column — the URL identity moved to `store.slug` and a tenant with
> multiple stores has multiple slugs.

Reading the intent as "find a tenant by something a human knows", `q` matches
`tenants.name`, `tenants.owner_email`, and any of the tenant's `stores.slug`
via a join.

Not a contract change — the console sends `q`; what it matches is ours. Noted
on the issue so the console does not discover it by surprise.

### Search and pagination happen in `platform-api`

In SQL, not in `marketplace-api`. Filtering a fetched page in the caller makes
`pagination.total` a lie and the page size meaningless.

### The internal tenant-directory endpoint fails closed

`platform-api`'s `RequireInternalAuth` no-ops when its secret is empty:

```go
if secret == "" { c.Next(); return }
```

That is right for the existing `/internal` routes — `/internal/tenants/{id}/members`
needs a tenant id the caller already has, so an unconfigured deploy leaks little.

It is wrong for this one. The directory returns **every tenant on the platform,
unscoped** — that is the point of it, per the issue's "No caller-scoping".
An unconfigured deploy would serve the whole directory unauthenticated.

This endpoint refuses with `503` when the secret is unset. The other `/internal`
routes keep their permissive branch. Same precedent as the `platformadmin`
surface (#275), same reasoning, one guard.

## Architecture

```
console ──signed──> marketplace-api                    platform-api
                     platformadmin/entities_tenants.go
                       └── tenantdirectory client ───> GET /internal/tenants
                                                       GET /internal/tenants/:id
                                                         └── tenant + store rollup
```

Three units, each with one responsibility:

- **`platform-api` handler** — owns the query: search, filters, pagination, rollup
- **client** — owns transport and error mapping, modelled on `internal/teamproxy/client.go`
- **`platformadmin` handler** — owns the wire shape and nothing else

## Component: `platform-api` internal endpoints

### `GET /internal/tenants`

Query: `q`, `status`, `created_from`, `created_to`, `page`, `limit`.

- `q` — case-insensitive partial match across `tenants.name`,
  `tenants.owner_email`, and `stores.slug` (join, distinct)
- `status` — exact, from the existing `tenant.Status*` constants. No second
  vocabulary.
- `created_from` / `created_to` — inclusive bounds on `tenants.created_at`
- Ordering: `created_at DESC`
- **No caller-scoping.** This is the platform view, not "tenants I belong to".

### `GET /internal/tenants/:id`

Tenant plus a store rollup: count, and `{id, slug, name, status}` per store.

**One grouped query, not N+1.** The issue calls this out explicitly ("Detail
returns stores without a round trip per store").

### Registration

On the existing `internal` group in `cmd/server/main.go:340`, via
`tenantHandler.Register(v1, internal)` — the pattern already in place.

The fail-closed guard wraps these two routes only.

## Component: the client

New package `marketplace-api/internal/tenantdirectory`, modelled on
`internal/teamproxy/client.go`: same `X-Internal-Auth` header, same envelope
unwrapping, same typed error handling.

Not added to `teamproxy` itself — that package's stated purpose is team
membership. A directory read is a different concern, and the two will diverge.

`ErrPlatformUnavailable` maps from transport failure, as `teamproxy` already does.

## Component: `platformadmin` handlers

`GET /admin/entities/tenants` and `/admin/entities/tenants/{id}`.

Envelope, per the pinned contract and identical to `/admin/audit-logs`:

```json
{ "data": [ ... ], "pagination": { "page": 1, "limit": 50, "total": 412 } }
```

Reuses the existing `pagination` type in `platformadmin` rather than declaring
a second one.

**Row shape**

| field | source |
|---|---|
| `id` | `tenants.id`, bare |
| `name` | `tenants.name` |
| `owner_email` | `tenants.owner_email` |
| `status` | `tenants.status` |
| `created_at` | RFC3339 UTC |

Detail adds `stores`: `store_count`, and per store `{id, slug, name, status}`.

**Conventions inherited from the surface**

- Empty result is `200` with `[]` — never `null`, never `{}`
- `limit` clamped, not refused; a missing parameter takes the default
- `pagination.limit` reports the **effective** limit, so `total / limit` is a
  correct page count
- Ids bare; the platform API namespaces on arrival
- No `source` field

## Error handling

| condition | status | `error` |
|---|---|---|
| platform-api unreachable | 503 | `upstream_unavailable` |
| platform-api returned 5xx | 503 | `upstream_unavailable` |
| tenant not found (detail) | 404 | `not_found` |
| platform admin secret unset | 503 | `not_configured` |
| signature/timestamp/nonce failure | 401 | `unauthenticated` |

`upstream_unavailable` is deliberately distinct from `not_configured`. The
console can then separate "mark8ly is refusing" from "mark8ly's dependency is
down" — and neither is an empty result, which is what a fail-open would produce
and what a caller would misread as "no tenants".

## Testing

**`platform-api`**

- Search matches name, owner email, and store slug — one case each, plus a
  tenant matched only by its store's slug, which is the case the literal
  reading of the issue would have missed
- Status and created-range filters
- Pagination: `total` is the unpaginated count
- Rollup issues one query, not N+1 — assert via query count or logged SQL, not
  by reading the code
- A tenant with no stores returns `store_count: 0` and an empty list
- **No caller-scoping**: a tenant the caller has no membership in still appears
- **Fail-closed**: unset secret refuses; the other `/internal` routes still
  behave permissively

**`marketplace-api`**

- Unit tests with a stub client
- **Golden fixture** for the envelope, compared against real handler output —
  the mechanism that already caught both renames and additions on
  `/admin/audit-logs`
- Empty result marshals as `[]`
- `upstream_unavailable` on client failure, and specifically **not** an empty
  `200`

## Consequences

- The console's `tenant_names` enrichment can move off the cross-database read
- #283 (onboarding funnel) will want the same client package; it is scoped so
  that is additive
- #278 remains blocked on a decision the console team owns
- One more `/internal` route in `platform-api` fails closed, which is a
  divergence within that group. The comment records why, so it is not tidied
  into consistency later.
