import { type NextRequest, NextResponse } from "next/server";
import { CURRENCY_COOKIE_NAME, countryToCurrency } from "./countryToCurrency";

const COOKIE_NAME = CURRENCY_COOKIE_NAME;
const COOKIE_MAX_AGE = 86_400; // 24 hours in seconds

/**
 * Reads the Cloudflare `CF-IPCountry` header and sets the `mk8_currency`
 * cookie on the response so the pricing RSC can render the correct local
 * currency without an extra API round-trip.
 *
 * This function is intentionally side-effect only (void return) so the
 * caller decides which response object to attach the cookie to — keeping
 * auth/tenant logic completely separate.
 *
 * The cookie is set with:
 * - maxAge 86400 (24 h) — refreshed on every visit to /pricing
 * - path "/"          — readable by all routes on the same origin
 * - sameSite "lax"    — safe for same-site navigation + cross-site GET
 * - secure true only in production — avoids local-dev friction
 * - httpOnly false    — client components may read it for locale display
 *
 * @param request  - The incoming NextRequest (used to read CF-IPCountry).
 * @param response - The NextResponse to attach the cookie to (mutated in place).
 */
export function applyGeoCookie(
  request: NextRequest,
  response: NextResponse,
): void {
  const countryCode = request.headers.get("CF-IPCountry");
  const currency = countryToCurrency(countryCode);
  const isProduction = process.env.NODE_ENV === "production";

  response.cookies.set(COOKIE_NAME, currency, {
    maxAge: COOKIE_MAX_AGE,
    path: "/",
    sameSite: "lax",
    secure: isProduction,
    httpOnly: false,
  });
}
