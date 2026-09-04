# Zitadel Phase 3c-2b — Make Admin Google Sign-In Reachable

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close #646 — admin Google sign-in through Zitadel exists but is unreachable in production. Make it work, without duplicating tenant-selection logic.

**Architecture:** The merchant `idp/finish` endpoint currently demands `workspace_tenant` up front, but with Google the identity is unknown until after the redirect. Rather than deriving the tenant from the request host (which contradicts the canonical-host login page), `finish` will create the Zitadel session from the intent and, when no tenant was supplied, return that session plus the identity — the same shape the password path already uses for its TOTP step-up. The admin app then resolves the tenant with its existing `resolveWorkspaceTenant` and calls a complete endpoint. Tenant selection stays in one place.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md`
**Issue:** #646

## Why not the alternatives

- **Deriving the tenant from the host** is what the parked branch did. `/login` renders only on canonical `admin.mark8ly.com`, so `tenantIdForHostSlug`'s `{slug}-admin` regex never matches and every attempt ends in `store_not_found`.
- **Resolving membership inside `auth-bff`** would re-implement `resolveWorkspaceTenant` in Go. Its docstring says it is deliberately "shared by both the GIP (`signIn`) and Zitadel (`signInWithZitadel`) paths so they cannot diverge on which tenant they pick" — a second copy recreates exactly that risk, including the subdomain refinement and the multi-tenant picker.
- **A tenant hint in the return URL** has nothing to carry: the merchant has not identified themselves when they click the button.

## Global Constraints

- **The `user` query param is NEVER an identity.** It rides in a URL the browser followed. Identity comes only from retrieving the intent.
- **The IDP stays pinned** to the configured Google IDP, checked before any lookup, link, or session creation.
- **The verified-email gate is absolute.** `EmailVerified` defaults to false when the claim is absent; absent means refuse.
- **The merchant path stays LINK-ONLY.** It must never create a user — merchant authorization is FGA tenant membership keyed by user id, so a fresh user can never be a member.
- `internalauth` is the first statement of every endpoint, before the body is read.
- No secret, session token, intent token, or intent id in any log line.
- The flag rule is exactly `NEXT_PUBLIC_AUTH_PROVIDER === "zitadel"`; with it unset, GIP behaviour is byte-identical.
- **`npm run build` must pass** — the `"use server"` async-only export rule is invisible to vitest and tsc.
- Test key literals stay low-entropy (`"thirtytwo-bytes-for-testing-only"`).

---

### Task 1: `finish` returns a session when no tenant was supplied

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/handler.go`, and its tests

Today `idpFinish` requires `workspace_tenant` and completes in one call. Change it so that when `workspace_tenant` is **absent**, it still does everything up to and including creating the Zitadel session from the intent — retrieve, pin the IDP, verify the email, link to the existing account — and then returns `{"tenant_required": true, "session_id": ..., "session_token": ..., "login_name": ...}` instead of completing. When `workspace_tenant` **is** supplied, behaviour is exactly as today.

Mirror the existing `totp_required` response shape (`handler.go` around the `OutcomeFactorRequired` case) — this is the same idea: a session exists, something else is still needed before completion.

`login_name` must be the email from the retrieved identity, never anything the caller supplied.

- [ ] **Step 1: Write the failing tests** — absent tenant returns the new shape and does NOT mint a session cookie or finalize; present tenant behaves exactly as before; an unverified email still refuses before any of it; an intent from another IDP still refuses; a link-only refusal (`no_admin_account`) still happens before the new branch.
- [ ] **Step 2: Run them, confirm they fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: `go build ./... && go vet ./... && go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(auth-bff): let merchant IDP finish defer tenant selection`

---

### Task 2: A complete-from-session endpoint

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/handler.go`, `cmd/server/main.go`, and tests

Add `POST /auth/zitadel/idp/complete` taking `{auth_request_id, login_name, session_id, session_token, workspace_tenant}` — the same inputs the `totp` handler takes minus the code — which runs the sufficiency decision and the merchant gauntlet and mints `m8_session`, exactly as `totp` does after a successful code check. Read `totp` first and follow it closely; do not invent a parallel completion path.

Guard with `internalauth` as the first statement and add it to `guardedEndpoints()` so the `unreachableZitadel` fixture covers it.

- [ ] **Step 1: Write the failing tests**, including: an unauthenticated caller never reaches Zitadel; a missing field is rejected; a successful call mints the session; the gauntlet's refusals (not-a-member, FGA unreachable) still surface distinctly.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(auth-bff): complete a merchant Zitadel login from an existing session`

---

### Task 3: Restore the admin wiring on the two-step flow

**Files:**
- Restore from `feat/524-3c2-admin-google-followup` and adapt: `apps/admin/app/auth/idp/{actions.ts,finish/route.ts}`, `apps/admin/lib/auth/{auth-bff.ts,google-sign-in-admin.ts}`, `apps/admin/components/auth/SignInForm.tsx`, `apps/admin/app/login/{actions.ts,page.tsx}`
- **Delete** `apps/admin/lib/auth/tenant-host.ts` and its test if nothing else uses it — host-derived tenant resolution is what this plan removes. Check for other callers first.

The finish route now: call `idp/finish` **without** a workspace tenant → get `tenant_required` with the session and email → call the existing `resolveWorkspaceTenant(email)` → call `idp/complete` with the chosen tenant → mint `m8_session`. When `resolveWorkspaceTenant` reports multiple tenants, follow whatever the password path already does for that case rather than inventing new UI.

Do not reintroduce `tenantIdForHostSlug` as the primary mechanism. `resolveWorkspaceTenant` already applies the subdomain refinement internally and falls back correctly on the canonical host.

- [ ] **Step 1: Write the failing tests — and fix the fixtures first.** The parked branch's tests hardcode `demo-store-admin.mark8ly.com`, a host the real flow cannot present. Use the real canonical host `admin.mark8ly.com` and watch the old assumptions fail before fixing anything.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: `npx vitest run`, `npx tsc --noEmit`, `npm run build` — report the build exit status**
- [ ] **Step 5: Commit** — `feat(admin): Google sign-in through Zitadel on the canonical host`

---

### Task 4: The middleware allowlists

**Files:**
- Modify: `apps/admin/middleware.ts`, `apps/admin/lib/auth/host-policy.ts`, and their tests

`/auth/idp/finish` is in neither `PUBLIC_PREFIXES` nor `CANONICAL_ALLOWED_PREFIXES`, so a merchant returning from Google — who has no `m8_session` yet — gets a 404 before the route runs. Phase 3a added `/auth/callback` to both for the same reason; follow exactly what it did.

- [ ] **Step 1: Write the failing test** — a request to `/auth/idp/finish` with no session reaches the route rather than 404ing, on the canonical host. Confirm it fails first.
- [ ] **Step 2: Implement**
- [ ] **Step 3: `npx vitest run && npx tsc --noEmit && npm run build`**
- [ ] **Step 4: Commit** — `fix(admin): let the Google callback through middleware`

---

### Task 5: Documentation

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/README.md`

- [ ] **Step 1:** Record the two-step merchant Google flow and why tenant selection stays in the admin app; that the host cannot select the tenant because `/login` is canonical-only; and that `idp/finish` now has two shapes depending on whether a tenant was supplied. Correct anything the previous phase's section now states falsely.
- [ ] **Step 2: Commit** — `docs(auth-bff): record the two-step merchant Google flow`
