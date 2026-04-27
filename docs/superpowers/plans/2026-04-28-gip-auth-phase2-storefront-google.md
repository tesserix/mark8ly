# GIP Auth — Phase 2: Storefront Google Sign-In Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Continue with Google" button to the storefront's `CustomerSignInForm` and `CreateAccountForm`. Google credential flows through gsi/client → Identity Toolkit `signInWithIdp` against the **MP-Customer** GIP tenant pool, then through the existing `customerSignIn` server action, which already mints `mp_customer_session` per-host (Phase 1) and triggers `EnsureProfile` to create/look up the `customer_profiles` row.

**Architecture:** Pure additive on top of Phase 1. No auth-bff changes, no schema changes, no new server endpoints. The storefront's existing `gipConfig` prop pattern (page → form) is extended with `googleClientId`. A new `apps/storefront/lib/gip/google-gsi.ts` (ported from admin) handles the gsi popup. A new `signInWithGoogleCustomer(credential, tenantId, apiKey)` helper does the `signInWithIdp` REST call. Both forms get a Google button next to the password form.

**Tech Stack:** Next.js 16 server components/server actions, gsi/client browser SDK (loaded lazily), GIP Identity Toolkit REST.

**Spec:** `docs/superpowers/specs/2026-04-27-gip-auth-isolation-merge-design.md` § "Storefront Google sign-in"

**Branch policy:** all work commits directly to `main`. Each task ends with a commit. Single-line commit messages, no signoff, no `Co-Authored-By`.

---

## Pre-flight

- ✅ Phase 1 deployed (commit `201cce2`, image `sha-201cce2*`).
- ✅ Storefront passes `gipConfig: { apiKey, tenantId, projectId }` via prop from `apps/storefront/app/{sign-in,create-account}/page.tsx` to the auth form components.
- ✅ Existing `customerSignIn` (`apps/storefront/app/sign-in/actions.ts`) accepts `{ idToken, uid, storeSlug }` and trusts the verified id_token claims — no changes needed for Google id_tokens (signInWithIdp returns the same shape as signInWithPassword).
- ✅ Admin's `apps/admin/lib/gip/google-gsi.ts` (112 lines) uses `publicConfig.googleClientId`. Storefront has no `publicConfig` module — port the helper to take `clientId` as a parameter or via `gipConfig`, not via global config.
- ✅ Existing `verifyGIPIdToken` in `apps/storefront/lib/gip/verify-id-token.ts` validates the id_token signature/audience server-side, regardless of provider — no change.

## File structure

### Created
- `apps/storefront/lib/gip/google-gsi.ts` — gsi/client wrapper (parameterized client_id).
- `apps/storefront/lib/gip/signup.ts` — `signInWithGoogleCustomer(credential, gipConfig)`.
- `apps/storefront/lib/gip/google-gsi.test.ts` (optional — heavy DOM mocking; skip if not pragmatic).

### Modified
- `apps/storefront/components/auth/CustomerSignInForm.tsx` — add Google button + handler.
- `apps/storefront/components/auth/CreateAccountForm.tsx` — add Google button + handler.
- `apps/storefront/app/sign-in/page.tsx` — extend `gipConfig` to include `googleClientId`.
- `apps/storefront/app/create-account/page.tsx` — same.
- `apps/storefront/components/auth/types.ts` (or wherever the `GipConfig` interface lives — likely inline in each form; consider extracting if both forms use it).

### Untouched (intentional)
- `services/auth-bff/**` — admin path, separate.
- `services/marketplace-api/**` — `EnsureProfile` already handles customer creation from any verified id_token.
- `apps/admin/**` — Phase 2 is storefront-only.
- `apps/storefront/app/sign-in/actions.ts` — `customerSignIn` flow accepts the id_token from Google `signInWithIdp` exactly the same way as from `signInWithPassword`. No change needed.

---

## Task 1: Extend `gipConfig` shape with `googleClientId` and pass through pages

**Files:**
- Modify: `apps/storefront/app/sign-in/page.tsx`
- Modify: `apps/storefront/app/create-account/page.tsx`
- Modify: `apps/storefront/components/auth/CustomerSignInForm.tsx` (the inline `GipConfig` interface)
- Modify: `apps/storefront/components/auth/CreateAccountForm.tsx` (the inline `GipConfig` interface)

**Why:** Both forms need the Google OAuth client ID. The existing prop pattern (`gipConfig` carrying `apiKey`/`tenantId`/`projectId`) is the natural place to add it.

