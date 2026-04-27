# GIP Auth — Phase 3: Account Merge + Linked Providers Settings

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Both admin merchants and storefront customers can have a single account that supports BOTH password and Google sign-in. When a user with an existing password account signs in with Google for the same email, GIP returns `needConfirmation` + a pending credential; we render a password-confirm overlay, link the providers via Identity Toolkit, and continue. Plus a settings page on each app to view linked providers and link/unlink Google.

**Architecture:**

```
Path A — auto-merge during sign-in:
  admin SignInForm OR mark8ly.com/auth/google trampoline
    ↓ signInWithIdp (Google credential)
    ↓ GIP responds: { needConfirmation: true, pendingIdpCredential, email, oauthIdToken }
    ↓ render LinkProviderPrompt — collect existing password
    ↓ signInWithPassword(email, password) → fresh GIP id_token
    ↓ accounts:signInWithIdp again with pendingIdToken → providers linked
    ↓ continue normal autoLogin / customerSignIn

Path B — explicit link from settings (storefront):
  /account/security "Link Google" button
    ↓ window.location.assign("mark8ly.com/auth/google?intent=link&return_to=/account/security")
    ↓ same handshake as Path A
    ↓ exchange code → /auth/google/finish → already-signed-in customer gets refreshed cookie

Path C — explicit link from settings (admin):
  /settings/security "Link Google" button
    ↓ inline gsi popup (admin.mark8ly.com is fixed origin)
    ↓ signInWithIdp → needConfirmation likely false (already signed in to GIP)
    ↓ if needConfirmation, password prompt; otherwise direct link
    ↓ refresh providers list
```

**Tech Stack:** Next.js 16 + React 19. Identity Toolkit REST. Existing per-app GIP helpers. New shared `@repo/ui` components for the prompt + provider panel.

**Spec:** `docs/superpowers/specs/2026-04-27-gip-auth-isolation-merge-design.md` § "Account merge".

**Branch policy:** all work commits directly to `main`. Single-line commits, no signoff, no Co-Authored-By.

---

## Pre-flight (out-of-band)

- [ ] **GCP console — enable "Link accounts that use the same email" on MP-Internal-e986p**
  - URL: `https://console.cloud.google.com/customer-identity/settings?project=tesseracthub-480811`
  - Tenant selector → MP-Internal
  - Setting: "If you sign in with a different provider that uses an email address that's already in use" → choose **Link accounts that use the same email**
  - Save.

- [ ] **Same on MP-Customer-39opy**
  - Same page, switch tenant to MP-Customer-39opy
  - Same setting enabled
  - Save.

The auto-merge handshake fails closed if these toggles are off (GIP returns `EMAIL_EXISTS` instead of `needConfirmation`).

---

## File structure

### Created
- `packages/ui/src/auth/link-provider-prompt.tsx` — password confirmation overlay component.
- `packages/ui/src/auth/linked-providers-panel.tsx` — provider list / link / unlink UI.
- `apps/onboarding/lib/gip/customer-link.ts` — `linkGoogleToPassword(email, password, pendingIdToken)` helper (calls signInWithPassword + linkIdp).
- `apps/admin/lib/gip/link.ts` — admin equivalent for the MP-Internal pool.
- `apps/admin/app/(admin)/settings/security/page.tsx` — admin Linked Providers settings.
- `apps/storefront/app/account/security/page.tsx` — storefront Linked Providers settings.
- `services/auth-bff/internal/session/providers_handler.go` — `GET /auth/me/providers` (admin lookup by GIP UID).

