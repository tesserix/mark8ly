/**
 * TaxIdForm unit tests — Vitest + Testing Library.
 *
 * All hooks/modules mocked at module level. No React Query provider needed.
 *
 * Tests:
 *   1. Renders all required fields (business_name, country, tax_id, submit CTA)
 *   2. Empty business_name → inline validation error on submit
 *   3. Empty tax_id → inline validation error on submit
 *   4. Valid submission calls mutation with correct fields
 *   5. 400 invalid_format → inline error on tax_id field
 *   6. ClockPauseBadge rendered when status === 'clock_paused' + registry_unavailable
 *   7. ClockPauseBadge rendered when status === 'clock_paused' + sea_review
 *   8. Validated state shows "Update tax ID" CTA and hides form by default
 *   9. No urgency language or emoji in rendered output
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'

// ---------------------------------------------------------------------------
// Mocks — declared before component imports
// ---------------------------------------------------------------------------

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

// Stable proxy for tax ID state so tests can override per-case.
const taxIdStateMock = {
  status: 'not_submitted' as
    | 'not_submitted'
    | 'pending'
    | 'validated'
    | 'clock_paused'
    | 'rejected',
  pauseReason: null as 'registry_unavailable' | 'sea_review' | null,
  validationCode: null as string | null,
  reviewedAt: null as string | null,
  isLoading: false,
}

vi.mock('@/lib/api/subscription/hooks/useTaxId', () => ({
  useTaxIdStatus: () => ({ ...taxIdStateMock }),
  useSubmitTaxId: () => ({
    mutate: (...args: unknown[]) => submitMutationProxy.mutate(...args),
    isPending: submitMutationProxy.isPending,
  }),
}))

// Stable proxy for the submit mutation.
const submitMutationProxy = {
  mutate: vi.fn() as ReturnType<typeof vi.fn>,
  isPending: false,
}

// Mock @tesserix/web Select as a native <select> for JSDOM interaction.
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
      data-testid="country-select"
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
import { TaxIdForm } from '@/app/(admin)/settings/tax-id/TaxIdForm'
import { ApiError } from '@/lib/api/client'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STORE_ID = 'store-tax-test-abc'

function renderForm() {
  return render(<TaxIdForm storeId={STORE_ID} />)
}

function fillBusinessName(value: string) {
  const input = screen.getByLabelText(/Legal business name/i)
  fireEvent.change(input, { target: { value } })
}

function fillTaxId(value: string) {
  const input = screen.getByLabelText(/Tax ID/i)
  fireEvent.change(input, { target: { value } })
}

function selectCountry(value: string) {
  const select = screen.getByTestId('country-select')
  fireEvent.change(select, { target: { value } })
}

async function submitForm() {
  const button = screen.getByRole('button', { name: /Submit for verification/i })
  await act(async () => {
    fireEvent.click(button)
  })
}

// ---------------------------------------------------------------------------
// beforeEach
// ---------------------------------------------------------------------------

beforeEach(() => {
  taxIdStateMock.status = 'not_submitted'
  taxIdStateMock.pauseReason = null
  taxIdStateMock.validationCode = null
  taxIdStateMock.isLoading = false
  submitMutationProxy.mutate = vi.fn()
  submitMutationProxy.isPending = false
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('TaxIdForm', () => {
  it('renders all required fields', () => {
    renderForm()

    expect(screen.getByLabelText(/Legal business name/i)).toBeInTheDocument()
    expect(screen.getByTestId('country-select')).toBeInTheDocument()
    expect(screen.getByLabelText(/Tax ID/i)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Submit for verification/i }),
    ).toBeInTheDocument()
  })

  it('empty business_name shows inline validation error on submit', async () => {
    renderForm()

    selectCountry('GB')
    fillTaxId('GB123456789')
    await submitForm()

    await waitFor(() => {
      const alerts = screen.getAllByRole('alert')
      const texts = alerts.map((el) => el.textContent ?? '')
      expect(texts.some((t) => /business name/i.test(t))).toBe(true)
    })
  })

  it('empty tax_id shows inline validation error on submit', async () => {
    renderForm()

    fillBusinessName('Acme Ltd')
    selectCountry('GB')
    await submitForm()

    await waitFor(() => {
      const alerts = screen.getAllByRole('alert')
      const texts = alerts.map((el) => el.textContent ?? '')
      expect(texts.some((t) => /tax id is required/i.test(t))).toBe(true)
    })
  })

  it('valid submission calls mutation with correct fields', async () => {
    submitMutationProxy.mutate.mockImplementation(
      (
        _values: unknown,
        opts: { onSuccess?: () => void; onError?: (e: Error) => void },
      ) => {
        opts.onSuccess?.()
      },
    )

    renderForm()

    fillBusinessName('Acme Ltd')
    selectCountry('GB')
    fillTaxId('GB123456789')
    await submitForm()

    await waitFor(() => {
      expect(submitMutationProxy.mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          business_name: 'Acme Ltd',
          country: 'GB',
          tax_id: 'GB123456789',
        }),
        expect.any(Object),
      )
    })

    expect(mockToastSuccess).toHaveBeenCalledWith(
      expect.stringContaining('submitted for verification'),
    )
  })

  it('400 invalid_format shows inline error on tax_id field', async () => {
    submitMutationProxy.mutate.mockImplementation(
      (
        _values: unknown,
        opts: { onSuccess?: () => void; onError?: (e: Error) => void },
      ) => {
        opts.onError?.(new ApiError('invalid_format', 400, 'bad format'))
      },
    )

    renderForm()

    fillBusinessName('Acme Ltd')
    selectCountry('GB')
    fillTaxId('BADFORMAT')
    await submitForm()

    await waitFor(() => {
      const alerts = screen.getAllByRole('alert')
      const texts = alerts.map((el) => el.textContent ?? '')
      expect(texts.some((t) => /format/i.test(t))).toBe(true)
    })
  })

  it('renders ClockPauseBadge with registry_unavailable when status is clock_paused', () => {
    taxIdStateMock.status = 'clock_paused'
    taxIdStateMock.pauseReason = 'registry_unavailable'

    renderForm()

    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText('Clock paused: registry')).toBeInTheDocument()
  })

  it('renders ClockPauseBadge with sea_review when status is clock_paused', () => {
    taxIdStateMock.status = 'clock_paused'
    taxIdStateMock.pauseReason = 'sea_review'

    renderForm()

    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText('Clock paused: review queue')).toBeInTheDocument()
  })

  it('validated state hides form and shows Update CTA', () => {
    taxIdStateMock.status = 'validated'

    renderForm()

    // The "Update tax ID" button should be visible
    expect(
      screen.getByRole('button', { name: /Update tax ID/i }),
    ).toBeInTheDocument()

    // The submit form button should NOT be visible by default
    expect(
      screen.queryByRole('button', { name: /Submit for verification/i }),
    ).not.toBeInTheDocument()
  })

  it('does not contain urgency language or emoji', () => {
    renderForm()

    const text = document.body.textContent ?? ''

    expect(text).not.toMatch(/action required/i)
    expect(text).not.toMatch(/verify now/i)
    expect(text).not.toMatch(/immediately/i)
    expect(text).not.toMatch(/hurry/i)
    expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })
})
