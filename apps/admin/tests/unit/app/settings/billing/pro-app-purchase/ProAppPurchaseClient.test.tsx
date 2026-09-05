/**
 * ProAppPurchaseClient unit tests — Vitest + Testing Library.
 *
 * All hooks are mocked at module level. No real React Query store needed.
 *
 * Tests:
 *   1. Non-Pro plan → renders "Upgrade to Pro" gate with plan-change link
 *   2. Pro plan + add-on active → renders "already active" state with credentials link
 *   3. Pro without add-on → renders purchase form
 *   4. Purchase form — acknowledgement checkbox enables submit button
 *   5. Purchase form — submit fires mutation with correct payload
 *   6. Purchase form — on success: opens invoice URL in new tab + redirects to credentials
 *   7. Purchase form — on error `already_active`: redirects to credentials without toast
 *   8. Purchase form — on generic error: shows error toast
 *   9. Loading state → renders skeleton
 *   10. Error state → renders error message
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// ---------------------------------------------------------------------------
// Mocks — declared before component imports
// ---------------------------------------------------------------------------

const mockPush = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
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

// Mutation factory: default isPending=false, no calls
const makeMutation = (overrides: Record<string, unknown> = {}) => ({
  mutate: vi.fn(),
  isPending: false,
  isError: false,
  isSuccess: false,
  data: undefined,
  error: null,
  ...overrides,
})

const mockUsePurchaseProApp = vi.fn(() => makeMutation())

vi.mock('@/lib/api/subscription/hooks/useProApp', () => ({
  usePurchaseProApp: (...args: unknown[]) => mockUsePurchaseProApp(...args),
}))

const mockUseCurrentPlan = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useBilling', () => ({
  useCurrentPlan: (...args: unknown[]) => mockUseCurrentPlan(...args),
}))

// Money component stub — renders amount/100 formatted simply.
// Everything else (SHARED_PRICING_CATALOGUE, getAddOnPrice) stays REAL. The
// component no longer reads a full-year price (#650 — the add-on does not
// recur), but the stub is kept unstubbed-around so that if a price is ever
// rendered here again it shows up in these assertions rather than being
// silently mocked away.
vi.mock('@repo/ui/subscription', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@repo/ui/subscription')>()),
  Money: ({ amount, currency }: { amount: number; currency: string }) => (
    <span>{currency.toUpperCase()} {(amount / 100).toFixed(2)}</span>
  ),
}))

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const STORE_ID = 'f47ac10b-58cc-4372-a567-0e02b2c3d479'

function makePlan(overrides: Record<string, unknown> = {}) {
  return {
    id: 'sub_1',
    storeId: STORE_ID,
    plan: 'pro',
    status: 'active',
    periodStart: '2026-04-01T00:00:00Z',
    periodEnd: '2027-04-01T00:00:00Z',
    cancelAtPeriodEnd: false,
    arbitrageFlag: false,
    billingCurrency: 'USD',
    addOns: [],
    createdAt: '2025-01-01T00:00:00Z',
    trialEndsAt: null,
    nextInvoiceAmountMinor: null,
    paymentMethodBrand: null,
    paymentMethodLast4: null,
    ...overrides,
  }
}

// ---------------------------------------------------------------------------
// Import component after mocks
// ---------------------------------------------------------------------------

// Dynamic import ensures mocks are in place before module resolution
import { ProAppPurchaseClient } from '@/app/(admin)/settings/billing/pro-app-purchase/ProAppPurchaseClient'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderComponent() {
  return render(<ProAppPurchaseClient storeId={STORE_ID} />)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('ProAppPurchaseClient', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUsePurchaseProApp.mockReturnValue(makeMutation())
    // Default: loading state
    mockUseCurrentPlan.mockReturnValue({ data: undefined, isLoading: true, isError: false })
  })

  it('loading state — renders skeleton with aria-busy', () => {
    renderComponent()
    // The skeleton div carries aria-busy="true" but is not a landmark role —
    // query it directly via the DOM attribute.
    const skeleton = document.querySelector('[aria-busy="true"]')
    expect(skeleton).toBeTruthy()
  })

  it('error state — renders error message', () => {
    mockUseCurrentPlan.mockReturnValue({ data: undefined, isLoading: false, isError: true })
    renderComponent()
    expect(screen.getByText(/something went wrong/i)).toBeInTheDocument()
  })

  it('non-Pro plan — renders Upgrade to Pro gate with plan-change link', () => {
    mockUseCurrentPlan.mockReturnValue({
      data: makePlan({ plan: 'growth' }),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    expect(screen.getByRole('heading', { name: /upgrade to pro/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /view plans/i })).toHaveAttribute(
      'href',
      '/settings/billing/plan-change',
    )
  })

  it('Pro plan + add-on active — renders already active state with credentials link', () => {
    mockUseCurrentPlan.mockReturnValue({
      data: makePlan({ addOns: ['white_label_app'] }),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    expect(screen.getByText(/white-label app is active/i)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /manage credentials/i }),
    ).toHaveAttribute('href', '/settings/billing/pro-app-purchase/credentials')
  })

  it('Pro without add-on — renders purchase form', () => {
    mockUseCurrentPlan.mockReturnValue({
      data: makePlan(),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    expect(screen.getByRole('form', { name: /add the white-label app/i })).toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /add to plan/i })).toBeInTheDocument()
  })

  // #650: the add-on is a ONE-TIME purchase. It is charged once — a prorated
  // year plus the $2,000 setup fee — and never re-billed: there is no Stripe
  // Price object for it, no subscription item, and nothing reads
  // has_white_label_app_add_on for billing after the webhook sets it.
  //
  // This page used to render "Next full-year charge: $2,388 on <date>",
  // promising a charge no code produces. These tests are the gate against
  // reinstating it without a recurring subscription item to back it.
  it('states the charge is one-time and does not renew', () => {
    mockUseCurrentPlan.mockReturnValue({
      data: makePlan(),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    expect(screen.getByText(/one-time charge/i)).toBeInTheDocument()
    expect(screen.getByText(/does not renew/i)).toBeInTheDocument()
  })

  it('promises no recurring charge, for any merchant currency', () => {
    for (const billingCurrency of ['USD', 'INR'] as const) {
      mockUseCurrentPlan.mockReturnValue({
        data: makePlan({ billingCurrency }),
        isLoading: false,
        isError: false,
      })
      const { unmount } = renderComponent()

      // No recurring-charge wording, and no amount rendered as one. The
      // proration preview still describes the up-front charge; what must not
      // appear is a future one.
      expect(screen.queryByText(/next full-year charge/i)).not.toBeInTheDocument()
      expect(screen.queryByText(/renews|renewal charge|per year|\/yr/i)).not.toBeInTheDocument()
      expect(screen.queryByText('USD 2388.00')).not.toBeInTheDocument()

      unmount()
    }
  })

  it('acknowledgement checkbox controls submit button state', async () => {
    mockUseCurrentPlan.mockReturnValue({
      data: makePlan(),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    const submitBtn = screen.getByRole('button', { name: /add to plan/i })
    // Button is enabled by default — validation only fires on submit
    // Tick checkbox then submit — expect mutate to be called
    const checkbox = screen.getByRole('checkbox')
    expect(checkbox).not.toBeChecked()

    await userEvent.click(checkbox)
    expect(checkbox).toBeChecked()

    await userEvent.click(submitBtn)
    // mutate should be called
    await waitFor(() => {
      expect(mockUsePurchaseProApp().mutate).toHaveBeenCalled()
    })
  })

  it('submit without ticking checkbox shows validation error', async () => {
    mockUseCurrentPlan.mockReturnValue({
      data: makePlan(),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    const submitBtn = screen.getByRole('button', { name: /add to plan/i })
    await userEvent.click(submitBtn)

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
  })

  it('on success — opens invoice URL in new tab and redirects to credentials', async () => {
    const windowOpenSpy = vi.spyOn(window, 'open').mockImplementation(() => null)

    const purchaseMutation = {
      ...makeMutation(),
      mutate: vi.fn((_vars: unknown, opts: { onSuccess?: (data: unknown) => void }) => {
        opts.onSuccess?.({
          hosted_invoice_url: 'https://invoice.stripe.com/i/test_123',
          stripe_invoice_id: 'in_test_123',
          amount_due_cents: 150000,
          currency: 'usd',
        })
      }),
    }
    mockUsePurchaseProApp.mockReturnValue(purchaseMutation)

    mockUseCurrentPlan.mockReturnValue({
      data: makePlan(),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    const checkbox = screen.getByRole('checkbox')
    await userEvent.click(checkbox)
    await userEvent.click(screen.getByRole('button', { name: /add to plan/i }))

    await waitFor(() => {
      expect(windowOpenSpy).toHaveBeenCalledWith(
        'https://invoice.stripe.com/i/test_123',
        '_blank',
        'noopener,noreferrer',
      )
      expect(mockToastSuccess).toHaveBeenCalledWith(
        expect.stringMatching(/welcome/i),
      )
      expect(mockPush).toHaveBeenCalledWith(
        '/settings/billing/pro-app-purchase/credentials',
      )
    })

    windowOpenSpy.mockRestore()
  })

  it('on error already_active — redirects to credentials without error toast', async () => {
    const { ApiError } = await import('@/lib/api/client')

    const purchaseMutation = {
      ...makeMutation(),
      mutate: vi.fn((_vars: unknown, opts: { onError?: (err: unknown) => void }) => {
        opts.onError?.(new ApiError('already_active', 409))
      }),
    }
    mockUsePurchaseProApp.mockReturnValue(purchaseMutation)

    mockUseCurrentPlan.mockReturnValue({
      data: makePlan(),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    const checkbox = screen.getByRole('checkbox')
    await userEvent.click(checkbox)
    await userEvent.click(screen.getByRole('button', { name: /add to plan/i }))

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith(
        '/settings/billing/pro-app-purchase/credentials',
      )
      expect(mockToastError).not.toHaveBeenCalled()
    })
  })

  it('on generic error — shows error toast', async () => {
    const purchaseMutation = {
      ...makeMutation(),
      mutate: vi.fn((_vars: unknown, opts: { onError?: (err: unknown) => void }) => {
        opts.onError?.(new Error('network failure'))
      }),
    }
    mockUsePurchaseProApp.mockReturnValue(purchaseMutation)

    mockUseCurrentPlan.mockReturnValue({
      data: makePlan(),
      isLoading: false,
      isError: false,
    })
    renderComponent()

    const checkbox = screen.getByRole('checkbox')
    await userEvent.click(checkbox)
    await userEvent.click(screen.getByRole('button', { name: /add to plan/i }))

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalled()
    })
  })
})
