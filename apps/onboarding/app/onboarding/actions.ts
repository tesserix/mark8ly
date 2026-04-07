"use server";

// Server actions for the magic-link onboarding flow.
//
// Two top-level actions:
//
//   1. submitOnboarding(form)  → creates session, saves draft, sends magic
//                                link. Called from the single-page form.
//
//   2. verifyAndLogin(token)   → consumes the magic link token, completes
//                                onboarding (creates tenant + outbox FGA),
//                                refreshes the GIP id_token, calls auth-bff
//                                auto-login, sets session cookie. Called
//                                from the verify landing page.
//
// Plus support actions: checkSlug, resendMagicLink.

import { cookies } from "next/headers";

import { onboarding, tenants, PlatformApiError } from "@/lib/api/platform-api";
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

interface SubmitInput {
  email: string;
  businessName: string;
  slug: string;
  countryCode: string;
  currencyCode: string;
  timezone: string;
}

// ─── submitOnboarding: form submit → create session + send magic link ──
export async function submitOnboarding(
  input: SubmitInput,
): Promise<Result<{ sessionId: string }>> {
  try {
    const sess = await onboarding.createSession(input.email);

    // Save the business draft so the verify endpoint has it later.
    // (We use the existing /complete endpoint AT verify time, so we don't
    // actually need to save the draft separately — the verify action will
    // pass everything fresh.)

    // Send the magic link.
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

interface VerifyInput {
  token: string;
  // The form fields, captured at submit time, sent again here so the verify
  // action has everything it needs to complete onboarding in one shot.
  businessName: string;
  slug: string;
  countryCode: string;
  currencyCode: string;
  timezone: string;
  // GIP credentials from the client-side signup at form-submit time.
  gipUid: string;
  gipIdToken: string;
}

// ─── verifyAndLogin: magic link click → complete + auto-login ──────────
//
// The verify landing page calls this on mount. It does the entire
// completion + auto-login pipeline in one server action so the client
// just shows a spinner and waits.
export async function verifyAndLogin(
  input: VerifyInput,
): Promise<Result<{ tenantId: string; slug: string }>> {
  try {
    // Step 1: validate the magic link token. Returns the session_id +
    // email, marks the session verified.
    const verifyRes = await fetch(
      `${config.platformApiUrl}/api/v1/onboarding/verify-token`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: input.token }),
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
    const sessionId = verifyBody.data.session_id;
    const email = verifyBody.data.email;

    // Step 2: complete onboarding (creates tenant + outbox FGA writes).
    const completion = await onboarding.complete(sessionId, {
      business_name: input.businessName,
      slug: input.slug,
      owner_user_id: input.gipUid,
      owner_email: email,
      country_code: input.countryCode,
      currency_code: input.currencyCode,
      timezone: input.timezone,
    });

    // Step 3: auto-login. auth-bff retries the FGA check until the
    // outbox drainer has shipped the membership tuple.
    const result = await bffAutoLogin({
      idToken: input.gipIdToken,
      expectedTenantId: publicConfig.gipTenantId,
      workspaceTenant: completion.tenant_id,
    });

    // Step 4: forward auth-bff's session cookie to the browser response.
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

  const out: ReturnType<typeof parseSetCookie> = {
    name,
    value,
    httpOnly: false,
    secure: false,
  };
  for (const attr of attrs) {
    const lower = attr.toLowerCase();
    if (lower === "httponly") out!.httpOnly = true;
    else if (lower === "secure") out!.secure = true;
    else if (lower.startsWith("path=")) out!.path = attr.slice(5);
    else if (lower.startsWith("domain=")) out!.domain = attr.slice(7);
    else if (lower.startsWith("max-age=")) out!.maxAge = parseInt(attr.slice(8), 10);
  }
  return out;
}
