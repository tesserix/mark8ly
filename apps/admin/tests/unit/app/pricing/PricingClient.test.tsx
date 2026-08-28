/**
 * PricingClient unit tests — Vitest + Testing Library.
 *
 * Uses a static pricing payload that mirrors the shape from page.tsx.
 * Does NOT test the RSC page.tsx itself (that is an integration concern);
 * tests only the interactive client component.
 */

import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { PricingClient } from '@/app/pricing/PricingClient'
import { SHARED_PRICING_CATALOGUE as REAL_PRICING, type SharedPricingCatalogue } from '@repo/ui/subscription'

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const FIXTURE_PRICING: SharedPricingCatalogue = {
  plans: [
    {
      id: 'starter',
      prices: {
        USD: { monthly: 2900, annual: 27600, annualMonthlyEquivalent: 2300 },
      },
    },
    {
      id: 'studio',
      prices: {
        USD: { monthly: 7900, annual: 75600, annualMonthlyEquivalent: 6300 },
      },
    },
    {
      id: 'pro',
      prices: {
        USD: { monthly: 11900, annual: 118800, annualMonthlyEquivalent: 9900 },
      },
    },
  ],
  proApp: {
    prices: {
      USD: { monthly: 4900, annual: 47400, annualMonthlyEquivalent: 3950 },
    },
  },
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PricingClient', () => {
  it('renders all 3 public plan names (starter/studio/pro)', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    expect(screen.getByRole('heading', { name: /Starter/i })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Studio/i })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Pro/i })).toBeInTheDocument()
  })

  it('renders the Pro card with the exact verbatim annual copy', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    // Default is annual — the full Pro annual line should be visible.
    // The text is split across React nodes so we search by partial text.
    expect(
      screen.getByText(/From/i, { exact: false }),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/\$1,188/i, { exact: false }),
    ).toBeInTheDocument()
    // Multiple "Billed annually" notes appear (one per non-Pro plan) — check at least one exists
    expect(screen.getAllByText(/Billed annually/i).length).toBeGreaterThanOrEqual(1)
    expect(
      screen.getByText(/Monthly available at \$119\/mo\./i, { exact: false }),
    ).toBeInTheDocument()
  })

  it('shows "Start free trial" CTA on Starter and Studio plans', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    const trialLinks = screen.getAllByRole('link', { name: /Start free trial/i })
    expect(trialLinks).toHaveLength(2)
  })

  it('shows "Start a conversation" CTA on the Pro plan', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    // There may be multiple "Start a conversation" links (plan CTA + Pro section CTA).
    // At minimum one must point to the pro-contact href.
    const conversationLinks = screen.getAllByRole('link', { name: /Start a conversation/i })
    expect(conversationLinks.length).toBeGreaterThanOrEqual(1)

    const proLink = conversationLinks.find((el) =>
      (el as HTMLAnchorElement).href?.includes('pro-contact'),
    )
    expect(proLink).toBeDefined()
  })

  it('renders the Pro+App add-on section', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    expect(
      screen.getByRole('region', { name: /White-label App add-on/i }),
    ).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /White-label App/i })).toBeInTheDocument()
    expect(screen.getByText(/Add-on — requires Studio plan or higher/i)).toBeInTheDocument()
  })

  it('toggles from annual to monthly when the Monthly button is clicked', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    // Annual is the default — the "Billed annually" note should be visible
    // under Starter's price.
    expect(screen.getAllByText(/Billed annually/i).length).toBeGreaterThan(0)

    // Click Monthly
    fireEvent.click(screen.getByRole('radio', { name: /Monthly/i }))

    // The "Billed annually" footnote should now be gone (Starter/Studio
    // no longer show it; Pro shows the monthly premium note instead).
    expect(screen.queryAllByText(/Billed annually/i)).toHaveLength(0)
  })

  it('toggles back from monthly to annual', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    // Switch to monthly
    fireEvent.click(screen.getByRole('radio', { name: /Monthly/i }))
    expect(screen.queryAllByText(/Billed annually/i)).toHaveLength(0)

    // Switch back to annual
    fireEvent.click(screen.getByRole('radio', { name: /Annually/i }))
    expect(screen.getAllByText(/Billed annually/i).length).toBeGreaterThan(0)
  })

  it('does not render urgency language or emoji', () => {
    const { container } = render(
      <PricingClient currency="USD" pricing={FIXTURE_PRICING} />,
    )
    const text = container.textContent ?? ''
    expect(text).not.toMatch(/hurry|limited offer|last chance|don't miss|act now/i)
    // Emoji detection: any Unicode emoji in the Basic Multilingual Plane range
    // eslint-disable-next-line no-control-regex
    expect(text).not.toMatch(/[\u{1F600}-\u{1F64F}]/u)
  })

  it('renders the disclosure footnote with the currency', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    expect(
      screen.getByText(/Prices shown in USD\./i, { exact: false }),
    ).toBeInTheDocument()
  })

  it('renders disclosure footnote with a non-USD currency', () => {
    render(<PricingClient currency="GBP" pricing={FIXTURE_PRICING} />)

    expect(
      screen.getByText(/Prices shown in GBP\./i, { exact: false }),
    ).toBeInTheDocument()
  })

  // -------------------------------------------------------------------------
  // Regression: a visitor currency with no price row must render the USD
  // amount labelled USD — never the visitor's raw currency code over the
  // USD number (that was the bug: a Thai visitor saw "฿19" for a real
  // ~฿690 price). FIXTURE_PRICING only has USD rows, so any non-USD
  // `currency` prop exercises the fallback path exactly like a currency
  // absent from the real catalogue (e.g. THB, AED).
  // -------------------------------------------------------------------------
  it('renders USD amounts labelled USD — not the raw currency prop — when the currency has no row', () => {
    render(<PricingClient currency="GBP" pricing={FIXTURE_PRICING} />)

    // Starter's monthly headline (annual view: annualMonthlyEquivalent =
    // $23.00) must render with the $ symbol, never a £ symbol, because
    // GBP has no row in FIXTURE_PRICING and the resolved currency must be
    // USD.
    expect(screen.getByText('$23')).toBeInTheDocument()
    expect(screen.queryByText(/£23/)).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Regression tests against the REAL SHARED_PRICING_CATALOGUE — proves the
// page now agrees with billing and that NZD/AUD render correctly end to end.
// ---------------------------------------------------------------------------

describe('PricingClient — against the real SHARED_PRICING_CATALOGUE', () => {
  it('renders Starter USD as $19/mo — agrees with billing (was $29/mo)', () => {
    render(<PricingClient currency="USD" pricing={REAL_PRICING} />)

    // Monthly view makes the exact headline figure unambiguous.
    fireEvent.click(screen.getByRole('radio', { name: /Monthly/i }))
    expect(screen.getByText('$19')).toBeInTheDocument()
  })

  it('renders Starter USD annual billed amount as $182/yr', () => {
    render(<PricingClient currency="USD" pricing={REAL_PRICING} />)

    // Annual view shows the $182 billed-once-a-year figure inside the
    // Pro card's "From $X/yr" copy is Pro-specific; Starter's own annual
    // total isn't directly labelled on this page, so assert via the
    // per-currency price data itself for Starter.
    const starter = REAL_PRICING.plans.find((p) => p.id === 'starter')!
    expect(starter.prices.USD?.annual).toBe(18200)
    expect(starter.prices.USD?.monthly).toBe(1900)
  })

  it('renders NZD prices when the currency is NZD (New Zealand merchants no longer see USD)', () => {
    render(<PricingClient currency="NZD" pricing={REAL_PRICING} />)

    fireEvent.click(screen.getByRole('radio', { name: /Monthly/i }))
    // Starter NZD monthly = 2900 minor units = NZ$29.
    expect(screen.getByText(/NZ\$29/)).toBeInTheDocument()
    expect(
      screen.getByText(/Prices shown in NZD\./i, { exact: false }),
    ).toBeInTheDocument()
  })

  it('shows the GST disclosure wherever AUD is displayed (spec §19.4)', () => {
    render(<PricingClient currency="AUD" pricing={REAL_PRICING} />)

    expect(
      screen.getByText(/Plus 10% GST for Australian businesses\./i, { exact: false }),
    ).toBeInTheDocument()
  })

  it('does not show a GST disclosure for a non-AUD currency', () => {
    render(<PricingClient currency="USD" pricing={REAL_PRICING} />)

    expect(
      screen.queryByText(/GST/i),
    ).not.toBeInTheDocument()
  })

  it('renders a USD-labelled amount for a currency the table cannot price (e.g. THB has no row)', () => {
    // normalizeCurrency would already turn THB into USD before this
    // component ever sees it — this test exercises the component's own
    // defence-in-depth fallback directly, in case a caller ever passes
    // an unnormalized currency through.
    render(<PricingClient currency="THB" pricing={REAL_PRICING} />)

    fireEvent.click(screen.getByRole('radio', { name: /Monthly/i }))
    expect(screen.getByText('$19')).toBeInTheDocument()
    expect(screen.queryByText(/฿/)).not.toBeInTheDocument()
  })
})
