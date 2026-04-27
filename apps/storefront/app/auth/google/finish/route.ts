// Redeems an exchange code minted by the mark8ly.com/auth/google
// trampoline. Verifies HMAC + expiry + per-host scope, then calls
// the existing customerSignIn server action which mints the
// per-host mp_customer_session cookie (Phase 1) via the existing
// EnsureProfile flow.

import { NextResponse } from "next/server";
import {
  verifyExchangeCode,
  ExchangeCodeError,
} from "@repo/ui/auth/exchange-code";
import { resolveStoreSlug } from "@/lib/slug";
import { customerSignIn } from "@/app/sign-in/actions";

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

  let claims;
  try {
    claims = verifyExchangeCode(code, SESSION_ENCRYPT_KEY);
  } catch (err) {
    if (err instanceof ExchangeCodeError) {
      return errorResponse(req, err.code);
    }
    return errorResponse(req, "verify_failed");
  }

  // Resolve the request host through the storefront's existing host →
  // slug helper. For *.mark8ly.com it parses the slug; for custom
  // domains it queries the platform-api custom-domain registry.
  const forwardedHost =
    req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const requestHostname = forwardedHost.split(":")[0] ?? "";
  const resolvedSlug = await resolveStoreSlug(forwardedHost);
  if (!resolvedSlug || resolvedSlug !== claims.storeSlug) {
    return errorResponse(req, "store_mismatch");
  }

  // Defense in depth: the trampoline put the customer's intended
  // destination URL in claims.returnTo. Reject if its host doesn't
  // match the request host — an attacker holding a stolen code
  // shouldn't be able to redirect off-host.
  let returnHost: string;
  try {
    returnHost = new URL(claims.returnTo).hostname;
  } catch {
    return errorResponse(req, "invalid_return_to");
  }
  if (returnHost !== requestHostname) {
    return errorResponse(req, "return_to_host_mismatch");
  }

  // customerSignIn re-verifies the GIP id_token, mints the per-host
  // mp_customer_session cookie, and triggers EnsureProfile.
  // The `uid` field is documented as deprecated/ignored.
  const result = await customerSignIn({
    idToken: claims.idToken,
    uid: "",
    storeSlug: claims.storeSlug,
  });
  if (!result.ok) {
    return errorResponse(req, result.code ?? "signin_failed");
  }

  return NextResponse.redirect(claims.returnTo, { status: 303 });
}

function errorResponse(req: Request, code: string): Response {
  const forwardedHost =
    req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const isLocal =
    forwardedHost.startsWith("localhost") || forwardedHost.startsWith("127.");
  const proto =
    req.headers.get("x-forwarded-proto") ?? (isLocal ? "http" : "https");
  const dest = forwardedHost
    ? `${proto}://${forwardedHost}/sign-in?error=${encodeURIComponent(code)}`
    : `/sign-in?error=${encodeURIComponent(code)}`;
  return NextResponse.redirect(dest, { status: 303 });
}
