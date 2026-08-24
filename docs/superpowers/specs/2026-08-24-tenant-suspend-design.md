# Design — tenant suspend / unsuspend (#287)

**Status:** approved in principle, spec awaiting review
**Issue:** #287 · **Umbrella:** #260 · **Date:** 2026-08-24

## The problem this design exists to avoid

#287 reads as "add two endpoints and some reason codes". Implemented literally, it
would ship an operator action that **enforces nothing**.

Verified before designing:

| path | gates on status today? |
|---|---|
| Storefront `{slug}.mark8ly.com` | **Yes.** `platform-api/internal/store/handler.go:150` — `if s.Status != StatusActive` → 404 |
| Merchant admin API (marketplace-api) | **No.** `internal/stores/models.go:44` declares `StatusSuspended`; nothing reads it |
| Merchant admin UI (Next.js) | **No**, though the internal by-slug endpoint it calls returns `status` |
| **Tenant status, anywhere** | **No. Entirely inert.** |

`tenant.StatusSuspended` exists at `platform-api/internal/tenant/models.go:41`. Nothing
writes it, and searching all four Go services, their middleware, `auth-bff`, and the TS
frontends found nothing that reads it to deny anything — the only two references count
active tenants (`estate/counts.go:43`) and set active on creation
(`onboarding/service.go:290`).

So a `tenant.status = 'suspended'` write, alone, would leave the storefront serving, the
merchants logging in, and orders processing, while the console displayed "suspended".
That is the same defect class as #282's structurally-zero counter and #289's dropped
`errored` metric — except here it is a **write an operator would rely on for an abuse or
compliance decision**, which makes it the most consequential instance the milestone has
produced.

**Therefore: suspension is defined by what it enforces, and tenant status is the record
of the decision rather than the mechanism.**

## Architecture

Enforcement rides on **store** status, which is already enforced at the storefront and is
already projected into marketplace-api. Tenant status is written as the operator's record
and is the cascade's source of truth.

```
console → POST /api/v1/platform/admin/tenants/{id}/suspend   (marketplace-api, HMAC + operator + capability)
              │
              ├─→ platform-api  POST /internal/tenants/{id}/suspend   (strictInternal, NEW)
              │        ├─ tenants.status         = 'suspended'
              │        └─ stores.status          = 'suspended'  WHERE status = 'active'
              │           stores.suspended_by_tenant = true      (only for those rows)
              │
              └─→ marketplace-api local `stores` projection: same rows set to 'suspended'
                  immediately, so enforcement does not wait out the 5-minute TTL
```

Enforcement points, all reading store status:

1. **Storefront** — already works. No change.
2. **Merchant admin API** — `internal/stores/middleware.go` `StoreMiddleware` resolves and
   ownership-checks every store-scoped request and sets `c.Set("store", …)`. It gains a
   status check. This is the single chokepoint for the API.
3. **Merchant admin UI** — the Next.js middleware already resolves
   `{slug}-admin.mark8ly.com` through platform-api's internal by-slug endpoint, whose
   response carries `status`. It gains a check.
4. **New logins** — `auth-bff` gates session issuance on tenant status
   (`internal/loginotp/handler.go:108` and `internal/autologin/service.go:181`). A cold
   path, one check, and without it an existing session keeps working until it happens to
   touch a store-scoped route, and non-store-scoped admin routes stay ungated entirely.

### Why new platform-api endpoints rather than the existing PATCHes

`PATCH /internal/tenants/:id` takes `{Name, UID}` and `PATCH /internal/stores/:id` takes
`{Name, Timezone, StorefrontTheme, UID}` — **neither accepts status**. Both also authorize
by running an FGA check against the *caller's merchant GIP UID*
(`can_edit_settings` / `can_edit_store_settings`), a model a platform operator has no
relation in. Reusing them would require widening a merchant-authorized endpoint to accept
a privileged field — exactly the wrong direction.

The new endpoints mount on **`strictInternal`** (`platform-api/cmd/server/main.go:353`,
`RequireInternalAuthStrict`), the fail-closed group, because they are estate-wide operator
actions and not scoped by anything the caller had to know. The permissive
`RequireInternalAuth` variant would serve the whole estate on an unconfigured deploy.

## Reversibility — the part a naive cascade gets wrong

If a tenant has one store already suspended individually, a cascade-back on unsuspend
would wrongly reactivate it.

`stores.suspended_by_tenant BOOLEAN NOT NULL DEFAULT false` (new column, platform-api)
records who suspended each store.

**Migration numbering:** platform-api uses a **4-digit** scheme and its latest is
`0014_unique_tenant_owner_email`, so this is `0015_stores_suspended_by_tenant`. Do not
copy marketplace-api's 6-digit scheme (`000001…`) — they are different services with
different sequences, and the two-service confusion is trap 4's whole lesson.

- **Suspend:** for stores currently `active` → set `suspended` and `suspended_by_tenant = true`.
  A store already `suspended` or `archived` is untouched and keeps the flag `false`.
- **Unsuspend:** set `active` **only where `suspended_by_tenant = true`**, then clear the flag.

