/**
 * PaymentMethodCard unit tests.
 *
 * Tests:
 *  - Shows masked last4 when payment method is present
 *  - Shows brand label in the card line
 *  - Shows "Update card" CTA when card is on file
 *  - Shows "No payment method on file" when no card data
 *  - Shows "Add payment method" CTA when no card data
 *  - Portal CTA is disabled + shows loading label while pending
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { PaymentMethodCard } from '@/app/(admin)/settings/billing/PaymentMethodCard'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'

// ---------------------------------------------------------------------------
// Mock the hook module
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
// Fixtures
// ---------------------------------------------------------------------------

function makePlan(overrides: Partial<CurrentPlan> = {}): CurrentPlan {
  return {
    id: 'sub_1',
    storeId: 'store_1',
    plan: 'growth',
    status: 'active',
    periodStart: '2026-04-01T00:00:00Z',
    periodEnd: '2027-04-01T00:00:00Z',
    cancelAtPeriodEnd: false,
    arbitrageFlag: false,
    billingCurrency: 'USD',
    addOns: [],
    trialEndsAt: null,
    nextInvoiceAmountMinor: null,
    paymentMethodBrand: null,
    paymentMethodLast4: null,
    ...overrides,
  }
}

function idlePortal() {
  return { mutate: mockMutate, isPending: false }
}

function pendingPortal() {
  return { mutate: mockMutate, isPending: true }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PaymentMethodCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseOpenPortal.mockReturnValue(idlePortal())
  })

  describe('when a card is on file', () => {
    it('renders the masked last4', () => {
      render(
        <PaymentMethodCard
          plan={makePlan({ paymentMethodBrand: 'visa', paymentMethodLast4: '4242' })}
          storeId="store_1"
        />,
      )
      // The copy formats as "Visa ending in •••• 4242"
      expect(screen.getByText(/\u2022\u2022\u2022\u2022 4242/)).toBeInTheDocument()
    })

    it('renders the card brand capitalised', () => {
      render(
        <PaymentMethodCard
          plan={makePlan({ paymentMethodBrand: 'mastercard', paymentMethodLast4: '9999' })}
          storeId="store_1"
        />,
      )
      expect(screen.getByText(/Mastercard/i)).toBeInTheDocument()
    })

    it('renders the "Update card" CTA', () => {
      render(
        <PaymentMethodCard
          plan={makePlan({ paymentMethodBrand: 'visa', paymentMethodLast4: '4242' })}
          storeId="store_1"
        />,
      )
      expect(
        screen.getByRole('button', { name: /Update card/i }),
      ).toBeInTheDocument()
    })

    it('renders the renewal date line', () => {
      render(
        <PaymentMethodCard
          plan={makePlan({
            paymentMethodBrand: 'visa',
            paymentMethodLast4: '4242',
            periodEnd: '2027-04-18T00:00:00Z',
          })}
          storeId="store_1"
        />,
      )
      expect(screen.getByText(/Renewed automatically on/i)).toBeInTheDocument()
      expect(screen.getByText(/18 April 2027/i)).toBeInTheDocument()
    })
  })

  describe('when no payment method is on file', () => {
    it('renders the "No payment method on file" message', () => {
      render(<PaymentMethodCard plan={makePlan()} storeId="store_1" />)
      expect(
        screen.getByText(/No payment method on file/i),
      ).toBeInTheDocument()
    })

    it('renders the "Add payment method" CTA', () => {
      render(<PaymentMethodCard plan={makePlan()} storeId="store_1" />)
      expect(
        screen.getByRole('button', { name: /Add payment method/i }),
      ).toBeInTheDocument()
    })
  })

  describe('portal button interactions', () => {
    it('clicking the CTA triggers the mutation', () => {
      render(
        <PaymentMethodCard
          plan={makePlan({ paymentMethodBrand: 'visa', paymentMethodLast4: '4242' })}
          storeId="store_1"
        />,
      )
      fireEvent.click(screen.getByRole('button', { name: /Update card/i }))
      expect(mockMutate).toHaveBeenCalledOnce()
    })

    it('button is disabled while mutation is pending', () => {
      mockUseOpenPortal.mockReturnValue(pendingPortal())
      render(
        <PaymentMethodCard
          plan={makePlan({ paymentMethodBrand: 'visa', paymentMethodLast4: '4242' })}
          storeId="store_1"
        />,
      )
      const btn = screen.getByRole('button')
      expect(btn).toBeDisabled()
    })

    it('shows loading label while pending', () => {
      mockUseOpenPortal.mockReturnValue(pendingPortal())
      render(<PaymentMethodCard plan={makePlan()} storeId="store_1" />)
      expect(
        screen.getByRole('button', { name: /Opening portal/i }),
      ).toBeInTheDocument()
    })
  })

  it('renders the section heading', () => {
    render(<PaymentMethodCard plan={makePlan()} storeId="store_1" />)
    expect(
      screen.getByRole('heading', { name: /Payment method/i }),
    ).toBeInTheDocument()
  })
})
