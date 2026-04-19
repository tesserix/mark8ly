/**
 * PlanCard unit tests.
 *
 * Tests:
 *  - Renders plan name (via PlanBadge) + status badge + renewal line
 *  - "Change plan" CTA visible on non-Pro plans
 *  - "Contact sales" CTA on Pro plans
 *  - Trial ends line when trialEndsAt is set
 *  - Cancel scheduled line when status is cancel_scheduled
 *  - cancel_scheduled: hides normal CTAs, shows restore CTA + editorial note
 *  - cancel_scheduled: clicking Restore invokes the mutation
 *  - cancel_scheduled: inline error shown on mutation failure
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { PlanCard } from '@/app/(admin)/settings/billing/PlanCard'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'

// ---------------------------------------------------------------------------
// Mock the cancellation hook module
// ---------------------------------------------------------------------------

const mockMutate = vi.fn()
const mockRevertResult = {
  mutate: mockMutate,
  isPending: false,
  isError: false,
  isSuccess: false,
}

vi.mock('@/lib/api/subscription/hooks/useCancellation', () => ({
  useRevertCancellation: vi.fn(() => mockRevertResult),
  useSubmitCancellation: vi.fn(),
}))

// Mock useToast — no-op so tests don't need a Toaster wrapper
vi.mock('@/components/feedback/Toaster', () => ({
  useToast: () => ({
    toast: {
      success: vi.fn(),
      error: vi.fn(),
      info: vi.fn(),
      dismiss: vi.fn(),
    },
  }),
}))

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

const STORE_ID = 'store_abc'

// ---------------------------------------------------------------------------
// Tests — normal (non-cancel_scheduled) states
// ---------------------------------------------------------------------------

describe('PlanCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Reset to non-error, non-pending state before each test
    Object.assign(mockRevertResult, {
      mutate: mockMutate,
      isPending: false,
      isError: false,
      isSuccess: false,
    })
  })

  it('renders the plan badge with plan name', () => {
    render(<PlanCard plan={makePlan({ plan: 'growth' })} storeId={STORE_ID} />)
    const badge = screen.getByRole('status', { name: /growth/i })
    expect(badge).toBeInTheDocument()
  })

  it('renders the status badge', () => {
    render(<PlanCard plan={makePlan({ status: 'active' })} storeId={STORE_ID} />)
    const badges = screen.getAllByRole('status')
    // Two status elements: PlanBadge + SubscriptionStatusBadge
    expect(badges.length).toBeGreaterThanOrEqual(2)
    expect(screen.getByTestId('status-badge')).toBeInTheDocument()
    expect(screen.getByTestId('status-badge')).toHaveTextContent('Active')
  })

  it('renders the renewal line with formatted date', () => {
    render(
      <PlanCard
        plan={makePlan({ periodEnd: '2027-04-18T00:00:00Z' })}
        storeId={STORE_ID}
      />,
    )
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
        storeId={STORE_ID}
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
        storeId={STORE_ID}
      />,
    )
    expect(
      screen.getByText(/full access continues until/i),
    ).toBeInTheDocument()
  })

  it('shows "Change plan" link for non-Pro plans', () => {
    render(<PlanCard plan={makePlan({ plan: 'growth' })} storeId={STORE_ID} />)
    const link = screen.getByRole('link', { name: /Change plan/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/settings/billing/plan-change')
  })

  it('shows "Contact sales" instead of "Change plan" for Pro plan', () => {
    render(<PlanCard plan={makePlan({ plan: 'pro' })} storeId={STORE_ID} />)
    expect(screen.getByRole('link', { name: /Contact sales/i })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Change plan/i })).not.toBeInTheDocument()
  })

  it('always shows "Cancel subscription" link when status is active', () => {
    render(<PlanCard plan={makePlan()} storeId={STORE_ID} />)
    const link = screen.getByRole('link', { name: /Cancel subscription/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/settings/billing/cancel')
  })

  it('renders the section heading', () => {
    render(<PlanCard plan={makePlan()} storeId={STORE_ID} />)
    expect(
      screen.getByRole('heading', { name: /Current plan/i }),
    ).toBeInTheDocument()
  })

  // -------------------------------------------------------------------------
  // cancel_scheduled state
  // -------------------------------------------------------------------------

  describe('when status is cancel_scheduled', () => {
    const cancelPlan = makePlan({
      status: 'cancel_scheduled',
      cancelAtPeriodEnd: true,
      periodEnd: '2027-05-18T00:00:00Z',
    })

    it('does NOT render "Change plan" CTA', () => {
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      expect(
        screen.queryByRole('link', { name: /Change plan/i }),
      ).not.toBeInTheDocument()
    })

    it('does NOT render "Cancel subscription" CTA', () => {
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      expect(
        screen.queryByRole('link', { name: /Cancel subscription/i }),
      ).not.toBeInTheDocument()
    })

    it('renders the editorial note with formatted period_end date', () => {
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      // Note includes the formatted date "18 May 2027"
      expect(screen.getByText(/18 May 2027/i)).toBeInTheDocument()
      expect(screen.getByText(/full access continues until/i)).toBeInTheDocument()
    })

    it('renders "Restore subscription" CTA', () => {
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      expect(
        screen.getByRole('button', { name: /Restore subscription/i }),
      ).toBeInTheDocument()
    })

    it('editorial note has aria-live="polite" for screen readers', () => {
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      const note = screen.getByText(/full access continues until/i)
      expect(note).toHaveAttribute('aria-live', 'polite')
    })

    it('calls useRevertCancellation with storeId', async () => {
      const { useRevertCancellation } = await import(
        '@/lib/api/subscription/hooks/useCancellation'
      )
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      expect(useRevertCancellation).toHaveBeenCalledWith(STORE_ID)
    })

    it('calls mutate when Restore subscription is clicked', () => {
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      const btn = screen.getByRole('button', { name: /Restore subscription/i })
      fireEvent.click(btn)
      expect(mockMutate).toHaveBeenCalledTimes(1)
    })

    it('shows loading text while mutation is pending', () => {
      Object.assign(mockRevertResult, { isPending: true })
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      expect(screen.getByRole('button', { name: /Restoring/i })).toBeInTheDocument()
    })

    it('disables the button while mutation is pending', () => {
      Object.assign(mockRevertResult, { isPending: true })
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      expect(screen.getByRole('button', { name: /Restoring/i })).toBeDisabled()
    })

    it('shows inline error text when mutation has failed', () => {
      Object.assign(mockRevertResult, { isError: true })
      render(<PlanCard plan={cancelPlan} storeId={STORE_ID} />)
      expect(
        screen.getByRole('alert'),
      ).toBeInTheDocument()
      expect(
        screen.getByRole('alert'),
      ).toHaveTextContent(/Couldn't restore/i)
    })
  })
})
