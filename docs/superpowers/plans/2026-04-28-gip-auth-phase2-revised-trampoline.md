# GIP Auth — Phase 2 (Revised): Centralized Google Sign-In via mark8ly.com Trampoline

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status:** Supersedes `2026-04-28-gip-auth-phase2-storefront-google.md` after the discovery that Google's OAuth client config does not support wildcards in Authorized JavaScript origins. Per-tenant subdomain origins are incompatible with Mark8ly's automated tenant onboarding — the OAuth Admin API for programmatic origin management is deprecated (Mar 2026 shutdown).

**Goal:** Customer Google sign-in works on every tenant subdomain (`<slug>.mark8ly.com` and custom domains) without registering each origin with Google. Achieved by routing the Google flow through a single fixed origin (`mark8ly.com/auth/google`) and bouncing the verified result back to the originating store via a short-lived HMAC-signed exchange code.

**Architecture:**

```
Customer at india-store.mark8ly.com/sign-in
  ↓ click "Continue with Google"
  ↓ window.location.assign("https://mark8ly.com/auth/google?return_to=...&store_slug=india-store&intent=signin")
mark8ly.com/auth/google (page in onboarding app)
  ↓ load gsi/client (works — mark8ly.com is registered OAuth origin)
  ↓ get Google credential
  ↓ call Identity Toolkit signInWithIdp (MP-Customer pool) → GIP id_token
  ↓ POST to mintCustomerExchangeCode action: { id_token, store_slug, return_to }
  ↓ server action mints HMAC JWT { id_token, store_slug, return_to, intent, exp:30s }
  ↓ window.location.assign("https://india-store.mark8ly.com/auth/google/finish?code=<jwt>")
india-store.mark8ly.com/auth/google/finish (Route Handler in storefront)
  ↓ verify HMAC + check store_slug matches request host (per-host scope check)
  ↓ extract id_token from claims, call existing customerSignIn(idToken, uid, store_slug)
  ↓ customerSignIn mints mp_customer_session per-host (Phase 1)
  ↓ NextResponse.redirect(return_to)
```

**Tech Stack:** Next.js 16 + React 19. HMAC-SHA256 (Node `crypto`) for exchange-code signing. Identity Toolkit REST. Existing `SESSION_ENCRYPT_KEY` env var (already in both onboarding and storefront pods) reused as the HMAC key.

**Spec:** `docs/superpowers/specs/2026-04-27-gip-auth-isolation-merge-design.md` § "Storefront Google sign-in" — design intent unchanged; mechanism revised.

**Branch policy:** all work commits directly to `main`. Each task ends with a commit. Single-line commit messages, no signoff, no `Co-Authored-By`.

---

## Pre-flight (already verified)

- ✅ Phase 1 deployed; per-host `mp_customer_session` cookie working.
- ✅ `mark8ly.com` is in OAuth Authorized JavaScript origins.
- ✅ `MP-Customer-39opy` GIP tenant pool has Google provider enabled.
- ✅ Onboarding pod already has `NEXT_PUBLIC_GOOGLE_CLIENT_ID` env var wired (currently empty value in chart — needs populating).
- ✅ Onboarding already has `lib/gip/google-gsi.ts` (admin-MP-Internal flavor) — we extend it with a customer-pool path.
- ✅ `SESSION_ENCRYPT_KEY` env var is present on both onboarding and storefront pods (used for the HMAC exchange-code key — they MUST share the same value).

---

## Work breakdown

### Reverts (one commit)

The earlier Phase 2 commits that put gsi/client on the storefront are dead code under the trampoline pattern. Remove them cleanly so the storefront codebase doesn't carry unused Google-pool helpers.

Commits to revert (in reverse chronological order so reverts apply cleanly):

| Commit | What |
|---|---|
| `4e2cc07` (in tesserix-k8s) | `chore(mark8ly-storefront): expose NEXT_PUBLIC_GOOGLE_CLIENT_ID for Phase 2 customer Google sign-in` |
| `a510206` | `feat(storefront): add Continue with Google to create account form` — keep the BUTTON, but the handler will be rewritten in a later task. We will revert the inline `getGoogleCredential`/`signInWithGoogleCustomer` calls and the `googlePending` plumbing in the same task; not a clean `git revert`, just an edit. |
| `963657f` | `feat(storefront): add Continue with Google to customer sign-in form` — same as above. |
| `7d4c424` | `feat(storefront): signInWithGoogleCustomer GIP helper for MP-Customer pool` — clean revert |
| `26f7eae` | `feat(storefront): port google-gsi helper for customer sign-in` — clean revert |
| `0f96b0c` | `feat(storefront): thread googleClientId through gipConfig prop` — clean revert |

