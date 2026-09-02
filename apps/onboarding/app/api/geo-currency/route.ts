import { countryToCurrency } from "@repo/ui/subscription";
import { NextResponse } from "next/server";

/**
 * Geo-resolved billing currency, as JSON, for the client to fetch.
 *
 * This endpoint exists so the marketing pages can be cached at the
 * edge. Middleware used to attach `Set-Cookie: mk8_currency` to every
 * response, which meant Cloudflare could never cache any of them — a
 * cached response carries its `Set-Cookie` header with it, so serving
 * one visitor's HTML to another would hand them the first visitor's
 * currency. Correct, and expensive: every visit to every marketing page
 * round-tripped to a single pod in Sydney (tesserix/mark8ly#597).
 *
 * Moving the geo lookup here confines the per-visitor bit to one small
 * JSON response that nobody caches, and leaves the HTML identical for
 * everyone — which is the precondition for caching it.
 *
 * Deliberately `force-dynamic` and `no-store`: the entire value of this
 * route is that it reads a per-request header. A cached response would
 * report the currency of whoever happened to warm the cache, which is
 * the exact bug this design removes from the HTML.
 */
export const dynamic = "force-dynamic";

export function GET(request: Request): NextResponse {
  const currency = countryToCurrency(request.headers.get("CF-IPCountry"));

  return NextResponse.json(
    { currency },
    { headers: { "cache-control": "private, no-store" } },
  );
}
