# GIP Auth — Phase 1: Per-Host Customer Cookie + Schema Integrity

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind the storefront `mp_customer_session` cookie to the exact request host (instead of the parent `.mark8ly.com`), so the browser refuses to send a customer cookie set at `store-a` to `store-b`, to admin, or to any other host. Add a partial unique index on `customer_profiles (store_id, gip_uid)` so Phase 2/3 Google sign-in races cannot create duplicate rows.

**Architecture:** Storefront customer auth lives in Next.js server actions (NOT auth-bff). The actual cross-store leak is one line in `apps/storefront/app/sign-in/actions.ts:160` where the cookie Domain is hardcoded to `.mark8ly.com`. Replace with the sanitized inbound request host (`headers().get("host")`). Sign-out must clear with the same Domain. Admin (`m8_session` from auth-bff, `.mark8ly.com`) is unchanged.

**Tech Stack:** Next.js 16 server actions + `next/headers`, vitest for unit tests, Playwright for E2E, golang-migrate for the Postgres index.

**Spec:** `docs/superpowers/specs/2026-04-27-gip-auth-isolation-merge-design.md` (revised post pre-flight)

**Branch policy:** all work commits directly to `main` (no PRs, no feature branches). Each task ends with a commit. CI may need the public→build→private cycle (per memory `feedback_ci_billing_workaround.md`).

---

## File structure

### Created
- `services/marketplace-api/migrations/000084_customer_profiles_gip_uid_uq.up.sql` + `.down.sql`
- `apps/storefront/lib/host.ts`
- `apps/storefront/lib/host.test.ts`
- `tests/e2e/auth-isolation.spec.ts`

### Modified
- `apps/storefront/app/sign-in/actions.ts` — cookie Domain becomes per-host
- `apps/storefront/app/sign-out/page.tsx` — delete cookie with the same per-host Domain (plus transitional `.mark8ly.com` clear)

### Untouched (intentional — design discovery)
- `services/auth-bff/**` — admin cookie path is correct as-is
- `apps/admin/**` — admin already uses `m8_session`, distinct from `mp_customer_session`
- `apps/storefront/lib/session.ts` — HMAC + scope check are correct; only the Domain is wrong

---

## Pre-flight (already done in design phase)

- ✅ Latest marketplace-api migration is `000083_shipments_pickup_columns` → new migration is `000084`.
- ✅ Storefront sets `mp_customer_session` (not `m8_session`); HMAC-signed via `apps/storefront/lib/session.ts`; scope-checked via `decodeSessionForScope`.
- ✅ Cookie set in exactly one place: `apps/storefront/app/sign-in/actions.ts` (also reused by `create-account/actions.ts` via `customerSignUp` → `customerSignIn`).
- ✅ Cookie deleted in exactly one place: `apps/storefront/app/sign-out/page.tsx`.
- ✅ `headers()` from `next/headers` is reachable inside server actions (already used by `app/account/layout.tsx:23`).

---

## Task 1: Schema migration — partial unique on customer_profiles (store_id, gip_uid)

**Files:**
- Create: `services/marketplace-api/migrations/000084_customer_profiles_gip_uid_uq.up.sql`
- Create: `services/marketplace-api/migrations/000084_customer_profiles_gip_uid_uq.down.sql`

**Why:** Phase 2 (Google customer sign-in) and Phase 3 (account merge) need a guarantee that two simultaneous sign-ins for the same `(store_id, gip_uid)` cannot create duplicate customer rows. The partial unique index does this without conflicting on existing NULL `gip_uid` rows (legacy password-only customers).

- [ ] **Step 1: Inspect the migrate runner to decide whether to wrap in BEGIN/COMMIT**

```bash
grep -n 'Transactional\|TX\|BEGIN' services/marketplace-api/pkg/migrate/migrate.go services/marketplace-api/migrations.go 2>/dev/null
```

If the runner wraps each file in a transaction by default, **drop the `BEGIN`/`COMMIT`** below — `CREATE INDEX CONCURRENTLY` cannot run inside a transaction in Postgres. If unclear, look at any existing migration that already uses `CONCURRENTLY` for the local convention.

