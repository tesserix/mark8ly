# Break-glass session consumer audit (Task 0, #642)

Gates Tasks 1–8 of `docs/superpowers/plans/2026-09-07-break-glass-activation-v2.md`.

Two things under audit:

- **D1** — adding `AuthContext string \`json:"auth_context,omitempty"\`` to
  `services/auth-bff/internal/session/cookie.go:41-53`.
- **D3** — a `Session.UID` of `BreakGlassUserID(tenantID)`
  (`services/marketplace-api/internal/handlers/admin/break_glass_login.go:251-258`)
  that resolves to no user row anywhere.

## Headline

**D1 is safe. D3 is not.** Adding the field breaks nothing — no decoder in the
estate rejects unknown fields, and `/auth/session` hands out a hand-written
allow-list that the new field cannot leak into. But a non-resolving UID is
refused by the two authorization gates every admin request passes through, so a
break-glass login that mints a cookie today produces a session that **cannot
render a single admin page and cannot call a single admin API route**. This is
not a corner case; it is the main path.

## Scope of the search

- The cookie is AES-GCM sealed by a key only auth-bff holds
  (`cookie.go:106-114`, `pkg/config/config.go:38`). `SESSION_ENCRYPT_KEY` does
  appear in `apps/admin` and `apps/storefront`, but only to HMAC handoff /
  exchange codes (`apps/admin/app/auth/handoff/route.ts:28`,
  `apps/storefront/lib/auth/join-grant.ts:63`) — **nothing outside auth-bff
  decodes the session cookie**. Every other consumer sees either the
  `/auth/session` JSON or the `X-User-Id` / `X-Tenant-Id` / `X-User-Email`
  headers derived from it.
- Grepped `-e m8_session -e SESSION_COOKIE_NAME -e session.Manager -e /auth/session`
  across `services/{auth-bff,marketplace-api,otto,platform-api}` and
  `apps/{admin,storefront,onboarding,mobile-admin,mobile-storefront,storefront-mobile}`
  plus `packages/`.
- Grepped `-e X-User-Id -e X-User-ID -e X-Tenant-Id -e X-Tenant-ID -e X-User-Email`
  across the same tree.

## Verdict table

Legend: **OK** = unaffected · **DEGRADES** = works, loses fidelity · **BREAKS** = refuses the request.

