/**
 * FailedPaymentBanner unit tests.
 *
 * Tests:
 *  1. Returns null when usePastDueState says isPastDue = false.
 *  2. Renders heading + body with formatted retry date when past_due.
 *  3. CTA label is "Add a new card".
 *  4. Clicking CTA invokes openPortal mock.
 *
 * Strategy: mock useDunning + useBilling hooks at module level so the
 * component can render without a QueryProvider or MSW server.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FailedPaymentBanner } from '@/components/shell/banners/FailedPaymentBanner'

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

const mockUsePastDueState = vi.fn()
const mockMutate = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useDunning', () => ({
  usePastDueState: (...args: unknown[]) => mockUsePastDueState(...args),
  useCompleteActionUrl: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}))

vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useCurrentPlan: vi.fn(),
  useOpenPortal: vi.fn(() => ({
    mutate: mockMutate,
    isPending: false,
  })),
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STORE_ID = 'store-abc'

function renderBanner() {
  return render(<FailedPaymentBanner storeId={STORE_ID} />)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('FailedPaymentBanner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns null when isPastDue is false', () => {
    mockUsePastDueState.mockReturnValue({
      isPastDue: false,
      retryAt: null,
      lastRetryFailedAt: null,
      periodEnd: null,
    })

    const { container } = renderBanner()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders heading and body with formatted retry date', () => {
    // retryAt = 21 April 2026 → "21 April 2026"
    mockUsePastDueState.mockReturnValue({
      isPastDue: true,
      retryAt: new Date('2026-04-21T00:00:00Z'),
      lastRetryFailedAt: null,
      periodEnd: new Date('2026-04-21T00:00:00Z'),
    })

    renderBanner()

    expect(screen.getByText("Payment didn't go through")).toBeInTheDocument()
    // The body should contain the formatted retry date
    expect(
      screen.getByText(/Your most recent payment did not go through/i),
    ).toBeInTheDocument()
    expect(screen.getByText(/21 April 2026/i)).toBeInTheDocument()
  })

  it('renders "Add a new card" as the CTA label', () => {
    mockUsePastDueState.mockReturnValue({
      isPastDue: true,
      retryAt: new Date('2026-04-21T00:00:00Z'),
      lastRetryFailedAt: null,
      periodEnd: new Date('2026-04-21T00:00:00Z'),
    })

    renderBanner()

    expect(
      screen.getByRole('button', { name: 'Add a new card' }),
    ).toBeInTheDocument()
  })

  it('calls openPortal.mutate when CTA is clicked', async () => {
    mockUsePastDueState.mockReturnValue({
      isPastDue: true,
      retryAt: new Date('2026-04-21T00:00:00Z'),
      lastRetryFailedAt: null,
      periodEnd: new Date('2026-04-21T00:00:00Z'),
    })

    renderBanner()

    await userEvent.click(screen.getByRole('button', { name: 'Add a new card' }))
    expect(mockMutate).toHaveBeenCalledOnce()
  })
})