- [ ] **Step 2: Write the up migration**

```sql
-- 000084: Partial unique index on (store_id, gip_uid) so Phase 2/3
-- Google sign-in cannot create duplicate customer_profiles rows in the
-- race window. WHERE clause keeps existing rows with NULL gip_uid
-- (password-only signups) compatible. CONCURRENTLY avoids blocking
-- writes in prod — IMPORTANT: this migration must NOT be wrapped in a
-- transaction by the migrate runner. See migrate.go for runner config.

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS customer_profiles_store_gip_uid_uq
ON customer_profiles (store_id, gip_uid)
WHERE gip_uid IS NOT NULL;
```

- [ ] **Step 3: Write the down migration**

```sql
DROP INDEX IF EXISTS customer_profiles_store_gip_uid_uq;
```

- [ ] **Step 4: Apply locally and verify**

Run from `services/marketplace-api/`:
```bash
go run ./cmd/migrate up
psql "$DATABASE_URL" -c "\d customer_profiles" | grep gip_uid_uq
```
Expected: `customer_profiles_store_gip_uid_uq` listed as a partial UNIQUE index with `WHERE (gip_uid IS NOT NULL)`.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/migrations/000084_*.sql
git commit -m "feat(marketplace-api): partial unique index on customer_profiles(store_id, gip_uid)"
```

---

## Task 2: Storefront — host sanitizer

**Files:**
- Create: `apps/storefront/lib/host.ts`
- Create: `apps/storefront/lib/host.test.ts`

**Why:** The cookie Domain must be a verified, plain hostname — anything weird (port, path char, double-dot) goes to `null` and the caller refuses to mint. Defense in depth: even though the inbound `Host` header is normally clean, an attacker who can control it could otherwise pin a cookie to an attacker-controlled domain.

- [ ] **Step 1: Write failing tests**

```ts
// apps/storefront/lib/host.test.ts
import { describe, it, expect } from "vitest";
import { sanitizeHost } from "./host";

describe("sanitizeHost", () => {
  it("strips port", () => {
    expect(sanitizeHost("store-a.mark8ly.com:443")).toBe("store-a.mark8ly.com");
  });
  it("returns null for empty / null / undefined", () => {
    expect(sanitizeHost("")).toBeNull();
    expect(sanitizeHost(null)).toBeNull();
    expect(sanitizeHost(undefined)).toBeNull();
  });
  it("rejects path characters", () => {
    expect(sanitizeHost("store-a.mark8ly.com/evil")).toBeNull();
    expect(sanitizeHost("store-a.mark8ly.com#evil")).toBeNull();
    expect(sanitizeHost("store-a.mark8ly.com?x=1")).toBeNull();
  });
  it("rejects double dots and edge dots", () => {
    expect(sanitizeHost("store-a..mark8ly.com")).toBeNull();
    expect(sanitizeHost(".mark8ly.com")).toBeNull();
    expect(sanitizeHost("mark8ly.com.")).toBeNull();
  });
  it("accepts standard mark8ly subdomain", () => {
    expect(sanitizeHost("store-a.mark8ly.com")).toBe("store-a.mark8ly.com");
  });
  it("accepts custom domain", () => {
    expect(sanitizeHost("shop.brand-a.com")).toBe("shop.brand-a.com");
  });
  it("accepts apex", () => {
    expect(sanitizeHost("mark8ly.com")).toBe("mark8ly.com");
  });
  it("rejects userinfo / @ / spaces", () => {
    expect(sanitizeHost("user@evil.com")).toBeNull();
    expect(sanitizeHost("evil.com\nfoo")).toBeNull();
    expect(sanitizeHost("a b.com")).toBeNull();
  });
  it("rejects raw IP literals", () => {
    // Per design, customer cookie is for hostnames only; reject IPs.
    expect(sanitizeHost("[::1]")).toBeNull();
  });
});
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd apps/storefront && npx vitest run lib/host.test.ts
```
Expected: FAIL — `sanitizeHost` undefined.

- [ ] **Step 3: Implement**

```ts
// apps/storefront/lib/host.ts
//
// Validates the inbound Host header for use as a cookie Domain. Strips
// :port. Rejects anything that is not a plain hostname (no path chars,
// no consecutive dots, no leading/trailing dot, no IP literal brackets).
//
// The output is fed verbatim into Set-Cookie Domain=, so an unsafe host
// MUST return null and the caller MUST refuse to mint.

