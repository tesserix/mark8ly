import { type NextRequest, NextResponse } from 'next/server'
import { SITE_JSON_LD } from './lib/seo/site-json-ld'
import {
  buildCsp,
  buildStaticCsp,
  newNonce,
  sha256Source,
  usesNonce,
} from './lib/security/csp'

/**
 * Onboarding middleware — sets the Content-Security-Policy, and
 * deliberately nothing else.
 *
 * It used to also write an `mk8_currency` cookie on every response,
 * resolved from Cloudflare's `CF-IPCountry`, so the pricing section
 * could render geo-localized prices without an extra round-trip.
 *
 * That cookie is why nothing here was ever cacheable. A response
 * carrying `Set-Cookie` cannot be shared between visitors — a cached
 * copy would hand the next visitor the first visitor's currency — so
 * Cloudflare reported `cf-cache-status: DYNAMIC` for every marketing
 * page, and every visit paid a full origin round-trip to one pod in
 * Sydney (tesserix/mark8ly#597).
 *
 * It had also stopped earning its keep. Since #607 nothing on the
 * server reads the cookie: `/` prerenders at PRERENDER_CURRENCY and the
 * currency-bearing islands correct themselves on mount, reading
 * `document.cookie` from the client. Middleware was making the whole
 * site uncacheable to write a value only client JS consumed.
 *
 * So the geo lookup moved to `app/api/geo-currency/route.ts`, which the
 * client fetches once and then caches in the cookie itself. The HTML is
 * now byte-identical for every visitor, which is what makes an edge
 * Cache Rule safe.
 *
 * NOTE: removing the cookie makes caching *safe*, not automatic —
 * Cloudflare does not cache HTML without a Cache Rule. If the marketing
 * pages still report DYNAMIC, that rule is the missing half, and it
 * must exclude `/api/*` so this endpoint stays per-request.
 */
// The layout's JSON-LD is a constant, so its hash is computed once per
// worker rather than per request.
const jsonLdHash = sha256Source(SITE_JSON_LD)

export async function middleware(request: NextRequest): Promise<NextResponse> {
  const nonce = newNonce()
  const strict = usesNonce(request.nextUrl.pathname)
  const csp = strict ? buildCsp(nonce, await jsonLdHash) : buildStaticCsp()

  // Next reads these request headers to stamp the nonce onto its own
  // script tags. Only the per-request routes can use it.
  const headers = new Headers(request.headers)
  if (strict) {
    headers.set('x-nonce', nonce)
    headers.set('Content-Security-Policy', csp)
  }

  const response = NextResponse.next({ request: { headers } })
  response.headers.set('Content-Security-Policy', csp)

  return response
}

export const config = {
  // Skip Next internals + static assets. Runs on every user-facing
  // route because every one of them needs the CSP header.
  matcher: ['/((?!_next/static|_next/image|favicon.ico|robots.txt|sitemap.xml).*)'],
}
