/**
 * ReadOnlyBanner unit tests.
 *
 * The states covered here — expired, store_closed, pending_hard_delete —
 * were the only billing states with NO banner. A merchant whose trial
 * lapsed got no explanation anywhere in the product: writes failed and
 * (until marketplace-api#919) reads failed too, surfacing as a generic
 * "Something went wrong". These tests exist to keep that from returning.
 *
 * Strategy matches the sibling banner tests: mock useBilling at module
 * level so the component renders without a QueryProvider or MSW.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  ReadOnlyBanner,
  isReadOnlyStatus,
} from '@/components/shell/banners/ReadOnlyBanner'

const mockUseCurrentPlan = vi.fn()
const mockMutate = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useCurrentPlan: (...args: unknown[]) => mockUseCurrentPlan(...args),
  useOpenPortal: vi.fn(() => ({ mutate: mockMutate, isPending: false })),
}))

const STORE_ID = 'store-abc'

function withStatus(status: string | undefined) {
  mockUseCurrentPlan.mockReturnValue({ data: status ? { status } : null })
  return render(<ReadOnlyBanner storeId={STORE_ID} />)
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('ReadOnlyBanner', () => {
  it('renders nothing for states that are not read-only', () => {
    for (const status of [
      'active',
      'trialing',
      'past_due',
      'payment_action_required',
      'signup',
      'cancel_scheduled',
      undefined,
    ]) {
      const { container, unmount } = withStatus(status)
      expect(
        container.querySelector('[data-testid="read-only-banner"]'),
        `status ${String(status)} must not show the read-only banner`,
      ).toBeNull()
      unmount()
    }
  })

  it('explains an expired trial rather than showing a generic error', () => {
    withStatus('expired')
    expect(screen.getByText('Your trial has ended')).toBeInTheDocument()
    // The merchant keeps read access — say so, since it is the thing they
    // will wonder about first.
    expect(screen.getByText(/still view your orders/i)).toBeInTheDocument()
  })

  it('distinguishes store_closed from expired', () => {
    withStatus('store_closed')
    expect(screen.getByText('Your store is closed')).toBeInTheDocument()
    expect(screen.getByText(/data is intact/i)).toBeInTheDocument()
  })

  it('warns about deletion for pending_hard_delete', () => {
    withStatus('pending_hard_delete')
    expect(
      screen.getByText('Your store is scheduled for deletion'),
    ).toBeInTheDocument()
    expect(screen.getByText(/export your data/i)).toBeInTheDocument()
  })

  it('offers a way back to paying, and it works', async () => {
    const user = userEvent.setup()
    withStatus('store_closed')
    await user.click(screen.getByRole('button', { name: 'Add a card' }))
    expect(mockMutate).toHaveBeenCalledTimes(1)
  })

  it('escalates tone for the states that stopped the shop, not just billing', () => {
    // expired: the trial lapsed but the shop is recoverable and nothing is
    // lost — same weight as a failed payment.
    const expired = withStatus('expired')
    expect(
      expired.container
        .querySelector('[data-testid="banner-shell"]')
        ?.getAttribute('data-tone'),
    ).toBe('warning')
    expired.unmount()

    // store_closed and pending_hard_delete are different in kind: the
    // storefront has stopped serving customers, and the data is on a
    // deletion clock.
    for (const status of ['store_closed', 'pending_hard_delete']) {
      const { container, unmount } = withStatus(status)
      expect(
        container
          .querySelector('[data-testid="banner-shell"]')
          ?.getAttribute('data-tone'),
        `${status} should use the danger tone`,
      ).toBe('danger')
      unmount()
    }
  })

  it('tags the rendered banner with its status for debugging', () => {
    const { container } = withStatus('expired')
    expect(
      container
        .querySelector('[data-testid="read-only-banner"]')
        ?.getAttribute('data-status'),
    ).toBe('expired')
  })
})

describe('isReadOnlyStatus', () => {
  // Mirrors readonly.RequireActive in marketplace-api. If a state is added
  // server-side and not here, the banner silently stops covering it — which
  // is the failure this whole component exists to fix.
  it('matches exactly the three states the server treats as read-only', () => {
    expect(isReadOnlyStatus('expired')).toBe(true)
    expect(isReadOnlyStatus('store_closed')).toBe(true)
    expect(isReadOnlyStatus('pending_hard_delete')).toBe(true)

    expect(isReadOnlyStatus('active')).toBe(false)
    expect(isReadOnlyStatus('past_due')).toBe(false)
    expect(isReadOnlyStatus(undefined)).toBe(false)
  })
})
