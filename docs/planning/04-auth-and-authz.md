# Auth and Authz — GIP + OpenFGA

## The current state

- **Auth provider:** Google Identity Platform (GIP), recently migrated from
  Keycloak. Per-product GIP **tenant pools** for user isolation:
  - `platform` — TH super-admins
  - `mp-internal` — Marketplace admins/staff
  - `mp-customer` — Marketplace storefront end-users
- **Sessions:** Encrypted JWT cookies issued by `auth-bff`. No Redis sessions.
- **Authorization:** OpenFGA, deployed on GKE with Cloud SQL backend. Multi-store
  setup (platform store + marketplace store).
- **Symptoms:** Intermittent "tenant not found," login failures after onboarding,
  flaky session validation. The bugs are real but the **architecture isn't the
  cause**. They're config bugs and state-propagation race bugs.

## Decision: real GIP (no emulator) and real OpenFGA from day one

Earlier discussion considered stubbing one or both for the onboarding-only
slice. Rejected. Reasoning:

- **The bugs you're trying to fix live in this exact integration.** Stubbing
  would defeat the point.
- **Real Google Identity Platform in dev too**, not the Firebase Auth
  Emulator. We use the existing prod GCP project (`tesseracthub-480811`) and
  the existing GIP tenants (`MP-Internal-e986p`, `MP-Customer-zoe11`,
  `Platform-2c9z0`). Local dev hits the same OIDC issuer, the same token
  shapes, the same tenant pools as prod. Bugs found in dev are real bugs.
- **OpenFGA** runs as a container against Postgres in dev. ~10 min to wire up.
- **The timing of OpenFGA tuple writes is one of the suspected bug sources.**
  Doing it for real now is exactly how we find and fix that bug.

### Why no emulator

The Firebase Auth Emulator was the first plan because it gives zero-config
local dev. Dropped because:

- One fewer container, one fewer image to babysit, one fewer "is this still
  maintained" question.
- No `if dev else prod` branching in `auth-bff`. The OIDC client points at one
  URL regardless of environment. Same code path, debugged the same way.
- Emulator behavior differs subtly from real GIP (token claims, tenant pool
  semantics, MFA flows, recaptcha enforcement). Anything you "fix" against
  the emulator might still be broken against real GIP.
- E2E tests run against the real thing. If they pass locally they pass in prod.

### Local dev setup (real GIP, no emulator)

The `infra/dev/load-secrets.sh` script pulls the required credentials from
GCP Secret Manager (project `tesseracthub-480811`) into a gitignored
`infra/dev/.env.local`. `make dev` runs it automatically.

Required secrets in GCP Secret Manager:
- `prod-gip-web-api-key` — Identity Platform Web API key
- `prod-mp-admin-client-secret` — OAuth client secret for the marketplace
  admin/onboarding app

The OAuth client ID, project ID/number, and tenant IDs are all hard-coded in
`load-secrets.sh` (they're not secrets — they're public config). Re-generate
the env file with `make dev-secrets`.

Anyone running the stack needs `gcloud` authenticated as a user with
`secretAccessor` on the `tesseracthub-480811` project.

## Suspected bug sources (to diagnose before porting)

This list is the input to **Phase B: Diagnose old auth bugs**. Each suspect
should be confirmed or ruled out by reading the old code, before any new
code is written.

### Auth (login / session)

1. **Cookie domain misconfiguration.** Sessions issued at `auth.mark8ly.com`
   need `Domain=.mark8ly.com` to be readable at `{tenant}-admin.mark8ly.com`
   and `{tenant}.mark8ly.com`. Off-by-one here = "logged in but admin says
   logged out."
2. **GIP tenant pool routing.** If `auth-bff` verifies a token against the
   wrong pool, signature verification passes but user lookup fails → "user
   not found" / "tenant not found." Check `auth-bff/internal/gip/client.go`
   and how it picks the pool per request.
