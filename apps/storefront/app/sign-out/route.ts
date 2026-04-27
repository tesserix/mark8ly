import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { sanitizeHost } from "@/lib/host";

/**
 * GET /sign-out — clears the customer session cookie and redirects home.
 *
 * Lives as a Route Handler rather than a Server Component page. Next.js
 * disallows cookie mutation from server components in production, which
 * threw a runtime error on every visit and surfaced as the storefront
 * "We hit a snag" page. Route Handlers are the only sanctioned home for
 * cookie writes outside of Server Actions and middleware.
 *
 * Cookie was set with Domain=<request-host> in customerSignIn (Phase 1).
 * We must delete with the same Domain or the browser leaves it alive
 * (it would just set a second deletion cookie on the wrong scope).
 *
 * The transitional `.mark8ly.com` clear catches any pre-Phase-1 cookies
 * still kicking around; drop one release after Phase 1 lands.
 */
export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: Request): Promise<Response> {
  const c = await cookies();

  // Resolve the customer-facing host. Behind Istio / Cloudflare,
  // `req.url` reports the internal pod bind address (e.g. https://
  // 0.0.0.0:4203) rather than the host the buyer actually used —
  // prefer x-forwarded-host then host. Same helper that ships the
  // redirect; reuse for the cookie Domain so set + delete agree.
  const h = req.headers;
  const forwardedHost = h.get("x-forwarded-host") ?? h.get("host") ?? "";
  const cookieHost = sanitizeHost(forwardedHost);
  const isLocal =
    forwardedHost.startsWith("localhost") || forwardedHost.startsWith("127.");
  const forwardedProto =
    h.get("x-forwarded-proto") ?? (isLocal ? "http" : "https");

  if (cookieHost) {
    c.set({
      name: "mp_customer_session",
      value: "",
      path: "/",
      domain: cookieHost,
      maxAge: 0,
      httpOnly: true,
      secure: !isLocal,
      sameSite: "lax",
    });
  }

  // Transitional: clear the legacy parent-domain cookie set before Phase 1.
  // Drop one release after Phase 1 lands (cookie max-age was 30 days).
  c.set({
    name: "mp_customer_session",
    value: "",
    path: "/",
    domain: ".mark8ly.com",
    maxAge: 0,
    httpOnly: true,
    secure: true,
    sameSite: "lax",
  });

  const origin = forwardedHost
    ? `${forwardedProto}://${forwardedHost}`
    : new URL(req.url).origin;
  return NextResponse.redirect(`${origin}/`, { status: 303 });
}
