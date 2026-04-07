"use server";

// Server actions for the onboarding wizard.
//
// Each action is a thin wrapper around the typed platform-api / auth-bff
// clients. They return discriminated-union results so the client never has
// to throw — failures show up as { ok: false, code, message } and the UI
// renders an error inline.
//
// The session cookie minted by auto-login is forwarded to the browser via
// next/headers cookies().set(), so the user lands on the welcome page
// already authenticated.

import { cookies } from "next/headers";

import { onboarding, tenants, PlatformApiError } from "@/lib/api/platform-api";
import { autoLogin as bffAutoLogin, AuthBffError } from "@/lib/auth/auth-bff";
import { publicConfig } from "@/lib/config";

type Result<T> =
  | { ok: true; data: T }
  | { ok: false; code: string; message: string };

function fail(err: unknown): { ok: false; code: string; message: string } {
  if (err instanceof PlatformApiError || err instanceof AuthBffError) {
    return { ok: false, code: err.code, message: err.message };
  }
  return { ok: false, code: "unknown", message: String(err) };
}

// ─── Step 1: create session ─────────────────────────────────────────────
export async function createSession(email: string): Promise<Result<{ sessionId: string }>> {
  try {
    const sess = await onboarding.createSession(email);
    return { ok: true, data: { sessionId: sess.id } };
  } catch (err) {
    return fail(err);
  }
}

// ─── Step 2: send OTP ───────────────────────────────────────────────────
export async function sendVerification(
  sessionId: string,
): Promise<Result<{ sent: true }>> {
  try {
    await onboarding.sendVerification(sessionId);
    return { ok: true, data: { sent: true } };
  } catch (err) {
    return fail(err);
  }
}

// ─── Step 2b: verify OTP ────────────────────────────────────────────────
export async function verifyCode(
  sessionId: string,
  code: string,
): Promise<Result<{ verified: true }>> {
  try {
    await onboarding.verifyCode(sessionId, code);
    return { ok: true, data: { verified: true } };
  } catch (err) {
    return fail(err);
  }
}

// ─── Step 3: live slug check ────────────────────────────────────────────
export async function checkSlug(slug: string): Promise<Result<{ available: boolean }>> {
  try {
    const r = await tenants.isSlugAvailable(slug);
    return { ok: true, data: { available: r.available } };
  } catch (err) {
    return fail(err);
  }
}

// ─── Step 4: complete + auto-login (THE bug-fix path) ───────────────────
//
// Wraps three calls in one server action so the client only awaits once:
//   1. POST /onboarding/sessions/:id/complete  (creates tenant + outbox FGA writes)
//   2. POST /auth/auto-login                    (verifies token, retries FGA, mints cookie)
//   3. cookies().set(...)                       (forwards the session cookie to browser)
export async function completeAndLogin(input: {
  sessionId: string;
  businessName: string;
  slug: string;
  ownerUserId: string;
  ownerEmail: string;
  countryCode: string;
  currencyCode: string;
  timezone: string;
  idToken: string;
}): Promise<Result<{ tenantId: string; slug: string }>> {
  try {
    // Step 1: complete onboarding (atomic tenant + outbox tx)
    const completion = await onboarding.complete(input.sessionId, {
      business_name: input.businessName,
      slug: input.slug,
      owner_user_id: input.ownerUserId,
      owner_email: input.ownerEmail,
      country_code: input.countryCode,
      currency_code: input.currencyCode,
      timezone: input.timezone,
    });

    // Step 2: auto-login. auth-bff retries the FGA check until the
    // outbox drainer has shipped the membership tuple.
    const result = await bffAutoLogin({
      idToken: input.idToken,
      expectedTenantId: publicConfig.gipTenantId,
      workspaceTenant: completion.tenant_id,
    });

    // Step 3: forward auth-bff's session cookie to the browser response.
    if (result.setCookie) {
      // Parse the Set-Cookie header into name + value + attrs and set it.
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

// parseSetCookie pulls out the bits of a Set-Cookie header that next/headers
// cookies().set() needs. Minimal parser — only handles the attributes
// auth-bff actually emits.
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