- [ ] **Step 1: Add `googleClientId` to the page-level `gipConfig` object literals.**

In both `apps/storefront/app/sign-in/page.tsx` and `apps/storefront/app/create-account/page.tsx`:

```ts
const gipConfig = {
  apiKey:
    process.env.GIP_WEB_API_KEY ??
    process.env.NEXT_PUBLIC_GIP_API_KEY ??
    "",
  tenantId: process.env.GIP_CUSTOMER_TENANT_ID ?? "",
  projectId: process.env.GIP_PROJECT_ID ?? "",
  googleClientId: process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID ?? "",
};
```

- [ ] **Step 2: Update the `GipConfig` interface in both forms.**

`apps/storefront/components/auth/CustomerSignInForm.tsx`:
```ts
interface GipConfig {
  apiKey: string;
  tenantId: string;
  projectId: string;
  googleClientId: string;
}
```

Same shape in `CreateAccountForm.tsx`.

- [ ] **Step 3: Build the storefront**

```bash
cd apps/storefront && npm run build 2>&1 | tail -20
```

Expected: clean build (the field is added but not yet used).

- [ ] **Step 4: Commit**

```bash
git add apps/storefront/app/sign-in/page.tsx apps/storefront/app/create-account/page.tsx apps/storefront/components/auth/CustomerSignInForm.tsx apps/storefront/components/auth/CreateAccountForm.tsx
git commit -m "feat(storefront): thread googleClientId through gipConfig prop"
```

---

## Task 2: Port `google-gsi.ts` to storefront

**Files:**
- Create: `apps/storefront/lib/gip/google-gsi.ts`

**Why:** The browser-side gsi/client wrapper. Admin's version reads `publicConfig.googleClientId` at module level — that doesn't fit the storefront's per-form prop pattern. Adapt to take `clientId` as a parameter.

- [ ] **Step 1: Create the file**

```ts
// apps/storefront/lib/gip/google-gsi.ts
//
// Browser-only wrapper around Google Identity Services (gsi/client).
// Loads the GSI script lazily, then triggers the popup/one-tap and
// resolves with the Google credential JWT. The caller exchanges that
// credential via signInWithGoogleCustomer (Identity Toolkit
// signInWithIdp) for a GIP id_token in the MP-Customer tenant pool.
//
// Diverges from admin's identical-looking helper only in that the
// client_id is a parameter — storefront has no global publicConfig
// module, and the value comes via the gipConfig prop pattern.

const SCRIPT_URL = "https://accounts.google.com/gsi/client";

interface GsiCredentialResponse {
  credential: string;
}

interface GsiNotification {
  isNotDisplayed(): boolean;
  isSkippedMoment(): boolean;
  isDismissedMoment(): boolean;
  getNotDisplayedReason(): string;
  getSkippedReason(): string;
  getDismissedReason(): string;
}

interface GsiAccountsId {
  initialize(opts: {
    client_id: string;
    callback: (resp: GsiCredentialResponse) => void;
    auto_select?: boolean;
    cancel_on_tap_outside?: boolean;
    use_fedcm_for_prompt?: boolean;
  }): void;
  prompt(callback?: (n: GsiNotification) => void): void;
  cancel(): void;
}

declare global {
  interface Window {
    google?: {
      accounts: {
        id: GsiAccountsId;
      };
    };
  }
}

let scriptPromise: Promise<void> | null = null;

function loadScript(): Promise<void> {
  if (typeof window === "undefined") {
    return Promise.reject(new Error("google sign-in only available in the browser"));
  }
  if (window.google?.accounts?.id) return Promise.resolve();
  if (scriptPromise) return scriptPromise;

  scriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${SCRIPT_URL}"]`,
    );
    if (existing) {
      existing.addEventListener("load", () => resolve());
      existing.addEventListener("error", () => reject(new Error("gsi load failed")));
      return;
    }
    const s = document.createElement("script");
    s.src = SCRIPT_URL;
    s.async = true;
    s.defer = true;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error("gsi load failed"));
    document.head.appendChild(s);
  });
  return scriptPromise;
}

