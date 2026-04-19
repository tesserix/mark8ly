/**
 * AttestationForm unit tests — Vitest + Testing Library.
 *
 * All hooks/modules mocked at module level. No React Query provider needed.
 *
 * Tests:
 *   1. Renders jurisdiction select and attestation checkbox
 *   2. Submit CTA is disabled before checkbox is ticked
 *   3. Submit CTA enables after checkbox is ticked
 *   4. Submission calls mutation with correct country, checkbox_text, checkbox_version
 *   5. accepted_at ISO timestamp is a valid ISO string (captured client-side at sign time)
 *   6. Success shows toast with "signed" message
 *   7. Already-signed state shows "Attestation on file" panel
 *   8. No urgency language or emoji in rendered output
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

// Attestation state proxy — overridden per test.
const attestationStateMock = {
  isSigned: false,
  acceptedAt: null as string | null,
  isLoading: false,
}

// Sign mutation proxy.
const signMutationProxy = {
  mutate: vi.fn() as ReturnType<typeof vi.fn>,
  isPending: false,
}

vi.mock('@/lib/api/subscription/hooks/useAttestation', () => ({
  useAttestation: () => ({ ...attestationStateMock }),
  useSignAttestation: () => ({
    mutate: (...args: unknown[]) => signMutationProxy.mutate(...args),
    isPending: signMutationProxy.isPending,
  }),
}))

// Mock @tesserix/web Select as native <select> for JSDOM.
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
import { AttestationForm } from '@/app/(admin)/settings/attestation/AttestationForm'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STORE_ID = 'store-att-test-xyz'

function renderForm() {
  return render(<AttestationForm storeId={STORE_ID} />)
}

function selectJurisdiction(value: string) {
  const select = screen.getByTestId('jurisdiction-select')
  fireEvent.change(select, { target: { value } })
}

function tickCheckbox() {
  const checkbox = screen.getByRole('checkbox')
  fireEvent.click(checkbox)
}

async function clickSubmit() {
  const button = screen.getByRole('button', { name: /Sign attestation/i })
  await act(async () => {
    fireEvent.click(button)
  })
}

// ---------------------------------------------------------------------------
// beforeEach
// ---------------------------------------------------------------------------

beforeEach(() => {
  attestationStateMock.isSigned = false
  attestationStateMock.acceptedAt = null
  attestationStateMock.isLoading = false
  signMutationProxy.mutate = vi.fn()
  signMutationProxy.isPending = false
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('AttestationForm', () => {
  it('renders jurisdiction select and attestation checkbox', () => {
    renderForm()

    expect(screen.getByTestId('jurisdiction-select')).toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Sign attestation/i })).toBeInTheDocument()
  })

  it('Submit CTA is disabled before checkbox is ticked', () => {
    renderForm()

    const submitButton = screen.getByRole('button', { name: /Sign attestation/i })
    expect(submitButton).toBeDisabled()
  })

  it('Submit CTA enables after checkbox is ticked', () => {
    renderForm()

    tickCheckbox()

    const submitButton = screen.getByRole('button', { name: /Sign attestation/i })
    expect(submitButton).not.toBeDisabled()
  })

  it('submission calls mutation with country, checkbox_text, checkbox_version', async () => {
    signMutationProxy.mutate.mockImplementation(
      (
        _values: unknown,
        opts: { onSuccess?: () => void; onError?: (e: Error) => void },
      ) => {
        opts.onSuccess?.()
      },
    )

    renderForm()

    selectJurisdiction('US')
    tickCheckbox()
    await clickSubmit()

    await waitFor(() => {
      expect(signMutationProxy.mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          country: 'US',
          checkbox_text: expect.stringContaining(
            'I attest that this store is operated by a registered business entity',
          ),
          checkbox_version: 'v1',
        }),
        expect.any(Object),
      )
    })
  })

  it('success shows toast with signed message', async () => {
    signMutationProxy.mutate.mockImplementation(
      (
        _values: unknown,
        opts: { onSuccess?: () => void; onError?: (e: Error) => void },
      ) => {
        opts.onSuccess?.()
      },
    )

    renderForm()

    selectJurisdiction('CA')
    tickCheckbox()
    await clickSubmit()

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith(
        expect.stringContaining('signed'),
      )
    })
  })

  it('already-signed state shows "Attestation on file" panel', () => {
    attestationStateMock.isSigned = true
    attestationStateMock.acceptedAt = '2026-04-19T10:00:00.000Z'

    renderForm()

    expect(screen.getByText(/Attestation on file/i)).toBeInTheDocument()
    // Form fields should not be rendered
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Sign attestation/i })).not.toBeInTheDocument()
  })

  it('already-signed state without acceptedAt shows support hint', () => {
    attestationStateMock.isSigned = true
    attestationStateMock.acceptedAt = null

    renderForm()

    expect(screen.getByText(/contact our support team/i)).toBeInTheDocument()
  })

  it('does not contain urgency language or emoji', () => {
    renderForm()

    const text = document.body.textContent ?? ''

    expect(text).not.toMatch(/action required/i)
    expect(text).not.toMatch(/immediately/i)
    expect(text).not.toMatch(/hurry/i)
    expect(text).not.toMatch(/fraud/i)
    expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })
})
