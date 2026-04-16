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

  // Stay on the same admin host — custom domain merchants should land
  // on their own admin.<domain>/login, not bounce back to mark8ly.com.
  const target = req.nextUrl.clone();
  target.pathname = "/login";
  target.search = "";

  const response = NextResponse.redirect(target);
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
