/**
 * WhiteLabelAppLifecycleWidget unit tests — Vitest + Testing Library.
 *
 * Tests:
 *   1. No white_label_app add-on → returns null.
 *   2. pending_credentials → renders "Upload credentials" CTA for both platforms.
 *   3. live_both → both rows show "Live" with moss accent styling.
 *   4. update_needed → renders disabled "Build new version" button + update note.
 *   5. No urgency language in any rendered output.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'

// ---------------------------------------------------------------------------
// Mock next/link
// ---------------------------------------------------------------------------

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

// ---------------------------------------------------------------------------
// Mock useCurrentPlan — controlled per test
// ---------------------------------------------------------------------------

let planMock: CurrentPlan | null = null
let isLoadingMock = false

vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useCurrentPlan: () => ({ data: planMock, isLoading: isLoadingMock }),
}))

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import { WhiteLabelAppLifecycleWidget } from '@/components/settings/billing/WhiteLabelAppLifecycleWidget'

// ---------------------------------------------------------------------------
// Fixture factory
// ---------------------------------------------------------------------------

function makePlan(overrides: Partial<CurrentPlan> = {}): CurrentPlan {
  return {
    id: 'sub_1',
    storeId: 'store_1',
    plan: 'pro',
    status: 'active',
    periodStart: '2026-04-01T00:00:00Z',
    periodEnd: '2027-04-01T00:00:00Z',
    cancelAtPeriodEnd: false,
    arbitrageFlag: false,
    billingCurrency: 'USD',
    addOns: ['white_label_app'],
    createdAt: '2026-01-01T00:00:00Z',
    trialEndsAt: null,
    nextInvoiceAmountMinor: null,
    paymentMethodBrand: 'visa',
    paymentMethodLast4: '4242',
    whiteLabelApp: null,
    ...overrides,
  }
}

const STORE_ID = 'store-wl-test'

function renderWidget() {
  return render(<WhiteLabelAppLifecycleWidget storeId={STORE_ID} />)
}

// ---------------------------------------------------------------------------
// beforeEach
// ---------------------------------------------------------------------------

beforeEach(() => {
  planMock = null
  isLoadingMock = false
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('WhiteLabelAppLifecycleWidget', () => {
  it('returns null when plan does not have white_label_app add-on', () => {
    planMock = makePlan({ addOns: [] })
    const { container } = renderWidget()
    expect(container.firstChild).toBeNull()
  })

  it('returns null while loading', () => {
    isLoadingMock = true
    planMock = null
    const { container } = renderWidget()
    expect(container.firstChild).toBeNull()
  })

  it('renders "Upload credentials" CTA when state is pending_credentials', () => {
    planMock = makePlan({
      whiteLabelApp: { lifecycleState: 'pending_credentials', updatedAt: null },
    })
    renderWidget()

    expect(screen.getByTestId('wl-lifecycle-widget')).toBeInTheDocument()

    // Both platform rows should show the credentials CTA
    const ctaLinks = screen.getAllByRole('link', { name: /Upload credentials/i })
    expect(ctaLinks.length).toBeGreaterThanOrEqual(1)
    ctaLinks.forEach((link) => {
      expect(link).toHaveAttribute(
        'href',
        '/settings/billing/pro-app-purchase/credentials',
      )
    })
  })

  it('renders "Live" for both rows when state is live_both', () => {
    planMock = makePlan({
      whiteLabelApp: { lifecycleState: 'live_both', updatedAt: '2026-04-10T00:00:00Z' },
    })
    renderWidget()

    const appleState = screen.getByTestId('lifecycle-state-apple')
    const googleState = screen.getByTestId('lifecycle-state-google')

    expect(appleState).toHaveTextContent('Live')
    expect(googleState).toHaveTextContent('Live')

    // Moss accent class applied (text-[var(--moss-700)])
    expect(appleState.className).toContain('moss-700')
    expect(googleState.className).toContain('moss-700')
  })

  it('renders disabled "Build new version" and update note when state is update_needed', () => {
    planMock = makePlan({
      whiteLabelApp: { lifecycleState: 'update_needed', updatedAt: null },
    })
    renderWidget()

    const buildBtns = screen.getAllByTestId(/lifecycle-cta-build/)
    expect(buildBtns.length).toBeGreaterThanOrEqual(1)
    buildBtns.forEach((btn) => {
      expect(btn).toBeDisabled()
      expect(btn).toHaveTextContent('Build new version')
    })

    expect(screen.getByTestId('lifecycle-update-note')).toBeInTheDocument()
    expect(screen.getByTestId('lifecycle-update-note')).toHaveTextContent(
      'new build is required',
    )
  })

  it('does not contain urgency language', () => {
    planMock = makePlan({
      whiteLabelApp: { lifecycleState: 'update_needed', updatedAt: null },
    })
    renderWidget()

    const text = document.body.textContent ?? ''
    expect(text).not.toMatch(/!/g)
    expect(text).not.toMatch(/urgent/i)
    expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })
})
