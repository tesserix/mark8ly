import { NextResponse, type NextRequest } from "next/server";

/**
 * Admin middleware — Phase I stub.
 *
 * Today this is a passthrough that only redirects unauthenticated
 * visitors to the marketing login page based on the presence of the
 * session cookie. It does NOT yet validate the cookie against auth-bff
 * (that's Phase J) so a forged cookie will get past this check — which
 * is fine because every downstream data-fetch still goes through
 * auth-bff before returning any real information.
 *
 * Phase J will replace the cookie-presence check with a real call to
 * `GET /auth/session` on auth-bff, populate the request with the
 * resolved tenant, and reject invalid sessions.
 */
const SESSION_COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "m8_session";
const MARKETING_URL =
  process.env.NEXT_PUBLIC_MARKETING_URL ?? "http://localhost:4201";

// Routes that should never be gated — login redirect targets, static
// assets, and the health endpoint if we ever add one.
const PUBLIC_PREFIXES = ["/login", "/_next", "/favicon", "/icon-"];

export function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl;

  if (PUBLIC_PREFIXES.some((p) => pathname.startsWith(p))) {
    return NextResponse.next();
  }

  const session = req.cookies.get(SESSION_COOKIE_NAME);
  if (!session || !session.value) {
    const loginUrl = new URL(`${MARKETING_URL}/login`);
    loginUrl.searchParams.set("returnUrl", req.nextUrl.toString());
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  // Match every route except Next internals and static files. The
  // public-prefix check inside middleware() handles /login etc.
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
