// Cross-TLD admin session handoff — receiver side.
//
// The canonical admin session cookie (`m8_session`) is scoped to
// `.mark8ly.com`. Custom-domain admins (admin.<merchant-tld>) live on
// a different TLD, so the cookie can't ride along on the navigation.
//
// This route redeems a short-lived HMAC handoff code minted by the
// canonical admin (after a verified pick-tenant or sign-in), forwards
// it to auth-bff `/auth/mint-from-admin-handoff` (which runs the FGA
// membership check), and re-emits the resulting Set-Cookie via Next's
// cookies() API so it lands on the custom-admin host.
//
// Defense in depth: this route verifies the HMAC locally too. A code
// minted for `admin.A` redeemed at `admin.B` is rejected here before
// auth-bff is even called.

import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import {
  verifyAdminHandoffCode,
  AdminHandoffError,
} from "@repo/ui/auth/admin-handoff-code";
import { config } from "@/lib/config";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const SESSION_ENCRYPT_KEY = process.env.SESSION_ENCRYPT_KEY ?? "";

export async function GET(req: Request): Promise<Response> {
  if (!SESSION_ENCRYPT_KEY) {
    return errorResponse(req, "session_key_missing");
  }

  const url = new URL(req.url);
  const code = url.searchParams.get("code");
  if (!code) {
    return errorResponse(req, "missing_code");
  }

  // Local HMAC verify (auth-bff will verify again — this is a fast
  // reject so we don't even hop to the BFF for an obviously bad code).
  let claims;
  try {
    claims = verifyAdminHandoffCode(code, SESSION_ENCRYPT_KEY);
  } catch (err) {
    if (err instanceof AdminHandoffError) {
      return errorResponse(req, err.code);
    }
    return errorResponse(req, "verify_failed");
  }

  // Bind the handoff to the request host. A code minted for
  // admin.primasyss.com must be redeemed there — not at admin.evil.com.
  const forwardedHost =
    req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const requestHostname = forwardedHost.split(":")[0]?.toLowerCase() ?? "";
  if (claims.target_host.toLowerCase() !== requestHostname) {
    return errorResponse(req, "host_mismatch");
  }

  // POST to auth-bff. It re-verifies the HMAC, runs the FGA membership
  // check (privilege gate that prevents an arbitrary handoff from
  // upgrading to a session for a tenant the user has no access to),
  // then mints the session with Domain=target_host.
  let bffRes: Response;
  try {
    bffRes = await fetch(`${config.authBffUrl}/auth/mint-from-admin-handoff`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
      cache: "no-store",
    });
  } catch {
    return errorResponse(req, "auth_bff_unreachable");
  }

  if (!bffRes.ok) {
    let upstreamCode = "mint_failed";
    try {
      const body = (await bffRes.json()) as { error?: string };
      if (body.error) upstreamCode = body.error;
    } catch {
      // ignore body parse errors — surface generic mint_failed
    }
    return errorResponse(req, upstreamCode);
  }

  // Forward the auth-bff Set-Cookie onto this response via Next's
  // cookies() API. parseSetCookie is the same helper the login flow
  // uses — see apps/admin/app/login/actions.ts.
  const setCookieHeader = bffRes.headers.get("set-cookie");
  if (!setCookieHeader) {
    return errorResponse(req, "missing_set_cookie");
  }

  const parsed = parseSetCookie(setCookieHeader);
  if (!parsed) {
    return errorResponse(req, "set_cookie_parse_failed");
  }

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

  // `next` is the path to land on after the cookie is set. Path-only
  // (no host) so an attacker can't reuse the handoff to redirect
  // somewhere external; if the value isn't a valid path we fall back
  // to /dashboard.
  const nextPath = sanitizeNextPath(url.searchParams.get("next"));
  const proto =
    req.headers.get("x-forwarded-proto") ??
    (forwardedHost.startsWith("localhost") || forwardedHost.startsWith("127.")
      ? "http"
      : "https");
  const dest = `${proto}://${forwardedHost}${nextPath}`;
  return NextResponse.redirect(dest, { status: 303 });
}

function errorResponse(req: Request, code: string): Response {
  const forwardedHost =
    req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const isLocal =
    forwardedHost.startsWith("localhost") || forwardedHost.startsWith("127.");
  const proto =
    req.headers.get("x-forwarded-proto") ?? (isLocal ? "http" : "https");
  const dest = forwardedHost
    ? `${proto}://${forwardedHost}/login?error=${encodeURIComponent(code)}`
    : `/login?error=${encodeURIComponent(code)}`;
  return NextResponse.redirect(dest, { status: 303 });
}

function sanitizeNextPath(raw: string | null | undefined): string {
  if (!raw) return "/dashboard";
  // Reject absolute URLs and protocol-relative paths.
  if (!raw.startsWith("/") || raw.startsWith("//")) return "/dashboard";
  return raw;
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

  const out: {
    name: string;
    value: string;
    path?: string;
    domain?: string;
    httpOnly: boolean;
    secure: boolean;
    maxAge?: number;
  } = {
    name,
    value,
    httpOnly: false,
    secure: false,
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
