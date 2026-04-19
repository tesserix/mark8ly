/**
 * InvoicesList unit tests.
 *
 * Tests:
 *  - Renders the heading and portal CTA description
 *  - Clicking "Open billing portal" triggers the useOpenPortal mutation
 *  - Button is disabled + shows loading label while mutation is pending
 *
 * Uses a manual mock of the useBilling hook to avoid React Query setup
 * complexity in unit tests. The API-level integration test covers the
 * actual mutation flow.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { InvoicesList } from '@/app/(admin)/settings/billing/InvoicesList'

// ---------------------------------------------------------------------------
// Mock the hook module — each test resets the mock
// ---------------------------------------------------------------------------

const mockMutate = vi.fn()
const mockUseOpenPortal = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useOpenPortal: () => mockUseOpenPortal(),
}))

// Mock the in-house Toaster so useToast() doesn't need a Provider
vi.mock('@/components/feedback/Toaster', () => ({
  useToast: () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn(), dismiss: vi.fn() } }),
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function idlePortal() {
  return { mutate: mockMutate, isPending: false }
}

function pendingPortal() {
  return { mutate: mockMutate, isPending: true }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('InvoicesList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseOpenPortal.mockReturnValue(idlePortal())
  })

  it('renders the section heading', () => {
    render(<InvoicesList storeId="store_1" />)
    expect(
      screen.getByRole('heading', { name: /Invoices and receipts/i }),
    ).toBeInTheDocument()
  })

  it('renders the portal description paragraph', () => {
    render(<InvoicesList storeId="store_1" />)
    expect(
      screen.getByText(/Open the Stripe billing portal/i),
    ).toBeInTheDocument()
  })

  it('renders the "Open billing portal" CTA button', () => {
    render(<InvoicesList storeId="store_1" />)
    expect(
      screen.getByRole('button', { name: /Open billing portal/i }),
    ).toBeInTheDocument()
  })

  it('clicking the portal button triggers the mutation', () => {
    render(<InvoicesList storeId="store_1" />)
    fireEvent.click(screen.getByRole('button', { name: /Open billing portal/i }))
    expect(mockMutate).toHaveBeenCalledOnce()
  })

  it('button is disabled while mutation is pending', () => {
    mockUseOpenPortal.mockReturnValue(pendingPortal())
    render(<InvoicesList storeId="store_1" />)
    const btn = screen.getByRole('button')
    expect(btn).toBeDisabled()
  })

  it('button shows loading label while pending', () => {
    mockUseOpenPortal.mockReturnValue(pendingPortal())
    render(<InvoicesList storeId="store_1" />)
    expect(
      screen.getByRole('button', { name: /Opening portal/i }),
    ).toBeInTheDocument()
  })
})