### Modified
- `apps/admin/components/auth/SignInForm.tsx` — handle `needConfirmation` from `signInWithGoogle`; render LinkProviderPrompt.
- `apps/onboarding/app/auth/google/page.tsx` — handle `needConfirmation` from `signInWithGoogleCustomer`; render LinkProviderPrompt.
- `apps/onboarding/lib/gip/customer-signin.ts` — return `{needConfirmation, pendingIdpCredential, email}` instead of throwing on `FEDERATED_USER_ID_ALREADY_LINKED`/needConfirmation paths.
- `apps/onboarding/lib/gip/signup.ts` — same shape change for the existing admin/internal signInWithGoogle (already exists).
- `apps/admin/lib/auth/auth-bff.ts` — `getMyProviders()` helper that calls `/auth/me/providers`.
- `services/auth-bff/internal/session/handler.go` — register the new providers route.

---

## Task 1: Shared `LinkProviderPrompt` component in `@repo/ui`

**Files:**
- Create: `packages/ui/src/auth/link-provider-prompt.tsx`
- Modify: `packages/ui/package.json` — add `./auth/link-provider-prompt` export.

**Why:** Same overlay used by admin SignInForm and onboarding trampoline. Shared component avoids drift.

**Component contract:**

```tsx
interface LinkProviderPromptProps {
  email: string;
  // Called when user submits password. Returns nothing on success;
  // the parent dispatches the actual link API call. Receives the
  // entered password.
  onConfirm: (password: string) => Promise<void>;
  onCancel: () => void;
  // Surface from the parent: if the link call fails (wrong password,
  // network error), the parent sets this so the prompt re-renders
  // with a friendly message and re-enables the form.
  error?: string | null;
  // Display variant: "admin" | "storefront" — affects copy + colors
  // (admin uses neutral admin theme, storefront uses --storefront-* tokens).
  variant?: "admin" | "storefront";
}
```

UI: a centred card (modal-ish) with:
- Headline: "Link Google to your existing account"
- Body: "An account with **{email}** already exists. Enter your password to add Google sign-in to it."
- Single password input
- Confirm + Cancel buttons (Confirm spinner-state during async)

- [ ] **Step 1: Implement the component**