export async function getGoogleCredential(
  clientId: string,
): Promise<{ credential: string }> {
  if (!clientId) {
    throw new Error(
      "Google sign-in is not configured (NEXT_PUBLIC_GOOGLE_CLIENT_ID missing)",
    );
  }
  await loadScript();
  const gsi = window.google!.accounts.id;

  return new Promise((resolve, reject) => {
    gsi.initialize({
      client_id: clientId,
      callback: (resp) => {
        if (resp?.credential) {
          resolve({ credential: resp.credential });
        } else {
          reject(new Error("google: empty credential"));
        }
      },
      auto_select: false,
      cancel_on_tap_outside: true,
      use_fedcm_for_prompt: true,
    });
    gsi.prompt((n) => {
      if (n.isNotDisplayed() || n.isSkippedMoment() || n.isDismissedMoment()) {
        reject(
          new Error(
            n.getNotDisplayedReason() ||
              n.getSkippedReason() ||
              n.getDismissedReason() ||
              "google: prompt dismissed",
          ),
        );
      }
    });
  });
}
```

- [ ] **Step 2: Build to verify**

```bash
cd apps/storefront && npm run build 2>&1 | tail -10
```

- [ ] **Step 3: Commit**

```bash
git add apps/storefront/lib/gip/google-gsi.ts
git commit -m "feat(storefront): port google-gsi helper for customer sign-in"
```

---

## Task 3: Add `signInWithGoogleCustomer` helper

**Files:**
- Create: `apps/storefront/lib/gip/signup.ts`

**Why:** The Identity Toolkit `signInWithIdp` exchange — converts a Google credential into a GIP id_token in the MP-Customer pool. Returns the same shape (`{uid, idToken}`) the existing `customerSignIn` server action already accepts.

- [ ] **Step 1: Create the file**

```ts
// apps/storefront/lib/gip/signup.ts
//
// Browser-side helper that exchanges a Google credential JWT for a GIP
// id_token in the MP-Customer tenant pool. Mirrors admin's signInWithGoogle
// but is parameterized on apiKey + tenantId so it fits the storefront's
// per-form gipConfig prop pattern.
//
// On success the caller hands { idToken, uid } to the customerSignIn
// server action, which mints mp_customer_session and triggers
// EnsureProfile in marketplace-api.

export class StorefrontGIPError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

export interface CustomerSignInResult {
  uid: string;
  idToken: string;
}

export async function signInWithGoogleCustomer(
  googleIdToken: string,
  config: { apiKey: string; tenantId: string },
): Promise<CustomerSignInResult> {
  if (!config.apiKey) {
    throw new StorefrontGIPError(
      "config_missing",
      "GIP Web API key is not configured",
    );
  }
  if (!config.tenantId) {
    throw new StorefrontGIPError(
      "config_missing",
      "GIP customer tenant id is not configured",
    );
  }

  const url =
    "https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=" +
    encodeURIComponent(config.apiKey);
  const requestUri =
    typeof window !== "undefined" ? window.location.origin : "https://mark8ly.com";

  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: config.tenantId,
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
    throw new StorefrontGIPError(
      "google_signin_failed",
      body.error?.message ?? `HTTP ${res.status}`,
    );
  }

  const wrapper = (await res.json()) as {
    localId: string;
    idToken: string;
  };
  return { uid: wrapper.localId, idToken: wrapper.idToken };
}
```

- [ ] **Step 2: Build to verify**

```bash
cd apps/storefront && npm run build 2>&1 | tail -10
```

- [ ] **Step 3: Commit**

```bash
git add apps/storefront/lib/gip/signup.ts
git commit -m "feat(storefront): signInWithGoogleCustomer GIP helper for MP-Customer pool"
```

---

## Task 4: Add Google button to `CustomerSignInForm`

**Files:**
- Modify: `apps/storefront/components/auth/CustomerSignInForm.tsx`

**Why:** Returning Google customer signs in.

- [ ] **Step 1: Read the current form**

```bash
sed -n '1,200p' apps/storefront/components/auth/CustomerSignInForm.tsx
```

Confirm the structure: a `<form onSubmit>` with email + password fields and a submit button. The Google button should sit BELOW the form's submit button with a divider ("or") between, mirroring admin's `SignInForm.tsx:314-333`.

- [ ] **Step 2: Add Google handler + button**

Imports at top:
```ts
import { useState as useStateForGoogle } from "react"; // already imported as useState
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  signInWithGoogleCustomer,
  StorefrontGIPError,
} from "@/lib/gip/signup";
```

(If `useState` is already imported, don't add a duplicate. If imports already include `react` types, just add the GIP imports.)

State + handler inside the component:
```ts
const [googlePending, setGooglePending] = useState(false);

