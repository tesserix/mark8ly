# GIP Auth: Isolation, Storefront Google Sign-In, and Account Merge

**Date:** 2026-04-27
**Status:** Approved (design phase)
**Owner:** Mahesh

## Summary

Three coordinated GIP auth improvements:

1. **Account merge (admin + storefront)** — a user who signed up with password should be able to add Google as a second sign-in method on the same account, plus a settings page to manage linked providers.
2. **Storefront Google sign-in** — add the gsi/client + `signInWithIdp` flow to storefront create-account and sign-in pages (currently password-only).
3. **Isolation** — split admin and storefront into distinct session cookies; bind the customer cookie to a specific store host (including custom domains) so a customer cookie at `store-a` cannot authorize at `store-b` or at any admin host.

Customer identity model: **per-store**. Same email at `store-a.mark8ly.com` and `store-b.mark8ly.com` are two separate customer accounts.

## Non-goals

- Cross-store customer accounts ("one mark8ly login for all stores") — explicitly rejected; per-store accounts only.
- Migrating any existing users between stores or pools.
- Replacing GIP — the migration to GIP is locked, this design works within it.
- Changing the workspace tenant model on admin (admin merchants keep their multi-store membership via `tenants.owner_user_id` + `invitations`).

## Architecture

### Two distinct session cookies (both minted by auth-bff)

| Cookie | Domain | Used by | Carries |
|---|---|---|---|
| `m8a_session` | `.mark8ly.com` (parent — travels across `*-admin.mark8ly.com`) | admin app only | `uid`, workspace `tenant_id`, active `store_id` |
| `m8c_session` | exact request host (e.g. `store-a.mark8ly.com` or `shop.brand-a.com`) — request-driven, no leading dot | storefront only | `uid`, `tenant_id`, `store_id`, `customer_id` |

The browser is the enforcer — `m8c_session` cannot be sent to admin or to another store. The session struct grows a `Kind` field (`"admin" | "customer"`) + a `customer_id`. auth-bff refuses to mint or read a cookie whose `Kind` doesn't match the requesting endpoint (belt + suspenders on top of cookie scope).

### Two GIP audiences, validated separately

- `POST /auth/admin/auto-login` — verifies the GIP id_token belongs to **MP-Internal** tenant pool. Mints `m8a_session`.
- `POST /auth/customer/auto-login` — verifies the id_token belongs to **MP-Customer**, takes the request host (or `store_slug`), resolves to `store_id`/`tenant_id`, looks up or creates `customer_profiles` row, mints `m8c_session` for that exact host.

The current `POST /auth/auto-login` becomes a transparent alias to `/auth/admin/auto-login` for one release window, then is removed.

### Custom domain support

Cookie `Domain` for `m8c_session` is request-driven (not config-driven). The flow:

1. Storefront server action reads request `host` via `headers().get('host')`.
2. Calls `POST /auth/customer/auto-login` with `{ id_token, host }`.
3. auth-bff resolves `host` → `(tenant_id, store_id)` via the existing `tenant-router-service` (which already handles custom domains: see `marketplace_api.000014_custom_domains` + `000032_custom_domains_manual_method`).
4. auth-bff stamps `Set-Cookie: m8c_session=…; Domain=<host>; Path=/; HttpOnly; Secure; SameSite=Lax`.
5. Server action proxies the `Set-Cookie` into the response via `cookies().set()` — same parser that already exists in `apps/admin/app/login/actions.ts`.

This means `shop.brand-a.com` and `store-a.mark8ly.com` both work without config changes.

### Customer identity model

Already supported by the current schema:

- `marketplace_api.customer_profiles` is keyed `(store_id, email)` UNIQUE with nullable `gip_uid`.

One schema addition (Phase 1):

```sql
CREATE UNIQUE INDEX CONCURRENTLY customer_profiles_store_gip_uid_uq
ON customer_profiles (store_id, gip_uid)
WHERE gip_uid IS NOT NULL;
```

Partial unique so existing NULL rows don't conflict. `CONCURRENTLY` so it doesn't lock writes.

### Account merge layer

Sits on top of the cookie + audience changes. Uses GIP-native account linking:

