/**
 * PricingClient unit tests — Vitest + Testing Library.
 *
 * Uses a static pricing payload that mirrors the shape from page.tsx.
 * Does NOT test the RSC page.tsx itself (that is an integration concern);
 * tests only the interactive client component.
 */

import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { PricingClient } from '@/app/pricing/PricingClient'
import { SHARED_PRICING_CATALOGUE as REAL_PRICING, type Currency, type SharedPricingCatalogue } from '@repo/ui/subscription'

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
    // "Monthly available at $119/mo." is rendered from the catalogue (via
    // <Money>), not a hardcoded literal, so "$119" sits in its own element
    // and the phrase is split across nodes — assert by fragment, matching
    // the pattern used above for the "From ... $1,188" line.
    expect(
      screen.getByText(/Monthly available at/i, { exact: false }),
    ).toBeInTheDocument()
    expect(screen.getByText('$119')).toBeInTheDocument()
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

  it('renders disclosure footnote with a non-USD currency that resolves cleanly', () => {
    // Uses REAL_PRICING (not FIXTURE_PRICING, which only has USD rows) so
    // GBP actually resolves to GBP rather than falling back to USD.
    render(<PricingClient currency="GBP" pricing={REAL_PRICING} />)

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

  // -------------------------------------------------------------------------
  // Regression: Starter USD's annual-billed monthly equivalent (18200/12 =
  // 1516.67, rounded to 1517 minor units by the generator, then displayed
  // with showCents=false) renders as "$15/mo" — but the plan is billed at
  // $182/yr, not $15 x 12 = $180. Showing the rounded equivalent alone with
  // no annual total is a misleading quote on a search-indexed page. The fix
  // must show both figures, mirroring the onboarding Pricing section.
  // -------------------------------------------------------------------------
  it('renders both the annual billed total AND the rounded monthly equivalent for Starter USD (default annual view)', () => {
    render(<PricingClient currency="USD" pricing={REAL_PRICING} />)

    const starter = REAL_PRICING.plans.find((p) => p.id === 'starter')!
    expect(starter.prices.USD).toEqual({ monthly: 1900, annual: 18200, annualMonthlyEquivalent: 1517 })

    // Rounded equivalent headline ($15.17 -> $15) ...
    expect(screen.getByText('$15')).toBeInTheDocument()
    // ... AND the exact annual total, so "$15/mo" is never left standing
    // alone as an implicit (and wrong) x12 claim.
    expect(screen.getByText('$182')).toBeInTheDocument()
  })

  it('exposes the annual total for every plan/currency combo where annual is not evenly divisible by 12 — not just Starter USD', () => {
    // getByText normalizes DOM text (collapsing \u00A0 to a regular space)
    // but does NOT normalize a string matcher — Intl inserts \u00A0 between
    // a currency code and the amount for currencies like SGD ("SGD\u00A020"),
    // so the matcher needs the same collapse or it never matches.
    const formatMajor = (minorUnits: number, currency: string) =>
      new Intl.NumberFormat('en-US', {
        style: 'currency',
        currency,
        currencyDisplay: 'symbol',
        minimumFractionDigits: 0,
        maximumFractionDigits: 0,
      })
        .format(minorUnits / 100)
        .replace(/\u00A0/g, ' ')

    // Every currency row on Starter and Studio (Pro already shows its exact
    // annual total unconditionally via ProPriceBlock's "From $X/yr" copy —
    // this exercises the two plans that go through StandardPriceBlock).
    let ambiguousRowsChecked = 0
    for (const planId of ['starter', 'studio'] as const) {
      const plan = REAL_PRICING.plans.find((p) => p.id === planId)!
      for (const [currency, price] of Object.entries(plan.prices)) {
        if (price.annual % 12 === 0) continue // no rounding ambiguity for this row
        ambiguousRowsChecked += 1

        const { unmount } = render(<PricingClient currency={currency as Currency} pricing={REAL_PRICING} />)

        expect(
          screen.getByText(formatMajor(price.annualMonthlyEquivalent, currency)),
          `${planId}/${currency}: expected rounded equivalent ${formatMajor(price.annualMonthlyEquivalent, currency)}`,
        ).toBeInTheDocument()
        expect(
          screen.getByText(formatMajor(price.annual, currency)),
          `${planId}/${currency}: expected exact annual total ${formatMajor(price.annual, currency)} alongside the equivalent`,
        ).toBeInTheDocument()

        unmount()
      }
    }

    // Sanity: prove this scenario is common, not a one-off — Starter/Studio
    // both have several currencies where annual isn't evenly divisible by 12.
    expect(ambiguousRowsChecked).toBeGreaterThan(5)
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
    // The footer disclosure must resolve the same way as the prices above —
    // "Prices shown in THB." over USD amounts would be the same mislabel
    // bug one level up. Assert the resolved label directly, not just the
    // absence of the Thai baht symbol.
    expect(
      screen.getByText(/Prices shown in USD\./i, { exact: false }),
    ).toBeInTheDocument()
    expect(screen.queryByText(/Prices shown in THB\./i)).not.toBeInTheDocument()
  })

  // -------------------------------------------------------------------------
  // Regression: the old private pricing table advertised the White-label
  // App add-on at $49/mo, and quoted only the recurring half of its
  // two-part price (missing the $2,000 one-time setup fee onboarding
  // states in full). Neither number had a test. Assert both here, and
  // assert the add-on always reads the USD row regardless of the visitor's
  // currency (spec §4.1.2 — it bills in USD globally).
  // -------------------------------------------------------------------------
  it('renders the White-label App add-on at $199/mo plus the $2,000 one-time setup fee', () => {
    render(<PricingClient currency="USD" pricing={REAL_PRICING} />)

    const addOn = REAL_PRICING.proApp.prices.USD!
    expect(addOn.monthly).toBe(19900)

    fireEvent.click(screen.getByRole('radio', { name: /Monthly/i }))

    const addOnSection = screen.getByRole('region', { name: /White-label App add-on/i })
    expect(within(addOnSection).getByText('$199')).toBeInTheDocument()
    expect(
      within(addOnSection).getByText(/\$2,000 one-time setup/i),
    ).toBeInTheDocument()
  })

  it('renders the add-on at $199/mo even for a visitor whose page currency is not USD', () => {
    // The add-on always bills in USD globally — its amount must not shift
    // when the surrounding plan prices resolve to a different currency.
    render(<PricingClient currency="NZD" pricing={REAL_PRICING} />)

    fireEvent.click(screen.getByRole('radio', { name: /Monthly/i }))

    const addOnSection = screen.getByRole('region', { name: /White-label App add-on/i })
    expect(within(addOnSection).getByText('$199')).toBeInTheDocument()
    expect(
      within(addOnSection).getByText(/\$2,000 one-time setup/i),
    ).toBeInTheDocument()
  })
})
