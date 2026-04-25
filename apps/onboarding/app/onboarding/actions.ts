"use server";

// Server actions for the magic-link onboarding flow.
//
// Phase M restructure: the form no longer collects a password or GIP
// credentials. The flow is now:
//
//   1. submitOnboarding(form)   → creates session + saves business draft +
//                                 sends magic link. Pure form data.
//
//   2. (user clicks magic link)
//
//   3. verifyToken(token)       → marks the session verified server-side
//                                 and returns its id. The verify page
//                                 then redirects to /onboarding/set-password.
//
//   4. completeOnboarding(...)  → called from the set-password page after
//                                 the user picks a password OR completes
//                                 the Google popup. Reads the draft,
//                                 calls platform-api complete (creates
//                                 tenant + outbox FGA writes), calls
//                                 auth-bff /auth/auto-login, forwards
//                                 the session cookie to the browser.

import { cookies } from "next/headers";

import { onboarding, tenants, users, PlatformApiError } from "@/lib/api/platform-api";
import { autoLogin as bffAutoLogin, AuthBffError } from "@/lib/auth/auth-bff";
import { config, publicConfig } from "@/lib/config";

type Result<T> =
  | { ok: true; data: T }
  | { ok: false; code: string; message: string };

function fail(err: unknown): { ok: false; code: string; message: string } {
  if (err instanceof PlatformApiError || err instanceof AuthBffError) {
    return { ok: false, code: err.code, message: err.message };
  }
  return { ok: false, code: "unknown", message: String(err) };
}

// ─── Live slug check (live as user types in the form) ──────────────────
export async function checkSlug(
  slug: string,
): Promise<Result<{ available: boolean }>> {
  try {
    const r = await tenants.isSlugAvailable(slug);
    return { ok: true, data: { available: r.available } };
  } catch (err) {
    return fail(err);
  }
}

// ─── submitOnboarding: form submit → create session + send magic link ──
interface SubmitInput {
  email: string;
  businessName: string;
  slug: string;
  countryCode: string;
  currencyCode: string;
  timezone: string;
  // §5.1.1 — optional; persisted to draft so completeOnboarding can
  // forward them to the backend tax-ID endpoint once the store exists.
  taxId?: string;
  migrationType?: "new" | "migrating";
  whoisUrl?: string;
  // screenshot_url is the GCS URL returned by the upload helper; the
  // raw File object never reaches the server action.
  screenshotUrl?: string;
}

export async function submitOnboarding(
  input: SubmitInput,
): Promise<Result<{ sessionId: string }>> {
  try {
    const sess = await onboarding.createSession(input.email);

    // Persist the business fields into the session draft so the
    // /onboarding/set-password page (reached after the magic link click)
    // can read them server-side without depending on per-tab state.
    // §5.1.1: include migration fast-path evidence in the draft so
    // completeOnboarding can forward them to the tax-ID endpoint once
    // the store row has been created.  Fields absent when "new store"
    // is selected are simply omitted — the backend ignores them.
    const draft: Record<string, unknown> = {
      business_name: input.businessName,
      slug: input.slug,
      country_code: input.countryCode,
      currency_code: input.currencyCode,
      timezone: input.timezone,
    };
    if (input.taxId) draft.tax_id = input.taxId;
    if (input.migrationType) draft.migration_type = input.migrationType;
    if (input.whoisUrl) draft.whois_url = input.whoisUrl;
    if (input.screenshotUrl) draft.screenshot_url = input.screenshotUrl;

    await onboarding.saveDraft(sess.id, draft);

    await onboarding.sendVerification(sess.id, input.businessName);

    return { ok: true, data: { sessionId: sess.id } };
  } catch (err) {
    return fail(err);
  }
}

// ─── resendMagicLink: re-send the verification email ───────────────────
export async function resendMagicLink(
  sessionId: string,
  businessName: string,
): Promise<Result<{ sent: true }>> {
  try {
    await onboarding.sendVerification(sessionId, businessName);
    return { ok: true, data: { sent: true } };
  } catch (err) {
    return fail(err);
  }
}

// ─── verifyToken: magic link click → mark verified, return session id ──
//
// The verify landing page calls this on mount. It only marks the session
// verified and returns the session id + email so the page can redirect
// to /onboarding/set-password. Tenant creation + auto-login happen later
// in completeOnboarding, after the user has picked a credential.
export async function verifyToken(
  token: string,
): Promise<Result<{ sessionId: string; email: string }>> {
  try {
    const verifyRes = await fetch(
      `${config.platformApiUrl}/api/v1/onboarding/verify-token`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
        cache: "no-store",
      },
    );
    if (!verifyRes.ok) {
      const body = (await verifyRes.json().catch(() => ({}))) as {
        error?: string;
        message?: string;
      };
      return {
        ok: false,
        code: body.error ?? "verify_failed",
        message: body.message ?? "Verification link is invalid or expired",
      };
    }
    const verifyBody = (await verifyRes.json()) as {
      data: { session_id: string; email: string };
    };
    return {
      ok: true,
      data: {
        sessionId: verifyBody.data.session_id,
        email: verifyBody.data.email,
      },
    };
  } catch (err) {
    return fail(err);
  }
}

