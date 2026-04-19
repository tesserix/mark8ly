/**
 * Public /pricing page — RSC.
 *
 * Reads the `mk8_currency` cookie set by geo middleware (Task 5).
 * Falls back to USD when the cookie is absent or the pricing API is not yet wired.
 *
 * TODO(p16-task7): Replace STATIC_PRICING_CATALOGUE with the real
 * usePricing(currency) hook / pricing API client once Task 7 lands.
 * The static fallback below mirrors the shape so the swap is a one-liner.
 */
import type { Metadata } from 'next'
import { cookies } from 'next/headers'
import { pricingCopy } from '@/lib/copy/pricing'
import { PricingClient } from './PricingClient'

export const metadata: Metadata = {
  title: 'Pricing',
  description:
    'Four plans for merchants at every stage. Clear limits, no surprise fees. Change plans any time.',
  robots: { index: true, follow: true },
}

// ---------------------------------------------------------------------------
// Static pricing catalogue — replaced by Task 7 API client when available.
// Minor-unit integers (cents). USD base; geo middleware adjusts currency.
// ---------------------------------------------------------------------------

export interface PlanPrice {
  monthly: number
  annual: number
  /** Annual cost expressed as a per-month equivalent for display. */
  annualMonthlyEquivalent: number
}

export interface Plan {
  id: 'starter' | 'studio' | 'pro'
  prices: Record<string, PlanPrice>
}

export interface ProAppAddon {
  prices: Record<string, PlanPrice>
}

export interface PricingCatalogue {
  plans: Plan[]
  proApp: ProAppAddon
}

const STATIC_PRICING_CATALOGUE: PricingCatalogue = {
  plans: [
    {
      id: 'starter',
      prices: {
        USD: { monthly: 2900, annual: 27600, annualMonthlyEquivalent: 2300 },
        GBP: { monthly: 2300, annual: 21600, annualMonthlyEquivalent: 1800 },
        EUR: { monthly: 2700, annual: 25200, annualMonthlyEquivalent: 2100 },
        INR: { monthly: 249900, annual: 2399900, annualMonthlyEquivalent: 199900 },
        AUD: { monthly: 4500, annual: 43200, annualMonthlyEquivalent: 3600 },
        CAD: { monthly: 3900, annual: 37200, annualMonthlyEquivalent: 3100 },
      },
    },
    {
      id: 'studio',
      prices: {
        USD: { monthly: 7900, annual: 75600, annualMonthlyEquivalent: 6300 },
        GBP: { monthly: 6300, annual: 59400, annualMonthlyEquivalent: 4950 },
        EUR: { monthly: 7300, annual: 69600, annualMonthlyEquivalent: 5800 },
        INR: { monthly: 649900, annual: 6239900, annualMonthlyEquivalent: 519900 },
        AUD: { monthly: 12200, annual: 115200, annualMonthlyEquivalent: 9600 },
        CAD: { monthly: 10700, annual: 100800, annualMonthlyEquivalent: 8400 },
      },
    },
    {
      id: 'pro',
      prices: {
        // Pro: annual = $1,188/yr; monthly = $119/mo (no annual-equivalent shown on monthly)
        USD: { monthly: 11900, annual: 118800, annualMonthlyEquivalent: 9900 },
        GBP: { monthly: 9500, annual: 94800, annualMonthlyEquivalent: 7900 },
        EUR: { monthly: 10900, annual: 108000, annualMonthlyEquivalent: 9000 },
        INR: { monthly: 989900, annual: 9839900, annualMonthlyEquivalent: 819900 },
        AUD: { monthly: 18500, annual: 183600, annualMonthlyEquivalent: 15300 },
        CAD: { monthly: 16200, annual: 160800, annualMonthlyEquivalent: 13400 },
      },
    },
  ],
  proApp: {
    prices: {
      USD: { monthly: 4900, annual: 47400, annualMonthlyEquivalent: 3950 },
      GBP: { monthly: 3900, annual: 37200, annualMonthlyEquivalent: 3100 },
      EUR: { monthly: 4500, annual: 43200, annualMonthlyEquivalent: 3600 },
      INR: { monthly: 399900, annual: 3839900, annualMonthlyEquivalent: 319900 },
      AUD: { monthly: 7600, annual: 73200, annualMonthlyEquivalent: 6100 },
      CAD: { monthly: 6700, annual: 64800, annualMonthlyEquivalent: 5400 },
    },
  },
}

export default async function PricingPage() {
  const cookieStore = await cookies()
  const rawCurrency = cookieStore.get('mk8_currency')?.value ?? 'USD'

  // Normalise: only accept supported currencies; fall back to USD.
  const SUPPORTED = ['USD', 'GBP', 'EUR', 'INR', 'AUD', 'CAD']
  const currency = SUPPORTED.includes(rawCurrency.toUpperCase())
    ? rawCurrency.toUpperCase()
    : 'USD'

  return (
    <PricingClient
      currency={currency}
      pricing={STATIC_PRICING_CATALOGUE}
    />
  )
}