Actually, the simplest is **NOT to revert the buttons** (Tasks 4+5). Keep the JSX, just rewrite the handlers in Task 6 below. So the only true reverts are the helper files and the chart env. The button JSX is correct as-is; only the click handler logic flips.

### New tasks (six commits)

1. Revert dead helpers + storefront chart env (one commit each in mark8ly + tesserix-k8s)
2. Set actual `googleClientId` value in admin + onboarding charts (tesserix-k8s)
3. Shared exchange-code helper in `packages/ui`
4. Onboarding trampoline page + customer-pool `signInWithIdp` helper
5. Storefront `/auth/google/finish` route handler
6. Rewrite both storefront Google button handlers to redirect to trampoline

---

## File structure

### Created
- `packages/ui/src/auth/exchange-code.ts` — `mintExchangeCode`, `verifyExchangeCode` (HMAC JWT helpers).
- `packages/ui/src/auth/exchange-code.test.ts` — unit tests.
- `apps/onboarding/lib/gip/customer-signin.ts` — `signInWithGoogleCustomer(googleCredential)` (browser-side, uses `signInWithIdp` against MP-Customer pool).
- `apps/onboarding/app/auth/google/page.tsx` — trampoline page.
- `apps/onboarding/app/auth/google/actions.ts` — server action `mintCustomerExchangeCode({ idToken, storeSlug, returnTo, intent })`.
- `apps/storefront/app/auth/google/finish/route.ts` — exchange-code redemption + cookie mint.

### Modified
- `apps/storefront/components/auth/CustomerSignInForm.tsx` — replace inline gsi flow with redirect to trampoline.
- `apps/storefront/components/auth/CreateAccountForm.tsx` — same.
- `apps/onboarding/lib/config.ts` — extend `publicConfig` with `gipCustomerTenantId` (currently only has `gipTenantId` for MP-Internal).
- `tesserix-k8s/charts/apps/mark8ly-onboarding/values.yaml` — set actual OAuth client ID; add `gipCustomerTenantId` if not present.
- `tesserix-k8s/charts/apps/mark8ly-onboarding/templates/deployment.yaml` — add `NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID` env var.
- `tesserix-k8s/charts/apps/mark8ly-admin/values.yaml` — set actual OAuth client ID.

### Reverted (delete files / undo edits)
- `apps/storefront/lib/gip/google-gsi.ts` — DELETE
- `apps/storefront/lib/gip/signup.ts` — DELETE
- `apps/storefront/components/auth/CustomerSignInForm.tsx` — remove `getGoogleCredential` import + handler logic, keep button (handler becomes redirect — see Task 6).
- `apps/storefront/components/auth/CreateAccountForm.tsx` — same.
- `apps/storefront/app/sign-in/page.tsx` — remove `googleClientId` from `gipConfig` literal.
- `apps/storefront/app/create-account/page.tsx` — same.
- `tesserix-k8s/charts/apps/mark8ly-storefront/templates/deployment.yaml` — remove `NEXT_PUBLIC_GOOGLE_CLIENT_ID` env entry.
- `tesserix-k8s/charts/apps/mark8ly-storefront/values.yaml` — remove `public.googleClientId` entry.

---

## Task 1: Revert dead-code Phase 2 commits + storefront chart env

**Files:**
- DELETE: `apps/storefront/lib/gip/google-gsi.ts`
- DELETE: `apps/storefront/lib/gip/signup.ts`
- Modify: `apps/storefront/app/sign-in/page.tsx` (remove `googleClientId` line)
- Modify: `apps/storefront/app/create-account/page.tsx` (remove `googleClientId` line)
- Modify: `apps/storefront/components/auth/CustomerSignInForm.tsx` (remove `googleClientId` field from `GipConfig`; remove the helper imports and `handleGoogle`/`googlePending` plumbing — but **keep the button JSX** which Task 6 will rewire)
- Modify: `apps/storefront/components/auth/CreateAccountForm.tsx` (same)

Plus on `tesserix-k8s`:
- Modify: `charts/apps/mark8ly-storefront/values.yaml` (drop `public.googleClientId`)
- Modify: `charts/apps/mark8ly-storefront/templates/deployment.yaml` (drop `NEXT_PUBLIC_GOOGLE_CLIENT_ID` env entry)

- [ ] **Step 1: Inspect each commit and craft a single reverting commit per repo**

In `mark8ly`:

```bash
# Quick way: git rm + edits
rm apps/storefront/lib/gip/google-gsi.ts apps/storefront/lib/gip/signup.ts
# Then hand-edit the form components + page literals. Detail in Step 2.
```

