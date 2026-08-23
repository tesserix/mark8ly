import { type NextRequest, NextResponse } from 'next/server'
import { CURRENCY_COOKIE_NAME, countryToCurrency } from '@repo/ui/subscription'
import { SITE_JSON_LD } from './lib/seo/site-json-ld'
import {
  buildCsp,
  buildStaticCsp,
  newNonce,
  sha256Source,
  usesNonce,
} from './lib/security/csp'

/**
 * Onboarding middleware — sets the `mk8_currency` cookie on every
 * request so the marketing landing (`/#pricing`) can render geo-
 * localized prices without an extra round-trip. Mirrors the admin
 * app's `/pricing` middleware using the same shared country→currency
 * map, so both surfaces show the same currency for a given visitor.
 *
 * Cloudflare injects `CF-IPCountry` at the edge. Local dev lacks it —
 * the fallback to USD keeps the page rendering.
 *
 * We set the cookie on every response rather than only on `/` because
 * marketing visitors hit various routes (/, /about, /guides, ...) and
 * the landing's pricing section needs the cookie regardless of their
 * entry point. Overhead is one header + one cookie write per request.
 */
const COOKIE_MAX_AGE = 86_400 // 24 hours

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
  const countryCode = request.headers.get('CF-IPCountry')
  const currency = countryToCurrency(countryCode)
  const isProduction = process.env.NODE_ENV === 'production'

  response.cookies.set(CURRENCY_COOKIE_NAME, currency, {
    maxAge: COOKIE_MAX_AGE,
    path: '/',
    sameSite: 'lax',
    secure: isProduction,
    httpOnly: false,
  })

  return response
}

export const config = {
  // Skip Next internals + static assets. Run on every user-facing
  // route so the cookie is present on every page the pricing section
  // might appear on.
  matcher: ['/((?!_next/static|_next/image|favicon.ico|robots.txt|sitemap.xml).*)'],
}
