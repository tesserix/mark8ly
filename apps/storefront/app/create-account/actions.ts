"use server";

// Server action for storefront customer account creation.
// Same flow as customerSignIn but uses a freshly-created GIP account.

import { cookies } from "next/headers";

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8085";
const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";
const GIP_CUSTOMER_TENANT_ID =
  process.env.GIP_CUSTOMER_TENANT_ID ?? "";

type Result =
  | { ok: true }
  | { ok: false; code: string; message: string };

interface CustomerSignUpInput {
  idToken: string;
  uid: string;
  storeSlug: string;
}

async function resolveTenantId(storeSlug: string): Promise<string | null> {
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/by-slug/${encodeURIComponent(storeSlug)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: { tenant_id?: string } };
    return body.data?.tenant_id ?? null;
  } catch {
    return null;
  }
}

export async function customerSignUp(
  input: CustomerSignUpInput,
): Promise<Result> {
  try {
    const tenantId = await resolveTenantId(input.storeSlug);
    if (!tenantId) {
      return {
        ok: false,
        code: "store_not_found",
        message: "Could not resolve this store. Please try again.",
      };
    }

    const res = await fetch(`${AUTH_BFF_URL}/auth/auto-login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id_token: input.idToken,
        expected_tenant_id: GIP_CUSTOMER_TENANT_ID,
        workspace_tenant: tenantId,
      }),
      cache: "no-store",
    });

    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as {
        error?: string;
        message?: string;
      } | null;
      return {
        ok: false,
        code: body?.error ?? "auth_error",
        message:
          body?.message ??
          "Account creation failed. Please try again.",
      };
    }

    const setCookieHeader = res.headers.get("set-cookie");
    if (setCookieHeader) {
      const parsed = parseSetCookie(setCookieHeader);
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
    return {
      ok: false,
      code: "unknown",
      message: err instanceof Error ? err.message : String(err),
    };
  }
}

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