The button JSX stays for now (Task 6 rewires the handler). To stay surgical, leave the button + divider JSX untouched and only remove:
- `getGoogleCredential` import
- `signInWithGoogleCustomer` import
- `StorefrontGIPError` import
- `googlePending` state declaration
- `handleGoogle` function body
- The button's `onClick={handleGoogle}` and `disabled={pending || googlePending}` references — replace `disabled` with just `disabled={pending}` and replace `onClick` with a placeholder `onClick={() => undefined}` that Task 6 will fill in. Add a `// TODO(Phase 2): wire to trampoline in Task 6` comment.

Or simpler: delete the entire button + divider JSX too, and Task 6 reintroduces them with the right handler. Cleaner diff. Let's do that.

- [ ] **Step 2: Edit form components**

In `apps/storefront/components/auth/CustomerSignInForm.tsx`:
- Remove `googleClientId: string;` from the `GipConfig` interface.
- Remove the helper imports (`getGoogleCredential`, `signInWithGoogleCustomer`, `StorefrontGIPError`).
- Remove the `googlePending` state.
- Remove the `handleGoogle` function.
- Remove the `<div className="relative py-1">` divider block + `<button type="button" onClick={handleGoogle}>` Google button block.
- Restore the password submit button's `disabled` prop to just `disabled={pending}`.

Same in `CreateAccountForm.tsx`.

- [ ] **Step 3: Edit page literals**

In `apps/storefront/app/sign-in/page.tsx` and `apps/storefront/app/create-account/page.tsx`:
- Remove the `googleClientId: process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID ?? "",` line from the `gipConfig` object literal.

- [ ] **Step 4: Build storefront**

```bash
cd apps/storefront && npm run build 2>&1 | tail -20
```

Expected: clean build, no unused-import warnings.

- [ ] **Step 5: Commit on mark8ly**

```bash
git add apps/storefront/
git commit -m "revert(storefront): remove inline Google gsi flow (replaced by mark8ly.com trampoline)"
```

- [ ] **Step 6: Edit tesserix-k8s storefront chart**

```bash
cd ../tesserix-k8s
```

