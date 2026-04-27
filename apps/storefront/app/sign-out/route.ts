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

  // Redirect to the storefront root on the same host the request came in
  // on so a per-tenant subdomain (e.g. playwrite-test.mark8ly.com) doesn't
  // accidentally bounce to the wildcard apex.
  const url = new URL("/", req.url);
  return NextResponse.redirect(url, { status: 303 });
}
