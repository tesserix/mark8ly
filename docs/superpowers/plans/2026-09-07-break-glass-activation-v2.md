# Break-glass activation — implementation plan (v2)

**Supersedes:** `2026-09-04-break-glass-activation.md`, whose errata section
lists the six wrong claims that made it unexecutable.
**Issue:** #642. **Unblocks:** #404.
**Corrects:** the merged design spec `2026-09-04-break-glass-activation-design.md`
(§5.1 and its "correction" footnote), which is wrong about the same things.

Every symbol below was read in the tree before being written down.

## Verified state — what is actually there

| Thing | Reality | Where |
|---|---|---|
| Session cookie type | `session.Manager`, `NewManager(cfg Config)` | `auth-bff/internal/session/cookie.go:91` |
| Encode | **`m.encode(s Session)` already exists**, already used by `MintWithDomain` | `cookie.go:222`, `:156` |
| Writers / readers | `Mint`, `MintWithDomain`, `Read`, `decode` — take `http.ResponseWriter` / `*http.Request` | `cookie.go:128,143,182,240` |
| `Session` fields | `UID, Email, TenantID, StoreID, IssuedAt, ExpiresAt` — `time.Time`, **no auth context** | `cookie.go:41-53` |
| `/internal` auth | shared **`X-Internal-Auth`** header, `internalauth.Equal` (sha256 + constant time), applied **per route inside the handler** | `internalauth/internalauth.go:29`, `cmd/server/main.go:425` |
| Break-glass principal id | **already deterministic** — `BreakGlassUserID(tenantID)` = UUIDv5 in a fixed namespace | `marketplace-api/internal/handlers/admin/break_glass_login.go:251-258` |
| Session TTL | `BreakGlassSessionTTL = 2h`, already const | `break_glass_login.go:21` |
| `SecretClient` | `AddVersion` / `AccessLatest` — matches `carriersecrets.BaoClient`'s `CreateOrAddVersion` / `AccessLatest` | `breakglass/secret_manager.go:25`, `carriersecrets/bao.go:61,88` |
| `FeatureSSO` | exists, `Disabled` on the low tier | `plangate/matrix.go:57,158` |
| Break-glass config | **absent** from `pkg/config` | — |

## Decisions taken before planning

**D1 — `AuthContext` becomes a `Session` field.** `auth_context` is the
spec's central safety control ("must never default to `staff`") and there is
nothing for it to guard today. Added as
`AuthContext string \`json:"auth_context,omitempty"\`` so existing cookies stay
valid and decode with an empty value.

> **Amended after the Task 0 audit.** Three call sites rebuild `Session` as a
> fresh struct literal, copying named fields: `switchTenant`
> (`auth-bff/internal/session/handler.go:483`), `switchStore` (`:538`) and
> `adminhandoff` (`handler.go:135`). Adding the field without patching all
> three ships a safety control that **any merchant strips by calling
> `POST /auth/switch-store` once** — a control that would be trusted and be
> wrong. All four changes land in one commit, with a test per site asserting
> the value survives. This is the same shape as the mobile IDP provider being
> pinned in three services where two hops dropped it invisibly.

**D2 — the internal endpoint follows `X-Internal-Auth`.** Not Bearer, not
mTLS. Both the code comment in `session_issuer.go` and the design spec's
"correction" of it are wrong; the pattern that exists and works is the header
compared by `internalauth.Equal`. Introducing a second inbound auth mechanism
for one endpoint would be a new thing to get wrong.

**D3 — the synthetic principal keeps the id the code already derives.**
`Session.UID` = `BreakGlassUserID(tenantID).String()`. It is stable per
tenant, so audit trails join, and no new identity scheme is invented.

> **Amended after the Task 0 audit: this is not safe on its own.** A UID with
> no FGA tuple is refused by three authorization gates, and every admin
> request passes at least one:
>
> - `apps/admin/middleware.ts:407-435` → platform-api `getMe`
>   (`internal/tenant/handler.go:176-182`) answers 404 `no_role`, and the
>   middleware redirects to login. **A correct break-glass login bounces back
>   to `/login` in a loop, with nothing naming the cause.**
> - `marketplace-api/internal/authz/middleware.go:41` `RequireTenantRelation`
>   → 404 on every admin API route.
> - auth-bff `switchTenant` (`handler.go:460`) and `adminhandoff` (`:113`)
>   `CheckMembership` → 403. The `/pick-tenant` recovery surface is empty too,
>   since it is driven by `listMyTenants(uid)`.
>
> **It passes in local dev and fails only in production**: `handler.go:171-174`
> returns `role: owner` unconditionally when `fga == nil`. A green local login
> is not evidence.
>
> One fix covers all three — an FGA tuple for `BreakGlassUserID(tenant)` on
> that tenant. Tenant membership in this estate is a tuple, never a DB column,
> so this is one additive write. It becomes **Task 5a**, and it gates Task 6.

---

### Task 0 — consumer audit — **DONE**

