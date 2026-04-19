/**
 * Unit tests for the getPricing API client.
 *
 * Uses MSW to intercept fetch calls per-test so each case is fully isolated.
 * The test server is set up inline — no shared global server state.
 */
import { describe, it, expect, afterEach } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'
import { getPricing } from '@/lib/api/subscription/pricing'
import { LOCAL_PRICING_FALLBACK } from '@/lib/api/subscription/pricing-fallback'
import type { PricingCatalogue } from '@/lib/api/subscription/schemas/plan'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const PRICING_URL = '/api/v1/public/pricing'

function makePricingUrl(currency: string) {
  return `${PRICING_URL}?currency=${currency}`
}

/** Minimal valid PricingCatalogue for USD, used in the happy-path test. */
const VALID_USD_CATALOGUE: PricingCatalogue = {
  currency: 'USD',
  plans: [
    {
      id: 'starter',
      name: 'Starter',
      tagline: 'For merchants launching their first store.',
      pricing: {
        monthly: { amount: 1900, currency: 'USD', per: 'month' },
        annual: { amount: 18200, currency: 'USD', per: 'year', monthlyEquivalent: 1516 },
      },
      features: [
        { label: '2 stores', included: true },
      ],
    },
    {
      id: 'studio',
      name: 'Studio',
      tagline: 'For growing brands.',
      featured: true,
      pricing: {
        monthly: { amount: 4900, currency: 'USD', per: 'month' },
        annual: { amount: 47000, currency: 'USD', per: 'year', monthlyEquivalent: 3916 },
      },
      features: [
        { label: '5 stores', included: true },
      ],
    },
    {
      id: 'pro',
      name: 'Pro',
      tagline: 'Built for teams scaling past serious order volumes.',
      pricing: {
        monthly: { amount: 11900, currency: 'USD', per: 'month' },
        annual: { amount: 118800, currency: 'USD', per: 'year', monthlyEquivalent: 9900 },
      },
      features: [
        { label: '10 stores', included: true, highlight: true },
      ],
    },
  ],
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('getPricing', () => {
  // Each test creates its own server so there is zero shared state between cases.

  it('returns the parsed catalogue when the endpoint responds 200 with a valid body', async () => {
    const server = setupServer(
      http.get(makePricingUrl('USD'), () =>
        HttpResponse.json(VALID_USD_CATALOGUE),
      ),
    )
    server.listen({ onUnhandledRequest: 'error' })

    try {
      const result = await getPricing('USD')
      expect(result.currency).toBe('USD')
      expect(result.plans).toHaveLength(3)
      expect(result.plans[0].id).toBe('starter')
    } finally {
      server.close()
    }
  })

  it('returns the fallback for the requested currency when the endpoint 404s', async () => {
    const server = setupServer(
      http.get(makePricingUrl('GBP'), () =>
        new HttpResponse(null, { status: 404 }),
      ),
    )
    server.listen({ onUnhandledRequest: 'error' })

    try {
      const result = await getPricing('GBP')
      expect(result).toEqual(LOCAL_PRICING_FALLBACK['GBP'])
      expect(result.currency).toBe('GBP')
    } finally {
      server.close()
    }
  })

  it('returns the USD fallback when the endpoint 404s and the currency is unknown', async () => {
    // Unknown currency 'ZZZ' → normalises to USD → fetches /pricing?currency=USD
    const server = setupServer(
      http.get(makePricingUrl('USD'), () =>
        new HttpResponse(null, { status: 404 }),
      ),
    )
    server.listen({ onUnhandledRequest: 'error' })

    try {
      const result = await getPricing('ZZZ')
      expect(result).toEqual(LOCAL_PRICING_FALLBACK['USD'])
      expect(result.currency).toBe('USD')
    } finally {
      server.close()
    }
  })

  it('returns the fallback when a network error occurs', async () => {
    const server = setupServer(
      http.get(makePricingUrl('EUR'), () =>
        HttpResponse.error(),
      ),
    )
    server.listen({ onUnhandledRequest: 'error' })

    try {
      const result = await getPricing('EUR')
      expect(result).toEqual(LOCAL_PRICING_FALLBACK['EUR'])
      expect(result.currency).toBe('EUR')
    } finally {
      server.close()
    }
  })

  it('falls back to USD fallback when currency is lowercase', async () => {
    // lowercase 'usd' → normalised to 'USD'
    const server = setupServer(
      http.get(makePricingUrl('USD'), () =>
        new HttpResponse(null, { status: 404 }),
      ),
    )
    server.listen({ onUnhandledRequest: 'error' })

    try {
      const result = await getPricing('usd')
      expect(result).toEqual(LOCAL_PRICING_FALLBACK['USD'])
    } finally {
      server.close()
    }
  })

  it('throws when the server returns a malformed response that fails Zod validation', async () => {
    const malformedBody = {
      // missing required 'currency' field; plans contain an invalid plan id
      plans: [
        {
          id: 'unknown_plan',
          name: 'Bogus',
          tagline: 'Invalid',
          pricing: {
            monthly: { amount: 'not-a-number', currency: 'USD', per: 'month' },
            annual: { amount: 100, currency: 'USD', per: 'year', monthlyEquivalent: 8 },
          },
          features: [],
        },
      ],
    }

    const server = setupServer(
      http.get(makePricingUrl('USD'), () =>
        HttpResponse.json(malformedBody, { status: 200 }),
      ),
    )
    server.listen({ onUnhandledRequest: 'error' })

    try {
      await expect(getPricing('USD')).rejects.toThrow()
    } finally {
      server.close()
    }
  })
})
