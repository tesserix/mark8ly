# GIP Auth: Isolation, Storefront Google Sign-In, and Account Merge

**Date:** 2026-04-27 (revised after pre-flight discovery)
**Status:** Approved (design phase)
**Owner:** Mahesh

## Summary

Three coordinated GIP auth improvements:

1. **Account merge (admin + storefront)** — a user who signed up with password should be able to add Google as a second sign-in method on the same account, plus a settings page to manage linked providers.
2. **Storefront Google sign-in** — add the gsi/client + `signInWithIdp` flow to storefront create-account and sign-in pages (currently password-only).
3. **Isolation** — bind the storefront customer cookie to the specific store host (including custom domains) so a customer cookie at `store-a` cannot be sent to `store-b` or to admin. Admin and storefront already use distinct cookie names; the actual gap is the customer cookie's parent-domain scope.

Customer identity model: **per-store**. Same email at `store-a.mark8ly.com` and `store-b.mark8ly.com` are two separate customer accounts.

## Discovered architecture (post pre-flight)

The original design assumed both admin and storefront customer auth flow through `auth-bff`. They do not. Storefront customer auth is intentionally Next.js-side, sidestepping auth-bff because customers don't have OpenFGA tenant membership tuples. The two paths are already separate:

| | Admin | Storefront customer |
|---|---|---|
| Cookie name | `m8_session` | `mp_customer_session` |
| Cookie format | AES-GCM encrypted (auth-bff) | HMAC-signed `<base64>.<hex>` (storefront) |
| Cookie Domain (today) | `.mark8ly.com` ✓ correct | `.mark8ly.com` ✗ should be per-host |
| Minted by | `auth-bff POST /auth/auto-login` | `apps/storefront/app/sign-in/actions.ts customerSignIn` |
| GIP id_token verifier | `services/auth-bff/internal/gip/verifier.go` | `apps/storefront/lib/gip/verify-id-token.ts` (already uses `GIP_CUSTOMER_TENANT_ID`) |
| Per-store scope check | n/a | `decodeSessionForScope({storeSlug})` already validates `store_slug` claim |
| Customer profile creation | n/a | Auto-created by marketplace-api `OptionalCustomerAuth` middleware on `GET /account` |
| OpenFGA membership check | yes | no |

**Implication**: app isolation (admin vs storefront) is already achieved via distinct cookie names. The remaining gap is **cross-store leak on the customer cookie** because Domain is the parent `.mark8ly.com`. The design now respects the existing intentional separation.

## Non-goals

- Cross-store customer accounts ("one mark8ly login for all stores") — explicitly rejected; per-store accounts only.
- Migrating any existing users between stores or pools.
- Replacing GIP — the migration to GIP is locked, this design works within it.
- Routing customer auth through auth-bff. The two-system separation is intentional and stays.
- Renaming `m8_session` to `m8a_session`. Cosmetic; does not fix any real isolation bug.

## Architecture

### Customer cookie: per-host Domain (the actual fix for cross-store isolation)

Today `apps/storefront/app/sign-in/actions.ts:160` sets:

```ts
c.set({ name: "mp_customer_session", domain: ".mark8ly.com", ... })
```

Replace with the request host (sanitized), no leading dot:

```ts
const host = sanitizeHost((await headers()).get("host"));
if (!host) return { ok: false, code: "invalid_host", message: "..." };
c.set({ name: "mp_customer_session", domain: host, ... })
```

Browser-enforced consequence:
- Cookie set at `store-a.mark8ly.com` is NOT sent to `store-b.mark8ly.com`, NOT sent to `admin.mark8ly.com`, NOT sent to `*-admin.mark8ly.com`.
- Cookie set at `shop.brand-a.com` (custom domain) works the same way — Domain stamping is request-driven, not config-driven.
- `decodeSessionForScope({storeSlug})` becomes belt + suspenders (server-side double-check).

The `/sign-out` route (`apps/storefront/app/sign-out/page.tsx`) must clear the cookie with the **same** Domain it was set on, otherwise the browser sets a second deletion cookie and leaves the original alive. This means sign-out also needs the request host.

### Admin cookie: unchanged

Admin uses `m8_session` (AES-GCM), Domain `.mark8ly.com`, minted by auth-bff. This is correct because admin spans `admin.mark8ly.com` + every `*-admin.mark8ly.com` for the merchant's stores. No changes needed for isolation.

### Custom domain support

Already implicit in the per-host approach. The storefront server action reads the inbound `host` header (works for `*.mark8ly.com` and custom domains alike). The existing `tenant-router-service` + `marketplace_api.000014_custom_domains` already resolve custom-domain → store, so `resolveStore(storeSlug)` keeps working.