async function handleGoogle() {
  setError(null);
  setGooglePending(true);
  try {
    const { credential } = await getGoogleCredential(gipConfig.googleClientId);
    const gip = await signInWithGoogleCustomer(credential, {
      apiKey: gipConfig.apiKey,
      tenantId: gipConfig.tenantId,
    });
    const result = await customerSignIn({
      idToken: gip.idToken,
      uid: gip.uid,
      storeSlug,
    });
    if (!result.ok) {
      setError(result.message);
      return;
    }
    router.push(returnUrl);
    router.refresh();
  } catch (err) {
    if (err instanceof StorefrontGIPError) {
      setError(
        err.code === "config_missing"
          ? "Google sign-in is not available for this store yet."
          : "Google sign-in failed. Please try again or use email and password.",
      );
    } else {
      setError(
        err instanceof Error
          ? `Google sign-in failed: ${err.message}`
          : "Google sign-in failed. Please try again.",
      );
    }
  } finally {
    setGooglePending(false);
  }
}
```

JSX: after the existing submit button (the password "Sign in" button), add:

```tsx
<div className="relative py-1">
  <div className="absolute inset-0 flex items-center" aria-hidden="true">
    <div className="w-full border-t border-[color:var(--storefront-text,var(--ink-900))]/15" />
  </div>
  <div className="relative flex justify-center">
    <span className="bg-[color:var(--storefront-background,var(--paper-200))] px-3 text-xs uppercase tracking-wider text-[color:var(--storefront-text,var(--ink-900))]/55">
      or
    </span>
  </div>
</div>

<button
  type="button"
  onClick={handleGoogle}
  disabled={pending || googlePending}
  className="inline-flex h-11 w-full items-center justify-center gap-3 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-6 text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))] transition-colors hover:border-[color:var(--storefront-text,var(--ink-900))]/40 disabled:cursor-not-allowed disabled:opacity-50"
>
  {/* Inline Google G mark — keeps the form free of new shared-component deps. */}
  <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
    <path d="M17.64 9.205c0-.638-.057-1.252-.164-1.841H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.259h2.908c1.702-1.567 2.684-3.875 2.684-6.615z" fill="#4285F4"/>
    <path d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" fill="#34A853"/>
    <path d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z" fill="#FBBC05"/>
    <path d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z" fill="#EA4335"/>
  </svg>
  {googlePending ? "Opening Google..." : "Continue with Google"}
</button>
```

The disabled prop on the password Sign-in button must also include `googlePending` so password and Google can't fire simultaneously:

```tsx
<button
  type="submit"
  disabled={pending || googlePending}
  ...
