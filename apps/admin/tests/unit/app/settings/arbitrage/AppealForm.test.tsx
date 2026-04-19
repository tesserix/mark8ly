/**
 * AppealForm unit tests — Vitest + Testing Library.
 *
 * All hooks/modules mocked at module level. No React Query provider needed.
 *
 * Tests:
 *   1. Renders jurisdiction select, justification textarea, document_url input, submit button.
 *   2. Empty jurisdiction → inline validation error on submit.
 *   3. Justification over 2000 chars → inline validation error.
 *   4. Submit with valid data → calls mutation, shows success toast.
 *   5. 404 arbitrage_appeal_no_open_flag → inline form error message.
 *   6. No urgency language or emoji in rendered output.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'

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
      info: vi.fn(),
      dismiss: vi.fn(),
    },
  }),
}))

// Stable proxy object so the mock factory always references the current .mutate.
// Reassigning appealMutationState.mutate in beforeEach is visible inside the
// factory without re-hoisting.
const appealMutationState = {
  mutate: vi.fn() as ReturnType<typeof vi.fn>,
  isPending: false,
}

vi.mock('@/lib/api/subscription/hooks/useArbitrage', () => ({
  useSubmitArbitrageAppeal: () => ({
    mutate: (...args: unknown[]) => appealMutationState.mutate(...args),
    isPending: appealMutationState.isPending,
  }),
}))

// @tesserix/web Select — render as a native <select> so JSDOM can interact.
vi.mock('@tesserix/web', () => ({
  Select: ({
    value,
    onValueChange,
    children,
  }: {
    value: string
    onValueChange: (v: string) => void
    children: React.ReactNode
  }) => (
    <select
      data-testid="jurisdiction-select"
      value={value}
      onChange={(e) => onValueChange(e.target.value)}
    >
      {children}
    </select>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  SelectValue: ({ placeholder }: { placeholder?: string }) => (
    <option value="" disabled>
      {placeholder ?? 'Select'}
    </option>
  ),
  SelectContent: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  SelectItem: ({
    value,
    children,
  }: {
    value: string
    children: React.ReactNode
  }) => <option value={value}>{children}</option>,
}))

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import React from 'react'
import { AppealForm } from '@/app/(admin)/settings/arbitrage/AppealForm'
import { ApiError } from '@/lib/api/client'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STORE_ID = 'store-test-abc'

function renderForm() {
  return render(<AppealForm storeId={STORE_ID} />)
}

function selectJurisdiction(value: string) {
  const select = screen.getByTestId('jurisdiction-select')
  fireEvent.change(select, { target: { value } })
}

function fillJustification(text: string) {
  const textarea = screen.getByRole('textbox', { name: /Additional context/i })
  fireEvent.change(textarea, { target: { value: text } })
}

async function submitForm() {
  const button = screen.getByRole('button', { name: /Submit appeal/i })
  await act(async () => {
    fireEvent.click(button)
  })
}

// ---------------------------------------------------------------------------
// beforeEach
// ---------------------------------------------------------------------------

beforeEach(() => {
  appealMutationState.mutate = vi.fn()
  appealMutationState.isPending = false
  mockPush.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('AppealForm', () => {
  it('renders all required fields', () => {
    renderForm()

    expect(screen.getByTestId('jurisdiction-select')).toBeInTheDocument()

    expect(
      screen.getByRole('textbox', { name: /Additional context/i }),
    ).toBeInTheDocument()

    expect(screen.getByLabelText(/Supporting document/i)).toBeInTheDocument()

    expect(
      screen.getByRole('button', { name: /Submit appeal/i }),
    ).toBeInTheDocument()
  })

  it('empty jurisdiction shows inline validation error on submit', async () => {
    renderForm()

    // Leave jurisdiction empty and submit.
    await submitForm()

    await waitFor(() => {
      // Multiple field-level alerts may appear; verify the jurisdiction one is present.
      const alerts = screen.getAllByRole('alert')
      const texts = alerts.map((el) => el.textContent ?? '')
      expect(texts.some((t) => /2-letter country code/i.test(t))).toBe(true)
    })
  })

  it('justification over 2000 chars shows inline validation error', async () => {
    renderForm()

    selectJurisdiction('GB')
    fillJustification('a'.repeat(2001))

    await submitForm()

    await waitFor(() => {
      expect(screen.getByText(/cannot exceed 2000/i)).toBeInTheDocument()
    })
  })

  it('submit with valid data calls mutation and shows success toast', async () => {
    appealMutationState.mutate.mockImplementation(
      (
        _values: unknown,
        opts: { onSuccess?: () => void; onError?: (e: Error) => void },
      ) => {
        opts.onSuccess?.()
      },
    )

    renderForm()

    selectJurisdiction('GB')
    fillJustification('UK-registered company, EU customer base.')

    await submitForm()

    await waitFor(() => {
      expect(appealMutationState.mutate).toHaveBeenCalledWith(
        expect.objectContaining({ jurisdiction: 'GB' }),
        expect.any(Object),
      )
    })

    expect(mockToastSuccess).toHaveBeenCalledWith(
      expect.stringContaining('5 business days'),
    )
    expect(mockPush).toHaveBeenCalledWith('/admin/settings/arbitrage')
  })

  it('404 no_open_flag response shows inline form error', async () => {
    appealMutationState.mutate.mockImplementation(
      (
        _values: unknown,
        opts: { onSuccess?: () => void; onError?: (e: Error) => void },
      ) => {
        opts.onError?.(
          new ApiError(
            'arbitrage_appeal_no_open_flag',
            404,
            'no open arbitrage flag',
          ),
        )
      },
    )

    renderForm()

    selectJurisdiction('US')

    await submitForm()

    await waitFor(() => {
      expect(
        screen.getByText(/this store may already be clear/i),
      ).toBeInTheDocument()
    })
  })

  it('does not contain urgency language or emoji', () => {
    renderForm()

    const formText = document.body.textContent ?? ''

    expect(formText).not.toMatch(/fraud/i)
    expect(formText).not.toMatch(/verify now/i)
    expect(formText).not.toMatch(/immediately/i)
    expect(formText).not.toMatch(/hurry/i)
    expect(formText).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })
})