In `charts/apps/mark8ly-storefront/values.yaml`: remove the `public:` block we added (the `googleClientId: ""` line and its parent if it's now empty).

In `charts/apps/mark8ly-storefront/templates/deployment.yaml`: remove the `NEXT_PUBLIC_GOOGLE_CLIENT_ID` env entry.

- [ ] **Step 7: Helm-lint + commit + push**

```bash
helm template charts/apps/mark8ly-storefront/ 2>&1 | tail -10
git add charts/apps/mark8ly-storefront/
git commit -m "revert(mark8ly-storefront): drop NEXT_PUBLIC_GOOGLE_CLIENT_ID (Google flow moves to onboarding trampoline)"
git push origin main
```

---

## Task 2: Set actual OAuth client ID in admin + onboarding charts

**Files (in `tesserix-k8s` repo):**
- Modify: `charts/apps/mark8ly-admin/values.yaml`
- Modify: `charts/apps/mark8ly-onboarding/values.yaml`

The OAuth client ID `849928263410-5djgu3n40c5tpr86votuptkitqveegor.apps.googleusercontent.com` is already configured in GIP for both pools. Both admin and onboarding charts have an empty `public.googleClientId: ""` placeholder. Set the value.

- [ ] **Step 1: Edit admin chart**

In `charts/apps/mark8ly-admin/values.yaml`, change:

```yaml
public:
  ...
  googleClientId: ""
```

to:

```yaml
public:
  ...
  googleClientId: "849928263410-5djgu3n40c5tpr86votuptkitqveegor.apps.googleusercontent.com"
```

- [ ] **Step 2: Edit onboarding chart**

Same edit in `charts/apps/mark8ly-onboarding/values.yaml`.

- [ ] **Step 3: Commit + push**

```bash
git add charts/apps/mark8ly-admin/values.yaml charts/apps/mark8ly-onboarding/values.yaml
git commit -m "chore(mark8ly): wire NEXT_PUBLIC_GOOGLE_CLIENT_ID for admin + onboarding"
git push origin main
```

---

## Task 3: Shared exchange-code helper in `packages/ui`

**Files:**
- Create: `packages/ui/src/auth/exchange-code.ts`
- Create: `packages/ui/src/auth/exchange-code.test.ts`

**Why:** Onboarding mints, storefront verifies. Same key (`SESSION_ENCRYPT_KEY`), same algorithm. Single source of truth.

- [ ] **Step 1: Write failing tests**

```ts
import { describe, it, expect } from "vitest";
import {
  mintExchangeCode,
  verifyExchangeCode,
  ExchangeCodeError,
} from "./exchange-code";

const KEY = "test-key-32-bytes-padded-padded!!";

describe("exchange-code", () => {
  it("round-trips a payload", () => {
    const code = mintExchangeCode(
      { idToken: "id-token", storeSlug: "store-a", returnTo: "https://store-a.mark8ly.com/account", intent: "signin" },
      KEY,
      30,
    );
    const claims = verifyExchangeCode(code, KEY);
    expect(claims.storeSlug).toBe("store-a");
    expect(claims.intent).toBe("signin");
    expect(claims.idToken).toBe("id-token");
  });
  it("rejects a tampered payload", () => {
    const code = mintExchangeCode(
      { idToken: "id-token", storeSlug: "store-a", returnTo: "https://store-a.mark8ly.com/account", intent: "signin" },
      KEY,
      30,
    );
    const tampered = code.replace(/\.[^.]+$/, ".bad-signature");
    expect(() => verifyExchangeCode(tampered, KEY)).toThrow(ExchangeCodeError);
  });
  it("rejects an expired code", async () => {
    const code = mintExchangeCode(
      { idToken: "x", storeSlug: "s", returnTo: "https://s.mark8ly.com/", intent: "signin" },
      KEY,
      0, // already expired
    );
    expect(() => verifyExchangeCode(code, KEY)).toThrow(/expired/i);
  });
  it("rejects a code signed with a different key", () => {
    const code = mintExchangeCode(
      { idToken: "x", storeSlug: "s", returnTo: "https://s.mark8ly.com/", intent: "signin" },
      KEY,
      30,
    );
    expect(() => verifyExchangeCode(code, "different-key-32-bytes-padded!!!")).toThrow(ExchangeCodeError);
  });
});
```

- [ ] **Step 2: Implement the helper**

```ts
// packages/ui/src/auth/exchange-code.ts
//
// HMAC-SHA256 signed token used to bounce a verified Google sign-in
// from the mark8ly.com trampoline back to the originating tenant
// store. Cookie format: `<base64-payload>.<hex-signature>`.
//
// Carries the GIP id_token (already verified by signInWithIdp on the
// trampoline) plus store_slug + return_to + intent. The receiving
// storefront /auth/google/finish handler verifies the HMAC and matches
// store_slug to the request host before completing sign-in.
//
// 30-second default TTL keeps the bearer-style risk small.

import { createHmac, timingSafeEqual } from "node:crypto";

export interface ExchangeCodeClaims {
  idToken: string;
  storeSlug: string;
  returnTo: string;
  intent: "signin" | "signup";
  /** Unix epoch seconds. */
  exp: number;
}

export interface ExchangeCodeInput {
  idToken: string;
  storeSlug: string;
  returnTo: string;
  intent: "signin" | "signup";
}

export class ExchangeCodeError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

function sign(payload: string, key: string): string {
  return createHmac("sha256", key).update(payload).digest("hex");
}

export function mintExchangeCode(
  input: ExchangeCodeInput,
  key: string,
  ttlSeconds: number,
): string {
  if (!key) throw new ExchangeCodeError("missing_key", "key is required");
  const claims: ExchangeCodeClaims = {
    ...input,
    exp: Math.floor(Date.now() / 1000) + ttlSeconds,
  };
  const payload = Buffer.from(JSON.stringify(claims)).toString("base64url");
  const sig = sign(payload, key);
  return `${payload}.${sig}`;
}

export function verifyExchangeCode(
  code: string,
  key: string,
): ExchangeCodeClaims {
  if (!key) throw new ExchangeCodeError("missing_key", "key is required");
  const dot = code.lastIndexOf(".");
  if (dot < 0) throw new ExchangeCodeError("malformed", "code missing signature");

  const payload = code.slice(0, dot);
  const sig = code.slice(dot + 1);
  const expected = sign(payload, key);

  if (sig.length !== expected.length) {
    throw new ExchangeCodeError("invalid_signature", "signature length mismatch");
  }
  if (!timingSafeEqual(Buffer.from(sig), Buffer.from(expected))) {
    throw new ExchangeCodeError("invalid_signature", "signature mismatch");
  }

  let claims: ExchangeCodeClaims;
  try {
    claims = JSON.parse(Buffer.from(payload, "base64url").toString()) as ExchangeCodeClaims;
  } catch {
    throw new ExchangeCodeError("malformed_payload", "payload is not valid JSON");
  }

  if (Math.floor(Date.now() / 1000) > claims.exp) {
    throw new ExchangeCodeError("expired", "code expired");
  }

  return claims;
}
```

- [ ] **Step 3: Confirm exports + run tests**

Add to `packages/ui/src/index.ts` (or equivalent barrel) — confirm the package's existing pattern:

```bash
grep -rn 'export\s' packages/ui/src/index.ts 2>/dev/null | head -5
```

If a barrel exists, add:
```ts
export { mintExchangeCode, verifyExchangeCode, ExchangeCodeError } from "./auth/exchange-code";
export type { ExchangeCodeClaims } from "./auth/exchange-code";
```

If no barrel pattern, importers can use `@repo/ui/auth/exchange-code` deep import (consistent with how the package's other sub-paths work — see `@repo/ui/role-badge` etc.).

```bash
cd packages/ui && npx vitest run src/auth/exchange-code.test.ts
```

Expected: 4 tests pass.

- [ ] **Step 4: Commit**

```bash
git add packages/ui/src/auth/
git commit -m "feat(ui): exchange-code helper for cross-host Google sign-in trampoline"
```

---

## Task 4: Onboarding trampoline — page + customer signInWithIdp helper + server action

**Files:**
- Create: `apps/onboarding/lib/gip/customer-signin.ts`
- Create: `apps/onboarding/app/auth/google/page.tsx`
- Create: `apps/onboarding/app/auth/google/actions.ts`
- Modify: `apps/onboarding/lib/config.ts` (add `gipCustomerTenantId`)

### Step 1: Extend `publicConfig`

In `apps/onboarding/lib/config.ts`, add to `publicConfig`:

```ts
gipCustomerTenantId: process.env.NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID ?? "",
```

### Step 2: Customer-pool signInWithIdp helper

```ts
// apps/onboarding/lib/gip/customer-signin.ts
//
// Browser-side helper that exchanges a Google credential JWT for a
// GIP id_token in the MP-Customer tenant pool. Used by the
// /auth/google trampoline page that bounces customer sign-ins from
// per-tenant subdomains back to the originating store. Mirrors the
// existing onboarding/lib/gip/signup.ts but targets MP-Customer
// instead of MP-Internal.

import { publicConfig } from "@/lib/config";

export class CustomerGIPError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

export interface CustomerSigninResult {
  uid: string;
  idToken: string;
}

export async function signInWithGoogleCustomer(
  googleIdToken: string,
): Promise<CustomerSigninResult> {
  if (!publicConfig.gipApiKey) {
    throw new CustomerGIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipCustomerTenantId) {
    throw new CustomerGIPError("config_missing", "GIP customer tenant id is not configured");
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=${encodeURIComponent(publicConfig.gipApiKey)}`;
  const requestUri =
    typeof window !== "undefined" ? window.location.origin : "https://mark8ly.com";

  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: publicConfig.gipCustomerTenantId,
      requestUri,
      postBody: `id_token=${encodeURIComponent(googleIdToken)}&providerId=google.com`,
      returnSecureToken: true,
      returnIdpCredential: true,
    }),
  });

  if (!res.ok) {
    let body: { error?: { message?: string } } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new CustomerGIPError(
      "google_signin_failed",
      body.error?.message ?? `HTTP ${res.status}`,
    );
  }

  const data = (await res.json()) as { localId: string; idToken: string };
  return { uid: data.localId, idToken: data.idToken };
}
```

### Step 3: Server action `mintCustomerExchangeCode`

```ts
// apps/onboarding/app/auth/google/actions.ts
"use server";