### Customer identity model — already supported

`marketplace_api.customer_profiles` is keyed `(store_id, email)` UNIQUE with nullable `gip_uid`. One schema addition (Phase 1):

```sql
CREATE UNIQUE INDEX CONCURRENTLY customer_profiles_store_gip_uid_uq
ON customer_profiles (store_id, gip_uid)
WHERE gip_uid IS NOT NULL;
```

Partial unique so existing NULL rows don't conflict. `CONCURRENTLY` so it doesn't lock writes.

### Storefront Google sign-in

Pure additive on top of the existing flow.

- Port `apps/admin/lib/gip/google-gsi.ts` → `apps/storefront/lib/gip/google-gsi.ts`.
- Add `signInWithGoogleCustomer(credential, tenantId)` in `apps/storefront/lib/gip/signup.ts` that POSTs to GIP REST `accounts:signInWithIdp`.
- Add Google button to `CreateAccountForm.tsx` and `CustomerSignInForm.tsx`.
- Returning Google customer: existing `customerSignIn` handles cookie + EnsureProfile.
- First-time Google customer: same path — marketplace-api's `OptionalCustomerAuth` calls `EnsureProfile` which creates a new `customer_profiles` row keyed by `(store_id, email)` with `gip_uid` populated from the verified token. Already works.

### Account merge

Native GIP linking on both sides:

- **Admin (auth-bff path)**: in `apps/admin/components/auth/SignInForm.tsx`, when `signInWithIdp` returns `needConfirmation: true` + `pendingIdToken`, render the **password-confirm overlay** (`@repo/ui/auth/LinkProviderPrompt`). User submits existing password → `signInWithPassword` succeeds → call GIP REST `accounts:signInWithIdp` again with `pendingIdToken` + the password id_token via the linking flow → providers linked → continue normal admin `autoLogin`.
- **Storefront (Next.js path)**: same `LinkProviderPrompt` component, same handshake, but the post-link continuation calls `customerSignIn` instead of admin `autoLogin`.
- **Settings pages**: `/settings/security` (admin) and `/account/security` (storefront) list linked providers, allow link/unlink with the "can't remove last provider" guard. Backend lookup via GIP REST `accounts:lookup`.

The password-confirm overlay is **not optional**. Without it, anyone who can sign in with Google to your email could take over your password account.

GIP console: enable "Link accounts that use the same email" on both `MP-Internal` and `MP-Customer` tenant pools. One-time toggle.

### On-demand backfill (existing password-only customers)

Already happens via the existing `EnsureProfile` middleware in marketplace-api: when a customer with `gip_uid IS NULL` exists for `(store_id, email)` and signs in with Google, the lookup-by-email path matches the row and the profile gets updated with the new `gip_uid`. No new code path needed in this design — but Phase 1's partial unique index ensures we never create a duplicate during the race window.

## Components

### marketplace-api (`services/marketplace-api/`)

| File | Change |
|---|---|
| `migrations/000084_customer_profiles_gip_uid_uq.up.sql` + `.down.sql` | Add partial unique index. |

(No handler changes — `EnsureProfile` already does lookup-or-create.)

### storefront (`apps/storefront/`)

| File | Change |
|---|---|
| `lib/host.ts` | New. `sanitizeHost(raw: string \| null \| undefined): string \| null` — strips port, validates hostname. |
| `lib/host.test.ts` | New. Coverage for valid/invalid hosts and custom domains. |
| `app/sign-in/actions.ts` | Use sanitized request host for cookie Domain. Reject if host invalid. |
| `app/sign-out/page.tsx` | Delete cookie using sanitized request host (must match the set Domain). |
| `lib/gip/google-gsi.ts` | New (Phase 2). Port from admin. |
| `lib/gip/signup.ts` | New (Phase 2). `signInWithGoogleCustomer(credential, tenantId)`. |
| `components/auth/CreateAccountForm.tsx` | Add Google button + handler (Phase 2). Wire `LinkProviderPrompt` (Phase 3). |
| `components/auth/CustomerSignInForm.tsx` | Add Google button + handler (Phase 2). Wire `LinkProviderPrompt` (Phase 3). |
| `app/account/security/page.tsx` | New (Phase 3). Uses `LinkedProvidersPanel`. |

### admin (`apps/admin/`)

| File | Change |
|---|---|
| `components/auth/SignInForm.tsx` | Wire `LinkProviderPrompt` for `needConfirmation` (Phase 3). |
| `app/(admin)/settings/security/page.tsx` | New (Phase 3). Uses `LinkedProvidersPanel`. |
| `lib/auth/auth-bff.ts` | `getProviders()`, `linkProvider()`, `unlinkProvider()` helpers (Phase 3). |

