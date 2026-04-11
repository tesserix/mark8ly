"use server";

// Server actions for the admin /login page.
//
// The client gathers GIP credentials (via signInWithPassword or
// signInWithGoogle), then hands the resulting id_token + uid to signIn.
// We look up the workspace tenant by uid, call auth-bff /auth/auto-login,
// and forward the resulting Set-Cookie back to the browser response.

import { cookies } from "next/headers";

import { autoLogin, AuthBffError } from "@/lib/auth/auth-bff";
import {
  listMemberTenants,
  PlatformApiError,
} from "@/lib/api/platform-api";
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
    const primary = tenants[0];
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

    return {
      ok: true,
      data: {
        tenantId: primary.tenant_id,
        multipleTenants: tenants.length > 1,
      },
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
