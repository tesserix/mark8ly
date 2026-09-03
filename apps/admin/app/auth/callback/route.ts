// Completes the Zitadel side of the login-client OIDC flow (Task 4 of
// docs/superpowers/plans/2026-09-03-zitadel-phase3a-admin-frontend.md).
//
// By the time the browser reaches this route, auth-bff has already
// minted the `m8_session` cookie: `signInWithZitadel` drove Zitadel's
// login-client APIs directly with the `auth_request_id` obtained by
// `app/login/authorize/route.ts`, and auth-bff handed back this
// route's URL as `callback_url` once the session existed. This
// route's only jobs are to (1) confirm the `state` that round-tripped
// through Zitadel matches the one minted at the start of the flow, so
// a forged or replayed callback can't complete someone else's flow or
// redirect a freshly-authenticated user anywhere, and (2) land the
// browser on the right destination. It does NOT exchange `code` for
// anything — there is nothing left to exchange it for, and it must
// not try.

import { NextResponse, type NextRequest } from "next/server";
import {
  ZITADEL_STATE_COOKIE,
  ZITADEL_VERIFIER_COOKIE,
  ZITADEL_RETURN_URL_COOKIE,
} from "@/lib/auth/zitadel-oidc";
import { sanitizeReturnUrl } from "@/lib/auth/sanitize-return-url";
import { publicConfig } from "@/lib/config";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const DEFAULT_DESTINATION = "/dashboard";

export async function GET(req: NextRequest): Promise<Response> {
  // This route only applies under the Zitadel provider — under GIP there
  // is no flow that could ever land a browser here.
  if (publicConfig.authProvider !== "zitadel") {
    return new NextResponse(null, { status: 404 });
  }

  const stateParam = req.nextUrl.searchParams.get("state");
  const stateCookie = req.cookies.get(ZITADEL_STATE_COOKIE)?.value;

  // Missing cookie (never set, or already consumed by a prior request
  // — see clearFlowCookies below) and a mismatched value are rejected
  // identically: neither proves this callback belongs to a flow this
  // browser actually started.
  if (!stateParam || !stateCookie || !timingSafeEqual(stateParam, stateCookie)) {
    return errorResponse(req, "state_mismatch");
  }

  // The destination never comes from the query string or from
  // Zitadel — only from the cookie this app itself set before
  // handing off to Zitadel, and even that is re-sanitized here. A
  // `state` or query param must never be able to steer a freshly
  // authenticated user off-platform.
  const returnUrlCookie = req.cookies.get(ZITADEL_RETURN_URL_COOKIE)?.value;
  const destination = sanitizeReturnUrl(returnUrlCookie) ?? DEFAULT_DESTINATION;

  const res = NextResponse.redirect(new URL(destination, req.url), 303);
  clearFlowCookies(res);
  return res;
}

function errorResponse(req: NextRequest, code: string): NextResponse {
  const res = NextResponse.redirect(
    new URL(`/login?error=${encodeURIComponent(code)}`, req.url),
    303,
  );
  // Clear on the failure path too — a state cookie that survives a
  // rejected attempt is still a single credential worth burning.
  clearFlowCookies(res);
  return res;
}

// Single-use: whether this callback succeeds or is rejected, the
// cookies it consumed must not still be valid for a second request —
// otherwise a replayed callback (same query string, resent) could
// succeed twice.
function clearFlowCookies(res: NextResponse): void {
  for (const name of [
    ZITADEL_STATE_COOKIE,
    ZITADEL_VERIFIER_COOKIE,
    ZITADEL_RETURN_URL_COOKIE,
  ]) {
    res.cookies.set({ name, value: "", path: "/", maxAge: 0 });
  }
}

// Constant-time string comparison so a mismatched state can't be
// distinguished by timing. Both inputs are short, random tokens —
// this is cheap insurance, not a bottleneck.
function timingSafeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) {
    diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }
  return diff === 0;
}
