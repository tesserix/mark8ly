/**
 * CancelWizard integration tests — Vitest + Testing Library.
 *
 * Hooks are mocked at the module level. Tests exercise the 4-step state
 * machine, step transitions, save-offer eligibility, and final submission.
 *
 * Tests:
 *   1. Starts on the confirm step with the correct heading.
 *   2. "Continue" on confirm advances to survey step.
 *   3. Step indicator advances when confirm → survey.
 *   4. Selecting too_expensive + submit → save-offer step (eligible reason).
 *   5. Selecting going_out_of_business + submit → skips save-offer, goes to final.
 *   6. Save-offer Accept → calls revert hook, toast success, router.push billing.
 *   7. Save-offer Decline → final step fires submit hook.
 *   8. "Keep my subscription" at confirm routes back to /settings/billing.
 *   9. No emoji in any rendered text.
 *  10. No bare "!" outside sentence context in rendered text.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'

// ---------------------------------------------------------------------------
// Mocks — declared before component imports
// ---------------------------------------------------------------------------

const mockPush = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
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

// useCurrentPlan — return a plan with a known periodEnd
vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useCurrentPlan: () => ({
    data: {
      id: 'sub_1',
      storeId: 'store_1',
      plan: 'starter',
      status: 'active',
      periodEnd: '2026-05-18T00:00:00Z',
      cancelAtPeriodEnd: false,
    },
    isLoading: false,
  }),
}))

// Controlled mutation mocks
let mockSubmitMutate = vi.fn()
let mockSubmitIsPending = false
let mockSubmitIsError = false
let mockSubmitData: Record<string, unknown> | undefined = undefined

let mockRevertMutate = vi.fn()
let mockRevertIsPending = false

vi.mock('@/lib/api/subscription/hooks/useCancellation', () => ({
  useSubmitCancellation: () => ({
    mutate: mockSubmitMutate,
    isPending: mockSubmitIsPending,
    isError: mockSubmitIsError,
    data: mockSubmitData,
  }),
  useRevertCancellation: () => ({
    mutate: mockRevertMutate,
    isPending: mockRevertIsPending,
  }),
}))

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import { CancelWizard } from '@/app/(admin)/settings/billing/cancel/CancelWizard'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderWizard(storeId = 'store_1') {
  return render(<CancelWizard storeId={storeId} />)
}

beforeEach(() => {
  mockPush.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  mockSubmitMutate = vi.fn()
  mockRevertMutate = vi.fn()
  mockSubmitIsPending = false
  mockSubmitIsError = false
  mockSubmitData = undefined
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('CancelWizard', () => {
  it('starts on confirm step with the correct heading', () => {
    renderWizard()
    expect(
      screen.getByRole('heading', { name: 'Cancel your subscription' }),
    ).toBeInTheDocument()
  })

  it('Continue on confirm advances to survey step', () => {
    renderWizard()
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(
      screen.getByRole('heading', { name: 'Before you go' }),
    ).toBeInTheDocument()
  })

  it('step indicator advances from confirm to survey (2nd dot is current)', () => {
    renderWizard()

    // Initially: first dot is current (ink-900 border, no fill)
    let progressbar = screen.getByRole('progressbar')
    expect(progressbar).toHaveAttribute('aria-valuenow', '1')

    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    progressbar = screen.getByRole('progressbar')
    expect(progressbar).toHaveAttribute('aria-valuenow', '2')
  })

  it('too_expensive + submit → save-offer step (eligible)', async () => {
    renderWizard()
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    // Select the "too expensive" reason
    const radio = screen.getByDisplayValue('too_expensive')
    fireEvent.click(radio)

    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: 'Before you cancel' }),
      ).toBeInTheDocument()
    })
  })

  it('going_out_of_business + submit → skips save-offer, goes to final', async () => {
    renderWizard()
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    const radio = screen.getByDisplayValue('going_out_of_business')
    fireEvent.click(radio)
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))

    await waitFor(() => {
      // FinalConfirmStep fires submit on mount; mock fires immediately.
      // With isPending=false and isError=false but no data, the loading skeleton
      // renders. The heading appears after mutate resolves — check the mutate was called.
      expect(mockSubmitMutate).toHaveBeenCalledWith(
        expect.objectContaining({ accept_save_offer: false }),
        expect.any(Object),
      )
    })

    // The save-offer heading must NOT be present.
    expect(
      screen.queryByRole('heading', { name: 'Before you cancel' }),
    ).not.toBeInTheDocument()
  })

  it('switching_to + submit → skips save-offer, goes to final', async () => {
    renderWizard()
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    const radio = screen.getByDisplayValue('switching_to')
    fireEvent.click(radio)
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))

    await waitFor(() => {
      expect(mockSubmitMutate).toHaveBeenCalled()
    })

    expect(
      screen.queryByRole('heading', { name: 'Before you cancel' }),
    ).not.toBeInTheDocument()
  })

  it('Save-offer Accept → calls revert mutate', async () => {
    renderWizard()
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    // Select too_expensive to reach save-offer
    fireEvent.click(screen.getByDisplayValue('too_expensive'))
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))

    await waitFor(() =>
      screen.getByRole('heading', { name: 'Before you cancel' }),
    )

    fireEvent.click(screen.getByRole('button', { name: 'Take this offer' }))
    expect(mockRevertMutate).toHaveBeenCalledWith(
      undefined,
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    )
  })

  it('Save-offer Accept onSuccess → toast success and router push to billing', async () => {
    mockRevertMutate = vi.fn((_, { onSuccess }) => onSuccess?.())

    renderWizard()
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    fireEvent.click(screen.getByDisplayValue('too_expensive'))
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))

    await waitFor(() =>
      screen.getByRole('heading', { name: 'Before you cancel' }),
    )

    fireEvent.click(screen.getByRole('button', { name: 'Take this offer' }))

    expect(mockToastSuccess).toHaveBeenCalledWith('Subscription restored.')
    expect(mockPush).toHaveBeenCalledWith('/settings/billing')
  })

  it('Save-offer Decline → final step fires submit hook', async () => {
    renderWizard()
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    fireEvent.click(screen.getByDisplayValue('too_expensive'))
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }))

    await waitFor(() =>
      screen.getByRole('heading', { name: 'Before you cancel' }),
    )

    fireEvent.click(screen.getByRole('button', { name: 'No thanks, cancel my plan' }))

    await waitFor(() => {
      expect(mockSubmitMutate).toHaveBeenCalledWith(
        expect.objectContaining({ accept_save_offer: false }),
        expect.any(Object),
      )
    })
  })

  it('Keep my subscription at confirm routes back to /settings/billing', () => {
    renderWizard()
    fireEvent.click(screen.getByRole('button', { name: 'Keep my subscription' }))
    expect(mockPush).toHaveBeenCalledWith('/settings/billing')
  })

  it('no emoji in any rendered text', () => {
    renderWizard()
    const text = document.body.textContent ?? ''
    // Unicode emoji range: \u{1F000}–\u{1FFFF} and common emoticons
    expect(text).not.toMatch(
      /[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}]/u,
    )
  })

  it('no standalone exclamation mark as a sentence hook in rendered text', () => {
    renderWizard()
    // Advance through all accessible steps
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    const text = document.body.textContent ?? ''
    // A bare "!" at the end of a word/sentence is a guilt-trip tell.
    // Allowed: "You'll" contractions contain no "!". Check for terminal "!".
    expect(text).not.toMatch(/\w!(\s|$)/g)
  })
})
