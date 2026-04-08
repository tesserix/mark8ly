import { NextResponse, type NextRequest } from "next/server";

/**
 * Admin middleware — Phase J.
 *
 * Validates the session cookie against `auth-bff GET /auth/session`
 * before letting any authenticated route render. On success the
 * resolved user/tenant are forwarded to the downstream page via
 * request headers (`x-session-user-id`, `x-session-email`,
 * `x-session-tenant-id`) so the page can `headers()` them without
 * re-hitting auth-bff.
 *
 * Failure modes:
 *   - no cookie on request       → redirect to /login (admin's own form)
 *   - cookie present but invalid → redirect to /login
 *   - auth-bff unreachable       → redirect to /login (fail closed)
 *
 * The "fail closed" branch is deliberate: an unreachable auth-bff is a
 * real outage, not a gray zone. Rendering an admin page with no
 * session would leak tenant-scoped data to anonymous visitors.
 *
 * Phase M: /login is now hosted on the admin app itself (not the
 * marketing site) so returning users don't bounce across origins.
 */
const SESSION_COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "m8_session";
const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8087";

// Routes that should never be gated — login redirect targets, static
// assets, and anything that must render without a session.
const PUBLIC_PREFIXES = ["/login", "/logout", "/_next", "/favicon", "/icon-"];

interface SessionResponse {
  data: {
    user_id: string;
    email: string;
    tenant_id: string;
  };
}

export async function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl;

  if (PUBLIC_PREFIXES.some((p) => pathname.startsWith(p))) {
    return NextResponse.next();
  }

  const cookie = req.cookies.get(SESSION_COOKIE_NAME);
  if (!cookie || !cookie.value) {
    return redirectToLogin(req);
  }

  // Validate against auth-bff. We forward the entire Cookie header so
  // the HttpOnly cookie is seen exactly as the browser sent it.
  const cookieHeader = req.headers.get("cookie") ?? "";
  let session: SessionResponse["data"] | null = null;
  try {
    const res = await fetch(`${AUTH_BFF_URL}/auth/session`, {
      method: "GET",
      headers: { Cookie: cookieHeader },
      cache: "no-store",
    });
    if (res.ok) {
      const body = (await res.json()) as SessionResponse;
      session = body.data;
    }
  } catch {
    // Auth-bff unreachable — fall through to the login redirect.
  }

  if (!session) {
    return redirectToLogin(req);
  }

  // Forward the resolved session to the server component via request
  // headers. Headers are the cleanest Next.js-native way to pass
  // per-request data from middleware to pages.
  const headers = new Headers(req.headers);
  headers.set("x-session-user-id", session.user_id);
  headers.set("x-session-email", session.email);
  headers.set("x-session-tenant-id", session.tenant_id);

  return NextResponse.next({ request: { headers } });
}

function redirectToLogin(req: NextRequest): NextResponse {
  const loginUrl = new URL("/login", req.nextUrl.origin);
  loginUrl.searchParams.set("returnUrl", req.nextUrl.toString());
  return NextResponse.redirect(loginUrl);
}

export const config = {
  // Match every route except Next internals and static files. The
  // public-prefix check inside middleware() handles /login etc.
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