| # | Consumer | A: assumes UID resolves | B: assumes real mailbox | C: unknown JSON field | D: reads IdP token |
|---|---|---|---|---|---|
| 1 | `auth-bff` `Manager.encode/decode` — `internal/session/cookie.go:222,240` | OK | OK | **OK** — `json.Unmarshal`, no `DisallowUnknownFields` | OK — struct has no token field |
| 2 | `auth-bff` `GET /auth/session` — `internal/session/handler.go:126-158` | OK | OK | OK — emits `sessionResponse` (`:113-118`), a 4-field allow-list | OK |
| 3 | `auth-bff` `POST /auth/logout` — `internal/session/handler.go:271-306` | OK — `RevokeAllForUser` on a TEXT column, no FK (`migrations/0002_user_sessions.up.sql:10`) | OK — email is an audit label only | OK | OK |
| 4 | `auth-bff` `POST /auth/switch-tenant` — `internal/session/handler.go:437-501` | **BREAKS** — `CheckMembership` at `:460` | OK | OK | OK |
| 5 | `auth-bff` `POST /auth/switch-store` — `internal/session/handler.go:518-552` | OK — no FGA check by design | OK | **DEGRADES** — silently drops `AuthContext` on re-mint | OK |
| 6 | `auth-bff` `GET/DELETE /api/v1/sessions` — `internal/session/handler.go:322-410` | OK — returns an empty list | OK | OK | OK |
| 7 | `auth-bff` `GET /auth/me/providers` — `internal/session/providers_handler.go:126-132` | **BREAKS** — explicit `user_not_found` 404 | OK | OK | OK |
| 8 | `auth-bff` admin cross-TLD handoff — `internal/adminhandoff/handler.go:112-142` | **BREAKS** — `CheckMembership` at `:113` | OK | **DEGRADES** — rebuilds `Session` at `:135` without `AuthContext` | OK |
| 9 | `auth-bff` user MFA — `internal/usermfa/handler.go:41-52` | OK — `user_mfa` keyed on an opaque id | OK — email only labels the TOTP QR | OK | OK |
| 10 | `apps/admin` `middleware.ts` role gate — `middleware.ts:407-435` | **BREAKS** — `/internal/tenants/{t}/me?uid=` → no role → `redirectToLogin` | OK | OK — plain `as SessionResponse` cast (`:70-77`), no runtime validator | OK |
| 11 | `apps/admin` `lib/auth/session.ts:44-67` | OK | OK | OK — same untyped cast | OK |
| 12 | `apps/admin` `lib/auth/serverSession.ts:63-101` | DEGRADES — `listMemberTenants(userId)` returns `[]`, switcher hides | OK | OK | OK |
| 13 | `apps/admin` header forwarders (`lib/api/marketplace-api.ts:304-310`, `settings-tier2-api.ts:34-55`, `webhooks.ts:95`, `warehouses-api.ts:81`, `shipping-api.ts:114`, `campaigns-api.ts:17`, `settings-api.ts:162`, `app/api/admin/otto/_proxy.ts:168`, `app/api/otto-platform/[...path]/route.ts:58-68`) | OK — pass-through | OK — omits the header when blank | OK | OK |
| 14 | `marketplace-api` `auth.HeaderTrustAuth` — `internal/auth/middleware.go:24-46` | OK — non-empty check only | OK — `:41` treats email as optional | OK | OK |
| 15 | `marketplace-api` `authz.Middleware.RequireTenantRelation` — `internal/authz/middleware.go:34-56` | **BREAKS** — FGA `Check` → 404 on every admin route | OK | OK | OK |
| 16 | `marketplace-api` audit + actor strings (`internal/audit/emitter.go:367`, `handlers/admin/promo.go:121`, `orders.go:453`, `returns.go:341`, `refund.go:86`, `team.go:101`, …) | OK — opaque actor string | DEGRADES — NULL `actor_email` | OK | OK |
| 17 | `marketplace-api` account → auth-bff proxy — `internal/handlers/admin/account.go:478-496,535-537` | OK | DEGRADES — MFA QR labels with the opaque id | OK | OK |
| 18 | `marketplace-api` uuid-parsing handlers — `apikeys_handler.go:245`, `push_tokens.go:34,57`, `billing/migration/handler.go:228` | OK — a UUIDv5 parses fine | OK | OK | OK |
| 19 | `otto` `StaffAuth` / `CustomerContext` — `internal/auth/middleware.go:29-56,85-110` | OK — header trust, no lookup; `user_id` is only a message author id (`internal/conversation/admin_handler.go:48,561`) | OK — `:43` optional | OK | OK |
| 20 | `platform-api` `getMe` / `listMyTenants` — `internal/tenant/handler.go:162-188,213-221` | **BREAKS** — `GetRole` returns `""` → 404 `no_role` | OK | OK | OK |
| 21 | `apps/storefront`, `apps/onboarding`, `apps/mobile-admin`, `apps/mobile-storefront`, `apps/storefront-mobile` | n/a — **non-consumers**, see below | n/a | n/a | n/a |

## The negatives, and how they were established

- **C — no strict decoder anywhere.** `grep -rn --include='*.go' 'DisallowUnknownFields' .`
  over the whole repo returns **zero** hits. `grep -rn -e '\.strict()' -e 'strictObject'`
  over `apps/`, `packages/`, `services/` (node_modules excluded) returns **zero**.
  `grep -e additionalProperties -e valibot -e superstruct -e 'io-ts'` returns zero;
  `-e ErrorUnused -e UnmarshalStrict` over `services/` returns zero. Zod is present
  (`apps/admin/lib/api/subscription/schemas/*.ts`, `components/auth/*.tsx`) but no
  schema is applied to the session shape and none is `.strict()`. Both TS session
  decoders are unvalidated casts (`middleware.ts:70-77`, `lib/auth/session.ts:59`).
- **D — there is no IdP token on the session to read.** The `Session` struct
  (`cookie.go:41-53`) has no access/ID/refresh field. The estate's only token
  exchange lives in `services/auth-bff/internal/zitadellogin/token_exchange.go:34-58`
  and returns tokens **in the login response body** for the mobile bearer path — it
  never writes them to the cookie. The design spec's §7 risk ("a caller assumes
  `session-exchange` returns a non-empty access_token") names a symbol that does not
  exist; `grep -rn 'session-exchange'` finds no such route. **The risk is void**, and
  no fix is required for D on any consumer.
- **No DB will reject a synthetic id.** `grep -rn 'REFERENCES' --include='*.sql'`
  in `services/marketplace-api/migrations` and `services/otto` yields no foreign key
  to any user/staff table; `user_sessions.user_id` is `TEXT NOT NULL` with no FK
  (`services/auth-bff/migrations/0002_user_sessions.up.sql:10`). Breakage is
  authorization-layer, not storage-layer.
