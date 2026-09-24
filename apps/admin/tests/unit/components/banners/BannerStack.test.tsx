/**
 * BannerStack unit tests.
 *
 * Tests:
 *   1.  All four hooks return false → renders null (nothing).
 *   2.  Only trial active → renders TrialBanner, not the others.
 *   3.  Only past_due active → renders FailedPaymentBanner.
 *   4.  Trial AND past_due both active → renders FailedPaymentBanner only
 *       (past_due outranks trial).
 *   5.  All active below read-only → renders PaymentActionRequiredBanner only.
 *   5b. Read-only active → outranks everything, including
 *       payment_action_required. These are the states where the merchant
 *       cannot trade at all, so nothing else is worth saying first.
 *   6.  role="region" wrapper is present with aria-label="System notification"
 *       when a banner is active.
 *   7.  role="region" wrapper is absent when no banner is active (returns null).
 *
 * Strategy: mock the four isActive hooks and the four banner components at
 * the module boundary. The banner components are replaced with minimal stubs
 * so we verify WHICH component mounts without needing a QueryProvider.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { BannerStack } from '@/components/shell/banners/BannerStack'

// ---------------------------------------------------------------------------
// Mock the isActive hooks
// ---------------------------------------------------------------------------

const mockIsReadOnly = vi.fn<[], boolean>(() => false)
const mockIsPaymentActionRequired = vi.fn<[], boolean>(() => false)
const mockIsFailedPayment = vi.fn<[], boolean>(() => false)
const mockIsTrial = vi.fn<[], boolean>(() => false)

vi.mock('@/components/shell/banners/ReadOnlyBanner', () => ({
  useReadOnlyBannerActive: (...args: unknown[]) => mockIsReadOnly(...(args as [])),
  ReadOnlyBanner: () => <div data-testid="read-only-banner" />,
}))

vi.mock('@/components/shell/banners/PaymentActionRequiredBanner', () => ({
  usePaymentActionRequiredBannerActive: (...args: unknown[]) =>
    mockIsPaymentActionRequired(...(args as [])),
  PaymentActionRequiredBanner: () => (
    <div data-testid="payment-action-required-banner" />
  ),
}))

vi.mock('@/components/shell/banners/FailedPaymentBanner', () => ({
  useFailedPaymentBannerActive: (...args: unknown[]) =>
    mockIsFailedPayment(...(args as [])),
  FailedPaymentBanner: () => <div data-testid="failed-payment-banner" />,
}))

vi.mock('@/components/shell/banners/TrialBanner', () => ({
  useTrialBannerActive: (...args: unknown[]) => mockIsTrial(...(args as [])),
  TrialBanner: () => <div data-testid="trial-banner" />,
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STORE_ID = 'store-banner-stack-test'

function renderStack() {
  return render(<BannerStack storeId={STORE_ID} />)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('BannerStack', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockIsReadOnly.mockReturnValue(false)
    mockIsPaymentActionRequired.mockReturnValue(false)
    mockIsFailedPayment.mockReturnValue(false)
    mockIsTrial.mockReturnValue(false)
  })

  it('renders nothing when all three hooks return false', () => {
    const { container } = renderStack()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders TrialBanner when only trial is active', () => {
    mockIsTrial.mockReturnValue(true)

    renderStack()

    expect(screen.getByTestId('trial-banner')).toBeInTheDocument()
    expect(screen.queryByTestId('failed-payment-banner')).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('payment-action-required-banner'),
    ).not.toBeInTheDocument()
  })

  it('renders FailedPaymentBanner when only past_due is active', () => {
    mockIsFailedPayment.mockReturnValue(true)

    renderStack()

    expect(screen.getByTestId('failed-payment-banner')).toBeInTheDocument()
    expect(screen.queryByTestId('trial-banner')).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('payment-action-required-banner'),
    ).not.toBeInTheDocument()
  })

  it('renders FailedPaymentBanner when both trial and past_due are active (priority)', () => {
    mockIsTrial.mockReturnValue(true)
    mockIsFailedPayment.mockReturnValue(true)

    renderStack()

    expect(screen.getByTestId('failed-payment-banner')).toBeInTheDocument()
    expect(screen.queryByTestId('trial-banner')).not.toBeInTheDocument()
  })

  it('renders PaymentActionRequiredBanner when all three are active (highest priority wins)', () => {
    mockIsPaymentActionRequired.mockReturnValue(true)
    mockIsFailedPayment.mockReturnValue(true)
    mockIsTrial.mockReturnValue(true)

    renderStack()

    expect(
      screen.getByTestId('payment-action-required-banner'),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('failed-payment-banner')).not.toBeInTheDocument()
    expect(screen.queryByTestId('trial-banner')).not.toBeInTheDocument()
  })

  it('wraps the active banner in role="region" with aria-label="System notification"', () => {
    mockIsTrial.mockReturnValue(true)

    renderStack()

    const region = screen.getByRole('region', { name: 'System notification' })
    expect(region).toBeInTheDocument()
    expect(region).toContainElement(screen.getByTestId('trial-banner'))
  })

  it('does not render the role="region" wrapper when no banner is active', () => {
    renderStack()

    expect(
      screen.queryByRole('region', { name: 'System notification' }),
    ).not.toBeInTheDocument()
  })

  it('read-only outranks every other banner, including payment_action_required', () => {
    // A merchant in expired / store_closed / pending_hard_delete cannot
    // trade at all. Telling them their bank needs to confirm a payment, or
    // that their trial has 15 days left, is at best noise and at worst
    // contradicts the state they are actually in.
    mockIsReadOnly.mockReturnValue(true)
    mockIsPaymentActionRequired.mockReturnValue(true)
    mockIsFailedPayment.mockReturnValue(true)
    mockIsTrial.mockReturnValue(true)

    renderStack()

    expect(screen.getByTestId('read-only-banner')).toBeInTheDocument()
    expect(screen.queryByTestId('payment-action-required-banner')).toBeNull()
    expect(screen.queryByTestId('failed-payment-banner')).toBeNull()
    expect(screen.queryByTestId('trial-banner')).toBeNull()
  })

  it('renders the read-only banner inside the notification landmark', () => {
    mockIsReadOnly.mockReturnValue(true)
    renderStack()
    const region = screen.getByRole('region', { name: 'System notification' })
    expect(region).toContainElement(screen.getByTestId('read-only-banner'))
  })
})
