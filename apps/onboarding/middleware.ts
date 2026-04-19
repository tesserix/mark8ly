import { type NextRequest, NextResponse } from 'next/server'
import { CURRENCY_COOKIE_NAME, countryToCurrency } from '@repo/ui/subscription'

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

export function middleware(request: NextRequest): NextResponse {
  const response = NextResponse.next()
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