- **GIP console**: enable "Link accounts that use the same email" on both `MP-Internal` and `MP-Customer` tenant pools.
- **Auto-merge handshake**: when `signInWithIdp` returns `needConfirmation: true` + `pendingIdToken`, the frontend renders a password-confirm overlay. User submits existing password → `signInWithPassword` succeeds → call `accounts:linkWithCredential` with the pending Google credential → providers are now linked → continue normal `autoLogin`.
- **Settings page** (`/settings/security` on admin, `/account/security` on storefront): lists linked providers, allows linking/unlinking with the "can't remove last provider" guard.

The password-confirm overlay is **not optional**. Without it, anyone who can sign in with Google to your email could take over your password account. This is the standard GIP-native flow.

### On-demand backfill

When an existing customer with NULL `gip_uid` (created via password signup) signs in with Google for the first time:

1. `/auth/customer/auto-login` looks up `customer_profiles` by `(store_id, email_from_id_token)`.
2. If found with NULL `gip_uid`, populate it with the new GIP UID.
3. Return success — no friction for the user.

Idempotent. No proactive script needed. If we ever want a one-time sweep, it can be a post-Phase-3 job.

## Components

### auth-bff (`services/auth-bff/`)

| File | Change |
|---|---|
| `internal/session/cookie.go` | `Session` struct gains `Kind` + `CustomerID`. `Mint` for customer takes `host` param. Two `Manager` instances: admin (fixed `.mark8ly.com`) + customer (request-driven Domain). |
| `internal/session/handler.go` | New routes `POST /auth/admin/auto-login` and `POST /auth/customer/auto-login`. Old `/auth/auto-login` aliases to admin for one release. New `GET /auth/me/providers` (proxies GIP `accounts:lookup`, gates on session.Kind). |
| `internal/gip/verifier.go` | Verify id_token audience matches the expected tenant pool (admin vs customer). |
| `migrations.go` | (none for auth-bff — schema lives in `marketplace_api`) |

### marketplace-api (`services/marketplace-api/`)

| File | Change |
|---|---|
| `migrations/000084_customer_profiles_gip_uid_uq.up.sql` | Add partial unique index. |
| `migrations/000084_customer_profiles_gip_uid_uq.down.sql` | Drop index. |
| Customer profile handler used by `/auth/customer/auto-login` | Lookup-or-create logic; backfill `gip_uid` on existing email match. |

### admin app (`apps/admin/`)

| File | Change |
|---|---|
| `middleware.ts` | Read `m8a_session` (new) + `m8_session` (old, fallback) for one release. |
| `app/login/actions.ts` | Call `/auth/admin/auto-login` (rename only). |
| `components/auth/SignInForm.tsx` | Wire `LinkProviderPrompt` for `needConfirmation` case. |
| `app/(admin)/settings/security/page.tsx` | New. Uses `LinkedProvidersPanel` from `@repo/ui`. |
| `lib/auth/auth-bff.ts` | `getProviders()`, `linkProvider()`, `unlinkProvider()` helpers. |

### storefront app (`apps/storefront/`)

| File | Change |
|---|---|
| `middleware.ts` | Read ONLY `m8c_session`. Validate `store_id` claim matches resolved store for current host. |
| `app/create-account/actions.ts` | Call `/auth/customer/auto-login` with request `host`. |
| `app/sign-in/actions.ts` | Same. |
| `lib/gip/google-gsi.ts` | New (port from admin). |
| `lib/gip/signup.ts` | New `signInWithGoogleCustomer(credential, tenantId)`. |
| `components/auth/CreateAccountForm.tsx` | Add Google button + handler. |
| `components/auth/CustomerSignInForm.tsx` | Add Google button + handler + `LinkProviderPrompt` integration. |
| `app/account/security/page.tsx` | New. Uses `LinkedProvidersPanel` from `@repo/ui`. |

### shared UI (`packages/ui/`)

| File | Change |
|---|---|
| `src/auth/LinkProviderPrompt.tsx` | New. Password-confirm overlay for `needConfirmation`. |
| `src/auth/LinkedProvidersPanel.tsx` | New. Lists providers, link/unlink buttons, last-provider guard. |

## Data flow

### Customer sign-in (Google, returning user)