const HOSTNAME_RE = /^[a-zA-Z0-9.-]+$/;

export function sanitizeHost(raw: string | null | undefined): string | null {
  if (!raw) return null;
  const noPort = raw.split(":")[0] ?? "";
  if (!noPort) return null;
  if (noPort.startsWith(".") || noPort.endsWith(".")) return null;
  if (noPort.includes("..")) return null;
  if (!HOSTNAME_RE.test(noPort)) return null;
  return noPort;
}
```

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd apps/storefront && npx vitest run lib/host.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add apps/storefront/lib/host.ts apps/storefront/lib/host.test.ts
git commit -m "feat(storefront): sanitizeHost helper for per-host cookie Domain"
```

---

## Task 3: Storefront — `customerSignIn` mints with per-host Domain

**Files:**
- Modify: `apps/storefront/app/sign-in/actions.ts`

**Why:** This is the actual cross-store isolation fix. One line changes from `.mark8ly.com` parent scope to the exact request host.

- [ ] **Step 1: Read the current `customerSignIn` to understand the cookie set block (~lines 155-165)**

Confirm the cookie set call shape:

```ts
c.set({
  name: "mp_customer_session",
  value: cookieValue,
  path: "/",
  domain: ".mark8ly.com",
  httpOnly: true,
  secure: true,
  sameSite: "lax",
  maxAge: 60 * 60 * 24 * 30,
});
```

- [ ] **Step 2: Modify the action**

At the top of the file, add the import:

```ts
import { headers } from "next/headers";
import { sanitizeHost } from "@/lib/host";
```

Inside `customerSignIn`, BEFORE the cookie set call, resolve the host and fail-fast:

```ts
const h = await headers();
const cookieHost = sanitizeHost(h.get("host"));
if (!cookieHost) {
  return {
    ok: false,
    code: "invalid_host",
    message: "Could not validate the host for sign-in. Please try again.",
  };
}
```

Then change the cookie set call's `domain`:

```ts
c.set({
  name: "mp_customer_session",
  value: cookieValue,
  path: "/",
  domain: cookieHost, // per-host: browser refuses to send to other stores
  httpOnly: true,
  secure: true,
  sameSite: "lax",
  maxAge: 60 * 60 * 24 * 30,
});
```

- [ ] **Step 3: Confirm `customerSignUp` (in `app/create-account/actions.ts`) inherits the fix**

```bash
cat apps/storefront/app/create-account/actions.ts | grep customerSignIn
```

Expected: it just delegates `return customerSignIn(input)` — so the fix applies automatically. No change needed there.

- [ ] **Step 4: Build the storefront to catch any type / import errors**

```bash
cd apps/storefront && npm run build 2>&1 | tail -40
```
Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add apps/storefront/app/sign-in/actions.ts
git commit -m "fix(storefront): scope mp_customer_session cookie to request host (per-store isolation)"
```

---

## Task 4: Storefront — `/sign-out` deletes with per-host Domain (+ transitional safety net)

**Files:**
- Modify: `apps/storefront/app/sign-out/page.tsx`

**Why:** Cookies must be deleted with the SAME `Domain` they were set with — otherwise the browser sets a second deletion cookie on the missing scope and leaves the original alive. After Task 3, new cookies have per-host Domain; we delete with per-host Domain. Plus a transitional `.mark8ly.com` delete for one release so any leftover stale cookies from before Phase 1 also get cleared.

- [ ] **Step 1: Replace the sign-out logic**

```tsx
import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";
import { sanitizeHost } from "@/lib/host";

/**
 * /sign-out — clears the customer session cookie and redirects home.
 * Visiting this URL (GET) is enough; no form or button needed.
 *
 * Cookie was set with Domain=<request-host> in customerSignIn (Phase 1)
 * — we must delete with the same Domain or the browser leaves it alive.
 * The transitional `.mark8ly.com` delete catches any pre-Phase-1 cookies
 * still kicking around; can be removed one release after Phase 1.
 */