```tsx
// packages/ui/src/auth/link-provider-prompt.tsx
"use client";

import { useState } from "react";

export interface LinkProviderPromptProps {
  email: string;
  onConfirm: (password: string) => Promise<void>;
  onCancel: () => void;
  error?: string | null;
  variant?: "admin" | "storefront";
}

export function LinkProviderPrompt({
  email,
  onConfirm,
  onCancel,
  error,
  variant = "admin",
}: LinkProviderPromptProps) {
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!password) return;
    setSubmitting(true);
    try {
      await onConfirm(password);
    } finally {
      setSubmitting(false);
    }
  }

  const isStorefront = variant === "storefront";
  const cardBg = isStorefront
    ? "bg-[color:var(--storefront-surface,#fff)]"
    : "bg-background-elevated";
  const textColor = isStorefront
    ? "text-[color:var(--storefront-text,var(--ink-900))]"
    : "text-foreground";
  const accentRing = isStorefront
    ? "focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
    : "focus-visible:outline-moss-700";
  const primaryBtn = isStorefront
    ? "bg-[color:var(--storefront-accent,var(--ink-900))] text-[color:var(--storefront-on-accent,var(--paper-200))]"
    : "bg-primary text-primary-foreground hover:bg-primary-hover";

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="link-provider-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
    >
      <div className={`w-full max-w-md rounded-lg ${cardBg} p-6 shadow-2`}>
        <h2 id="link-provider-title" className={`font-serif text-2xl ${textColor}`}>
          Link Google to your existing account
        </h2>
        <p className={`mt-3 text-sm ${textColor} opacity-75`}>
          An account with <strong>{email}</strong> already exists. Enter your
          existing password to add Google sign-in to that account.
        </p>

        <form onSubmit={handleSubmit} className="mt-5 space-y-4">
          <div className="space-y-1.5">
            <label
              htmlFor="link-prompt-password"
              className={`block text-sm font-medium ${textColor}`}
            >
              Password
            </label>
            <input
              id="link-prompt-password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={submitting}
              autoFocus
              className={`w-full rounded-md border border-current/20 px-3 py-2.5 text-base ${textColor} focus-visible:outline-2 focus-visible:outline-offset-2 ${accentRing}`}
            />
          </div>

          {error && (
            <p
              role="alert"
              className={`text-sm ${
                isStorefront
                  ? "text-[color:var(--storefront-danger,#a3322a)]"
                  : "text-danger"
              }`}
            >
              {error}
            </p>
          )}

          <div className="flex gap-3">
            <button
              type="submit"
              disabled={submitting || !password}
              className={`inline-flex h-11 flex-1 items-center justify-center rounded-md px-6 text-sm font-medium transition-opacity ${primaryBtn} disabled:cursor-not-allowed disabled:opacity-50`}
            >
              {submitting ? "Linking…" : "Confirm and link"}
            </button>
            <button
              type="button"
              onClick={onCancel}
              disabled={submitting}
              className={`inline-flex h-11 items-center justify-center rounded-md border border-current/20 px-5 text-sm font-medium ${textColor} disabled:cursor-not-allowed disabled:opacity-50`}
            >
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Add export entry**

In `packages/ui/package.json`, add the export before the catchall:

```json
"./auth/link-provider-prompt": "./src/auth/link-provider-prompt.tsx",
```

- [ ] **Step 3: Build to verify**

```bash
cd packages/ui && npx tsc --noEmit
```

- [ ] **Step 4: Commit**

```bash
git add packages/ui/src/auth/link-provider-prompt.tsx packages/ui/package.json
git commit -m "feat(ui): LinkProviderPrompt overlay for account-merge handshake"
```

---

## Task 2: Update onboarding `signInWithGoogleCustomer` to surface `needConfirmation`

**Files:**
- Modify: `apps/onboarding/lib/gip/customer-signin.ts`

**Why:** The current helper throws `CustomerGIPError` on any non-200. GIP returns `needConfirmation` as a 200-with-payload (NOT an error), so the current path falls through. We need to inspect the response and return `{ kind: "ok"|"needConfirmation", ... }`.

- [ ] **Step 1: Refactor return type**

```ts
// apps/onboarding/lib/gip/customer-signin.ts
import { publicConfig } from "@/lib/config";

export class CustomerGIPError extends Error {
  constructor(public code: string, message: string) {
    super(message);
  }
}

export type CustomerSigninResult =
  | { kind: "ok"; uid: string; idToken: string }
  | {
      kind: "needConfirmation";
      email: string;
      pendingIdpCredential: string;
      verifiedProvider: string[];
    };

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

  const data = (await res.json()) as {
    localId?: string;
    idToken?: string;
    needConfirmation?: boolean;
    email?: string;
    oauthIdToken?: string;
    verifiedProvider?: string[];
  };

  if (data.needConfirmation && data.email && data.oauthIdToken) {
    return {
      kind: "needConfirmation",
      email: data.email,
      pendingIdpCredential: data.oauthIdToken,
      verifiedProvider: data.verifiedProvider ?? [],
    };
  }

  if (!data.localId || !data.idToken) {
    throw new CustomerGIPError("malformed_response", "GIP response missing required fields");
  }

  return { kind: "ok", uid: data.localId, idToken: data.idToken };
}
```

- [ ] **Step 2: Add link helper**

Create `apps/onboarding/lib/gip/customer-link.ts`:

```ts
// apps/onboarding/lib/gip/customer-link.ts
//
// Completes the GIP account-merge handshake for MP-Customer pool.
// Called from the trampoline page after the user enters their existing
// password in the LinkProviderPrompt overlay.
//
// Two REST calls:
//   1. accounts:signInWithPassword — verify the password, get a fresh
//      GIP id_token bound to the existing user.
//   2. accounts:signInWithIdp — re-call with the pending Google
//      credential. With the user now signed in (via id_token), GIP
//      links the Google provider to the existing account.

