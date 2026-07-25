# Web-Admin Account-Deletion Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Give a signed-in **web** admin user (`apps/admin`) the same genuine "Delete account" capability the mobile app already has — owner → full tenant teardown, staff → membership removal — satisfying deletion parity and giving a clickable prod route to exercise the deletion+purge pipeline.

**Architecture:** This is a **frontend-only** change in `apps/admin`. The authoritative backend — platform-api `DELETE /internal/tenants/:id/account` (owner/staff branching, GIP-user delete, FGA tuple cleanup, transactional `tenant.deleted` outbox → Phase 5 purge) — is ALREADY built and deployed (shipped MVP + Phase 5). It sits behind platform-api's `RequireInternalAuth` group, which `apps/admin`'s existing `platformInternalHeaders` (`X-Internal-Auth`) already authenticates to (that's how `settings/team/actions.ts` calls `/internal/tenants/:id/members/role` in prod today). We add: a client method, a server action, a UI section, and one page prop — plus vitest tests. **No Go changes. No backend deploy.**

**Tech Stack:** Next.js 16 (App Router, server actions), React 19, TypeScript, vitest (`vitest run`).

## Global Constraints

- **Frontend only.** All files under `apps/admin/`. NO changes to any Go service, chart, or infra. The platform-api endpoint already exists — do NOT create or modify it.
- **The endpoint contract (already deployed, do not change):** `DELETE {PLATFORM_API_URL}/internal/tenants/{tenantID}/account`, JSON body `{"uid": "<actorGipUid>"}`, header `X-Internal-Auth` (added by `platformInternalHeaders`). Responses: **204** success; **403** unknown role (`apperrors.Forbidden`); **404** tenant/actor not found. Non-2xx must surface as `PlatformApiError` (mirror the existing `revokeInvitation`/`createStore` error handling in `lib/api/platform-api.ts`).
- **actorUID = the GIP uid.** `x-session-user-id` (set by `apps/admin/middleware.ts` from the auth-bff session `user_id` = GIP `sub`/localId) IS the exact `actorUID` platform-api keys `fga.GetRole` and `gip.DeleteAccount` on. Verified. Pass `x-session-user-id` straight through as `uid`.
- **No `canEditSettings` gate on the delete action.** Deleting your OWN account is available to **any signed-in member** (owner/admin/staff/viewer) — it is not an "edit settings" permission. platform-api is authoritative on owner-vs-staff. Contrast with the existing `deleteAccountAction` (reset-profile), which DOES gate on `canEditSettings` — do not copy that gate onto the new action.
- **Keep the existing "Reset my profile" DangerZone exactly as-is.** The new "Delete account" is a SEPARATE section added alongside it, not a replacement. `DangerZone` stays gated by `editable`; the new section renders unconditionally.
- **Confirmation word is `DELETE`** (uppercase, exact match after trim), distinct from the reset flow's `reset my profile`.
- **Immutability / style:** follow existing patterns in each file; explicit types on exported functions (repo TS style); no `console.log`; small focused additions.
- **Git:** commit directly to `main`, **single-line** conventional-commit messages, **no** signature/attribution. One commit per task (the implementer commits).
- **Tests:** vitest (`cd apps/admin && npx vitest run <file>`). Mirror existing tests: server-action tests → `app/(admin)/products/actions.test.ts`; component tests → `components/settings/*.test.tsx` (e.g. `HeroEditor.test.tsx`). Do NOT introduce jest.
- **Type-check gate:** `cd apps/admin && npm run check-types` (tsc --noEmit) must stay clean.

## File Structure

- `apps/admin/lib/api/platform-api.ts` — **modify**: add `deleteTenantAccount(tenantId, uid)`.
- `apps/admin/app/(admin)/settings/actions.ts` — **modify**: add `deleteTenantAccountAction(confirmation)` (imports the new client method; NO canEditSettings gate).
- `apps/admin/components/settings/AccountSettingsClient.tsx` — **modify**: add a `DeleteAccountSection` component + render it (unconditional) in `AccountSettingsClient`; thread a new `isOwner` prop.
- `apps/admin/app/(admin)/settings/account/page.tsx` — **modify**: pass `isOwner={role === "owner"}` down through `AccountSettingsContent` → `AccountSettingsClient`.
- Tests: `apps/admin/app/(admin)/settings/deleteTenantAccount.test.ts` (action) + a component test for the new section (colocated with `AccountSettingsClient` or under `tests/unit/`, matching where the reviewer finds sibling tests).

---

## Task 1: platform-api client method `deleteTenantAccount`

**Files:**
- Modify: `apps/admin/lib/api/platform-api.ts`
- Test: `apps/admin/lib/api/deleteTenantAccount.test.ts` (new; mock `global.fetch`)

**Interfaces:**
- Produces: `export async function deleteTenantAccount(tenantId: string, uid: string): Promise<void>` → `DELETE ${PLATFORM_API_URL}/internal/tenants/${tenantId}/account` via `internalInit({ method: "DELETE", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ uid }) })`. On 204 → resolve. On non-2xx → `throw await platformError(res)` (reuses the existing helper + `PlatformApiError`). Mirror `revokeInvitation` (lines ~436-452) exactly for shape.

- [ ] **Step 1 (RED):** Write `deleteTenantAccount.test.ts` mocking `fetch`: (a) 204 → resolves, and asserts the request used method `DELETE`, path ending `/internal/tenants/t1/account`, and a body containing `"uid":"u1"`; (b) 403 with `{error,message}` → rejects with a `PlatformApiError` whose `.status === 403` and `.code` is the body error. Run `npx vitest run lib/api/deleteTenantAccount.test.ts` → FAIL (function undefined).
  > Note the `X-Internal-Auth` header is added inside `platformInternalHeaders`, which reads a server env secret; assert on method/path/body, not on the secret value. If the header helper throws without the env set in the test, stub the env or assert only method/path/body via the fetch mock's call args.
- [ ] **Step 2 (GREEN):** Implement `deleteTenantAccount` mirroring `revokeInvitation`. Run the test → PASS.
- [ ] **Step 3:** `npm run check-types` clean.
- [ ] **Step 4: Commit** `feat(admin): platform-api deleteTenantAccount client method`.

---

## Task 2: server action `deleteTenantAccountAction`

**Files:**
- Modify: `apps/admin/app/(admin)/settings/actions.ts`
- Test: `apps/admin/app/(admin)/settings/deleteTenantAccount.test.ts` (new)

**Interfaces:**
- Consumes: `getSession()` (already in the file — returns `{userId, tenantId, email, role, storeId}`), the new `deleteTenantAccount` client method, `PlatformApiError`.
- Produces: `export async function deleteTenantAccountAction(confirmation: string): Promise<ActionResult>`:
  1. `const { userId, tenantId } = await getSession();`
  2. if `!userId || !tenantId` → `noSession()`.
  3. **NO `canEditSettings` gate** (any member may delete their own account).
  4. if `confirmation !== "DELETE"` → `{ ok: false, code: "validation", message: "Confirmation text does not match." }`.
  5. `try { await deleteTenantAccount(tenantId, userId); return { ok: true }; } catch (err) { map PlatformApiError → {ok:false, code, message}; else {ok:false, code:"unknown", message} }`.
  - No `revalidatePath` (the user is being signed out). Import the new client method into the existing import block from `@/lib/api/platform-api`.

- [ ] **Step 1 (RED):** Write `deleteTenantAccount.test.ts` (action). Mock `next/headers` `headers()` to return the session headers (mirror how `products/actions.test.ts` mocks headers), and mock `@/lib/api/platform-api`'s `deleteTenantAccount`. Cases: (a) valid `"DELETE"` + session → calls `deleteTenantAccount(tenantId, userId)` and returns `{ok:true}`; (b) wrong confirmation → `{ok:false, code:"validation"}` and client NOT called; (c) missing session headers → `{ok:false, code:"no_session"}`; (d) client throws `PlatformApiError(403,"forbidden",...)` → `{ok:false, code:"forbidden"}`. Run → FAIL.
- [ ] **Step 2 (GREEN):** Implement the action. Run → PASS.
- [ ] **Step 3:** `npm run check-types` clean.
- [ ] **Step 4: Commit** `feat(admin): deleteTenantAccountAction server action (any member, no edit gate)`.

---

## Task 3: `DeleteAccountSection` UI + page wiring

**Files:**
- Modify: `apps/admin/components/settings/AccountSettingsClient.tsx`
- Modify: `apps/admin/app/(admin)/settings/account/page.tsx`
- Test: component test for `DeleteAccountSection` (colocate as `AccountSettingsClient.DeleteAccount.test.tsx` or under `tests/unit/`, matching sibling test placement; use `@testing-library/react` + vitest as the existing `components/settings/*.test.tsx` do).

**Interfaces:**
- `AccountSettingsClient` gains prop `isOwner: boolean`; renders `<DeleteAccountSection isOwner={isOwner} />` after the existing `<DangerZone editable={editable} />` (with an `<hr />` separator to match the section rhythm). `DeleteAccountSection` renders **unconditionally** (no `editable` gate).
- `DeleteAccountSection({ isOwner }: { isOwner: boolean })`: mirror the existing `DangerZone` structure (useState `showConfirm`/`confirmation`, `useTransition`, `useToast`). Type-to-confirm word is `DELETE`. Copy is owner-aware:
  - `isOwner` true: warn that this **permanently deletes the entire store and all its data** and cannot be undone.
  - `isOwner` false: warn that this **removes your access to this store** (the store itself is unaffected).
  - On confirm → `deleteTenantAccountAction(confirmation)`; on `!ok` → `toast.error(...)`; on `ok` → `window.location.href = "/logout"`. Disable the confirm button while pending or while `confirmation !== "DELETE"`. Reuse the exact Tailwind token classes from `DangerZone` (`--danger`, `--ink-900`, `--background-elevated`, etc.) — do not invent new colors (design system: Paper·Ink·Moss, functional `--danger` only).
- `page.tsx`: `AccountSettingsPage` already has `role`; pass `isOwner={role === "owner"}` into `AccountSettingsContent`, and from there into `<AccountSettingsClient ... isOwner={isOwner} />`.

- [ ] **Step 1 (RED):** Component test: render `DeleteAccountSection` with `isOwner`; mock `deleteTenantAccountAction` (from `@/app/(admin)/settings/actions`) and the toast. Cases: (a) reveal confirm, type `DELETE`, click confirm → `deleteTenantAccountAction` called with `"DELETE"`; (b) typing `delete` (wrong case) or other text keeps the confirm button disabled → action NOT called; (c) owner vs non-owner copy differs (assert on distinguishing text). Stub `window.location` navigation. Run → FAIL.
- [ ] **Step 2 (GREEN):** Implement `DeleteAccountSection`, wire `isOwner` through `AccountSettingsClient` and `page.tsx`. Run the component test → PASS.
- [ ] **Step 3:** `npm run check-types` clean; run the full account-settings-adjacent tests you touched.
- [ ] **Step 4: Commit** `feat(admin): in-app Delete Account section on account settings (owner/staff aware)`.

---

## Task 4: end-to-end verification

- [ ] `cd apps/admin && npm run check-types` → 0 errors.
- [ ] `cd apps/admin && npx vitest run lib/api/deleteTenantAccount.test.ts "app/(admin)/settings/deleteTenantAccount.test.ts"` + the component test → all green.
- [ ] `cd apps/admin && npm run build` → succeeds (App Router server-action + client component compile).
- [ ] Trace the assembled path by reading: `DeleteAccountSection` → `deleteTenantAccountAction` (reads `x-session-user-id` as uid) → `deleteTenantAccount(tenantId, uid)` → `DELETE /internal/tenants/:id/account {uid}` → (already-deployed) platform-api teardown → `tenant.deleted` → Phase 5 purge. Confirm `uid`/`tenantId` field names match across hops.
- [ ] Confirm the existing "Reset my profile" flow is untouched (its action, gate, and copy unchanged).

---

## Self-Review

- **Spec coverage:** any-member visibility (Task 3 unconditional render) ✓; owner teardown + staff removal decided server-side by the already-deployed endpoint (Tasks 1-2 just transport) ✓; separate from reset-profile (Task 3 additive) ✓; DELETE typed confirm ✓; no edit-gate on self-delete (Task 2) ✓.
- **Type consistency:** `deleteTenantAccount(tenantId, uid)` (Task 1) ← called by `deleteTenantAccountAction` (Task 2) ← called by `DeleteAccountSection` (Task 3). `isOwner` threads page → client → section (Task 3).
- **No backend risk:** zero Go/infra changes; the destructive teardown+purge is the already-validated deployed path — this feature only adds a second caller (web) of an endpoint the mobile app already calls.
