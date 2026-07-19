// Decides how a customer document endpoint (invoice / receipt PDF) should
// respond when there's no valid session.
//
// These routes are linked directly from order emails, so a logged-out click
// arrives as a top-level browser navigation with no cookie. Returning the raw
// {"error":"unauthorized"} JSON there is a dead-end for the buyer. Instead we
// bounce them through /sign-in and land them on their account order page,
// which carries an authenticated "Download PDF" control. Programmatic callers
// (in-app fetch, curl) still get a clean 401 so the API contract is unchanged.

import { NextResponse } from "next/server";

/**
 * A request is treated as a user-facing navigation when the browser marks it
 * as a top-level navigation, or (older browsers / email webviews) when it
 * accepts HTML. Fetch/XHR callers set neither, so they fall through to JSON.
 */
function isBrowserNavigation(req: Request): boolean {
  const secFetchMode = req.headers.get("sec-fetch-mode");
  if (secFetchMode) return secFetchMode === "navigate";
  return (req.headers.get("accept") ?? "").includes("text/html");
}

/**
 * Response to send from an invoice/receipt route when the session is missing.
 * Navigations redirect to sign-in with a return path to the order page;
 * everything else gets a 401 JSON body.
 *
 * The redirect Location is RELATIVE on purpose. Behind Istio the route's
 * `req.url` host is the pod's internal bind address (0.0.0.0:3000), so building
 * an absolute URL from it would send the browser to 0.0.0.0. A site-relative
 * Location is resolved by the browser against the real request URL
 * (my-store.mark8ly.com), which is exactly what we want.
 */
export function documentUnauthorizedResponse(
  req: Request,
  orderId: string,
): Response {
  if (isBrowserNavigation(req)) {
    const next = encodeURIComponent(`/account/orders/${orderId}`);
    return new NextResponse(null, {
      status: 302,
      headers: { Location: `/sign-in?next=${next}` },
    });
  }
  return NextResponse.json({ error: "unauthorized" }, { status: 401 });
}
