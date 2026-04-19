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
import type { PricingCatalogue } from '@/app/pricing/page'

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const FIXTURE_PRICING: PricingCatalogue = {
  plans: [
    {
      id: 'starter',
      prices: {
        USD: { monthly: 2900, annual: 27600, annualMonthlyEquivalent: 2300 },
      },
    },
    {
      id: 'growth',
      prices: {
        USD: { monthly: 7900, annual: 75600, annualMonthlyEquivalent: 6300 },
      },
    },
    {
      id: 'scale',
      prices: {
        USD: { monthly: 19900, annual: 190800, annualMonthlyEquivalent: 15900 },
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
  it('renders all 4 plan names', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    expect(screen.getByRole('heading', { name: /Starter/i })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Growth/i })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Scale/i })).toBeInTheDocument()
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

  it('shows "Start free trial" CTA on Starter, Growth, and Scale plans', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    const trialLinks = screen.getAllByRole('link', { name: /Start free trial/i })
    expect(trialLinks).toHaveLength(3)
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
    expect(screen.getByText(/Add-on — requires Growth plan or higher/i)).toBeInTheDocument()
  })

  it('toggles from annual to monthly when the Monthly button is clicked', () => {
    render(<PricingClient currency="USD" pricing={FIXTURE_PRICING} />)

    // Annual is the default — the "Billed annually" note should be visible
    // under Starter's price.
    expect(screen.getAllByText(/Billed annually/i).length).toBeGreaterThan(0)

    // Click Monthly
    fireEvent.click(screen.getByRole('radio', { name: /Monthly/i }))

    // The "Billed annually" footnote should now be gone (Starter/Growth/Scale
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
})
