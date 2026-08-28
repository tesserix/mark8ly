/**
 * Public /pricing page — RSC.
 *
 * Reads the `mk8_currency` cookie set by geo middleware (Task 5) and
 * renders `SHARED_PRICING_CATALOGUE` — the same table
 * `services/marketplace-api/internal/billing/pricing` generates for
 * onboarding's `/#pricing`. There is exactly one source of truth for
 * headline pricing; this page must never hand-maintain its own numbers.
 */
import type { Metadata } from 'next'
import { cookies } from 'next/headers'
import {
  CURRENCY_COOKIE_NAME,
  normalizeCurrency,
  SHARED_PRICING_CATALOGUE,
} from '@repo/ui/subscription'
import { PricingClient } from './PricingClient'

export const metadata: Metadata = {
  title: 'Pricing',
  description:
    'Four plans for merchants at every stage. Clear limits, no surprise fees. Change plans any time.',
  robots: { index: true, follow: true },
}

export default async function PricingPage() {
  const cookieStore = await cookies()
  // normalizeCurrency only ever returns a currency every plan in
  // SHARED_PRICING_CATALOGUE actually has a row for (falling back to USD
  // otherwise) — see packages/ui/src/subscription/country-map.ts. There is
  // no separate allowlist to keep in sync here.
  const currency = normalizeCurrency(cookieStore.get(CURRENCY_COOKIE_NAME)?.value)

  return (
    <PricingClient
      currency={currency}
      pricing={SHARED_PRICING_CATALOGUE}
    />
  )
}