- **No mail is sent to the session email.** `grep -rn -e SendEmail -e sendMail`
  over `services/marketplace-api/internal` and `services/auth-bff/internal` finds
  one unrelated webhook `notify`. `grep -e GetByEmail -e FindByEmail -e 'WHERE email'`
  finds only `internal/campaign/segment_engine.go:98` (customer segments, not staff).
  Every session-email read found is an attribution label with an explicit fallback
  (`tickets.go:216-228`, `loyalty.go:179-182`, `middleware.go:38-42`).
- **Only `apps/admin` consumes the cookie.** `grep -e m8_session -e SESSION_COOKIE_NAME`
  over `apps/storefront`, `apps/onboarding`, `apps/mobile-admin`,
  `apps/mobile-storefront`, `apps/storefront-mobile`: **zero non-test hits**.
  `grep -e '/auth/session'` over the same: zero. Storefront has its own
  `mp_customer_session`; the mobile apps authenticate with Zitadel bearer tokens
  verified in `marketplace-api/internal/auth/zitadel_verifier.go`, a path a cookie
  never enters.

## Consumers that break

### 10 + 20 — the admin role gate (BLOCKING)

`apps/admin/middleware.ts:407-435` calls
`GET {PLATFORM_API_URL}/internal/tenants/{session.tenant_id}/me?uid={session.user_id}`
on every authenticated request. `platform-api`'s `getMe`
(`services/platform-api/internal/tenant/handler.go:162-188`) answers from OpenFGA:

```go
role, err := h.fga.GetRole(c.Request.Context(), uid, tenantID)
...
if role == "" { c.JSON(http.StatusNotFound, gin.H{"error": "no_role", ...}) }
```

A break-glass UID has no FGA tuple, so `role` is `""` → 404 → `role` stays `null`
in middleware → `middleware.ts:431` `return redirectToLogin(req)`. The merchant
completes a correct dual-factor break-glass login, receives a valid cookie, and is
bounced straight back to `/login` — a loop, with no error that names the cause.

Note `handler.go:171-174`: with `fga == nil` (dev) it returns `role: owner`
unconditionally. So this **passes in local dev and fails only in production** —
exactly the shape that survives to deploy.

**Fix.** Write the FGA tuple. At break-glass provisioning (or at successful login,
before `Sessions.Issue`), write `user:{BreakGlassUserID(tenant)}` → `owner` (or a
dedicated `break_glass` role) on `tenant:{tenantID}`. One additive tuple, matching
the estate's existing "tenant ownership is a tuple" pattern, and it fixes #10, #15,
#20, #4 and #8 in one move. The alternative — special-casing `auth_context` in the
role gate — requires `/auth/session` to expose `auth_context` (see below) and adds
an authorization bypass branch to the hottest path in the admin app. Prefer the tuple.

### 15 — every admin API route (BLOCKING)

`services/marketplace-api/internal/authz/middleware.go:41`:

```go
ok, err := m.client.Check(c.Request.Context(), userID, string(role), tenantID)
```

`RequireTenantRelation` is attached per-route across the whole admin surface —
`internal/handlers/admin/routes.go:234-239` (products), `:258-270` (CSV imports),
`:165-180` (SSO config), `:184-187` (stores), `:196-219` (account), and the rest of
the `/admin/stores/:storeId` tree at `:230`. A break-glass UID fails `Check` and
gets **404 not_found** on all of them (deliberately 404, not 403, per §13.1.1) — so
even if the UI rendered, every panel would report "not found".

**Fix.** Same tuple as above. No code change needed here.

### 4 — `/auth/switch-tenant` (blocking in practice)

`internal/session/handler.go:460` runs `CheckMembership(existing.UID, req.TenantID)`
and 403s a non-member. `apps/admin/middleware.ts:376-400` calls this endpoint
whenever the host slug's tenant differs from the session tenant, and on failure
redirects to `/pick-tenant` — which is itself powered by `listMyTenants(uid)`
(`platform-api/internal/tenant/handler.go:213-221`), empty for break-glass. So the
recovery surface is empty too. The tuple fixes this as well.

Separately: **`switch-tenant` on a break-glass session should be refused outright**,
not merely fail. Once the tuple exists it would otherwise *succeed*, letting an
emergency credential scoped to one tenant walk to another. Gate it on
`AuthContext == "break_glass"` → 403.

### 5 + 8 — `AuthContext` is silently dropped on re-mint (BLOCKING for D1's purpose)