Chosen over a side table because it is one column, cannot drift out of sync with the row
it describes, and makes both operations idempotent by construction.

## Contract

```
POST /api/v1/platform/admin/tenants/{id}/suspend
  { "reason_code": "abuse", "reason": "optional free text" }

POST /api/v1/platform/admin/tenants/{id}/unsuspend
  { "reason_code": "resolved", "reason": "optional free text" }
```

- **Write**, so per the enforcement matrix both operator identity **and** capability are
  required; a read-only caller gets 403.
- Response is the tenant's **current state** plus what changed:
  `{"data": {"tenant_id", "status", "stores_affected": N, "changed": bool}}`.
- **Suspending an already-suspended tenant is a no-op returning current state** — not an
  error, and **no second audit row** (per #287's acceptance). `changed: false` says so
  explicitly rather than leaving the caller to infer it.
- Unsuspend is symmetric and equally attributed.
- Ids bare; timestamps RFC3339 UTC.

### Reason codes — proposed, for ruling

Required, from a fixed set; free text optional **in addition**, never instead. An audit row
saying *what* without *why* is the gap this series exists to close.

| code | meaning |
|---|---|
| `abuse` | abusive content or behaviour toward customers or staff |
| `fraud` | suspected fraudulent transactions or identity |
| `non_payment` | billing failed and the dunning ladder is exhausted |
| `legal` | legal or regulatory demand |
| `tos_violation` | terms breach not covered by `abuse` or `fraud` |
| `security` | compromised account or active security incident |
| `voluntary` | merchant asked for the store to be paused |

Unsuspend takes its own set: `resolved`, `appeal_upheld`, `operator_error`, `voluntary_end`.

An unknown code is a `400` naming the field — never silently coerced, and never stored as
free text.

## Staleness: fail open, except when the cache says suspended

marketplace-api's projection is deliberately fail-open — `StoreMiddleware` serves cached
stores for up to `StaleCeil` (24h) when platform-api is unreachable, refreshing on a
5-minute `FreshTTL`.

**Ruling:** keep that behaviour, with one asymmetry — **a cached `suspended` status is
authoritative regardless of age.** A stale `active` may still be served (an outage must not
lock out every merchant); a stale `suspended` is always enforced.

The asymmetry is deliberate: the two errors are not symmetric in cost. Serving a
briefly-stale active store is a small correctness cost; serving a suspended one is the
failure this endpoint exists to prevent. Suspension is also rare and deliberate, so the
window in which the two disagree is small and always closes toward enforcement.

Note the suspend endpoint writes the local projection directly in the same operation, so
the propagation delay for the API path is **zero**, not five minutes. The TTL only matters
for a suspension applied out-of-band in platform-api.

## Audit

Both operations audit through `platformadmin.EmitOperatorAction(c, emitter, tenantID, ev)`
— the tenant is a required parameter there precisely because nothing on this surface sets
`tenant_id` on the context and `audit.Emit` would otherwise silently write no row (trap 3,
#310). The event carries the operator, the reason code, the free text, and
`stores_affected`.

## Routing (trap 2)

`/admin/tenants/{id}/suspend` collides with the merchant tree's
`/admin/tenants/:tenantId/sso` — two different wildcard names at one path position makes
gin **panic at router build time**, taking the service down at startup rather than failing
a request. Safe only while these routes are registered on the `platformadmin` group under
`/api/v1/platform`, never the merchant group. Verify before touching routing.

## Testing

- Enforcement, per point, each proved by mutation: remove the check and the test fails.
  A test that passes with the guard deleted is testing something else.
- **`suspended_by_tenant` reversibility:** seed a tenant with one active and one
  individually-suspended store; suspend; unsuspend; assert the individually-suspended one
  is **still suspended**. This is the assertion the naive implementation fails.
- **Idempotency:** a second suspend returns `changed: false`, writes no second audit row,
  and leaves `suspended_by_tenant` untouched. Assert the audit row **count**, not just
  presence.
- **Stale-suspended is enforced:** a projection row older than `StaleCeil` with
  `suspended` must still be denied. Put the fixture at the exact boundary and 1ms either
  side — `timestamptz` is microsecond-resolution, so a nanosecond offset truncates and both
  fixtures become the same row.
- **Capability:** a caller with operator identity but without the capability gets 403.
- Golden fixture for both responses, proved by mutation against a field rename **and** a
  field addition.
- Integration runs use `-p 1`, the LAN IP DSN, and `go vet -tags=integration ./...` is part
  of the verification set (trap 8).

## Out of scope, stated so nobody assumes otherwise

- **Cloudflare edge cache.** A suspended storefront may keep serving from the edge until
  the cached response expires. Enforcement is at origin; no purge is issued. If operators
  need immediate darkness, that is a separate piece of work against the Worker.
- **Existing merchant sessions** are not revoked. They stop working when they touch a
  store-scoped route or re-authenticate. Active session revocation is a bigger change to
  `auth-bff`'s session model.
- **Archived tenants.** `archived` is a third status with its own semantics; this design
  touches only the active ↔ suspended transition.
