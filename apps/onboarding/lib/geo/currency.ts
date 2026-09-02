/**
 * Client-side resolution of the geo-localized billing currency.
 *
 * `mk8_currency` is still written by middleware.ts from Cloudflare's
 * `CF-IPCountry` — nothing about who *writes* the cookie changed. What
 * changed is who *reads* it.
 *
 * The marketing landing used to read the cookie in the server component
 * via `await cookies()`. In the App Router that single call opts the
 * whole route out of static generation: Next marks `/` dynamic, emits
 * `cache-control: private, no-store`, and Cloudflare reports
 * `cf-cache-status: DYNAMIC`. Every visit then round-trips to one pod in
 * Sydney — measured TTFB 380–460ms, worse from India, plus cold-start
 * risk on `minScale: 0`. Under Lighthouse mobile throttling that TTFB
 * was 1,380ms of a 3.27s LCP, the single largest term.
 *
 * The trade wasn't even paying for itself: geo-resolution defaults to
 * the crawler's own location, so the "crawlable localised pricing" the
 * dynamic render bought was one fixed currency anyway.
 *
 * So the page prerenders in PRERENDER_CURRENCY and the currency-bearing
 * islands correct themselves on mount. A real visitor outside the US
 * sees a brief flash of USD before the swap. That is a deliberate,
 * accepted trade: a loading skeleton or hidden prices would cost more
 * LCP than the flash does, and would hide the prices from crawlers that
 * don't run effects.
 */

import {
  CURRENCY_COOKIE_NAME,
  normalizeCurrency,
  type Currency,
} from '@repo/ui/subscription'

/**
 * The currency baked into the prerendered HTML — what crawlers, AI
 * answer engines, and the first paint all see.
 *
 * USD because `pricing-data.ts` guarantees a USD row on every plan and
 * add-on; every other currency is an optional row. Anchoring the static
 * render to a currency that could be missing would put `getPlanPrice`
 * on its USD fallback path and risk labelling a USD amount with another
 * currency's code.
 *
 * This module is deliberately NOT `'use client'` and deliberately holds
 * no React hooks: `app/page.tsx` reads PRERENDER_CURRENCY on the server
 * to pick the JSON-LD currency, and Next refuses to pull a module that
 * imports `useState` into a Server Component graph at all. The hook
 * lives next door in use-geo-currency.ts.
 */
export const PRERENDER_CURRENCY: Currency = normalizeCurrency(undefined)

/**
 * Reads `mk8_currency` from `document.cookie`. The cookie is written
 * with `httpOnly: false` precisely so this is possible.
 *
 * `normalizeCurrency` still guards the value: geo-targeting can resolve
 * a visitor to a currency the catalogue can't actually price, and
 * showing a USD amount under that label would misquote the price.
 */
export function readCurrencyCookie(): Currency {
  if (typeof document === 'undefined') return PRERENDER_CURRENCY
  const match = document.cookie.match(
    new RegExp(`(?:^|;\\s*)${CURRENCY_COOKIE_NAME}=([^;]*)`),
  )
  return normalizeCurrency(match ? decodeURIComponent(match[1]!) : undefined)
}
