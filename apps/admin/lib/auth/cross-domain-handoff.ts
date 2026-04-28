"use server";

// Cross-TLD admin handoff — sender side.
//
// Called from the canonical admin (or any slug-admin host on
// .mark8ly.com) when the next navigation target lives on a custom
// admin domain (admin.<merchant-tld>). The session cookie is scoped to
// .mark8ly.com so it can't ride along to admin.<custom>; we mint a
// short-lived (30s) HMAC handoff code carrying the current session's
// identity claims, then return a rewritten URL that points at the
// receiver route on the target host:
//
//   https://admin.<custom>/auth/handoff?code=<jwt>&next=/dashboard
//
// The receiver verifies the HMAC, posts the code to auth-bff which
// runs an FGA membership check, and finally lands a fresh m8_session
// cookie scoped to admin.<custom>.

import { cookies } from "next/headers";
import { mintAdminHandoffCode } from "@repo/ui/auth/admin-handoff-code";
import { config } from "@/lib/config";

const SESSION_COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "m8_session";
const SESSION_ENCRYPT_KEY = process.env.SESSION_ENCRYPT_KEY ?? "";
const HANDOFF_TTL_SECONDS = 30;

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface CrossDomainNavigationInput {
  /** Slug of the tenant the user just selected — used to look up the
   *  custom admin domain via marketplace-api. */
  slug?: string;
  /** A returnUrl coming from the /login page — used when sign-in
   *  finishes on canonical and the user was bounced from a slug-admin
   *  or a custom-admin host. We parse the host out and decide. */
  returnUrl?: string;
  /** Path to land on after the handoff. Defaults to /dashboard.
   *  Path-only — never an absolute URL. */
  nextPath?: string;
}

export type CrossDomainNavigationResult =
  | { kind: "no_handoff" }
  | { kind: "handoff"; url: string }
  | { kind: "error"; code: string; message: string };

export async function prepareCrossDomainNavigation(
  input: CrossDomainNavigationInput,
): Promise<CrossDomainNavigationResult> {
  if (!SESSION_ENCRYPT_KEY) {
    return {
      kind: "error",
      code: "session_key_missing",
      message: "SESSION_ENCRYPT_KEY is not configured",
    };
  }

  const targetHost = await resolveTargetHost(input);
  if (!targetHost) {
    return { kind: "no_handoff" };
  }

  // Validate the session cookie via auth-bff. We need the resolved
  // user/tenant claims (and proof the cookie is valid) before minting
  // a handoff. The cookie value is opaque to admin (auth-bff
  // AES-encrypted it), so introspection has to go through the BFF.
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get(SESSION_COOKIE_NAME);
  if (!sessionCookie) {
    return {
      kind: "error",
      code: "no_session",
      message: "no admin session cookie",
    };
  }

  let session: {
    user_id: string;
    email: string;
    tenant_id: string;
    store_id?: string;
  } | null = null;
  try {
    const res = await fetch(`${config.authBffUrl}/auth/session`, {
      method: "GET",
      headers: { Cookie: `${SESSION_COOKIE_NAME}=${sessionCookie.value}` },
      cache: "no-store",
    });
    if (res.ok) {
      const body = (await res.json()) as {
        data: {
          user_id: string;
          email: string;
          tenant_id: string;
          store_id?: string;
        };
      };
      session = body.data;
    }
  } catch {
    return {
      kind: "error",
      code: "auth_bff_unreachable",
      message: "auth-bff unreachable",
    };
  }
  if (!session) {
    return {
      kind: "error",
      code: "invalid_session",
      message: "session is invalid or expired",
    };
  }

  let code: string;
  try {
    code = mintAdminHandoffCode(
      {
        uid: session.user_id,
        email: session.email,
        tenant_id: session.tenant_id,
        store_id: session.store_id,
        target_host: targetHost,
      },
      SESSION_ENCRYPT_KEY,
      HANDOFF_TTL_SECONDS,
    );
  } catch (err) {
    return {
      kind: "error",
      code: "mint_failed",
      message: err instanceof Error ? err.message : String(err),
    };
  }

  const nextPath = sanitizeNextPath(input.nextPath);
  const url = new URL(`https://${targetHost}/auth/handoff`);
  url.searchParams.set("code", code);
  url.searchParams.set("next", nextPath);
  return { kind: "handoff", url: url.toString() };
}

// resolveTargetHost decides whether the next navigation actually
// crosses TLDs and, if so, what the target host is. Two input shapes:
//
//   • slug — pick-tenant flow. Look up the slug's custom admin
//     domain in marketplace-api. If empty, no handoff is needed
//     (target lives on .mark8ly.com).
//   • returnUrl — sign-in flow. Parse the host out; if it's an
//     off-mark8ly admin.<...> host, that's the target. Otherwise no
//     handoff is needed.
//
// Returns null when the target is on the same TLD as the current
// session cookie.
async function resolveTargetHost(
  input: CrossDomainNavigationInput,
): Promise<string | null> {
  if (input.slug) {
    const customDomain = await fetchCustomAdminDomain(input.slug);
    if (!customDomain) return null;
    return `admin.${customDomain}`;
  }

  if (input.returnUrl) {
    let parsed: URL;
    try {
      parsed = new URL(input.returnUrl);
    } catch {
      return null;
    }
    const host = parsed.host.toLowerCase();
    if (!host.startsWith("admin.")) return null;
    if (host.endsWith(".mark8ly.com") || host.endsWith(".mark8ly.dev")) {
      return null;
    }
    if (host.split(".").length < 3) return null;
    return host;
  }

  return null;
}

async function fetchCustomAdminDomain(slug: string): Promise<string | null> {
  if (!slug) return null;
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/internal/store-active-domain/${encodeURIComponent(slug)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json().catch(() => ({}))) as {
      custom_domain?: string;
    };
    const trimmed = (body.custom_domain ?? "").trim().toLowerCase();
    return trimmed || null;
  } catch {
    return null;
  }
}

function sanitizeNextPath(raw: string | null | undefined): string {
  if (!raw) return "/dashboard";
  if (!raw.startsWith("/") || raw.startsWith("//")) return "/dashboard";
  return raw;
}
