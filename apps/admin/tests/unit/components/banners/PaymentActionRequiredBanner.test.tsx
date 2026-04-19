/**
 * PaymentActionRequiredBanner unit tests.
 *
 * Tests:
 *  1. Returns null when isPending = false.
 *  2. Renders T-14 body when daysSinceFlagged = 1 (daysLeft = 13 > 7).
 *  3. Renders T-7 body when daysSinceFlagged = 8 (daysLeft = 6, 1 < 6 <= 7).
 *  4. Renders T-1 body when daysSinceFlagged = 13 (daysLeft = 1).
 *  5. Uses `info` tone (data-tone="info").
 *  6. CTA href is the hosted invoice URL when available.
 *  7. target="_blank" and rel="noopener noreferrer" are set on the CTA link.
 *  8. Falls back to mutation onClick when hostedInvoiceUrl is null.
 *
 * IMPORTANT (spec §4.7): payment_action_required keeps FULL admin access.
 * Tone must be `info`, never `warning` or `danger`.
 *
 * Strategy: mock useDunning hooks at module level.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PaymentActionRequiredBanner } from '@/components/shell/banners/PaymentActionRequiredBanner'

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

const mockUsePaymentActionRequiredState = vi.fn()
const mockCompleteActionMutate = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useDunning', () => ({
  usePastDueState: vi.fn(() => ({
    isPastDue: false,
    retryAt: null,
    lastRetryFailedAt: null,
    periodEnd: null,
  })),
  usePaymentActionRequiredState: (...args: unknown[]) =>
    mockUsePaymentActionRequiredState(...args),
  useCompleteActionUrl: vi.fn(() => ({
    mutate: mockCompleteActionMutate,
    isPending: false,
  })),
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STORE_ID = 'store-xyz'

function pendingState(overrides: {
  hostedInvoiceUrl?: string | null
  daysSinceFlagged?: number
}) {
  return {
    isPending: true,
    hostedInvoiceUrl: overrides.hostedInvoiceUrl ?? null,
    firstFlaggedAt: null,
    daysSinceFlagged: overrides.daysSinceFlagged ?? 0,
  }
}

function renderBanner() {
  return render(<PaymentActionRequiredBanner storeId={STORE_ID} />)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PaymentActionRequiredBanner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns null when isPending is false', () => {
    mockUsePaymentActionRequiredState.mockReturnValue({
      isPending: false,
      hostedInvoiceUrl: null,
      firstFlaggedAt: null,
      daysSinceFlagged: 0,
    })

    const { container } = renderBanner()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders T-14 body when daysSinceFlagged = 1 (daysLeft = 13)', () => {
    mockUsePaymentActionRequiredState.mockReturnValue(
      pendingState({ daysSinceFlagged: 1 }),
    )

    renderBanner()

    expect(
      screen.getByText(
        'Your bank needs to confirm this payment. You have 14 days to complete authentication.',
      ),
    ).toBeInTheDocument()
  })

  it('renders T-7 body when daysSinceFlagged = 8 (daysLeft = 6)', () => {
    mockUsePaymentActionRequiredState.mockReturnValue(
      pendingState({ daysSinceFlagged: 8 }),
    )

    renderBanner()

    expect(
      screen.getByText(
        'Your bank needs to confirm this payment. You have 7 days to complete authentication.',
      ),
    ).toBeInTheDocument()
  })

  it('renders T-1 body when daysSinceFlagged = 13 (daysLeft = 1)', () => {
    mockUsePaymentActionRequiredState.mockReturnValue(
      pendingState({ daysSinceFlagged: 13 }),
    )

    renderBanner()

    expect(
      screen.getByText(
        'Your bank needs to confirm this payment. You have 1 day to complete authentication.',
      ),
    ).toBeInTheDocument()
  })

  it('uses info tone (data-tone="info")', () => {
    mockUsePaymentActionRequiredState.mockReturnValue(
      pendingState({ daysSinceFlagged: 0 }),
    )

    renderBanner()

    expect(screen.getByTestId('banner-shell')).toHaveAttribute('data-tone', 'info')
  })

  it('CTA renders as anchor with the hosted invoice URL when available', () => {
    const invoiceUrl = 'https://invoice.stripe.com/i/acct_test/test_invoice'
    mockUsePaymentActionRequiredState.mockReturnValue(
      pendingState({ daysSinceFlagged: 0, hostedInvoiceUrl: invoiceUrl }),
    )

    renderBanner()

    const link = screen.getByRole('link', { name: 'Review the invoice' })
    expect(link).toHaveAttribute('href', invoiceUrl)
  })

  it('sets target="_blank" and rel="noopener noreferrer" on the CTA link', () => {
    const invoiceUrl = 'https://invoice.stripe.com/i/acct_test/test_invoice'
    mockUsePaymentActionRequiredState.mockReturnValue(
      pendingState({ daysSinceFlagged: 0, hostedInvoiceUrl: invoiceUrl }),
    )

    renderBanner()

    const link = screen.getByRole('link', { name: 'Review the invoice' })
    expect(link).toHaveAttribute('href', invoiceUrl)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('calls mutation onClick when hostedInvoiceUrl is null', async () => {
    mockUsePaymentActionRequiredState.mockReturnValue(
      pendingState({ daysSinceFlagged: 0, hostedInvoiceUrl: null }),
    )

    renderBanner()

    await userEvent.click(
      screen.getByRole('button', { name: 'Review the invoice' }),
    )
    expect(mockCompleteActionMutate).toHaveBeenCalledOnce()
  })
})