Report: `docs/superpowers/artifacts/2026-09-07-break-glass-session-consumer-audit.md`.
21 consumers examined. Verdict: `AuthContext` safe to add (subject to D1's
amendment); a non-resolving `UID` **not** safe (see D3's amendment, now Task 5a).

It also voided one of the design spec's risks: §7's "a caller assumes
`session-exchange` returns a non-empty access_token" describes a system that
does not exist — `Session` has no token field at all (`cookie.go:41-53`), and
the estate's only token exchange is the mobile bearer path in
`zitadellogin/token_exchange.go`, which never touches the cookie. Delete that
risk row rather than mitigating it.

And it settled a Task 3 question: `SessionIssuer` passing no email is fine —
every consumer treats `Email` as an optional attribution label with a fallback,
and nothing looks a user up by it. `auth_context` is the only reason to widen
the interface, because unlike email it must never be defaulted by the callee.

<details><summary>Original brief</summary>

Neither D1 nor D3 is safe until we know who reads a session and what they
assume. This task writes no feature code.

Enumerate every reader of the session cookie and of the `X-User-Id` header
derived from it — `apps/admin`, `apps/storefront`, `marketplace-api`'s
`auth.HeaderTrustAuth` path — and answer, per consumer:

- does it assume `UID` resolves to a real user row? A break-glass UID does not.
- does it assume `Email` is a real mailbox?
- does adding an unknown JSON field break its decode? (Go: no. TypeScript
  with a strict schema validator: possibly.)

**Done when:** a written list of consumers with a verdict each, and any that
break have a fix named. The design spec's §7 already flags "a caller assumes
`session-exchange` returns a non-empty access_token" as a risk and then names
a symbol that does not exist — this task is what that risk actually needed.

**Do not proceed to Task 2 until this is done.** It is the one step that can
turn "break-glass works" into "break-glass mints a session that half the
estate rejects".

</details>

### Task 1 — export the encoder in auth-bff

`encode` already exists. Add an exported wrapper that stamps `IssuedAt` /
`ExpiresAt` the way `MintWithDomain` does (`cookie.go:147-153`) and returns
the value, so `MintWithDomain` and the new endpoint share one path.

Add the `AuthContext` field from D1 in the same commit, with a round-trip
test through `encode`/`decode` (**not** `LoadFromValue` — it does not exist)
asserting an empty `AuthContext` on an old-shaped cookie.

**Done when:** `go test ./internal/session/` passes including the pre-existing
`Mint`/`Read` tests. This is a refactor plus one additive field; any existing
test changing behaviour means something was got wrong.

### Task 2 — `POST /internal/mint-session` in auth-bff

Mount on the existing `/internal` group (`cmd/server/main.go:425`), guarded
**inside the handler** by `internalauth.Equal(c.GetHeader(internalauth.Header), secret)`,
failing closed when the secret is unset — copying `InternalUsersHandler`
(`internal/session/internal_users.go:69`) rather than inventing a shape.

Request carries tenant id, user id, email, `auth_context`, ttl. Response is
`{"set_cookie": "..."}`.

`auth_context` is an explicit allow-list — `staff`, `customer`, `break_glass`
— rejecting anything else with 400 and **never defaulting**. One unit test per
accepted value plus one for a rejected typo.

### Task 3 — `HTTPIssuer` in marketplace-api

Implement `authbffclient.SessionIssuer` over that endpoint. `NoopIssuer`
stays the default when config is absent, so a misconfigured deploy fails
loudly rather than serving an unauthenticated route.

Note the interface passes only `(tenantID, userID, ttl)` — no email, no
auth context. Either widen it or have `HTTPIssuer` supply
`auth_context: "break_glass"` and a synthetic email itself. Widening is
cleaner but touches the SSO caller too; decide with Task 0's findings in hand
and record which was chosen and why.

### Task 5a — grant the break-glass principal its tenant relation (NEW)

Gates Task 6. Write an FGA tuple for `BreakGlassUserID(tenant)` on its tenant,
so the three gates in D3 admit it. Decide and record **where**: at provisioning
(`Bootstrapper`, alongside the secret and the row) or at login.

Provisioning is the better default — it keeps the emergency path free of a new
dependency, which is the whole point of break-glass and the constraint #404
insists on. Doing it at login would mean an FGA write on the path that must
work when everything else is broken.

**Done when:** a provisioned break-glass principal gets past
`apps/admin/middleware.ts` and `RequireTenantRelation` **against real FGA**.
Not against a `nil` client — that returns `role: owner` unconditionally and
proves nothing.

### Tasks 4–8 — carried over, spot-checked

`SecretClient` and `carriersecrets.BaoClient` line up exactly as the spec
says, and `gcp_secret_manager.go` and `FeatureSSO` exist, so §5.3–5.5 of the
design are sound. Detail:

4. **OpenBao adapter** — `breakglass/bao_secret_client.go`, thin, path
   `break-glass/{tenant_id}`. Preserve `Bootstrapper`'s secret-write-before-DB
   ordering so a Bao failure leaves no orphan row.
5. **Delete `gcp_secret_manager.go`** and the dead Terraform IAM. An unused
   client whose IAM was revoked is a trap for the next reader.
6. **Mount** `POST /admin/break-glass/login` outside the `RequireActive`
   group (it must survive `read_only` / `store_closed`), gated by
   `plangate.RequireFeature(FeatureSSO)`. Mount the SSO routes too — they
   share the issuer. **`BreakGlassWriteHandler` and `BreakGlassLoginHandler`
   must share ONE `*LoginRateLimiter`**; two instances means clear-lockout
   resets a map nobody reads and reports success while clearing nothing.
7. **Config** — `pkg/config` has no break-glass fields at all. Adding
   required config is a **k8s-first** change: env lands before the code that
   reads it, or the service crash-loops. Grep every reader, not just `Validate()`.
8. **Provision + verify end to end.** Zero accounts exist. Prove the route is
   **absent before and present after** — a GET on a POST-only route answers
   405 when mounted and 404 when not, which distinguishes the two without
   attempting a login.

These five were not re-verified symbol by symbol the way Tasks 0–3 were.
Treat their detail as probable, not established, and check before executing
each.

## Then #404

Its code is merged (#641) and deliberately unmounted. Once the above is real,
mounting it is the one-line dependency fill the issue describes — plus the
shared-rate-limiter wiring in Task 6, which is the part that silently breaks.