import { publicConfig } from "@/lib/config";
import { CustomerGIPError } from "./customer-signin";

export interface LinkResult {
  uid: string;
  idToken: string;
}

export async function linkGoogleToCustomerPassword(
  email: string,
  password: string,
  pendingIdpCredential: string,
): Promise<LinkResult> {
  if (!publicConfig.gipApiKey) {
    throw new CustomerGIPError("config_missing", "GIP Web API key is not configured");
  }
  if (!publicConfig.gipCustomerTenantId) {
    throw new CustomerGIPError("config_missing", "GIP customer tenant id is not configured");
  }

  // Step 1: password sign-in.
  const passUrl = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${encodeURIComponent(publicConfig.gipApiKey)}`;
  const passRes = await fetch(passUrl, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email,
      password,
      tenantId: publicConfig.gipCustomerTenantId,
      returnSecureToken: true,
    }),
  });
  if (!passRes.ok) {
    const body = await passRes.json().catch(() => ({})) as { error?: { message?: string } };
    const code = body.error?.message ?? `HTTP ${passRes.status}`;
    if (
      code === "INVALID_PASSWORD" ||
      code === "EMAIL_NOT_FOUND" ||
      code === "INVALID_LOGIN_CREDENTIALS"
    ) {
      throw new CustomerGIPError("invalid_credentials", "Email or password is incorrect");
    }
    throw new CustomerGIPError("link_failed", code);
  }
  const passData = (await passRes.json()) as { localId: string; idToken: string };

  // Step 2: link the Google credential to the now-signed-in user.
  const linkUrl = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp?key=${encodeURIComponent(publicConfig.gipApiKey)}`;
  const requestUri =
    typeof window !== "undefined" ? window.location.origin : "https://mark8ly.com";
  const linkRes = await fetch(linkUrl, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenantId: publicConfig.gipCustomerTenantId,
      requestUri,
      postBody: `id_token=${encodeURIComponent(pendingIdpCredential)}&providerId=google.com`,
      returnSecureToken: true,
      returnIdpCredential: false,
      idToken: passData.idToken, // signs the link as the existing user
    }),
  });
  if (!linkRes.ok) {
    const body = await linkRes.json().catch(() => ({})) as { error?: { message?: string } };
    throw new CustomerGIPError("link_failed", body.error?.message ?? `HTTP ${linkRes.status}`);
  }
  const linkData = (await linkRes.json()) as { localId: string; idToken: string };

  return { uid: linkData.localId, idToken: linkData.idToken };
}
```

- [ ] **Step 3: Build + commit**

```bash
cd apps/onboarding && npm run build 2>&1 | tail -10
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add apps/onboarding/lib/gip/
git commit -m "feat(onboarding): customer signInWithIdp surfaces needConfirmation + link helper"
```

---

## Task 3: Wire `LinkProviderPrompt` into onboarding trampoline

**Files:**
- Modify: `apps/onboarding/app/auth/google/page.tsx`

- [ ] **Step 1: Add prompt state + handler to trampoline**

Replace the existing useEffect logic so it handles the `needConfirmation` branch:

```tsx
"use client";

import { useEffect, useState, Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { LinkProviderPrompt } from "@repo/ui/auth/link-provider-prompt";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  signInWithGoogleCustomer,
  CustomerGIPError,
} from "@/lib/gip/customer-signin";
import { linkGoogleToCustomerPassword } from "@/lib/gip/customer-link";
import { mintCustomerExchangeCode } from "./actions";