(Cookie + endpoint stay as-is.)

### auth-bff (`services/auth-bff/`)

| File | Change |
|---|---|
| `internal/session/handler.go` | New `GET /auth/me/providers` endpoint (Phase 3). Proxies GIP `accounts:lookup`. |

(No cookie or autologin refactor — the existing flow stays.)

### shared UI (`packages/ui/`)

| File | Change |
|---|---|
| `src/auth/LinkProviderPrompt.tsx` | New (Phase 3). Password-confirm overlay for `needConfirmation`. |
| `src/auth/LinkedProvidersPanel.tsx` | New (Phase 3). Lists providers, link/unlink, last-provider guard. |

## Data flow

### Customer sign-in (post-fix, password)

```
Browser (store-a.mark8ly.com)
  ↓ POST email + password
gsi REST signInWithPassword (customer pool)
  ↓ id_token, uid
storefront server action customerSignIn
  ↓ verifyGIPIdToken(idToken, projectId, customerTenantId)
  ↓ resolve store from slug (existing)
  ↓ encodeSession({uid, email, store_slug, store_id, tenant_id})
  ↓ host = sanitizeHost((await headers()).get("host"))
  ↓ cookies().set({name: "mp_customer_session", domain: host, ...})
  ↓ ensureCustomerProfile() (existing — calls marketplace-api)
  ↓ ensureLoyaltyEnrollment() (existing)
  ↓ return ok
```

### Cookie isolation (browser-enforced)

```
mp_customer_session set at store-a.mark8ly.com (Domain=store-a.mark8ly.com)
  → request to store-a.mark8ly.com:    SENT ✓
  → request to store-b.mark8ly.com:    NOT SENT (Domain mismatch)
  → request to admin.mark8ly.com:      NOT SENT (Domain mismatch)
  → request to *-admin.mark8ly.com:    NOT SENT (Domain mismatch)
  → request to shop.brand-a.com:       NOT SENT (Domain mismatch)

m8_session set at admin.mark8ly.com (Domain=.mark8ly.com)
  → request to *-admin.mark8ly.com:    SENT ✓ (parent .mark8ly.com)
  → request to storefront hosts:       SENT (browser allows) but storefront does not read m8_session — never authorizes
```

### Account merge (admin password user adds Google)

```
Browser (admin.mark8ly.com)
  ↓ click "Continue with Google"
gsi popup → Google credential
  ↓
admin signInWithIdp → MP-Internal pool
  ↓ GIP returns { needConfirmation: true, pendingIdToken, email }
SignInForm renders LinkProviderPrompt
  ↓ user enters existing password
admin signInWithPassword(email, password)
  ↓ id_token, uid
admin link credentials (GIP REST signInWithIdp w/ pendingIdToken)
  ↓ providers linked
admin server action signIn
  ↓ POST { idToken, uid } to /auth/auto-login (auth-bff, unchanged)
auth-bff mints m8_session
```

## Error handling

- **Storefront `mp_customer_session` minted with bad host** → server action returns `{ ok: false, code: "invalid_host" }`; UI shows generic "Sign-in failed, try again."
- **Storefront cookie sent with stale Domain (pre-fix `.mark8ly.com`)** → on next sign-in we mint with the correct per-host Domain; the old cookie is shadowed by the new one. Stale cookies on other stores naturally expire (30 days) or get cleared on next visit. No active eviction needed.
- **Sign-out on a host different from where the cookie was set** → can happen if a stale `.mark8ly.com` cookie reaches `/sign-out` on a different store. Workaround: sign-out also issues a `Domain=.mark8ly.com` clear as a transitional safety net for one release window, then drop.
- **GIP `needConfirmation`** → `LinkProviderPrompt` overlay (Phase 3). Cancel returns to the login form with a banner.
- **Schema migration** → `CREATE INDEX CONCURRENTLY` doesn't block writes; the partial `WHERE` clause means it cannot violate uniqueness on existing data.

## Testing

### Phase 1 — `auth-isolation.spec.ts` (Playwright)
- Customer signs in at `store-a.mark8ly.com` → `mp_customer_session` Domain is exactly `store-a.mark8ly.com`, not `.mark8ly.com`.
- Customer cookie not sent to `store-b.mark8ly.com` (browser-level assertion via `context.cookies(url)`).
- Customer cookie not sent to `admin.mark8ly.com`.
- Custom domain: customer signs in at `shop.brand-a.com` → cookie Domain is `shop.brand-a.com`.
- Sign-out at the same host clears the cookie cleanly.
- (Optional, Phase 1.5) Stale `.mark8ly.com` cookie + new sign-in → only the per-host cookie remains in browser store.