export default async function SignOutPage() {
  const c = await cookies();
  const h = await headers();
  const host = sanitizeHost(h.get("host"));

  if (host) {
    c.set({
      name: "mp_customer_session",
      value: "",
      path: "/",
      domain: host,
      maxAge: 0,
      httpOnly: true,
      secure: true,
      sameSite: "lax",
    });
  }

  // Transitional: clear the legacy parent-domain cookie set before Phase 1.
  // Drop one release after Phase 1 lands.
  c.set({
    name: "mp_customer_session",
    value: "",
    path: "/",
    domain: ".mark8ly.com",
    maxAge: 0,
    httpOnly: true,
    secure: true,
    sameSite: "lax",
  });

  redirect("/");
}
```

- [ ] **Step 2: Build the storefront**

```bash
cd apps/storefront && npm run build 2>&1 | tail -40
```

- [ ] **Step 3: Commit**

```bash
git add apps/storefront/app/sign-out/page.tsx
git commit -m "fix(storefront): delete mp_customer_session with per-host Domain (+ legacy clear)"
```

---

## Task 5: E2E — `auth-isolation.spec.ts`

**Files:**
- Create: `tests/e2e/auth-isolation.spec.ts`

**Why:** Lock in the cross-store + cross-app + custom-domain isolation guarantees as tests. Per memory `e2e_test_state.md`, the project has a Playwright suite with helpers; reuse what exists.

- [ ] **Step 1: Inspect existing E2E suite for helpers and config**

```bash
ls tests/e2e/ 2>/dev/null
test -f playwright.config.ts && cat playwright.config.ts | head -40
grep -rn 'signUpAsCustomer\|signInAsAdmin\|signInAsCustomer' tests/e2e/ 2>/dev/null | head
```

If helpers don't exist, the test should include minimal inline ones (page.goto, fill, click) using a seeded test store from the dev environment.

- [ ] **Step 2: Write the spec**

```ts
// tests/e2e/auth-isolation.spec.ts
import { test, expect, type BrowserContext } from "@playwright/test";

const STORE_A = process.env.E2E_STORE_A_URL ?? "http://store-a.mark8ly.local:4203";
const STORE_B = process.env.E2E_STORE_B_URL ?? "http://store-b.mark8ly.local:4203";
const ADMIN   = process.env.E2E_ADMIN_URL  ?? "http://admin.mark8ly.local:4202";
const CUSTOM_DOMAIN = process.env.E2E_CUSTOM_DOMAIN_URL; // optional — only run if set

const TEST_CUSTOMER_EMAIL = process.env.E2E_TEST_CUSTOMER_EMAIL ?? "isolation-test@example.com";
const TEST_CUSTOMER_PWD   = process.env.E2E_TEST_CUSTOMER_PWD   ?? "test-password-123";

async function customerSignIn(ctx: BrowserContext, baseUrl: string) {
  const page = await ctx.newPage();
  await page.goto(`${baseUrl}/sign-in`);
  await page.getByLabel(/email/i).fill(TEST_CUSTOMER_EMAIL);
  await page.getByLabel(/password/i).fill(TEST_CUSTOMER_PWD);
  await page.getByRole("button", { name: /sign in/i }).click();
  await page.waitForURL(/\/account/);
  await page.close();
}

