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
 */
export function documentUnauthorizedResponse(
  req: Request,
  orderId: string,
): Response {
  if (isBrowserNavigation(req)) {
    const target = new URL("/sign-in", req.url);
    target.searchParams.set("next", `/account/orders/${orderId}`);
    return NextResponse.redirect(target, 302);
  }
  return NextResponse.json({ error: "unauthorized" }, { status: 401 });
}