```
Browser (store-a.mark8ly.com)
  ↓ click "Continue with Google"
gsi popup → Google credential
  ↓
storefront `signInWithIdp` → MP-Customer GIP pool
  ↓ id_token, uid
storefront server action `customerSignIn`
  ↓ POST { id_token, host: "store-a.mark8ly.com" }
auth-bff /auth/customer/auto-login
  ↓ verify id_token aud=MP-Customer
  ↓ resolve host → (tenant_id=…, store_id=…) via tenant-router-service
  ↓ lookup customer_profiles by (store_id, gip_uid) OR (store_id, email)
  ↓ backfill gip_uid if found by email with NULL gip_uid
  ↓ mint Session{Kind:"customer", uid, tenant_id, store_id, customer_id}
  ← Set-Cookie: m8c_session=…; Domain=store-a.mark8ly.com
storefront server action proxies Set-Cookie via cookies().set()
  ↓ redirect /account
```

### Account merge (admin password user adds Google)

```
Browser (admin.mark8ly.com)
  ↓ click "Continue with Google" (user signed up earlier with password)
gsi popup → Google credential
  ↓
admin `signInWithIdp` → MP-Internal GIP pool
  ↓ GIP returns { needConfirmation: true, pendingIdToken, email }
SignInForm renders LinkProviderPrompt
  ↓ user enters existing password
admin `signInWithPassword(email, password)`
  ↓ id_token, uid
admin `accounts:linkWithCredential(id_token, pendingIdToken)`
  ↓ providers linked, returns final id_token
admin server action `signIn`
  ↓ POST { idToken, uid } to /auth/admin/auto-login
auth-bff mints m8a_session
  ↓ redirect /dashboard
```

### Cookie isolation (browser-enforced)

```
m8c_session at store-a.mark8ly.com
  → request to store-b.mark8ly.com:    NOT SENT (Domain mismatch)
  → request to admin.mark8ly.com:      NOT SENT (Domain mismatch)
  → request to *-admin.mark8ly.com:    NOT SENT (Domain mismatch)
  → request to shop.brand-a.com:       NOT SENT (Domain mismatch)
  → request to store-a.mark8ly.com:    SENT ✓

m8a_session at admin.mark8ly.com
  → request to slug-admin.mark8ly.com: SENT ✓ (parent .mark8ly.com)
  → request to store-a.mark8ly.com:    SENT (browser allows) but auth-bff rejects (Kind mismatch on customer endpoints; admin endpoints don't exist on storefront)
```

The Kind discriminator handles the leak case where `m8a_session` is sent to a storefront host because of the parent-domain scope. Storefront middleware reads only `m8c_session` and never even looks at `m8a_session`.

## Error handling

- **GIP id_token wrong audience** → 401 with `{ code: "wrong_pool", message: "..." }`. Frontend treats as "sign-in failed" generically.
- **Host doesn't resolve to a store** (typo, deleted store, custom domain not yet active) → 404 from `/auth/customer/auto-login`. Frontend shows "This store isn't ready yet — try again later."
- **Cookie `store_id` mismatch** in storefront middleware → clear the cookie + redirect to `/sign-in`. Don't 401 (no friendly UX).
- **`needConfirmation` from GIP** → render `LinkProviderPrompt`. If user cancels, return to login form with a banner "Linking cancelled — try password sign-in instead."
- **Unlink last provider** → 422 with `{ code: "last_provider", message: "Add another sign-in method first." }`. Settings UI disables the button proactively.
- **Schema migration failure on prod** → `CREATE INDEX CONCURRENTLY` doesn't block writes; if it fails it can be retried. The partial `WHERE` clause means it can never violate uniqueness on existing data.

## Testing

Per memory `e2e_test_state.md` the project has an existing E2E suite. New journeys per phase:

### Phase 1 — `auth-isolation.spec.ts`
- Admin signs in at `admin.mark8ly.com` → `m8a_session` cookie set with `Domain=.mark8ly.com`.
- Customer signs in at `store-a.mark8ly.com` → `m8c_session` cookie set with `Domain=store-a.mark8ly.com`.
- Customer cookie not sent to `store-b.mark8ly.com` (browser test).
- Customer signed in at custom domain `shop.brand-a.com` → cookie `Domain=shop.brand-a.com`.
- Customer cookie tampered with `store_id` → middleware clears it and redirects.