test.describe("auth isolation", () => {
  test("customer cookie has Domain = exact host (no leading dot)", async ({ browser }) => {
    const ctx = await browser.newContext();
    await customerSignIn(ctx, STORE_A);

    const cookies = await ctx.cookies();
    const customer = cookies.find((c) => c.name === "mp_customer_session");
    expect(customer, "mp_customer_session should be set").toBeDefined();
    expect(customer!.domain).toBe(new URL(STORE_A).hostname);
    expect(customer!.domain.startsWith(".")).toBe(false);
    await ctx.close();
  });

  test("customer cookie set at store-a is NOT sent to store-b", async ({ browser }) => {
    const ctx = await browser.newContext();
    await customerSignIn(ctx, STORE_A);

    const cookiesAtB = await ctx.cookies(STORE_B);
    expect(cookiesAtB.find((c) => c.name === "mp_customer_session")).toBeUndefined();
    await ctx.close();
  });

  test("customer cookie set at store-a is NOT sent to admin", async ({ browser }) => {
    const ctx = await browser.newContext();
    await customerSignIn(ctx, STORE_A);

    const cookiesAtAdmin = await ctx.cookies(ADMIN);
    expect(cookiesAtAdmin.find((c) => c.name === "mp_customer_session")).toBeUndefined();
    await ctx.close();
  });

  test("custom domain customer cookie is per-host", async ({ browser }) => {
    test.skip(!CUSTOM_DOMAIN, "E2E_CUSTOM_DOMAIN_URL not set");
    const ctx = await browser.newContext();
    await customerSignIn(ctx, CUSTOM_DOMAIN!);

    const cookies = await ctx.cookies();
    const customer = cookies.find((c) => c.name === "mp_customer_session");
    expect(customer!.domain).toBe(new URL(CUSTOM_DOMAIN!).hostname);
    await ctx.close();
  });

  test("sign-out clears the cookie", async ({ browser }) => {
    const ctx = await browser.newContext();
    await customerSignIn(ctx, STORE_A);

    const page = await ctx.newPage();
    await page.goto(`${STORE_A}/sign-out`);
    await page.waitForURL(STORE_A + "/");

    const cookies = await ctx.cookies(STORE_A);
    expect(cookies.find((c) => c.name === "mp_customer_session")).toBeUndefined();
    await ctx.close();
  });
});
```

- [ ] **Step 3: Run locally**

Per memory `e2e_test_state.md`, the dev stack runs at the local hosts above. If two stores aren't seeded yet, document the prerequisite at the top of the file (e.g. "requires `store-a` and `store-b` tenants seeded via the onboarding wizard").

```bash
npx playwright test tests/e2e/auth-isolation.spec.ts
```

- [ ] **Step 4: Commit**

```bash
git add tests/e2e/auth-isolation.spec.ts
git commit -m "test(e2e): per-host customer cookie + cross-store isolation"
```

---

## Verification & cutover

After Tasks 1-5 land and the bump-k8s job propagates:

- [ ] **Verify ArgoCD sync.** `kubectl -n argocd get application mark8ly-storefront mark8ly-marketplace-api` — both should be `Synced + Healthy` on the new revision.
- [ ] **Verify schema migration ran in prod.** `kubectl exec mark8ly-postgres-1 -c postgres -- psql -U postgres -d marketplace_api -c "\d customer_profiles"` — index `customer_profiles_store_gip_uid_uq` should be present.
- [ ] **Smoke test customer sign-in on a real store.** Visit `https://<slug>.mark8ly.com/sign-in`, sign in with a test customer, confirm in DevTools → Application → Cookies that `mp_customer_session` Domain is exactly `<slug>.mark8ly.com` (no leading dot).
- [ ] **Smoke test cross-store.** Open a second tab to a different store; the customer cookie should NOT appear in DevTools.
- [ ] **Smoke test custom domain (if a store has one).** Same test on `shop.brand-a.com` if available.
- [ ] **Smoke test sign-out.** `/sign-out` should clear the cookie; cookie disappears from DevTools.

After two weeks (one full session lifetime / cookie max-age):

- [ ] **Phase 1.5 cleanup**: drop the transitional `.mark8ly.com` clear from `apps/storefront/app/sign-out/page.tsx`. Commit.

---

## Rollback

Each task is independently reversible via `git revert`. The schema migration (Task 1) is safe to leave in place even after a code rollback — it doesn't break anything. Tasks 3 and 4 are the user-visible changes; reverting them restores the parent-domain cookie behavior, which is technically less secure but immediately functional. No data migration involved.

If the per-host cookie causes an unforeseen issue in prod (e.g. a customer journey that expects the cookie at a different host than where it was set):

1. `git revert <Task-3-sha>` and `git revert <Task-4-sha>`.
2. Push, wait for bump-k8s, ArgoCD sync.
3. Existing per-host cookies expire on the user's next sign-in (browser sets the parent-domain cookie over the top).
