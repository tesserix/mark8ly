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
 * Two clears are emitted — the per-host clear for the live Phase 1
 * cookie, and a `.mark8ly.com` transitional clear for pre-Phase-1
 * cookies. The two writes go onto the response via Headers.append, NOT
 * via `next/headers`'s cookies(), because that store keys by cookie
 * NAME and silently collapses the per-host write into the legacy one
 * (the bug we're fixing here). Drop the legacy clear one release
 * after Phase 1 lands.
 */
export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(req: Request): Promise<Response> {
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

  const origin = forwardedHost
    ? `${forwardedProto}://${forwardedHost}`
    : new URL(req.url).origin;

  const res = NextResponse.redirect(`${origin}/`, { status: 303 });

  if (cookieHost) {
    res.headers.append(
      "Set-Cookie",
      buildClearCookie("mp_customer_session", cookieHost, !isLocal),
    );
  } else {
    // Fallback: no validated host (dev without x-forwarded-host).
    // Clear the host-only cookie for the current request host so dev
    // sign-out still works without an explicit Domain.
    res.headers.append(
      "Set-Cookie",
      "mp_customer_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax",
    );
  }

  // Transitional: clear the legacy parent-domain cookie set before
  // Phase 1. Drop one release after Phase 1 lands (cookie max-age was
  // 30 days).
  res.headers.append(
    "Set-Cookie",
    buildClearCookie("mp_customer_session", ".mark8ly.com", true),
  );

  return res;
}

function buildClearCookie(name: string, domain: string, secure: boolean): string {
  const attrs = [
    `${name}=`,
    "Path=/",
    "Max-Age=0",
    `Domain=${domain}`,
    "HttpOnly",
    "SameSite=Lax",
  ];
  if (secure) attrs.push("Secure");
  return attrs.join("; ");
}
