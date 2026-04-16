import { NextResponse, type NextRequest } from "next/server";

import { revokeSession } from "@/lib/auth/session";

/**
 * Logout — invalidates the session both server-side (auth-bff) and
 * client-side (this origin's cookie), then redirects to the marketing
 * site.
 *
 * The server-side call goes first so a logout that succeeds locally
 * but fails to reach auth-bff never leaves a "valid in auth-bff, gone
 * from browser" phantom session. `revokeSession` is best-effort — if
 * auth-bff is unreachable we still clear the browser cookie and the
 * session will expire naturally on its original TTL.
 */
const SESSION_COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "m8_session";
const SESSION_COOKIE_DOMAIN =
  process.env.SESSION_COOKIE_DOMAIN ?? ".mark8ly.com";

export async function GET(req: NextRequest) {
  await revokeSession(req.headers.get("cookie"));

  // Stay on the same admin host the browser is actually connected to.
  // `req.nextUrl` reflects the internal bind (0.0.0.0:4202 in-cluster)
  // behind the Istio ingress + Cloudflare tunnel — use the forwarded
  // host/proto headers the proxy sets so the redirect lands back on
  // the merchant's admin subdomain or custom domain.
  const forwardedHost =
    req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "";
  const forwardedProto = req.headers.get("x-forwarded-proto") ?? "https";
  const targetHref = forwardedHost
    ? `${forwardedProto}://${forwardedHost}/login`
    : (() => {
        const u = req.nextUrl.clone();
        u.pathname = "/login";
        u.search = "";
        return u.toString();
      })();

  const response = NextResponse.redirect(targetHref);
  // Must match the Domain/Path used when setting the cookie or the
  // browser creates a duplicate cookie instead of deleting.
  response.cookies.set({
    name: SESSION_COOKIE_NAME,
    value: "",
    domain: SESSION_COOKIE_DOMAIN,
    path: "/",
    httpOnly: true,
    secure: true,
    sameSite: "lax",
    maxAge: 0,
  });
  return response;
}