function TrampolineInner() {
  const params = useSearchParams();
  const [error, setError] = useState<string | null>(null);
  const [linkPromptError, setLinkPromptError] = useState<string | null>(null);
  const [status, setStatus] = useState<"idle" | "popup" | "exchanging" | "redirecting">(
    "idle",
  );
  const [needConfirmation, setNeedConfirmation] = useState<{
    email: string;
    pendingIdpCredential: string;
  } | null>(null);

  const returnTo = params.get("return_to") ?? "";
  const storeSlug = params.get("store_slug") ?? "";
  const intentParam = params.get("intent");
  const intent: "signin" | "signup" | "link" =
    intentParam === "signup" ? "signup" : intentParam === "link" ? "link" : "signin";

  async function completeAndRedirect(idToken: string) {
    const result = await mintCustomerExchangeCode({
      idToken,
      storeSlug,
      returnTo,
      intent: intent === "link" ? "signin" : intent,
    });
    if (!result.ok) {
      setError(result.error);
      return;
    }
    setStatus("redirecting");
    window.location.assign(result.redirectUrl);
  }

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
        const result = await signInWithGoogleCustomer(credential);
        if (cancelled) return;
        if (result.kind === "needConfirmation") {
          setNeedConfirmation({
            email: result.email,
            pendingIdpCredential: result.pendingIdpCredential,
          });
          setStatus("idle");
          return;
        }
        await completeAndRedirect(result.idToken);
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [returnTo, storeSlug, intent]);

  async function handleLinkConfirm(password: string) {
    if (!needConfirmation) return;
    setLinkPromptError(null);
    try {
      const linked = await linkGoogleToCustomerPassword(
        needConfirmation.email,
        password,
        needConfirmation.pendingIdpCredential,
      );
      setNeedConfirmation(null);
      await completeAndRedirect(linked.idToken);
    } catch (err) {
      if (err instanceof CustomerGIPError && err.code === "invalid_credentials") {
        setLinkPromptError("That password is incorrect. Please try again.");
        return;
      }
      setLinkPromptError(
        err instanceof Error ? err.message : "Could not link Google. Please try again.",
      );
    }
  }

  function handleLinkCancel() {
    setNeedConfirmation(null);
    setError("Linking cancelled. Sign in with email and password instead.");
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col items-center justify-center px-6 py-16 text-center">
      <h1 className="font-serif text-2xl">
        Continuing to {storeSlug || "store"}…
      </h1>
      <p className="mt-3 text-sm opacity-70">
        {status === "popup" && "Opening Google sign-in…"}
        {status === "exchanging" && "Completing sign-in…"}
        {status === "redirecting" && "Returning you to the store…"}
        {status === "idle" && !error && !needConfirmation && "Preparing…"}
      </p>
      {error && (
        <p role="alert" className="mt-4 text-sm text-[color:var(--danger,#a3322a)]">
          {error}
        </p>
      )}
      {needConfirmation && (
        <LinkProviderPrompt
          email={needConfirmation.email}
          variant="storefront"
          error={linkPromptError}
          onConfirm={handleLinkConfirm}
          onCancel={handleLinkCancel}
        />
      )}
    </main>
  );
}