3. **JWT audience / issuer mismatch.** GIP issuer URLs include the tenant ID.
   If the verifier is configured for the wrong issuer, validation fails
   inconsistently across products.
4. **Clock skew on Knative cold start.** Tokens with `iat` slightly in the
   future (clock drift on a freshly scheduled pod) get rejected. Rare but
   real.
5. **Cookie `SameSite=Strict` blocking the OIDC redirect callback.** Should
   be `Lax` for the callback to carry the session cookie back. Strict breaks
   the dance.
6. **Cookie size > 4KB.** Encrypted JWT cookies + tenant claims + roles can
   blow past the limit. Browser silently truncates → next request has no
   session.
7. **`Secure` flag on cookies in local dev over plain HTTP.** Cookie set but
   never sent back. Looks like "logged in then immediately logged out."

### Authz (OpenFGA)

1. **Tuple write race.** Tenant created in `tenant-service` → user logs in
   immediately → OpenFGA tuple `user:X member tenant:Y` hasn't been written
   yet → permission check fails → admin shows "tenant not found." Classic.
2. **Multi-store routing.** With "platform store" and "marketplace store,"
   if a check goes to the wrong store, it fails. Check
   `go-shared/authz/client.go` store selection logic.
3. **Internal-bypass middleware ordering.** `authz.AllowInternal()` is
   supposed to skip FGA for service-to-service calls. If middleware order
   is wrong (auth runs before internal-bypass detection), legit internal
   calls get FGA-checked and fail.
4. **Tenant cache stale-while-revalidate fail-open.**
   `marketplace-admin/middleware.ts:335` falls back to cached data on error.
   If the cache holds *stale negative results* ("tenant doesn't exist" from
   before it was created), you get persistent "tenant not found" until cache
   TTL expires.
5. **Redis in admin middleware unavailable.** ioredis with no fallback →
   middleware throws → middleware fails open or closed inconsistently
   depending on env.

### "Sometimes works, sometimes doesn't" — the giveaway

Intermittent auth bugs almost always mean **state propagation timing**, not
logic bugs:
- Tenant exists in Postgres but not in OpenFGA yet
- Session cookie set but DNS hasn't propagated for the new tenant subdomain
- Pub/Sub event delivered to one consumer but not another
- Redis cache holds stale data
- Knative pod still warming up, returning 503s that get swallowed

**Local e2e with a deterministic scenario** — onboard a tenant, immediately
try to log into admin, capture every HTTP request and FGA query — surfaces
which one of these it is in an hour.

## Fix strategy

### Cookie / session bugs
- Set `Domain=.mark8ly.com` on session cookies (`Path=/`, `SameSite=Lax`,
  `Secure` only when HTTPS, `HttpOnly` always).
- Validate cookie size; if claims push it over 3.5KB, store the heavy claims
  server-side keyed by a session ID in the cookie.
- For local dev: use `*.localhost` or a hosts file to test the multi-subdomain
  cookie behavior end-to-end.

### GIP tenant pool routing
- Make pool selection **explicit and per-request**, not heuristic.
- The frontend (or auth-bff entry point) must know which product the user is
  signing into and pass the pool ID. Don't infer.
- Verifier instances are cached per pool (one OIDC verifier per pool, not per
  request).

### OpenFGA tuple write race — the outbox pattern
This is the **single most important fix**. It addresses both the tuple-write
race and the "tenant not found after onboarding" symptom.

In the "complete onboarding" handler:
1. Begin DB tx
2. Insert tenant row
3. Insert membership row
4. **Insert outbox row** for the FGA write (in the same tx)
5. Commit DB tx
6. Best-effort: write the FGA tuple immediately
7. If write succeeds, mark outbox row as completed
8. A background drainer processes outbox rows that are still pending
   (in-process for now, separate worker later)

This guarantees:
- The FGA tuple write is **never lost** (it's persisted in the same DB tx as
  the tenant)