`switchStore` (`internal/session/handler.go:538-546`) and `switchTenant` (`:483-489`)
both build a fresh `Session` literal copying `UID`, `Email`, `TenantID`, `StoreID`,
`IssuedAt`, `ExpiresAt` — and nothing else. `adminhandoff` (`internal/adminhandoff/handler.go:135-142`)
does the same from its code claims, which carry no `auth_context`.

Adding `AuthContext` without touching these three sites gives a field that any
merchant can strip by calling `POST /auth/switch-store` once. Since D1's stated
purpose is "the spec's central safety control", a control that launders itself away
on a routine UI action is worse than none: it will be trusted and it will be wrong.

**Fix (must land in the same commit as D1).** Copy `AuthContext` in both
`switchTenant` and `switchStore`; add an `auth_context` claim to the admin-handoff
code (`internal/adminhandoff/code.go`) and carry it into the minted session, or
refuse handoff for a break-glass session. Add a test per site that a
`break_glass` session survives the round trip — three one-line struct fields is
exactly the kind of change nobody writes a test for and everybody regresses.

### 7 — `/auth/me/providers` (cosmetic)

`internal/session/providers_handler.go:126-132` returns 404 `user_not_found` when
GIP's `accounts:lookup` finds no record for `s.UID`, which is guaranteed for a
synthetic id. Only feeds `/settings/security`. **Fix:** none required; optionally
short-circuit to an empty provider list when `AuthContext == "break_glass"` so the
page renders instead of erroring.

## Two things the plan should record

1. **`SessionIssuer` carries no email.** `internal/authbffclient/session_issuer.go:34`
   is `Issue(ctx, tenantID, userID uuid.UUID, ttl)`. A break-glass session therefore
   has `Email == ""`. Every email consumer audited (rows 14, 16, 17, 19) treats it as
   optional with a fallback, so **empty is safe** — the only cost is NULL
   `actor_email` on audit rows, where `actor_user_id` already carries the stable
   synthetic id. Task 3's open question resolves in favour of **widening the
   interface** anyway, because `auth_context` (unlike email) is load-bearing and must
   not be defaulted by the callee.
2. **`/auth/session` does not expose `auth_context`.** `sessionResponse`
   (`internal/session/handler.go:113-118`) is a 4-field allow-list. This is why C is
   safe — but it also means **no TypeScript consumer can see that a session is
   break-glass**. If any UI must show an emergency-mode banner, or if the role gate
   is fixed by special-casing rather than by a tuple, `sessionResponse` and
   `apps/admin`'s three copies of the interface (`middleware.ts:70-77`,
   `lib/auth/session.ts:20-26`, and the `x-session-*` header set at
   `middleware.ts:487-491`) must be extended. That is additive and safe — no
   validator would reject it — but it is not free, and it is not in the plan.

## Overall verdict

**Adding `AuthContext`: SAFE to land**, provided it lands together with the field
copies in `switchTenant`, `switchStore`, and the admin-handoff mint. Go ignores
unknown fields; no Go decoder in the repo sets `DisallowUnknownFields`; no
TypeScript consumer validates the session shape at all; and the field cannot reach
a TS consumer regardless because `/auth/session` emits a hand-written allow-list.
Old cookies decode with an empty value, as D1 intends.

**A non-resolving `UID`: NOT SAFE. This blocks Tasks 2–8.** Three gates refuse it —
`platform-api getMe` (`internal/tenant/handler.go:176-182`),
`marketplace-api RequireTenantRelation` (`internal/authz/middleware.go:41-53`), and
`auth-bff switchTenant` / `adminhandoff` `CheckMembership`. The admin app bounces
the session to `/login`; the API 404s every route. The plan's Task 8 ("provision +
verify end to end") would fail at first render, and the dev-mode `fga == nil`
fallback at `handler.go:171-174` means it would pass locally first.

**Required before Task 2:**

1. Add a task — before Task 6's mount — that writes an FGA tuple for
   `BreakGlassUserID(tenantID)` on `tenant:{tenantID}`, at provisioning or at
   successful login. Without it break-glass does not work, however correct the
   cookie is.
2. In Task 1, copy `AuthContext` through `switchTenant` (`handler.go:483`) and
   `switchStore` (`handler.go:538`), and decide handoff (`adminhandoff/handler.go:135`)
   — carry it or refuse it. With a test each.
3. In Task 3, widen `SessionIssuer` to carry `auth_context` explicitly rather than
   letting `HTTPIssuer` default it. Empty email is fine and needs no widening on its
   own account; `auth_context` is the reason to widen.
4. Delete the design spec's §7 access_token risk. There is no token on the session
   and no `session-exchange` route; the risk describes a system that does not exist.
