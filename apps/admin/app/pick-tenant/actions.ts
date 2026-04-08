"use server";

// Server action for switching the current session's tenant.
// Used by /pick-tenant (post-login) and by the tenant switcher
// dropdown in AdminShell.
//
// The action forwards the browser's Cookie header to auth-bff so
// the existing session is verified, then re-sets the Set-Cookie
// from the auth-bff response via Next's cookies() API.

import { cookies, headers } from "next/headers";

import { switchTenant, AuthBffError } from "@/lib/auth/auth-bff";

export type SwitchTenantResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export async function switchToTenant(
  tenantId: string,
): Promise<SwitchTenantResult> {
  if (!tenantId) {
    return {
      ok: false,
      code: "invalid_input",
      message: "tenant id is required",
    };
  }

  const h = await headers();
  const cookieHeader = h.get("cookie") ?? "";

  try {
    const res = await switchTenant(tenantId, cookieHeader);
    if (res.setCookie) {
      const parsed = parseSetCookie(res.setCookie);
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
    return { ok: true };
  } catch (err) {
    if (err instanceof AuthBffError) {
      return { ok: false, code: err.code, message: err.message };
    }
    return {
      ok: false,
      code: "unknown",
      message: err instanceof Error ? err.message : String(err),
    };
  }
}

// parseSetCookie is a local duplicate of the same helper in
// app/login/actions.ts. Small enough that a shared util isn't
// worth the import graph bend; will hoist to lib/ if a third
// caller shows up.
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
    else if (lower.startsWith("max-age="))
      out.maxAge = parseInt(attr.slice(8), 10);
  }
  return out;
}
