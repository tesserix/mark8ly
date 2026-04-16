"use server";

// Server actions for the admin /login page.
//
// The client gathers GIP credentials (via signInWithPassword or
// signInWithGoogle), then hands the resulting id_token + uid to signIn.
// We look up the workspace tenant by uid, call auth-bff /auth/auto-login,
// and forward the resulting Set-Cookie back to the browser response.

import { cookies, headers } from "next/headers";

import {
  autoLogin,
  completeMFAChallenge,
  AuthBffError,
} from "@/lib/auth/auth-bff";
import {
  listMemberTenants,
  PlatformApiError,
} from "@/lib/api/platform-api";
import { publicConfig } from "@/lib/config";

const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";

// If the sign-in request is arriving at `{slug}-admin.mark8ly.com`,
// resolve that slug to a tenant id. Returns null when the host is not
// a per-tenant subdomain, when platform-api is unreachable, or when
// the slug isn't a known store. Best-effort — never throws.
async function tenantIdForHostSlug(
  hostHeader: string | null | undefined,
): Promise<string | null> {
  if (!hostHeader) return null;
  // Strip the port if one was appended (e.g. during local dev or tests).
  const host = hostHeader.split(":")[0] ?? "";
  const match = host.match(/^([^.]+)-admin\.mark8ly\.com$/);
  if (!match || !match[1]) return null;
  const slug = match[1];
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/by-slug/${encodeURIComponent(slug)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: { tenant_id?: string } };
    return body.data?.tenant_id ?? null;
  } catch {
    return null;
  }
}

type Result<T> =
  | { ok: true; data: T }
  | { ok: false; code: string; message: string };

function fail(err: unknown): { ok: false; code: string; message: string } {
  if (err instanceof PlatformApiError || err instanceof AuthBffError) {
    return { ok: false, code: err.code, message: err.message };
  }
  return { ok: false, code: "unknown", message: String(err) };
}

interface SignInInput {
  idToken: string;
  uid: string;
}

interface SignInSuccess {
  tenantId: string;
  // true when the user has more than one tenant and the UI should
  // redirect them to /pick-tenant after the initial dashboard land.
  // The initial tenant is the first one returned by platform-api's
  // /users/me/tenants (priority: owner first, then by created_at).
  multipleTenants: boolean;
  // true when the user has MFA enabled — the signIn flow wrote the
  // short-lived m8_mfa_pending cookie instead of a real session.
  // The UI must collect a 6-digit code and call confirmMFALogin
  // before treating the sign-in as complete.
  mfaRequired: boolean;
}

export async function signIn(
  input: SignInInput,
): Promise<Result<SignInSuccess>> {
  try {
    // Phase P: multi-tenant membership. A user can own one tenant and
    // be staff on another, so "find my workspace tenant" is no longer
    // a single-row lookup. We list every tenant the uid has any role
    // on, pick the first, and flag the caller if there's more than
    // one so the UI can offer a picker.
    const tenants = await listMemberTenants(input.uid);
    if (tenants.length === 0) {
      return {
        ok: false,
        code: "tenant_not_found",
        message: "no store found for this account",
      };
    }

    // Subdomain-aware primary selection. When the user signs in on
    // `{slug}-admin.mark8ly.com` AND the slug resolves to a tenant
    // the user belongs to, use that tenant instead of `tenants[0]`.
    // Without this, a founder who owns `india-store` but also has a
    // staff role on `demo-store` lands on the demo-store picker even
    // though the india-store subdomain is unambiguous.
    const h = await headers();
    const hostTenantId = await tenantIdForHostSlug(h.get("host"));
    const hostMatched = hostTenantId
      ? tenants.find((t) => t.tenant_id === hostTenantId) ?? null
      : null;
    const primary = hostMatched ?? tenants[0];
    if (!primary) {
      return {
        ok: false,
        code: "tenant_not_found",
        message: "no store found for this account",
      };
    }

    const result = await autoLogin({
      idToken: input.idToken,
      expectedTenantId: publicConfig.gipTenantId,
      workspaceTenant: primary.tenant_id,
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

    // When the host uniquely selects a tenant, skip the picker even if
    // the user belongs to multiple stores — we've already committed to
    // the one their subdomain points at.
    const multipleTenants = tenants.length > 1 && !hostMatched;

    return {
      ok: true,
      data: {
        tenantId: primary.tenant_id,
        multipleTenants,
        mfaRequired: result.mfaRequired,
      },
    };
  } catch (err) {
    return fail(err);
  }
}

/**
 * confirmMFALogin finishes a two-factor sign-in. The client already
 * has the m8_mfa_pending cookie from the preceding signIn call; we
 * forward it to auth-bff /auth/mfa-challenge along with the 6-digit
 * code. On success auth-bff mints the real session cookie which we
 * set on the response.
 */
export async function confirmMFALogin(
  code: string,
): Promise<Result<{ tenantId: string }>> {
  try {
    if (!code.trim()) {
      return {
        ok: false,
        code: "invalid_code",
        message: "Enter the 6-digit code from your authenticator app.",
      };
    }
    const c = await cookies();
    const cookieHeader = c
      .getAll()
      .map((x) => `${x.name}=${x.value}`)
      .join("; ");
    const result = await completeMFAChallenge(code.trim(), cookieHeader);
    if (result.setCookie) {
      const parsed = parseSetCookie(result.setCookie);
      if (parsed) {
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
    return { ok: true, data: { tenantId: result.tenant_id } };
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
