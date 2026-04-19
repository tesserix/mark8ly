/**
 * PlanChangeClient unit tests — Vitest + Testing Library.
 *
 * All hooks are mocked at module level. Since every hook is mocked, no real
 * React Query store is needed — there is no QueryClientProvider wrapper.
 *
 * Tests:
 *   1. Renders all 3 plan options (Starter, Studio, Pro)
 *   2. Current plan gets "Current plan" label
 *   3. Radiogroup has correct ARIA attributes
 *   4. Selecting a target plan marks it as selected
 *   5. Upgrade decision shows "update immediately" copy
 *   6. Downgrade decision shows scheduled-for-period-end copy
 *   7. Pro monthly shows premium note
 *   8. Apply button is disabled for no-op (same plan + period)
 *   9. Apply button calls mutation, shows success toast, and redirects
 *   10. Downgrade shows "Plan change scheduled." toast
 *   11. Mutation error shows error toast and inline error
 *   12. Blocking decision disables Apply button
 *   13. Cancel link points to /settings/billing
 *   14. Skeleton renders when current plan is loading
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'

// ---------------------------------------------------------------------------
// Mocks — declared before any imports that reference these modules
// ---------------------------------------------------------------------------

const mockPush = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

const mockToastSuccess = vi.fn()
const mockToastError = vi.fn()

vi.mock('@/components/feedback/Toaster', () => ({
  useToast: () => ({
    toast: {
      success: mockToastSuccess,
      error: mockToastError,
    },
  }),
}))

// ---------------------------------------------------------------------------
// Hook control variables — updated per test in beforeEach / per-test overrides
// ---------------------------------------------------------------------------

type MockCurrentPlanResult = {
  data: unknown
  isLoading: boolean
  error: unknown
}

type MockPreflightResult = {
  data: unknown
  isLoading: boolean
  error: unknown
}

let mockCurrentPlanResult: MockCurrentPlanResult = {
  data: undefined,
  isLoading: false,
  error: null,
}

let mockPreflightResult: MockPreflightResult = {
  data: undefined,
  isLoading: false,
  error: null,
}

let mockMutateAsync = vi.fn()
let mockIsPending = false

vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useCurrentPlan: () => mockCurrentPlanResult,
}))

vi.mock('@/lib/api/subscription/hooks/usePlanChange', () => ({
  useProrationPreview: () => mockPreflightResult,
  useApplyPlanChange: () => ({
    mutateAsync: mockMutateAsync,
    isPending: mockIsPending,
  }),
}))

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import React from 'react'
import { PlanChangeClient } from '@/app/(admin)/settings/billing/plan-change/PlanChangeClient'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'
import type { PreflightReport } from '@/lib/api/subscription/schemas/planChange'

// ---------------------------------------------------------------------------
// Fixture factories
// ---------------------------------------------------------------------------

function makeCurrentPlan(overrides: Partial<CurrentPlan> = {}): CurrentPlan {
  return {
    id: 'sub_1',
    storeId: 'store_1',
    plan: 'starter',
    status: 'active',
    periodStart: '2026-04-01T00:00:00Z',
    periodEnd: '2026-05-01T00:00:00Z',
    cancelAtPeriodEnd: false,
    arbitrageFlag: false,
    billingCurrency: 'USD',
    addOns: [],
    trialEndsAt: null,
    nextInvoiceAmountMinor: null,
    paymentMethodBrand: null,
    paymentMethodLast4: null,
    ...overrides,
  }
}

function makePreflightReport(
  overrides: Partial<PreflightReport> = {},
): PreflightReport {
  return {
    decision: 'allow_upgrade_now',
    current_plan: 'starter',
    current_period: 'monthly',
    target_plan: 'studio',
    target_period: 'monthly',
    effective_at: '2026-04-19T10:00:00Z',
    current_plan_diff: {
      stores_delta: 1,
      images_per_product_delta: 5,
      campaign_emails_delta: -1,
    },
    ...overrides,
  }
}

function renderComponent(storeId = 'store_1') {
  return render(<PlanChangeClient storeId={storeId} />)
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  mockPush.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  mockMutateAsync = vi.fn()
  mockIsPending = false

  // Default: starter plan loaded, no preflight data
  mockCurrentPlanResult = {
    data: makeCurrentPlan(),
    isLoading: false,
    error: null,
  }

  mockPreflightResult = {
    data: undefined,
    isLoading: false,
    error: null,
  }
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PlanChangeClient', () => {
  it('renders all 3 plan options', () => {
    renderComponent()

    expect(screen.getByRole('radio', { name: /starter/i })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /studio/i })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /pro/i })).toBeInTheDocument()
  })

  it('marks the current plan row as selected by default', () => {
    renderComponent()

    const starterRadio = screen.getByRole('radio', { name: /starter/i })
    expect(starterRadio).toHaveAttribute('aria-checked', 'true')
  })

  it('shows "Current plan" label on the current plan option', () => {
    renderComponent()

    expect(screen.getByText('Current plan')).toBeInTheDocument()
  })

  it('radiogroup has correct ARIA role and label', () => {
    renderComponent()

    const group = screen.getByRole('radiogroup', { name: /choose a plan/i })
    expect(group).toBeInTheDocument()
  })

  it('selects a different plan when clicked', () => {
    renderComponent()

    const studioRadio = screen.getByRole('radio', { name: /studio/i })
    fireEvent.click(studioRadio)

    expect(studioRadio).toHaveAttribute('aria-checked', 'true')
    // Starter should no longer be checked
    expect(
      screen.getByRole('radio', { name: /starter/i }),
    ).toHaveAttribute('aria-checked', 'false')
  })

  it('shows upgrade copy when upgrade decision is returned', () => {
    mockPreflightResult = {
      data: makePreflightReport({ decision: 'allow_upgrade_now' }),
      isLoading: false,
      error: null,
    }

    renderComponent()

    fireEvent.click(screen.getByRole('radio', { name: /studio/i }))

    expect(
      screen.getByText(/your plan will update immediately/i),
    ).toBeInTheDocument()
  })

  it('shows scheduled-for-period-end copy for downgrade', () => {
    mockCurrentPlanResult = {
      data: makeCurrentPlan({ plan: 'studio' }),
      isLoading: false,
      error: null,
    }

    mockPreflightResult = {
      data: makePreflightReport({
        decision: 'allow_downgrade_at_period_end',
        current_plan: 'studio',
        target_plan: 'starter',
        effective_at: '2026-05-01T00:00:00Z',
      }),
      isLoading: false,
      error: null,
    }

    renderComponent()

    fireEvent.click(screen.getByRole('radio', { name: /starter/i }))

    expect(
      screen.getByText(/you'll move to the new plan on/i),
    ).toBeInTheDocument()
  })

  it('shows pro monthly premium note when Pro + monthly selected', () => {
    mockCurrentPlanResult = {
      data: makeCurrentPlan({ plan: 'starter' }),
      isLoading: false,
      error: null,
    }

    mockPreflightResult = {
      data: makePreflightReport({
        decision: 'allow_upgrade_now',
        target_plan: 'pro',
        target_period: 'monthly',
      }),
      isLoading: false,
      error: null,
    }

    renderComponent()

    fireEvent.click(screen.getByRole('radio', { name: /pro/i }))

    // Monthly is the default period — premium note must appear
    expect(
      screen.getByText(/Monthly Pro carries a 20% premium/i),
    ).toBeInTheDocument()
  })

  it('Apply button is disabled when selection is unchanged (no-op)', () => {
    renderComponent()

    // Default selection is starter (current plan) — no-op
    const applyButton = screen.getByRole('button', { name: /apply change/i })
    expect(applyButton).toBeDisabled()
  })

  it('Apply button calls mutation, shows success toast, and redirects', async () => {
    mockPreflightResult = {
      data: makePreflightReport({ decision: 'allow_upgrade_now' }),
      isLoading: false,
      error: null,
    }

    mockMutateAsync.mockResolvedValueOnce({
      result: 'upgrade_committed',
      effective_at: '2026-04-19T10:00:00Z',
      stripe_updated: true,
    })

    renderComponent()

    fireEvent.click(screen.getByRole('radio', { name: /studio/i }))

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /apply change/i }))
    })

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith({
        target_plan: 'studio',
        target_period: 'monthly',
      })
      expect(mockToastSuccess).toHaveBeenCalledWith('Plan changed.')
      expect(mockPush).toHaveBeenCalledWith('/settings/billing')
    })
  })

  it('downgrade shows "Plan change scheduled." toast', async () => {
    mockCurrentPlanResult = {
      data: makeCurrentPlan({ plan: 'studio' }),
      isLoading: false,
      error: null,
    }

    mockPreflightResult = {
      data: makePreflightReport({
        decision: 'allow_downgrade_at_period_end',
        current_plan: 'studio',
        target_plan: 'starter',
        effective_at: '2026-05-01T00:00:00Z',
      }),
      isLoading: false,
      error: null,
    }

    mockMutateAsync.mockResolvedValueOnce({
      result: 'downgrade_scheduled',
      effective_at: '2026-05-01T00:00:00Z',
      stripe_updated: false,
    })

    renderComponent()

    fireEvent.click(screen.getByRole('radio', { name: /starter/i }))

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /apply change/i }))
    })

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith('Plan change scheduled.')
    })
  })

  it('mutation error shows error toast and inline error message', async () => {
    mockPreflightResult = {
      data: makePreflightReport({ decision: 'allow_upgrade_now' }),
      isLoading: false,
      error: null,
    }

    mockMutateAsync.mockRejectedValueOnce(new Error('network failure'))

    renderComponent()

    fireEvent.click(screen.getByRole('radio', { name: /studio/i }))

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /apply change/i }))
    })

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "Couldn't change plan. Please try again.",
      )
    })

    expect(screen.getByRole('alert')).toHaveTextContent(
      "Couldn't change plan. Please try again.",
    )
  })

  it('blocking decision disables the Apply button', () => {
    mockPreflightResult = {
      data: makePreflightReport({ decision: 'block_read_only' }),
      isLoading: false,
      error: null,
    }

    renderComponent()

    fireEvent.click(screen.getByRole('radio', { name: /studio/i }))

    const applyButton = screen.getByRole('button', { name: /apply change/i })
    expect(applyButton).toBeDisabled()
  })

  it('Cancel link points to /settings/billing', () => {
    renderComponent()

    const cancelLink = screen.getByRole('link', { name: /cancel/i })
    expect(cancelLink).toHaveAttribute('href', '/settings/billing')
  })

  it('shows skeleton while current plan is loading', () => {
    mockCurrentPlanResult = {
      data: undefined,
      isLoading: true,
      error: null,
    }

    renderComponent()

    // When loading, the radiogroup should not be present
    expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  })
})