import { mintExchangeCode } from "@repo/ui/auth/exchange-code";

const SESSION_ENCRYPT_KEY = process.env.SESSION_ENCRYPT_KEY ?? "";

export interface MintInput {
  idToken: string;
  storeSlug: string;
  returnTo: string;
  intent: "signin" | "signup";
}

export interface MintResult {
  ok: boolean;
  redirectUrl?: string;
  error?: string;
}

export async function mintCustomerExchangeCode(
  input: MintInput,
): Promise<MintResult> {
  if (!SESSION_ENCRYPT_KEY) {
    return { ok: false, error: "Session key not configured." };
  }

  // Sanitize storeSlug + returnTo. Slug must be lowercase alphanumeric +
  // hyphens. returnTo must be a same-tenant URL on either *.mark8ly.com
  // or a known custom domain (we trust the slug → host mapping
  // resolved by the originating store's middleware on /auth/google/finish).
  if (!/^[a-z0-9][a-z0-9-]*[a-z0-9]$/.test(input.storeSlug)) {
    return { ok: false, error: "Invalid store_slug." };
  }
  let returnHost: string;
  try {
    returnHost = new URL(input.returnTo).hostname;
  } catch {
    return { ok: false, error: "Invalid return_to." };
  }
  // Loose hostname check — full host validation happens on the
  // storefront side via sanitizeHost when minting the cookie.
  if (!/^[a-zA-Z0-9.-]+$/.test(returnHost) || returnHost.length > 253) {
    return { ok: false, error: "Invalid return_to host." };
  }

  const code = mintExchangeCode(
    {
      idToken: input.idToken,
      storeSlug: input.storeSlug,
      returnTo: input.returnTo,
      intent: input.intent,
    },
    SESSION_ENCRYPT_KEY,
    30,
  );

  // Build the storefront finish URL. Store host must match storeSlug.
  // We construct it from the slug + .mark8ly.com OR the returnTo's
  // host if that's a custom domain. Use the returnTo's host directly —
  // the finish route will reject the code if storeSlug doesn't match.
  const finishUrl = new URL("/auth/google/finish", `https://${returnHost}`);
  finishUrl.searchParams.set("code", code);
  return { ok: true, redirectUrl: finishUrl.toString() };
}
```

### Step 4: Trampoline page

```tsx
// apps/onboarding/app/auth/google/page.tsx
"use client";

import { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  signInWithGoogleCustomer,
  CustomerGIPError,
} from "@/lib/gip/customer-signin";
import { mintCustomerExchangeCode } from "./actions";

export default function GoogleAuthTrampolinePage() {
  const params = useSearchParams();
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<"idle" | "popup" | "exchanging" | "redirecting">("idle");

  const returnTo = params.get("return_to") ?? "";
  const storeSlug = params.get("store_slug") ?? "";
  const intent = (params.get("intent") === "signup" ? "signup" : "signin") as "signin" | "signup";

  useEffect(() => {
    if (!returnTo || !storeSlug) {
      setError("Missing required parameters.");
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        setStatus("popup");
        const { credential } = await getGoogleCredential();
        if (cancelled) return;
        setStatus("exchanging");
        const gip = await signInWithGoogleCustomer(credential);
        if (cancelled) return;
        const result = await mintCustomerExchangeCode({
          idToken: gip.idToken,
          storeSlug,
          returnTo,
          intent,
        });
        if (cancelled) return;
        if (!result.ok || !result.redirectUrl) {
          setError(result.error ?? "Could not complete sign-in.");
          return;
        }
        setStatus("redirecting");
        window.location.assign(result.redirectUrl);
      } catch (err) {
        if (cancelled) return;
        if (err instanceof CustomerGIPError) {
          setError(
            err.code === "config_missing"
              ? "Google sign-in is not available right now."
              : "Google sign-in failed. Please try again.",
          );
        } else {
          setError(err instanceof Error ? err.message : "Google sign-in failed.");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [returnTo, storeSlug, intent]);

  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col items-center justify-center px-6 py-16 text-center">
      <h1 className="font-serif text-2xl">
        Continuing to {storeSlug || "store"}…
      </h1>
      <p className="mt-3 text-sm opacity-70">
        {status === "popup" && "Opening Google sign-in…"}
        {status === "exchanging" && "Completing sign-in…"}
        {status === "redirecting" && "Returning you to the store…"}
        {status === "idle" && !error && "Preparing…"}
      </p>
      {error && (
        <p role="alert" className="mt-4 text-sm text-[color:var(--danger,#a3322a)]">
          {error}
        </p>
      )}
    </main>
  );
}
```

### Step 5: Build + commit

```bash
cd apps/onboarding && npm run build 2>&1 | tail -20
cd ../..
git add apps/onboarding/
git commit -m "feat(onboarding): mark8ly.com/auth/google trampoline for customer sign-in"
```

---

## Task 5: Storefront `/auth/google/finish` route handler

**Files:**
- Create: `apps/storefront/app/auth/google/finish/route.ts`

The handler must:
1. Read `code` query param.
2. Verify HMAC + expiry via `verifyExchangeCode`.
3. Confirm `claims.storeSlug` matches the resolved store for the request host (use existing `resolveStoreSlug` helper).
4. Confirm `claims.returnTo` host matches the request host (defense in depth — prevents open-redirect even if claims are stolen).
5. Call the existing `customerSignIn` action with `{ idToken: claims.idToken, uid: "" /* customerSignIn re-verifies */, storeSlug: claims.storeSlug }`.
6. On success: `NextResponse.redirect(claims.returnTo)`.

```ts
// apps/storefront/app/auth/google/finish/route.ts
import { NextResponse } from "next/server";
import { verifyExchangeCode, ExchangeCodeError } from "@repo/ui/auth/exchange-code";
import { resolveStoreSlug } from "@/lib/slug";
import { customerSignIn } from "@/app/sign-in/actions";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const SESSION_ENCRYPT_KEY = process.env.SESSION_ENCRYPT_KEY ?? "";

export async function GET(req: Request): Promise<Response> {
  if (!SESSION_ENCRYPT_KEY) {
    return errorResponse(req, "session_key_missing");
  }

  const url = new URL(req.url);
  const code = url.searchParams.get("code");
  if (!code) {
    return errorResponse(req, "missing_code");
  }

  let claims;
  try {
    claims = verifyExchangeCode(code, SESSION_ENCRYPT_KEY);
  } catch (err) {
    if (err instanceof ExchangeCodeError) {
      return errorResponse(req, err.code);
    }
    return errorResponse(req, "verify_failed");
  }

  const forwardedHost =
    req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const storeSlug = await resolveStoreSlug(forwardedHost);
  if (!storeSlug || storeSlug !== claims.storeSlug) {
    return errorResponse(req, "store_mismatch");
  }

  let returnHost: string;
  try {
    returnHost = new URL(claims.returnTo).hostname;
  } catch {
    return errorResponse(req, "invalid_return_to");
  }
  if (returnHost !== forwardedHost.split(":")[0]) {
    return errorResponse(req, "return_to_host_mismatch");
  }

  // customerSignIn verifies the GIP id_token, mints the per-host
  // mp_customer_session cookie, and triggers EnsureProfile.
  const result = await customerSignIn({
    idToken: claims.idToken,
    uid: "",
    storeSlug: claims.storeSlug,
  });
  if (!result.ok) {
    return errorResponse(req, result.code ?? "signin_failed");
  }

  return NextResponse.redirect(claims.returnTo, { status: 303 });
}

function errorResponse(req: Request, code: string): Response {
  const forwardedHost = req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const isLocal =
    forwardedHost.startsWith("localhost") || forwardedHost.startsWith("127.");
  const proto = req.headers.get("x-forwarded-proto") ?? (isLocal ? "http" : "https");
  const dest = forwardedHost
    ? `${proto}://${forwardedHost}/sign-in?error=${encodeURIComponent(code)}`
    : `/sign-in?error=${encodeURIComponent(code)}`;
  return NextResponse.redirect(dest, { status: 303 });
}
```

- [ ] **Step 1: Confirm `customerSignIn`'s shape allows blank `uid`**

Inspect `apps/storefront/app/sign-in/actions.ts`. The current signature:

```ts
interface CustomerSignInInput {
  idToken: string;
  uid: string; // Deprecated: ignored. The trusted UID comes from the verified idToken.
  storeSlug: string;
  email?: string;
}
```

The `uid` field is documented as deprecated/ignored. So passing `uid: ""` is safe. If the comment is stale and uid is actually used, fix the action to derive it from the verified token.

- [ ] **Step 2: Build + commit**

```bash
cd apps/storefront && npm run build 2>&1 | tail -10
cd ../..
git add apps/storefront/app/auth/google/finish/
git commit -m "feat(storefront): /auth/google/finish redeems exchange code from trampoline"
```

---

## Task 6: Rewrite storefront Google button handlers to redirect to trampoline

**Files:**
- Modify: `apps/storefront/components/auth/CustomerSignInForm.tsx` — re-add Google button + handler that redirects.
- Modify: `apps/storefront/components/auth/CreateAccountForm.tsx` — same.

The button JSX is the same shape as Phase 2 Task 4/5; the handler body becomes:

```ts
function handleGoogle() {
  const trampolineBase =
    process.env.NEXT_PUBLIC_MARK8LY_AUTH_URL ?? "https://mark8ly.com";
  const returnTo =
    typeof window !== "undefined"
      ? `${window.location.origin}/account`
      : returnUrl;
  const url = new URL("/auth/google", trampolineBase);
  url.searchParams.set("return_to", returnTo);
  url.searchParams.set("store_slug", storeSlug);
  url.searchParams.set("intent", "signin"); // or "signup" in CreateAccountForm
  window.location.assign(url.toString());
}
```

Differences vs original Phase 2 Task 4/5:
- No `googlePending` state (the redirect leaves the page).
- No `gipConfig.googleClientId` reads — the trampoline page handles that.
- Disable the password submit button only briefly while the redirect fires, OR don't disable it at all (the redirect makes any further interaction impossible). Cleanest: don't disable.

- [ ] **Step 1: Update `CustomerSignInForm.tsx`**

Re-add the divider + Google button JSX (same as in the original Phase 2 plan), but with the redirect-style `handleGoogle`. Pass `intent="signin"`.

- [ ] **Step 2: Update `CreateAccountForm.tsx`**

Same with `intent="signup"`. Both use `customerSignUp` already via the redirect path — actually the trampoline + finish route is intent-agnostic from the storefront's perspective; both end at `customerSignIn`, which auto-creates the profile via `EnsureProfile` for new emails. The `intent` param is used by the trampoline page only for friendly UX wording (`Continuing to <store>...`). Keep it for future flexibility.

- [ ] **Step 3: Build + commit**

```bash
cd apps/storefront && npm run build 2>&1 | tail -10
cd ../..
git add apps/storefront/components/auth/
git commit -m "feat(storefront): customer Google buttons redirect to mark8ly.com/auth/google trampoline"
```

---

## Task 7: Onboarding chart — add `NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID`

**Files (in `tesserix-k8s` repo):**
- Modify: `charts/apps/mark8ly-onboarding/values.yaml`
- Modify: `charts/apps/mark8ly-onboarding/templates/deployment.yaml`

The trampoline page reads `NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID` (via `publicConfig.gipCustomerTenantId`) to know which GIP pool to authenticate against.

- [ ] **Step 1: Add to values.yaml**

Add to the existing onboarding `public:` block:

```yaml
public:
  gipProjectId: "tesseracthub-480811"
  gipTenantId: "MP-Internal-e986p"
  gipCustomerTenantId: "MP-Customer-39opy"  # NEW — for /auth/google trampoline
  googleClientId: "849928263410-..."  # set in Task 2
```

- [ ] **Step 2: Add env entry to deployment.yaml**

After the existing `NEXT_PUBLIC_GIP_TENANT_ID` env entry:

```yaml
            - name: NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID
              value: {{ .Values.public.gipCustomerTenantId | quote }}
```

- [ ] **Step 3: helm template + commit + push**

```bash
helm template charts/apps/mark8ly-onboarding/ 2>&1 | grep NEXT_PUBLIC_GIP_CUSTOMER
git add charts/apps/mark8ly-onboarding/
git commit -m "chore(mark8ly-onboarding): expose NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID for trampoline"
git push origin main
```

---

## Verification & smoke

After Tasks 1-7 land, CI runs, and bump-k8s pushes the new image tags:

- [ ] **Verify ArgoCD sync.** All three apps (admin, onboarding, storefront) should land on Synced + Healthy.
- [ ] **Verify env vars on pods.**
  ```bash
  kubectl -n mark8ly get deploy mark8ly-onboarding -o yaml | grep -A1 NEXT_PUBLIC_GIP_CUSTOMER
  kubectl -n mark8ly get deploy mark8ly-onboarding -o yaml | grep -A1 NEXT_PUBLIC_GOOGLE_CLIENT_ID | head -5
  ```
  Expected: customer tenant ID `MP-Customer-39opy`; Google client ID populated.
- [ ] **Smoke test:**
  1. Visit `https://demo-store.mark8ly.com/sign-in` → click "Continue with Google".
  2. Browser navigates to `https://mark8ly.com/auth/google?...` — page shows "Continuing to demo-store…" + opens Google popup.
  3. Choose a Google account.
  4. Browser redirects to `https://demo-store.mark8ly.com/auth/google/finish?code=...` then to `/account`.
  5. DevTools → `mp_customer_session` cookie present, Domain `demo-store.mark8ly.com`.
  6. `customer_profiles` row exists with `gip_uid` populated.
- [ ] **Cross-store integrity:** repeat on `india-store.mark8ly.com` with the SAME Google account → new `customer_profiles` row at `(india-store, gip_uid)` (per-store identity model from Phase 1 design).
- [ ] **Custom domain:** if `primasyss.com` is wired, repeat there → cookie Domain `primasyss.com`.

---

## Rollback

Each task is independently revertible. The single user-visible piece is Task 6 (the buttons). Reverting Task 6 alone removes the buttons; existing password sign-in keeps working untouched. The trampoline page (Task 4) and finish route (Task 5) are dormant without buttons calling them.

The exchange-code helper (Task 3) is library code — reverting it would break Tasks 4 + 5 simultaneously.

If a deploy fails post-cutover, the recovery is:
1. `git revert` the storefront button commit (Task 6).
2. Push, wait for bump-k8s + ArgoCD.
3. Customers see only password sign-in until the next forward fix.

---

## Notes

- **OAuth origins stay limited** to `https://mark8ly.com` + `https://admin.mark8ly.com`. **No per-tenant origins**. As new tenants onboard, no Google-side action is required.
- **Custom domains** are NOT in the Google OAuth origins list either. The trampoline always runs on `mark8ly.com` (a fixed origin) and bounces back to whatever host the customer started on (including custom domains). The custom-domain hostname appears only in the storefront's per-host cookie Domain (Phase 1) and in the `claims.returnTo` of the exchange code.
- **OAuth client SECRET** — kept server-side in GIP (already configured). Never sent to the browser.
- **Defense in depth on the finish route**: store_slug check + return_to host equality check prevent an exchange code minted for store-A from being redeemed at store-B (cross-store replay), even within the 30-second window.