### Phase 2 — `storefront-google-signin.spec.ts`
- First-time Google customer at `store-a.mark8ly.com` → new `customer_profiles` row created, `gip_uid` populated.
- Returning Google customer signs in cleanly.
- Existing password-only customer signs in with Google → `gip_uid` backfilled, no new row.

### Phase 3 — `account-merge-{admin,customer}.spec.ts`
- Sign up with password → sign out → sign in with Google → password-confirm overlay → linked → next sign-in (Google or password) both work.
- Cancel the password-confirm overlay → sign-in does not complete, banner shown.
- Settings page lists providers, link/unlink works, last-provider guard prevents removing only auth method.

### Unit tests
- `internal/session/cookie_test.go`: cookie kind enforcement, mint with request host, read with kind mismatch.
- Backfill logic: existing customer profile with NULL gip_uid + matching email → gip_uid populated, no new row.

## Sequencing & cutover

Three phases, shippable independently. Each phase is one logical commit batch (per "commit straight to main, no PRs" preference).

### Phase 1 — Isolation foundation

1. Schema migration (`000084` on `marketplace_api`, `CONCURRENTLY`).
2. auth-bff: new endpoints + new cookie kinds, old `/auth/auto-login` aliased to admin.
3. admin app: middleware reads both old + new cookie for one release; login action calls new endpoint.
4. storefront app: middleware reads only new cookie (clean cut — no existing per-host customer cookies); actions call new endpoint with host forwarding.

**Cutover behavior**: existing admin sessions keep working (alias). Existing storefront sessions invalidated → next visit lands on `/sign-in`.

**Rollback**: revert in reverse order. Schema migration is idempotent and safe to leave.

### Phase 2 — Storefront Google sign-in

1. Port `google-gsi.ts` to storefront.
2. Add Google button to `CreateAccountForm` + `CustomerSignInForm`.
3. Storefront server action handles the Google credential (calls customer endpoint already present from Phase 1).
4. **GIP console click**: confirm Google provider enabled on `MP-Customer` tenant pool.

**Rollback**: revert the storefront commits. No data migration involved.

### Phase 3 — Account merge + settings page

1. **GIP console click first** (one-time): enable "Link accounts that use the same email" on both `MP-Internal` and `MP-Customer` tenant pools.
2. `LinkProviderPrompt` in `@repo/ui` (shared between apps).
3. Wire prompt into both login forms (`needConfirmation` handling).
4. `GET /auth/me/providers` endpoint on auth-bff.
5. Settings pages on both apps using `LinkedProvidersPanel`.
6. On-demand customer backfill activates (built into Phase 1 endpoint).

**Rollback**: revert UI commits + flip GIP console setting back. Already-linked providers stay linked (GIP-side state).

## What backfill explicitly is NOT

We are NOT writing a script that proactively fills `gip_uid` for every existing `customer_profiles` row by reading GIP. That would require GIP admin SDK access keyed by email — slow, error-prone, and unnecessary because the on-demand path covers it the next time the user signs in.

If we ever want a proactive sweep, it can be a one-time job after Phase 3 lands.

## Out-of-band actions (console, not code)

- **GIP**: enable Google provider on `MP-Customer` tenant pool (verify only, may already be done from Phase 7b in deploy state).
- **GIP**: enable "Link accounts that use the same email" on both `MP-Internal` and `MP-Customer` tenant pools.
- **GCP OAuth client**: add storefront redirect URIs `https://*.mark8ly.com/api/auth/callback/google` and per-custom-domain entries as stores adopt them.

## Open questions

None remaining. All design decisions confirmed in the brainstorming session.

## References

- `docs/planning/04-auth-and-authz.md` — current auth model
- `docs/planning/auth-bugs.md` — historical fixes that informed cookie handling decisions
- `services/auth-bff/internal/session/cookie.go` — current cookie module
- `apps/admin/components/auth/SignInForm.tsx` — current admin sign-in (Google + password)
- `apps/storefront/components/auth/CustomerSignInForm.tsx` — current storefront sign-in (password only)
- `services/marketplace-api/migrations/000013_customer_profiles.up.sql` — current customer schema
- Memory: `mark8ly_deploy_state.md` (deploy state, GIP tenant pool names, cookie domain decision)
