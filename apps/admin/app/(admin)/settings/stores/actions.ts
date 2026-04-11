"use server";

// Phase Q.2 — store management server actions.
//
// Three actions live here:
//
//   1. createNewStore  — owner/admin creates a second (or third, …)
//      store under the current tenant. FGA-gated server-side.
//   2. switchToStore   — re-mints the session cookie with a new
//      store id so the user "enters" a different store.
//   3. (future)          archive / delete a store.

import { cookies, headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  createStore,
  PlatformApiError,
  type Store,
} from "@/lib/api/platform-api";
import { switchStore, AuthBffError } from "@/lib/auth/auth-bff";

type CreateResult =
  | { ok: true; store: Store }
  | { ok: false; code: string; message: string };

type SwitchResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

function failPlatform(err: unknown): {
  ok: false;
  code: string;
  message: string;
} {
  if (err instanceof PlatformApiError) {
    return { ok: false, code: err.code, message: err.message };
  }
  return {
    ok: false,
    code: "unknown",
    message: err instanceof Error ? err.message : String(err),
  };
}

export interface CreateStoreInput {
  slug: string;
  name: string;
  country_code: string;
  currency_code: string;
  timezone: string;
}

export async function createNewStore(
  input: CreateStoreInput,
): Promise<CreateResult> {
  const h = await headers();
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const uid = h.get("x-session-user-id") ?? "";
  if (!tenantId || !uid) {
    return {
      ok: false,
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
    };
  }

  try {
    const store = await createStore(tenantId, {
      uid,
      slug: input.slug.trim(),
      name: input.name.trim(),
      country_code: input.country_code.trim().toUpperCase(),
      currency_code: input.currency_code.trim().toUpperCase(),
      timezone: input.timezone.trim(),
    });
    revalidatePath("/settings/stores");
    revalidatePath("/", "layout");
    return { ok: true, store };
  } catch (err) {
    return failPlatform(err);
  }
}

export async function switchToStore(storeId: string): Promise<SwitchResult> {
  if (!storeId) {
    return {
      ok: false,
      code: "invalid_input",
      message: "store id is required",
    };
  }
  const h = await headers();
  const cookieHeader = h.get("cookie") ?? "";

  try {
    const res = await switchStore(storeId, cookieHeader);
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

// Local copy of the parseSetCookie helper — hoisting to lib/ when
// a fifth caller shows up.
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
