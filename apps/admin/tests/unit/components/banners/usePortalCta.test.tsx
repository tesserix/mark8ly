/**
 * usePortalCta unit tests.
 *
 * The bug: every banner CTA called .mutate() with no onError, and the
 * underlying mutations only act on SUCCESS (useOpenPortal redirects,
 * useCompleteActionUrl opens a tab). A failed call did nothing at all — no
 * redirect, no message — so "Add a card" looked like a dead button while
 * production was returning 500 to every one of those clicks.
 *
 * The billing PAGE surfaced that same 500 as "internal server error" because
 * its call sites passed onError. One broken call, two different stories,
 * depending on where you clicked.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { usePortalCta } from '@/components/shell/banners/usePortalCta'

const mockMutate = vi.fn()
const mockError = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useOpenPortal: () => ({ mutate: mockMutate, isPending: false }),
}))

vi.mock('@/components/feedback/Toaster', () => ({
  useToast: () => ({
    toast: { error: mockError, success: vi.fn(), info: vi.fn(), warning: vi.fn(), dismiss: vi.fn() },
  }),
}))

function Probe() {
  const portal = usePortalCta('store-1')
  return <button onClick={portal.open}>Add a card</button>
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('usePortalCta', () => {
  it('passes an onError, so a failure cannot be silent', async () => {
    const user = userEvent.setup()
    render(<Probe />)

    await user.click(screen.getByRole('button', { name: 'Add a card' }))

    expect(mockMutate).toHaveBeenCalledTimes(1)
    const [, opts] = mockMutate.mock.calls[0]
    expect(
      opts?.onError,
      'no onError means a failed portal call does nothing visible at all',
    ).toBeTypeOf('function')
  })

  it('reports the underlying error to the merchant', async () => {
    const user = userEvent.setup()
    // Drive the mutation straight down its failure path.
    mockMutate.mockImplementation((_v, opts) => {
      opts.onError(new Error('stripe portal session: 500'))
    })
    render(<Probe />)

    await user.click(screen.getByRole('button', { name: 'Add a card' }))

    expect(mockError).toHaveBeenCalledTimes(1)
    const [title, detail] = mockError.mock.calls[0]
    expect(title).toBe("Couldn't open billing")
    // The detail carries the real cause: "something went wrong" on a billing
    // control is indistinguishable from the button being broken.
    expect(detail).toContain('stripe portal session')
  })

  it('does not toast when the call succeeds', async () => {
    const user = userEvent.setup()
    mockMutate.mockImplementation(() => {})
    render(<Probe />)

    await user.click(screen.getByRole('button', { name: 'Add a card' }))

    expect(mockError).not.toHaveBeenCalled()
  })
})
