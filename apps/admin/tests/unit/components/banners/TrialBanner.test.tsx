/**
 * TrialBanner unit tests.
 *
 * Tests:
 *   1.  bannerVariant='none' → returns null.
 *   2.  bannerVariant='day60' → heading "30 days left in your trial".
 *   3.  bannerVariant='day75' → heading "2 weeks left in your trial".
 *   4.  bannerVariant='day85' → heading "5 days left in your trial".
 *   5.  Tone is always 'info' (data-tone="info") — never warning/danger.
 *   6.  No urgency language ("Hurry", "Don't lose", "Last chance", "!").
 *   7.  No emoji in rendered text.
 *   8.  CTA label is "Add a payment method".
 *   9.  Clicking CTA invokes openPortal.mutate.
 *   10. data-testid="trial-banner" is present when variant !== 'none'.
 *
 * Strategy: mock useTrialStatus + useOpenPortal at module level so the
 * component renders without a QueryProvider or MSW server.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TrialBanner } from '@/components/shell/banners/TrialBanner'
import type { TrialStatus } from '@/lib/api/subscription/hooks/useTrial'

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

const mockUseTrialStatus = vi.fn()
const mockMutate = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useTrial', () => ({
  useTrialStatus: (...args: unknown[]) => mockUseTrialStatus(...args),
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

const STORE_ID = 'store-trial-test'

const TRIAL_ENDS_AT = new Date('2026-05-18T00:00:00Z')

function makeStatus(overrides: Partial<TrialStatus>): TrialStatus {
  return {
    isTrialing: true,
    daysSinceSignup: 60,
    daysUntilTrialEnd: 30,
    trialEndsAt: TRIAL_ENDS_AT,
    bannerVariant: 'day60',
    ...overrides,
  }
}

function renderBanner() {
  return render(<TrialBanner storeId={STORE_ID} />)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('TrialBanner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns null when bannerVariant is none', () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({ bannerVariant: 'none', isTrialing: false }),
    )

    const { container } = renderBanner()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders heading "30 days left in your trial" for day60 variant', () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({ bannerVariant: 'day60', daysSinceSignup: 60 }),
    )

    renderBanner()

    expect(
      screen.getByText('30 days left in your trial'),
    ).toBeInTheDocument()
  })

  it('renders heading "2 weeks left in your trial" for day75 variant', () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({
        bannerVariant: 'day75',
        daysSinceSignup: 75,
        daysUntilTrialEnd: 15,
      }),
    )

    renderBanner()

    expect(
      screen.getByText('2 weeks left in your trial'),
    ).toBeInTheDocument()
  })

  it('renders heading "5 days left in your trial" for day85 variant', () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({
        bannerVariant: 'day85',
        daysSinceSignup: 85,
        daysUntilTrialEnd: 5,
      }),
    )

    renderBanner()

    expect(
      screen.getByText('5 days left in your trial'),
    ).toBeInTheDocument()
  })

  it('sets tone to info on the BannerShell — never warning or danger', () => {
    for (const variant of ['day60', 'day75', 'day85'] as const) {
      vi.clearAllMocks()
      mockUseTrialStatus.mockReturnValue(makeStatus({ bannerVariant: variant }))

      const { unmount } = renderBanner()

      const shell = screen.getByTestId('banner-shell')
      expect(shell).toHaveAttribute('data-tone', 'info')
      expect(shell).not.toHaveAttribute('data-tone', 'warning')
      expect(shell).not.toHaveAttribute('data-tone', 'danger')

      unmount()
    }
  })

  it('does not contain urgency language in rendered text', () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({ bannerVariant: 'day85', daysSinceSignup: 85 }),
    )

    renderBanner()

    const bannerEl = screen.getByTestId('trial-banner')
    const text = bannerEl.textContent ?? ''

    expect(text).not.toMatch(/hurry/i)
    expect(text).not.toMatch(/don't lose/i)
    expect(text).not.toMatch(/last chance/i)
    expect(text).not.toMatch(/expire/i)
    expect(text).not.toMatch(/!/i)
  })

  it('does not contain emoji in rendered text', () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({ bannerVariant: 'day60', daysSinceSignup: 60 }),
    )

    renderBanner()

    const text = screen.getByTestId('trial-banner').textContent ?? ''
    expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })

  it('CTA label is "Add a payment method"', () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({ bannerVariant: 'day60' }),
    )

    renderBanner()

    expect(
      screen.getByRole('button', { name: 'Add a payment method' }),
    ).toBeInTheDocument()
  })

  it('clicking CTA invokes openPortal.mutate', async () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({ bannerVariant: 'day60' }),
    )

    renderBanner()

    await userEvent.click(
      screen.getByRole('button', { name: 'Add a payment method' }),
    )

    expect(mockMutate).toHaveBeenCalledOnce()
  })

  it('data-testid="trial-banner" is present when variant is not none', () => {
    mockUseTrialStatus.mockReturnValue(
      makeStatus({ bannerVariant: 'day75' }),
    )

    renderBanner()

    expect(screen.getByTestId('trial-banner')).toBeInTheDocument()
  })
})