export default function GoogleAuthTrampolinePage() {
  return (
    <Suspense fallback={null}>
      <TrampolineInner />
    </Suspense>
  );
}
```

- [ ] **Step 2: Build + commit**

```bash
cd apps/onboarding && npm run build 2>&1 | tail -20
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add apps/onboarding/app/auth/google/page.tsx
git commit -m "feat(onboarding): trampoline handles GIP needConfirmation via password prompt"
```

---

## Task 4: Wire `LinkProviderPrompt` into admin SignInForm

**Files:**
- Modify: `apps/admin/lib/gip/signup.ts` — make `signInWithGoogle` surface needConfirmation.
- Create: `apps/admin/lib/gip/link.ts` — admin link helper.
- Modify: `apps/admin/components/auth/SignInForm.tsx` — wire prompt.

Same shape as Task 2/3 but for the admin app. Admin's `signInWithGoogle` currently throws on any non-200; needs the same `result.kind === "ok" | "needConfirmation"` shape.

- [ ] **Step 1: Refactor admin signInWithGoogle**

In `apps/admin/lib/gip/signup.ts`, change `signInWithGoogle` return type to a discriminated union (parallel to onboarding's `customer-signin.ts`). Existing callers (`SignInForm.tsx`) check `gip.idToken` and `gip.uid` directly — update to switch on `result.kind`.

- [ ] **Step 2: Create `apps/admin/lib/gip/link.ts`**

Mirror `apps/onboarding/lib/gip/customer-link.ts` but use `publicConfig.gipTenantId` (MP-Internal pool).

- [ ] **Step 3: Wire prompt into `SignInForm.tsx`**

Add state for `needConfirmation` and `linkPromptError`. In `handleGoogle`, when `signInWithGoogle` returns `kind: "needConfirmation"`, set the state. Render `<LinkProviderPrompt variant="admin" ...>` when state is non-null. On confirm, call `linkGoogleToInternalPassword`, then continue with the existing `signIn` server action using the linked id_token.

- [ ] **Step 4: Build + commit**

```bash
cd apps/admin && npm run build 2>&1 | tail -20
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add apps/admin/
git commit -m "feat(admin): SignInForm handles GIP needConfirmation via password prompt"
```

---

## Task 5: `GET /auth/me/providers` on auth-bff (admin only)

**Files:**
- Create: `services/auth-bff/internal/session/providers_handler.go`
- Modify: `services/auth-bff/internal/session/handler.go` — register the route.

The endpoint reads the admin session cookie (`m8_session`), extracts the GIP UID, calls Identity Toolkit `accounts:lookup` against MP-Internal pool, and returns the list of linked providers.

This is admin-only because admin's settings page calls auth-bff. Storefront's settings page (Task 7) calls Identity Toolkit directly because it lives in the storefront pod which already does that pattern.

- [ ] **Step 1: Implement handler**

```go
// services/auth-bff/internal/session/providers_handler.go
package session

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
)

type ProvidersResponse struct {
    Providers []LinkedProvider `json:"providers"`
}

type LinkedProvider struct {
    ProviderID string `json:"provider_id"`
    Email      string `json:"email,omitempty"`
}

// GetMyProviders returns the linked providers for the current admin user.
// Calls Identity Toolkit accounts:lookup against MP-Internal pool.
func (h *Handler) getMyProviders(c *gin.Context) {
    s, err := h.mgr.Read(c.Request)
    if err != nil || s == nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "no_session"})
        return
    }
    if h.gipAPIKey == "" || h.gipInternalTenantID == "" {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gip_not_configured"})
        return
    }

    body := map[string]any{
        "localId":  []string{s.UID},
        "tenantId": h.gipInternalTenantID,
    }
    raw, _ := json.Marshal(body)
    apiURL := fmt.Sprintf(
        "https://identitytoolkit.googleapis.com/v1/accounts:lookup?key=%s",
        url.QueryEscape(h.gipAPIKey),
    )
    req, err := http.NewRequestWithContext(c.Request.Context(), "POST", apiURL, strings.NewReader(string(raw)))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "build_request_failed"})
        return
    }
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{Timeout: 5 * time.Second}
    res, err := client.Do(req)
    if err != nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gip_unreachable"})
        return
    }
    defer res.Body.Close()
    respBody, _ := io.ReadAll(res.Body)
    if res.StatusCode != http.StatusOK {
        c.JSON(http.StatusBadGateway, gin.H{"error": "gip_error", "status": res.StatusCode})
        return
    }

    var lookup struct {
        Users []struct {
            Email           string `json:"email"`
            ProviderUserInfo []struct {
                ProviderID string `json:"providerId"`
                Email      string `json:"email"`
            } `json:"providerUserInfo"`
            PasswordHash string `json:"passwordHash"`
        } `json:"users"`
    }
    if err := json.Unmarshal(respBody, &lookup); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "parse_failed"})
        return
    }
    if len(lookup.Users) == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
        return
    }
    user := lookup.Users[0]

    out := []LinkedProvider{}
    if user.PasswordHash != "" {
        out = append(out, LinkedProvider{ProviderID: "password", Email: user.Email})
    }
    for _, p := range user.ProviderUserInfo {
        out = append(out, LinkedProvider{ProviderID: p.ProviderID, Email: p.Email})
    }
    _ = errors.New // keep import alive — compiler tolerates removal if unused
    c.JSON(http.StatusOK, gin.H{"data": ProvidersResponse{Providers: out}})
}
```

(Adjust to match the `Handler` struct's existing fields. If `gipAPIKey` and `gipInternalTenantID` aren't fields, add them in `NewHandler` constructor and pass from `cmd/server/main.go`.)

- [ ] **Step 2: Register the route in `Register`**

In `handler.go`'s `Register`:

```go
r.GET("/me/providers", h.getMyProviders)
```

- [ ] **Step 3: Wire config in `cmd/server/main.go`**

Pass `cfg.GIPWebAPIKey` and `cfg.GIPInternalTenantID` into `NewHandler` (extend the constructor).

- [ ] **Step 4: Build + commit**

```bash
cd services/auth-bff && go build ./...
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
git add services/auth-bff/
git commit -m "feat(auth-bff): GET /auth/me/providers returns linked providers for current admin"
```

---

## Task 6: Shared `LinkedProvidersPanel` component

**Files:**
- Create: `packages/ui/src/auth/linked-providers-panel.tsx`
- Modify: `packages/ui/package.json`

```tsx
// packages/ui/src/auth/linked-providers-panel.tsx
"use client";