>
```

- [ ] **Step 3: Build**

```bash
cd apps/storefront && npm run build 2>&1 | tail -20
```

- [ ] **Step 4: Commit**

```bash
git add apps/storefront/components/auth/CustomerSignInForm.tsx
git commit -m "feat(storefront): add Continue with Google to customer sign-in form"
```

---

## Task 5: Add Google button to `CreateAccountForm`

**Files:**
- Modify: `apps/storefront/components/auth/CreateAccountForm.tsx`

**Why:** First-time Google customer creates account.

- [ ] **Step 1: Mirror Task 4 structure exactly**

The handler is the same shape — calls `getGoogleCredential` → `signInWithGoogleCustomer` → `customerSignUp` (which delegates to `customerSignIn`).

```ts
async function handleGoogle() {
  setError(null);
  setGooglePending(true);
  try {
    const { credential } = await getGoogleCredential(gipConfig.googleClientId);
    const gip = await signInWithGoogleCustomer(credential, {
      apiKey: gipConfig.apiKey,
      tenantId: gipConfig.tenantId,
    });
    const result = await customerSignUp({
      idToken: gip.idToken,
      uid: gip.uid,
      storeSlug,
    });
    if (!result.ok) {
      setError(result.message);
      return;
    }
    router.push(returnUrl);
    router.refresh();
  } catch (err) {
    if (err instanceof StorefrontGIPError) {
      setError(
        err.code === "config_missing"
          ? "Google sign-up is not available for this store yet."
          : "Google sign-up failed. Please try again or use email and password.",
      );
    } else {
      setError(
        err instanceof Error
          ? `Google sign-up failed: ${err.message}`
          : "Google sign-up failed. Please try again.",
      );
    }
  } finally {
    setGooglePending(false);
  }
}
```

JSX: same divider + Google button, copy from Task 4. The button label should still read "Continue with Google" (not "Sign up with Google") — Google's brand guidelines are flexible, and the wording stays consistent across sign-in and create-account.

Disable the password Create-account button when `googlePending`.

- [ ] **Step 2: Build**

```bash
cd apps/storefront && npm run build 2>&1 | tail -20
```

- [ ] **Step 3: Commit**

```bash
git add apps/storefront/components/auth/CreateAccountForm.tsx
git commit -m "feat(storefront): add Continue with Google to create account form"
```

---

## Task 6: tesserix-k8s — wire `NEXT_PUBLIC_GOOGLE_CLIENT_ID` into storefront pod

**Files (in `tesserix-k8s` repo):**
- Modify: `charts/apps/mark8ly-storefront/values.yaml` (or equivalent)

**Why:** The build- and runtime-side env var must be set on the storefront pod. The admin app already has it; storefront did not need it pre-Phase-2.

- [ ] **Step 1: Inspect admin's chart**

```bash
cd ../tesserix-k8s
grep -rn 'NEXT_PUBLIC_GOOGLE_CLIENT_ID' charts/apps/mark8ly-admin/ 2>/dev/null
```

Note where it's defined (env block? configmap? from a secret?).

- [ ] **Step 2: Mirror in storefront chart**

Add the same env entry to `charts/apps/mark8ly-storefront/values.yaml`. The value source should be identical to the admin's (same OAuth client ID — single GCP project).

- [ ] **Step 3: Commit + push**

```bash
git add charts/apps/mark8ly-storefront/values.yaml
git commit -m "chore(mark8ly-storefront): expose NEXT_PUBLIC_GOOGLE_CLIENT_ID for Phase 2 customer Google sign-in"
git push origin main
```

- [ ] **Step 4: After ArgoCD syncs, verify the var is set**

```bash
kubectl -n mark8ly get deploy mark8ly-storefront -o yaml | grep -A1 NEXT_PUBLIC_GOOGLE
```

---

## Task 7: GIP console — confirm Google provider on MP-Customer pool

**(Out-of-band, no code change. Done in GCP console.)**

- [ ] GCP console → Identity Platform → Tenants → `MP-Customer-XXXXX` → Providers → Google → Enabled. Authorized domain `mark8ly.com` plus any custom domains in the list.
- [ ] GCP console → APIs & Services → Credentials → mark8ly OAuth client → Authorized JavaScript origins includes `https://*.mark8ly.com` (and per-custom-domain entries).

If Google is NOT yet enabled on the customer pool, enable it. Per memory `mark8ly_deploy_state.md`, the original Phase 7b lists this as user-action remaining; verify state.

---

## Verification & smoke

After Tasks 1-6 land and the next bump-k8s job propagates:

- [ ] **Build images include the new code.** `kubectl -n mark8ly get deploy mark8ly-storefront -o jsonpath='{.spec.template.spec.containers[*].image}'` should show `sha-<short>` matching the latest mark8ly main commit.
- [ ] **Smoke test on prod:**
  - Visit `https://<slug>.mark8ly.com/sign-in` → "Continue with Google" button visible.
  - Click → Google popup → choose an account → land on `/account` with `mp_customer_session` set.
  - DevTools → cookie Domain is exactly `<slug>.mark8ly.com`.
  - In another tab visit `https://<slug>.mark8ly.com/create-account` for a fresh email → "Continue with Google" → new `customer_profiles` row created.
- [ ] **Verify customer profile**: `psql ... -c "SELECT id, email, gip_uid FROM customer_profiles WHERE email='<your-test-email>';"` — `gip_uid` should be populated.
- [ ] **Existing password customer**: with a customer that already has a password account, sign in via Google with the same email. Phase 3 will add the auto-merge handshake; in Phase 2 the GIP-side behavior depends on whether "Link accounts that use the same email" is enabled. If it is, Phase 2 effectively gets auto-merge for free; if not, GIP returns `EMAIL_EXISTS` and the user gets a generic error. Either is acceptable for Phase 2 — the merge UX lands in Phase 3.

---

## Rollback

Each task is independently reversible via `git revert`. The most user-visible commits are Tasks 4 and 5 (the buttons). Reverting them removes the buttons; the storefront returns to password-only. No data migration involved.

For the env var in tesserix-k8s (Task 6), revert that commit to remove the var. The storefront app handles missing client ID gracefully (`getGoogleCredential` throws "google sign-in not configured").