- The FGA write happens **as soon as possible** (best-effort inline)
- The system recovers from FGA being temporarily down (drainer retries)

**The auto-login endpoint** in `auth-bff` complements this by doing an FGA
`Check` with **retry-on-not-found** (up to ~2 seconds) before issuing a
session. Backend writes tuple → frontend logs in → auth-bff confirms tuple
exists → session issued. The race window is closed.

### Tenant cache stale-while-revalidate
- **Never cache negative results.** If FGA returns "no membership," don't
  cache it. Re-query next time.
- Cache positive results with short TTL (~30s) and re-validate on cache
  miss.
- On cache backend failure (Redis down), **fail closed** for security-sensitive
  paths (tenant validation), not open.

## What "real OpenFGA in dev" looks like

```yaml
# infra/dev/docker-compose.yml (excerpt)
openfga:
  image: openfga/openfga:latest
  command: run
  environment:
    OPENFGA_DATASTORE_ENGINE: postgres
    OPENFGA_DATASTORE_URI: postgres://openfga:openfga@postgres:5432/openfga?sslmode=disable
  depends_on:
    openfga-migrate:
      condition: service_completed_successfully
  ports:
    - "8080:8080"   # http
    - "8081:8081"   # grpc
    - "3000:3000"   # playground

openfga-migrate:
  image: openfga/openfga:latest
  command: migrate
  environment:
    OPENFGA_DATASTORE_ENGINE: postgres
    OPENFGA_DATASTORE_URI: postgres://openfga:openfga@postgres:5432/openfga?sslmode=disable
  depends_on:
    postgres:
      condition: service_healthy
  restart: "no"

fga-seed:
  image: alpine/curl
  depends_on:
    openfga:
      condition: service_started
  volumes:
    - ../openfga:/fga
  command: sh /fga/seed/init.sh
  restart: "no"
```

`infra/openfga/model.fga` (minimal model for the onboarding-only phase):

```
model
  schema 1.1

type user

type tenant
  relations
    define member: [user]
    define owner: [user]
```

That's the entire authorization model needed for onboarding. Grow it when
admin/storefront port. **Do not copy the old `go-shared/authz/` model** —
it carries complexity for features that aren't in this phase.

## What "real GIP in dev" looks like in practice

There is no emulator container. The Mark8ly dev stack uses real Google
Identity Platform via the existing prod GCP project.

**What's hard-coded in `infra/dev/load-secrets.sh` (public values):**
- GCP project ID: `tesseracthub-480811`
- GCP project number: `849928263410`
- GIP tenant IDs:
  - `MP-Internal-e986p` (admin/onboarding/staff pool)
  - `MP-Customer-zoe11` (storefront end-user pool — used when storefront ports)
  - `Platform-2c9z0` (tesserix-home super-admin pool — used when admin ports)
- OAuth client ID: `849928263410-5djgu3n40c5tpr86votuptkitqveegor.apps.googleusercontent.com`

**What's pulled from GCP Secret Manager at boot:**
- `prod-gip-web-api-key` → `GIP_WEB_API_KEY`
- `prod-mp-admin-client-secret` → `OAUTH_CLIENT_SECRET`

**What's generated locally:**
- `SESSION_ENCRYPT_KEY` — a deterministic dev-only 32-byte string. Local
  sessions never validate against prod by design.

The script writes everything to `infra/dev/.env.local` (gitignored, mode 0600).
docker-compose feeds it into `auth-bff` via `env_file`. `make dev` runs
`make dev-secrets` automatically before starting the stack.

**No `if dev else prod` branching in the OIDC client code.** `auth-bff`
always talks to `https://identitytoolkit.googleapis.com`. Same code path in
dev and prod.

### One-time setup for a new contributor

1. `gcloud auth login` (as a user with `secretAccessor` on
   `tesseracthub-480811`)
2. `make dev`

That's it. No GCP project to create, no service accounts to download, no
emulator image to build.
