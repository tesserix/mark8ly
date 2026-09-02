'use client'

/**
 * The client half of lib/geo/currency.ts — kept separate because that
 * module is imported by `app/page.tsx` on the server, and Next rejects
 * a Server Component graph that reaches a module importing `useState`,
 * even when the server only reads a constant from it.
 */

import { useEffect, useState } from 'react'
import type { Currency } from '@repo/ui/subscription'

import {
  PRERENDER_CURRENCY,
  readCurrencyCookie,
  writeCurrencyCookie,
} from './currency'

/**
 * Returns PRERENDER_CURRENCY for the server render and the first client
 * render — they must agree or React reports a hydration mismatch — then
 * the visitor's real currency once mounted.
 *
 * The cookie is checked first and only missed on a visitor's first page
 * view, when we ask `/api/geo-currency` and cache the answer. A failed
 * fetch is swallowed on purpose: the page is already showing valid USD
 * pricing, so the correct behaviour on a network error is to leave it
 * alone rather than surface an error about a currency label.
 */
export function useGeoCurrency(): Currency {
  const [currency, setCurrency] = useState<Currency>(PRERENDER_CURRENCY)

  useEffect(() => {
    const cached = readCurrencyCookie()
    if (cached) {
      setCurrency(cached)
      return
    }

    // Guards against setting state after unmount, and against a slow
    // response arriving once the visitor has navigated away.
    const controller = new AbortController()

    fetch('/api/geo-currency', { signal: controller.signal })
      .then((response) => (response.ok ? response.json() : null))
      .then((body: { currency?: Currency } | null) => {
        if (!body?.currency) return
        writeCurrencyCookie(body.currency)
        setCurrency(body.currency)
      })
      .catch(() => {
        // Offline, blocked, or aborted. USD stands.
      })

    return () => controller.abort()
  }, [])

  return currency
}
