import { describe, it, expect } from 'vitest'
import {
  PRICEABLE_CURRENCIES,
  SUPPORTED_CURRENCIES,
  normalizeCurrency,
} from './country-map'
import { SHARED_PRICING_CATALOGUE, getPlanPrice, getAddOnPrice } from './pricing-data'

describe('PRICEABLE_CURRENCIES', () => {
  it('matches the pricing table\'s actual coverage: every plan carries every priceable currency', () => {
    for (const currency of PRICEABLE_CURRENCIES) {
      for (const plan of SHARED_PRICING_CATALOGUE.plans) {
        expect(
          plan.prices[currency],
          `plan "${plan.id}" is missing a row for priceable currency "${currency}"`,
        ).toBeDefined()
      }
    }
  })

  it('excludes a currency present on only some plans (fails if a plan gains or loses a currency)', () => {
    // Every currency that appears anywhere in the catalogue but is not
    // priceable must be missing from at least one plan — otherwise
    // PRICEABLE_CURRENCIES failed to include a currency it should have.
    const allCurrenciesSeen = new Set(
      SHARED_PRICING_CATALOGUE.plans.flatMap((plan) => Object.keys(plan.prices)),
    )
    for (const currency of allCurrenciesSeen) {
      const coveredByEveryPlan = SHARED_PRICING_CATALOGUE.plans.every(
        (plan) => plan.prices[currency as keyof typeof plan.prices] !== undefined,
      )
      expect(PRICEABLE_CURRENCIES.has(currency as never)).toBe(coveredByEveryPlan)
    }
  })

  it('is a proper subset of SUPPORTED_CURRENCIES', () => {
    for (const currency of PRICEABLE_CURRENCIES) {
      expect((SUPPORTED_CURRENCIES as readonly string[]).includes(currency)).toBe(true)
    }
  })

  it('includes NZD, since the shared table has NZD rows for every plan', () => {
    expect(PRICEABLE_CURRENCIES.has('NZD')).toBe(true)
  })

  it('excludes PPP/emerging-market currencies the table deliberately omits', () => {
    for (const currency of ['THB', 'VND', 'IDR', 'MYR', 'PHP', 'AED', 'JPY', 'SAR', 'KRW', 'HKD']) {
      expect(PRICEABLE_CURRENCIES.has(currency as never)).toBe(false)
    }
  })
})

describe('normalizeCurrency', () => {
  it('passes through a currency with a price row', () => {
    expect(normalizeCurrency('GBP')).toBe('GBP')
    expect(normalizeCurrency('nzd')).toBe('NZD')
  })

  it('falls back to USD for a currency with no price row, even though it is geo-recognized', () => {
    // THB is a real, geo-recognized currency (COUNTRY_TO_CURRENCY maps TH
    // -> THB) but the pricing table has no THB row for any plan.
    expect(normalizeCurrency('THB')).toBe('USD')
    expect(normalizeCurrency('AED')).toBe('USD')
  })

  it('falls back to USD for an unrecognized value', () => {
    expect(normalizeCurrency('XXX')).toBe('USD')
    expect(normalizeCurrency(undefined)).toBe('USD')
    expect(normalizeCurrency(null)).toBe('USD')
  })
})

describe('getPlanPrice / getAddOnPrice — resolved currency defence in depth', () => {
  it('resolves to the requested currency when a row exists', () => {
    const plan = SHARED_PRICING_CATALOGUE.plans.find((p) => p.id === 'starter')!
    const resolved = getPlanPrice(plan, 'GBP')
    expect(resolved.currency).toBe('GBP')
    expect(resolved.price).toBe(plan.prices.GBP)
  })

  it('resolves to USD — never the requested currency — when no row exists, and returns the USD amount', () => {
    const plan = SHARED_PRICING_CATALOGUE.plans.find((p) => p.id === 'starter')!
    // AED has no row anywhere in the catalogue.
    const resolved = getPlanPrice(plan, 'AED')
    expect(resolved.currency).toBe('USD')
    expect(resolved.price).toBe(plan.prices.USD)
    expect(resolved.price?.monthly).toBe(1900)
  })

  it('add-on resolution follows the same rule', () => {
    const resolved = getAddOnPrice(SHARED_PRICING_CATALOGUE.proApp, 'THB')
    expect(resolved.currency).toBe('USD')
    expect(resolved.price).toBe(SHARED_PRICING_CATALOGUE.proApp.prices.USD)
  })
})

describe('SHARED_PRICING_CATALOGUE — Starter USD headline (regression: was $29/mo, now $19/mo)', () => {
  it('matches billing: $19/mo, $182/yr', () => {
    const starter = SHARED_PRICING_CATALOGUE.plans.find((p) => p.id === 'starter')!
    expect(starter.prices.USD?.monthly).toBe(1900)
    expect(starter.prices.USD?.annual).toBe(18200)
  })
})
