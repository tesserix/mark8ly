import { cookies } from "next/headers";
import { NextResponse } from "next/server";

/**
 * GET /sign-out — clears the customer session cookie and redirects home.
 *
 * Lives as a Route Handler rather than a Server Component page. Next.js
 * disallows cookie mutation from server components in production, which
 * threw a runtime error on every visit and surfaced as the storefront
 * "We hit a snag" page. Route Handlers are the only sanctioned home for
 * cookie writes outside of Server Actions and middleware, so the delete
 * + redirect pair belongs here.
 *
 * Visiting this URL (GET) is enough; no form or button needed.
 */
export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: Request): Promise<Response> {
  const c = await cookies();
  c.delete("mp_customer_session");

  // Resolve the customer-facing origin. Behind Istio / Cloudflare,
  // `req.url` reports the internal pod bind address (e.g. https://
  // 0.0.0.0:4203) rather than the host the buyer actually used, so a
  // naive `new URL("/", req.url)` redirected the browser straight to
  // the pod IP and looked like a broken sign-out. Mirror the helper the
  // admin middleware uses for the same problem: prefer x-forwarded-host
  // / x-forwarded-proto when present.
  const h = req.headers;
  const forwardedHost = h.get("x-forwarded-host") ?? h.get("host") ?? "";
  const forwardedProto =
    h.get("x-forwarded-proto") ??
    (forwardedHost.startsWith("localhost") || forwardedHost.startsWith("127.")
      ? "http"
      : "https");
  const origin = forwardedHost
    ? `${forwardedProto}://${forwardedHost}`
    : new URL(req.url).origin;
  return NextResponse.redirect(`${origin}/`, { status: 303 });
}
