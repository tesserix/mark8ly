'use client'

/**
 * The client half of lib/geo/currency.ts — kept separate because that
 * module is imported by `app/page.tsx` on the server, and Next rejects
 * a Server Component graph that reaches a module importing `useState`,
 * even when the server only reads a constant from it.
 */

import { useEffect, useState } from 'react'
import type { Currency } from '@repo/ui/subscription'

import { PRERENDER_CURRENCY, readCurrencyCookie } from './currency'

/**
 * Returns PRERENDER_CURRENCY for the server render and the first client
 * render — they must agree or React reports a hydration mismatch — then
 * the visitor's real currency once mounted.
 */
export function useGeoCurrency(): Currency {
  const [currency, setCurrency] = useState<Currency>(PRERENDER_CURRENCY)

  useEffect(() => {
    setCurrency(readCurrencyCookie())
  }, [])

  return currency
}