import { useState } from "react";

export interface LinkedProvider {
  providerId: string; // "password" | "google.com" | etc.
  email?: string;
}

export interface LinkedProvidersPanelProps {
  providers: LinkedProvider[];
  onLinkGoogle: () => void | Promise<void>;
  onUnlink: (providerId: string) => Promise<void>;
  variant?: "admin" | "storefront";
  // Disable unlink when this is the only auth method.
  pending?: boolean;
}

const PROVIDER_LABELS: Record<string, string> = {
  password: "Email & password",
  "google.com": "Google",
};

export function LinkedProvidersPanel({
  providers,
  onLinkGoogle,
  onUnlink,
  variant = "admin",
  pending = false,
}: LinkedProvidersPanelProps) {
  const [busy, setBusy] = useState<string | null>(null);
  const isStorefront = variant === "storefront";

  const hasGoogle = providers.some((p) => p.providerId === "google.com");
  const hasPassword = providers.some((p) => p.providerId === "password");

  async function handleUnlink(providerId: string) {
    if (providers.length <= 1) return; // last-provider guard
    if (!hasPassword && providerId === "google.com") return; // can't remove only auth
    setBusy(providerId);
    try {
      await onUnlink(providerId);
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-4">
      <ul className="divide-y divide-current/15">
        {providers.map((p) => (
          <li
            key={p.providerId}
            className="flex items-center justify-between py-3"
          >
            <div>
              <span className="block text-sm font-medium">
                {PROVIDER_LABELS[p.providerId] ?? p.providerId}
              </span>
              {p.email && (
                <span className="block text-xs opacity-70">{p.email}</span>
              )}
            </div>
            {providers.length > 1 && (
              <button
                type="button"
                onClick={() => handleUnlink(p.providerId)}
                disabled={busy === p.providerId || pending}
                className="text-xs underline underline-offset-4 opacity-80 hover:opacity-100 disabled:opacity-50"
              >
                {busy === p.providerId ? "Unlinking…" : "Unlink"}
              </button>
            )}
          </li>
        ))}
      </ul>

      {!hasGoogle && (
        <button
          type="button"
          onClick={() => onLinkGoogle()}
          disabled={pending}
          className={`inline-flex h-11 w-full items-center justify-center gap-3 rounded-md border border-current/20 px-6 text-sm font-medium transition-colors hover:border-current/40 disabled:cursor-not-allowed disabled:opacity-50 ${
            isStorefront ? "bg-[color:var(--storefront-surface)]" : "bg-background-elevated"
          }`}
        >
          <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
            <path d="M17.64 9.205c0-.638-.057-1.252-.164-1.841H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.259h2.908c1.702-1.567 2.684-3.875 2.684-6.615z" fill="#4285F4" />
            <path d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" fill="#34A853" />
            <path d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z" fill="#FBBC05" />
            <path d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z" fill="#EA4335" />
          </svg>
          Link Google account
        </button>
      )}
    </div>
  );
}
```

Add export:

```json
"./auth/linked-providers-panel": "./src/auth/linked-providers-panel.tsx",
```

```bash
git add packages/ui/src/auth/linked-providers-panel.tsx packages/ui/package.json
git commit -m "feat(ui): LinkedProvidersPanel component for settings pages"
```

---

## Task 7: Admin `/settings/security` page

**Files:**
- Create: `apps/admin/app/(admin)/settings/security/page.tsx` (server component shell)
- Create: `apps/admin/app/(admin)/settings/security/SecurityClient.tsx` (client component for state)
- Modify: `apps/admin/lib/auth/auth-bff.ts` — add `getMyProviders()` + `unlinkProvider(providerId)` helpers.

The page fetches providers from auth-bff, renders `LinkedProvidersPanel`. "Link Google" runs the gsi popup (admin.mark8ly.com is a fixed origin). Unlink calls Identity Toolkit `accounts:update` with `deleteProvider`.

```bash
git add apps/admin/
git commit -m "feat(admin): /settings/security page with linked-providers panel"
```

---

## Task 8: Storefront `/account/security` page

**Files:**
- Create: `apps/storefront/app/account/security/page.tsx` (server component)
- Create: `apps/storefront/app/account/security/SecurityClient.tsx`
- Create: `apps/storefront/app/api/account/providers/route.ts` (server-side proxy that calls Identity Toolkit `accounts:lookup` with the customer's GIP UID from the session)

For storefront, "Link Google" is a redirect to the trampoline with `intent=link`:

```ts
function handleLinkGoogle() {
  const url = new URL("/auth/google", "https://mark8ly.com");
  url.searchParams.set("return_to", `${window.location.origin}/account/security`);
  url.searchParams.set("store_slug", storeSlug);
  url.searchParams.set("intent", "link");
  window.location.assign(url.toString());
}
```

Unlink calls a storefront API route that uses Identity Toolkit `accounts:update` with `deleteProvider`.

```bash
git add apps/storefront/
git commit -m "feat(storefront): /account/security page with linked-providers panel"
```

---

## Verification & smoke

After all tasks land:

- [ ] **GIP toggles confirmed** — both pools have "Link accounts that use the same email" enabled.
- [ ] **Auto-merge admin path:** sign up with password (admin), sign out, sign in with Google → password prompt → linked → dashboard.
- [ ] **Auto-merge storefront path:** sign up with password (`india-store.mark8ly.com`), sign out, sign in with Google → trampoline shows password prompt → linked → bounce back → /account.
- [ ] **Settings — admin:** /settings/security shows password + Google. Unlink Google → no Google. Re-link via gsi popup → both back.
- [ ] **Settings — storefront:** /account/security shows password + Google. Unlink Google → no Google. Re-link via "Link Google account" button → trampoline → password prompt (likely needConfirmation since password still set) → linked → bounce back.

---

## Rollback

Each task is independently revertible. The most user-visible piece is Task 3 (trampoline merge prompt) and Task 4 (admin merge prompt). Reverting them removes the merge UX; if a customer or admin had `EMAIL_EXISTS` collision, they'd see a generic "Google sign-in failed" instead of the helpful prompt.

The GIP console toggle (pre-flight) can be turned off to revert to legacy `EMAIL_EXISTS` behavior; existing linked providers stay linked.
