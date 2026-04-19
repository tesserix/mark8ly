/**
 * PlanCard unit tests.
 *
 * Tests:
 *  - Renders plan name (via PlanBadge) + status badge + renewal line
 *  - "Change plan" CTA visible on non-Pro plans
 *  - "Contact sales" CTA on Pro plans
 *  - Trial ends line when trialEndsAt is set
 *  - Cancel scheduled line when status is cancel_scheduled
 */

import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PlanCard } from '@/app/(admin)/settings/billing/PlanCard'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'

// ---------------------------------------------------------------------------
// Fixture factory
// ---------------------------------------------------------------------------

function makePlan(overrides: Partial<CurrentPlan> = {}): CurrentPlan {
  return {
    id: 'sub_1',
    storeId: 'store_1',
    plan: 'growth',
    status: 'active',
    periodStart: '2026-04-01T00:00:00Z',
    periodEnd: '2027-04-01T00:00:00Z',
    cancelAtPeriodEnd: false,
    arbitrageFlag: false,
    billingCurrency: 'USD',
    addOns: [],
    trialEndsAt: null,
    nextInvoiceAmountMinor: null,
    paymentMethodBrand: 'visa',
    paymentMethodLast4: '4242',
    ...overrides,
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PlanCard', () => {
  it('renders the plan badge with plan name', () => {
    render(<PlanCard plan={makePlan({ plan: 'growth' })} />)
    // PlanBadge renders with role="status" and aria-label
    const badge = screen.getByRole('status', { name: /growth/i })
    expect(badge).toBeInTheDocument()
  })

  it('renders the status badge', () => {
    render(<PlanCard plan={makePlan({ status: 'active' })} />)
    const badges = screen.getAllByRole('status')
    // Two status elements: PlanBadge + SubscriptionStatusBadge
    expect(badges.length).toBeGreaterThanOrEqual(2)
    // SubscriptionStatusBadge has data-testid="status-badge"
    expect(screen.getByTestId('status-badge')).toBeInTheDocument()
    expect(screen.getByTestId('status-badge')).toHaveTextContent('Active')
  })

  it('renders the renewal line with formatted date', () => {
    render(<PlanCard plan={makePlan({ periodEnd: '2027-04-18T00:00:00Z' })} />)
    expect(screen.getByText(/Renews on/i)).toBeInTheDocument()
    expect(screen.getByText(/18 April 2027/i)).toBeInTheDocument()
  })

  it('renders the trial ends line when trialEndsAt is set', () => {
    render(
      <PlanCard
        plan={makePlan({
          status: 'trialing',
          trialEndsAt: '2026-05-01T00:00:00Z',
        })}
      />,
    )
    expect(screen.getByText(/Trial ends/i)).toBeInTheDocument()
    expect(screen.getByText(/1 May 2026/i)).toBeInTheDocument()
  })

  it('renders the cancel scheduled line when cancel_at_period_end', () => {
    render(
      <PlanCard
        plan={makePlan({
          status: 'cancel_scheduled',
          cancelAtPeriodEnd: true,
          periodEnd: '2026-05-31T00:00:00Z',
        })}
      />,
    )
    expect(
      screen.getByText(/subscription will end at the close/i),
    ).toBeInTheDocument()
  })

  it('shows "Change plan" link for non-Pro plans', () => {
    render(<PlanCard plan={makePlan({ plan: 'growth' })} />)
    const link = screen.getByRole('link', { name: /Change plan/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/settings/billing/plan-change')
  })

  it('shows "Contact sales" instead of "Change plan" for Pro plan', () => {
    render(<PlanCard plan={makePlan({ plan: 'pro' })} />)
    expect(screen.getByRole('link', { name: /Contact sales/i })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Change plan/i })).not.toBeInTheDocument()
  })

  it('always shows "Cancel subscription" link', () => {
    render(<PlanCard plan={makePlan()} />)
    const link = screen.getByRole('link', { name: /Cancel subscription/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/settings/billing/cancel')
  })

  it('renders the section heading', () => {
    render(<PlanCard plan={makePlan()} />)
    expect(
      screen.getByRole('heading', { name: /Current plan/i }),
    ).toBeInTheDocument()
  })
})