// ─── completeOnboarding: set-password submit → tenant + auto-login ─────
//
// Called from the /onboarding/set-password page once the user has either
// (a) signed up with email + password via the GIP REST helper, or
// (b) completed the "Continue with Google" popup via signInWithGoogle.
// Both paths produce a fresh GIP id_token + uid + refreshToken; this
// action takes those plus the session id and finishes the pipeline:
//
//   1. Read the persisted business draft + email from the session.
//   2. Call platform-api complete — creates the tenant row + outbox FGA
//      writes in one transaction.
//   3. Call auth-bff /auth/auto-login (which retries the FGA check until
//      the outbox drainer ships the membership tuple).
//   4. Forward the resulting Set-Cookie to the browser response.
interface CompleteInput {
  sessionId: string;
  gipUid: string;
  gipIdToken: string;
}

export async function completeOnboarding(
  input: CompleteInput,
): Promise<Result<{ tenantId: string; slug: string }>> {
  try {
    const sess = await onboarding.getSession(input.sessionId);
    const draft = sess.draft ?? {};
    const businessName = draft.business_name ?? "";
    const slug = draft.slug ?? "";
    const countryCode = draft.country_code ?? "";
    const currencyCode = draft.currency_code ?? "";
    const timezone = draft.timezone ?? "UTC";

    if (!businessName || !slug || !countryCode || !currencyCode) {
      return {
        ok: false,
        code: "draft_incomplete",
        message:
          "We couldn't recover your store details. Please start onboarding again.",
      };
    }

    const existingTenants = await users.listMemberTenants(input.gipUid);
    if (existingTenants.length > 0) {
      return {
        ok: false,
        code: "identity_already_has_store",
        message:
          "This admin identity is already connected to a store. Use a different email for a separate admin identity, or sign in to the existing account and add a store from Settings.",
      };
    }

    const completion = await onboarding.complete(input.sessionId, {
      business_name: businessName,
      slug,
      owner_user_id: input.gipUid,
      owner_email: sess.email,
      country_code: countryCode,
      currency_code: currencyCode,
      timezone,
    });

    const result = await bffAutoLogin({
      idToken: input.gipIdToken,
      expectedTenantId: publicConfig.gipTenantId,
      workspaceTenant: completion.tenant_id,
    });

    if (result.setCookie) {
      const parsed = parseSetCookie(result.setCookie);
      if (parsed) {
        const c = await cookies();
        c.set({
          name: parsed.name,
          value: parsed.value,
          path: parsed.path ?? "/",
          domain: parsed.domain,
          httpOnly: parsed.httpOnly,
          secure: parsed.secure,
          sameSite: "lax",
          maxAge: parsed.maxAge,
        });
      }
    }

    return {
      ok: true,
      data: { tenantId: completion.tenant_id, slug: completion.slug },
    };
  } catch (err) {
    return fail(err);
  }
}

// parseSetCookie pulls the bits next/headers cookies().set() needs out of
// a Set-Cookie header. Minimal parser — only the attributes auth-bff emits.
function parseSetCookie(raw: string): {
  name: string;
  value: string;
  path?: string;
  domain?: string;
  httpOnly: boolean;
  secure: boolean;
  maxAge?: number;
} | null {
  const parts = raw.split(";").map((p) => p.trim());
  const [first, ...attrs] = parts;
  if (!first || !first.includes("=")) return null;
  const eq = first.indexOf("=");
  const name = first.slice(0, eq);
  const value = first.slice(eq + 1);

  const out = {
    name,
    value,
    httpOnly: false,
    secure: false,
  } as {
    name: string;
    value: string;
    path?: string;
    domain?: string;
    httpOnly: boolean;
    secure: boolean;
    maxAge?: number;
  };
  for (const attr of attrs) {
    const lower = attr.toLowerCase();
    if (lower === "httponly") out.httpOnly = true;
    else if (lower === "secure") out.secure = true;
    else if (lower.startsWith("path=")) out.path = attr.slice(5);
    else if (lower.startsWith("domain=")) out.domain = attr.slice(7);
    else if (lower.startsWith("max-age=")) out.maxAge = parseInt(attr.slice(8), 10);
  }
  return out;
}
