# Break-glass activation — design

**Status:** proposed
**Issues:** closes #642 (turn it on), unblocks #404 (write operations)
**Supersedes the unfinished parts of:** `docs/superpowers/plans/2026-04-18-p13-sso-break-glass.md`

**Goal:** make the break-glass emergency admin login reachable, and mount the
per-tenant SSO login routes that share its one missing dependency.

---

## 1. What break-glass is for

Pro-tier tenants attach their own SAML/OIDC IdP (P13). When that tenant's SSO
breaks or is misconfigured, their admins cannot sign in at all. Break-glass is
one emergency local account per tenant — password **and** TOTP, never SMS —
that bypasses SSO entirely.

The thing that is down is **the tenant's own IdP**. GIP, auth-bff and the
cluster are up. This matters: it is the constraint that shapes the whole
design, and it is narrower than "the IdP is down".

## 2. Verified current state

The code is far more finished than "not built". Verified by reading it, not
from the plan document:

| Piece | State |
|---|---|
| `internal/breakglass/` (~60KB) | credentials, TOTP, lockouts, rotation, audit, Slack, bootstrap — **complete** |
| `handlers/admin/break_glass_login.go` | **complete**, including a considered fail-closed degradation when the lockout store is unreachable (#457/#468) |
| `handlers/public/sso_login.go` | login / callback / logout — **complete** |
| Migrations `000073` (lockouts), `000129` (disable) | applied |
| `cmd/break-glass-rotation` | exists |
| Platform **read** path | **already mounted** (`platformadmin.BreakGlassListerFunc`, `main.go:2338`, `:2487`) |
| `authbffclient.SessionIssuer` | interface + `NoopIssuer` that always errors — **no implementation** |
| `POST /admin/break-glass/login` | **not mounted** |
| SSO login routes | **not mounted** |
| Break-glass accounts provisioned | **zero** |

**One missing dependency gates two features.** `SessionIssuer` is referenced
only by `break_glass_login.go` and `sso_login.go`. Implementing it unblocks
both; neither is reachable without it.

## 3. The two constraints

### 3.1 A break-glass session carries no IdP tokens

auth-bff's `session.Session` has `AccessToken` / `IDToken` / `RefreshToken`.
A break-glass principal never authenticated to any IdP — there is no federated
assertion to put there. Minting a GIP custom token for a synthetic user was
rejected: P13's own architecture note says *"we do not invent a second
token-verification layer"*, and it would add GIP account lifecycle for
synthetic users to buy nothing this design needs.

So the session ships with those three fields **empty, deliberately**.

### 3.2 Break-glass secrets live in a backend we decommissioned

P13 stores each account's password + TOTP secret in **GCP Secret Manager** at
`/projects/tesserix-prod/secrets/break-glass-{tenant_id}`.

Milestone 10 retired GCP Secret Manager: the secrets were deleted and the
service account's IAM revoked. **Turning on break-glass as written would fail
at first use**, and `Bootstrapper` writes Secret Manager *before* the DB row,
so provisioning would fail at step one.

This is not optional cleanup — it is a prerequisite.

## 4. Why nothing downstream needs to change

The reason the narrow approach is sufficient, verified against the running
cluster rather than assumed:

- `marketplace-api` authenticates callers with `auth.HeaderTrustAuth` —
  it reads **`X-User-Id` / `X-Tenant-Id`** headers, optionally gated by
  `MARKETPLACE_INTERNAL_AUTH_SECRET`. It does **not** validate GIP tokens on
  this path.
- The admin API's Istio policy `allow-marketplace-api-admin-callers` is
  **mTLS workload-identity** based — a list of SA principals
  (`.../sa/mark8ly-admin`, `.../sa/mark8ly-auth-bff`, …). It requires **no
  JWT**. The JWT `RequestAuthentication`s live in `istio-ingress`, on the
  browser hop.
- Therefore the admin BFF sets exactly the same headers from a break-glass
  session as from a normal one.

**The empty IdP token fields never reach a validator.** "Session-cookie trust"
is not a reduced-capability compromise here; it is complete for the admin
surface.

## 5. Design

### 5.1 auth-bff — mint a session for an already-authenticated principal

Add one endpoint to the **existing** `/internal` group in
`internal/handlers/internal.go`, which already carries
`requireServiceKey()` (Bearer `INTERNAL_SERVICE_KEY`) and `rateLimit()`:

```
POST /internal/mint-session
```

Request:

```json
{ "tenant_id": "...", "tenant_slug": "...", "user_id": "...",
  "email": "...", "auth_context": "break_glass",
  "app_name": "marketplace-admin", "ttl_seconds": 7200 }
```

Response: `{ "set_cookie": "<full Set-Cookie header value>" }`

> **Correction to the code's own comment.** `session_issuer.go` says the
> endpoint is "expected to be mTLS-gated". The `/internal` group's actual,
> working pattern is a Bearer service key plus rate limiting. Follow the
> pattern that exists; introducing a second auth mechanism for one endpoint
> would be a new thing to get wrong.

**`auth_context` must be allow-listed**, exactly as `IsKnownSessionCookie`
guards `session-exchange` today. Accept `break_glass` and the existing
`staff` / `customer`; reject anything else with 400. It must never default to
`staff` — a typo would silently mint a full staff session.

Cookie cryptography stays in auth-bff. Extract the encrypt step already inside
`CookieStore.Save` into an exported `Encode(sess *Session) (string, error)`,
and have both `Save` and this endpoint use it. marketplace-api never signs a
session cookie.

### 5.2 marketplace-api — implement the issuer

`internal/authbffclient/http_issuer.go`:

```go
type HTTPIssuer struct {
    BaseURL    string        // AUTH_BFF_INTERNAL_URL
    ServiceKey string        // AUTH_BFF_INTERNAL_SERVICE_KEY
    Client     *http.Client  // timeout 5s
    TenantSlug func(context.Context, uuid.UUID) (string, error)
}
```

`Issue` POSTs to `/internal/mint-session` with `auth_context: "break_glass"`
and returns the `set_cookie` string. Any non-200 returns an error — the
handler already maps that to `500 session_mint_failed` without leaking why.

`NoopIssuer` stays the default when config is absent, so a misconfigured
deploy fails loudly rather than serving an unauthenticated route.

### 5.3 Port break-glass secrets to OpenBao

`breakglass.SecretClient` is a two-method interface:

```go
AddVersion(ctx, path string, payload []byte) error
AccessLatest(ctx, path string) ([]byte, error)
```

`carriersecrets.BaoClient` already offers `CreateOrAddVersion(ctx, name,
payload)` and `AccessLatest(ctx, name)`. Add
`internal/breakglass/bao_secret_client.go` — a thin adapter, no new secret
machinery, reusing the KV v2 mount and Kubernetes auth proven in milestone 10.

Path scheme: `break-glass/{tenant_id}`, mirroring the carrier-secrets
convention.

`gcp_secret_manager.go` is **deleted**, not left in place. An unused GCP client
whose IAM has been revoked is a trap for the next reader — the same reasoning
that deleted the retired ArgoCD Application in tesserix-k8s#938.

### 5.4 Mount the routes

In `main.go`, alongside the already-wired `BreakGlassLister`:

- `POST /admin/break-glass/login` — mounted **outside** the store-scoped
  `RequireActive` group. The handler comment is explicit about why: it must
  survive `read_only` / `store_closed` subscription states (§12.4).
- Gate with `plangate.RequireFeature(FeatureSSO)` — break-glass exists for
  SSO tenants, so a Starter tenant hitting it should get the same 403 the SSO
  endpoints give.
- SSO login / callback / logout routes, now that their issuer exists.

### 5.5 Provision accounts

Use the existing `Bootstrapper` (bcrypt cost 12, Secret-Manager-write-first
ordering preserved against the Bao adapter). One account per Pro+SSO tenant.
Zero exist today, so this is the step that makes the feature real rather than
merely reachable.

## 6. #404 — write operations

Unblocked by the same work. `rotation.go`, migration `000129_break_glass_disable`
and `cmd/break-glass-rotation` already exist; `platformadmin` write handlers
need mounting plus the Bao port above. Rotate / disable / clear-lockout are a
follow-on plan, not this spec's scope.

Note for the 24-hour post-use rotation and 90-day cron: **CronJobs inherit the
deployment ServiceAccount**, so the OpenBao grant already applies; what a new
CronJob needs is env vars and the NetworkPolicy pod label, never new IAM.

## 7. Risks

| Risk | Mitigation |
|---|---|
| A caller assumes `session-exchange` returns a non-empty `access_token` | Enumerate every caller before mounting. Break-glass sessions return empty strings there. |
| `auth_context` typo silently mints a staff session | Explicit allow-list, reject-by-default, unit test per accepted value. |
| Break-glass depends on auth-bff being up | Accepted: the failure being addressed is the *tenant's* IdP, not auth-bff. Stated so nobody assumes wider coverage than exists. |
| A break-glass session is a full admin session for 2h | Unchanged from P13's design: every login triggers immediate 24h rotation, a `#security-alerts` Slack post, and a `severity=critical` audit event. |
| Obsolete Terraform | P13 provisions a `break-glass-responders` Google Group + IAM on `/projects/tesserix-prod/secrets/break-glass-*`. Those secrets no longer exist; the IAM policy is dead and should be removed with the GCP client. |

## 8. Deliberately not doing

- **No GIP custom tokens** for break-glass principals (§3.1).
- **No second token-verification layer** in go-shared or any service — §4
  shows none is needed.
- **No mTLS** for the new endpoint; it follows the `/internal` group's
  existing Bearer pattern.
- **No change to Istio policy.** Verified unnecessary, not assumed.

## 9. Test plan

- Unit: `HTTPIssuer` maps non-200 to error; `NoopIssuer` still the default
  when config is absent.
- Unit: `auth_context` allow-list rejects unknown values; no path defaults to
  `staff`.
- Unit: Bao adapter satisfies `SecretClient`; round-trips a `Blob`.
- Integration: `Bootstrapper` provisions against Bao with the
  secret-before-DB ordering intact (a Bao failure must leave no DB row).
- Integration: full login — correct password+TOTP returns 200 with a
  `Set-Cookie`; wrong either factor returns the uniform
  `{"error":"invalid_credentials"}`; the existing lockout tests still pass.
- End-to-end in a real cluster: a break-glass cookie reaches
  marketplace-api-admin and is accepted, proving §4 against the running Istio
  policy rather than the manifest.
- Negative: confirm the route is **absent** before the change and **present**
  after — an unmounted route is this codebase's recurring silent failure.