### Phase 2 — `storefront-google-signin.spec.ts`
- First-time Google customer at `store-a.mark8ly.com` → new `customer_profiles` row with `gip_uid` set.
- Returning Google customer → no new row, signs in cleanly.
- Existing password-only customer signs in with Google → `gip_uid` backfilled on the existing row, no duplicate (partial unique index from Phase 1 prevents it).

### Phase 3 — `account-merge-{admin,customer}.spec.ts`
- Sign up with password → sign out → sign in with Google → password-confirm overlay → linked → next sign-in (Google or password) both work.
- Cancel the password-confirm overlay → no link, banner shown.
- Settings page lists providers, link/unlink works, last-provider guard blocks removing the only auth method.

### Unit tests
- `apps/storefront/lib/host.test.ts` (Phase 1): port stripping, invalid char rejection, custom domain acceptance.
- Existing `apps/storefront/lib/session.test.ts` (if present, otherwise add): scope check still rejects mismatched `store_slug`.

## Sequencing & cutover

Three phases, shippable independently. Each phase is one logical commit batch (per "commit straight to main, no PRs" preference).

### Phase 1 — Per-host customer cookie + schema integrity

1. Schema migration `000084` (partial unique on `(store_id, gip_uid)`).
2. Storefront `lib/host.ts` sanitizer.
3. Storefront `customerSignIn` mints with per-host Domain.
4. Storefront `/sign-out` deletes with per-host Domain (+ transitional `.mark8ly.com` clear for one release).
5. E2E `auth-isolation.spec.ts`.

**Cutover**: existing customer cookies (Domain `.mark8ly.com`) keep working until the user signs out OR until they sign in again (which mints over the top with the per-host Domain). No forced re-sign-in. Stale `.mark8ly.com` cookies effectively become useless across stores after their owner re-authenticates anywhere.

**Rollback**: `git revert` per task. Schema migration is idempotent and safe to leave.

### Phase 2 — Storefront Google sign-in

1. Port `google-gsi.ts` to storefront.
2. Add `signInWithGoogleCustomer` GIP REST helper.
3. Add Google button to `CreateAccountForm` + `CustomerSignInForm`.
4. **GIP console**: confirm Google provider enabled on `MP-Customer` tenant pool.

**Rollback**: revert UI commits. No data migration involved.

### Phase 3 — Account merge + settings page

1. **GIP console**: enable "Link accounts that use the same email" on both `MP-Internal` and `MP-Customer` pools.
2. `LinkProviderPrompt` in `@repo/ui` (shared).
3. Wire prompt into both apps' sign-in forms (`needConfirmation` handling).
4. New `GET /auth/me/providers` endpoint on auth-bff (admin side); equivalent storefront-side helper that calls GIP REST `accounts:lookup` directly via the customer id_token.
5. Settings pages on both apps using `LinkedProvidersPanel`.

**Rollback**: revert UI commits + flip GIP console back. Already-linked providers stay linked (GIP-side state).

## What backfill explicitly is NOT

We are NOT writing a script that proactively fills `gip_uid` for every existing `customer_profiles` row. The on-demand path (existing `EnsureProfile`) covers it the next time the user signs in.

If we ever want a proactive sweep, it can be a one-time job after Phase 3 lands.

## Out-of-band actions (console, not code)

- **GIP**: enable Google provider on `MP-Customer` tenant pool (verify only, may already be done from Phase 7b in deploy state).
- **GIP**: enable "Link accounts that use the same email" on both `MP-Internal` and `MP-Customer` tenant pools.
- **GCP OAuth client**: add storefront redirect URIs `https://*.mark8ly.com/api/auth/callback/google` and per-custom-domain entries as stores adopt them.

## Open questions

None remaining.

## References

- `apps/storefront/app/sign-in/actions.ts` — current customer sign-in (Next.js side, NOT auth-bff)
- `apps/storefront/app/sign-out/page.tsx` — current sign-out (cookie delete)
- `apps/storefront/lib/session.ts` — HMAC-signed cookie + scope checker
- `apps/admin/components/auth/SignInForm.tsx` — current admin sign-in (Google + password, via auth-bff)
- `services/marketplace-api/migrations/000013_customer_profiles.up.sql` — current customer schema
- `services/auth-bff/internal/session/cookie.go` — admin cookie module (unchanged in this design)
- Memory: `mark8ly_deploy_state.md` — deploy state, GIP tenant pool names

## Revision history

- **2026-04-27 (initial)**: Assumed unified auth-bff for admin + customer. Pre-flight discovered storefront customer auth runs Next.js-side independently. Architecture revised to respect existing separation; Phase 1 reduced from 15 tasks to ~5.
