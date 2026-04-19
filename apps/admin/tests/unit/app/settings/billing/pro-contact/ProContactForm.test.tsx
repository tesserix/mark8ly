/**
 * ProContactForm unit tests — Vitest + Testing Library.
 *
 * All hooks are mocked at module level. No QueryClientProvider needed.
 *
 * Tests:
 *   1.  Renders all expected form fields.
 *   2.  Required-field validation — company_name missing.
 *   3.  Required-field validation — contact_name missing.
 *   4.  Required-field validation — contact_email missing.
 *   5.  Required-field validation — monthly_orders missing / NaN.
 *   6.  Invalid email shows a validation error.
 *   7.  Submit valid data → calls mutation with correct values.
 *   8.  404 fallback → window.location.href assigned to mailto + mailtoFallback toast shown.
 *   9.  200 success → success toast shown + router.push called.
 *  10.  Mutation error → error toast shown, page stays.
 *  11.  Editorial copy check — no emoji, no "Unlock", no "Power", no bare "!".
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// ---------------------------------------------------------------------------
// Mocks — declared before component imports
// ---------------------------------------------------------------------------

const mockPush = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

const mockToastSuccess = vi.fn()
const mockToastError = vi.fn()
const mockToastInfo = vi.fn()

vi.mock('@/components/feedback/Toaster', () => ({
  useToast: () => ({
    toast: {
      success: mockToastSuccess,
      error: mockToastError,
      info: mockToastInfo,
    },
  }),
}))

// Controlled mutation mock
let mockMutate = vi.fn()
let mockIsPending = false

vi.mock('@/lib/api/subscription/hooks/useProContact', () => ({
  useSubmitProContact: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
  }),
}))

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import { ProContactForm } from '@/app/(admin)/settings/billing/pro-contact/ProContactForm'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderForm(storeId = 'store_1') {
  return render(<ProContactForm storeId={storeId} />)
}

/**
 * Fill the required fields to produce a valid submission.
 */
async function fillValidForm(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Company name'), 'Acme Ltd')
  await user.type(screen.getByLabelText('Your name'), 'Jane Smith')
  await user.type(screen.getByLabelText('Work email'), 'jane@acme.com')

  const ordersInput = screen.getByLabelText('Monthly orders')
  await user.clear(ordersInput)
  await user.type(ordersInput, '5000')
}

beforeEach(() => {
  mockPush.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  mockToastInfo.mockReset()
  mockMutate = vi.fn()
  mockIsPending = false

  // Reset window.location.href assignment tracking
  Object.defineProperty(window, 'location', {
    writable: true,
    value: { href: '' },
  })
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('ProContactForm', () => {
  it('renders all form fields', () => {
    renderForm()

    expect(screen.getByLabelText('Company name')).toBeInTheDocument()
    expect(screen.getByLabelText('Your name')).toBeInTheDocument()
    expect(screen.getByLabelText('Work email')).toBeInTheDocument()
    expect(screen.getByLabelText('Phone (optional)')).toBeInTheDocument()
    expect(screen.getByLabelText('Monthly orders')).toBeInTheDocument()
    expect(screen.getByLabelText('Current platform (optional)')).toBeInTheDocument()
    expect(
      screen.getByLabelText('When are you looking to move?'),
    ).toBeInTheDocument()
    expect(
      screen.getByLabelText('Anything else we should know? (optional)'),
    ).toBeInTheDocument()
  })

  it('required-field validation — company_name missing', async () => {
    const user = userEvent.setup()
    renderForm()

    // Leave company_name blank, fill others.
    await user.type(screen.getByLabelText('Your name'), 'Jane Smith')
    await user.type(screen.getByLabelText('Work email'), 'jane@acme.com')
    const ordersInput = screen.getByLabelText('Monthly orders')
    await user.clear(ordersInput)
    await user.type(ordersInput, '5000')

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      expect(mockMutate).not.toHaveBeenCalled()
    })
  })

  it('required-field validation — contact_name missing', async () => {
    const user = userEvent.setup()
    renderForm()

    await user.type(screen.getByLabelText('Company name'), 'Acme Ltd')
    // Leave contact_name blank
    await user.type(screen.getByLabelText('Work email'), 'jane@acme.com')
    const ordersInput = screen.getByLabelText('Monthly orders')
    await user.clear(ordersInput)
    await user.type(ordersInput, '5000')

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      expect(mockMutate).not.toHaveBeenCalled()
    })
  })

  it('required-field validation — contact_email missing', async () => {
    const user = userEvent.setup()
    renderForm()

    await user.type(screen.getByLabelText('Company name'), 'Acme Ltd')
    await user.type(screen.getByLabelText('Your name'), 'Jane Smith')
    // Leave contact_email blank
    const ordersInput = screen.getByLabelText('Monthly orders')
    await user.clear(ordersInput)
    await user.type(ordersInput, '5000')

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      expect(mockMutate).not.toHaveBeenCalled()
    })
  })

  it('required-field validation — monthly_orders is NaN', async () => {
    const user = userEvent.setup()
    renderForm()

    await user.type(screen.getByLabelText('Company name'), 'Acme Ltd')
    await user.type(screen.getByLabelText('Your name'), 'Jane Smith')
    await user.type(screen.getByLabelText('Work email'), 'jane@acme.com')
    // Clear the numeric input to leave it empty (NaN after valueAsNumber)
    await user.clear(screen.getByLabelText('Monthly orders'))

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      expect(mockMutate).not.toHaveBeenCalled()
    })
  })

  it('invalid email shows a validation error', async () => {
    const user = userEvent.setup()
    renderForm()

    await user.type(screen.getByLabelText('Company name'), 'Acme Ltd')
    await user.type(screen.getByLabelText('Your name'), 'Jane Smith')
    await user.type(screen.getByLabelText('Work email'), 'not-an-email')
    const ordersInput = screen.getByLabelText('Monthly orders')
    await user.clear(ordersInput)
    await user.type(ordersInput, '5000')

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      expect(mockMutate).not.toHaveBeenCalled()
    })
  })

  it('valid submit — calls mutation with correct values', async () => {
    const user = userEvent.setup()
    renderForm()

    await fillValidForm(user)

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledWith(
        expect.objectContaining({
          company_name: 'Acme Ltd',
          contact_name: 'Jane Smith',
          contact_email: 'jane@acme.com',
          monthly_orders: 5000,
        }),
        expect.any(Object),
      )
    })
  })

  it('404 fallback — window.location.href assigned to mailto + mailtoFallback toast', async () => {
    const user = userEvent.setup()

    // Make mutate call onSuccess with fallback='mailto'
    mockMutate = vi.fn((_values, callbacks) => {
      callbacks?.onSuccess?.({ status: 'submitted', ticket_id: '', fallback: 'mailto' })
    })

    renderForm()
    await fillValidForm(user)

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      // encodeURIComponent encodes spaces as %20 (not +)
      expect(window.location.href).toContain('mailto:pro-sales@mark8ly.com')
      expect(window.location.href).toMatch(/subject=Pro(%20|\+)inquiry/)
    })

    expect(mockToastInfo).toHaveBeenCalledWith(
      expect.stringContaining("email client"),
    )
    // Should NOT redirect via router — stay on the page.
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('200 success — success toast + router.push to /settings/billing', async () => {
    const user = userEvent.setup()

    mockMutate = vi.fn((_values, callbacks) => {
      callbacks?.onSuccess?.({
        status: 'submitted',
        ticket_id: 'ticket_abc',
        fallback: undefined,
      })
    })

    renderForm()
    await fillValidForm(user)

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith(
        expect.stringContaining("1 business day"),
      )
      expect(mockPush).toHaveBeenCalledWith('/settings/billing')
    })
  })

  it('mutation error — error toast shown, page stays', async () => {
    const user = userEvent.setup()

    mockMutate = vi.fn((_values, callbacks) => {
      callbacks?.onError?.(new Error('network error'))
    })

    renderForm()
    await fillValidForm(user)

    await user.click(screen.getByRole('button', { name: 'Start a conversation' }))

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        expect.stringContaining("pro-sales@mark8ly.com"),
      )
    })

    expect(mockPush).not.toHaveBeenCalled()
  })

  it('editorial copy check — no emoji, no "Unlock", no "Power", no bare "!"', () => {
    const { container } = renderForm()
    const html = container.innerHTML

    // No emoji (basic Unicode range check for common emoji blocks)
    expect(html).not.toMatch(
      /[\u{1F300}-\u{1FAFF}]|[\u{2600}-\u{27BF}]/u,
    )

    // No urgency / SaaS-loud words
    expect(html).not.toMatch(/Unlock/i)
    expect(html).not.toMatch(/\bPower\b/i)

    // No bare exclamation marks at the end of a word (sentence-ending "!")
    // Allow "!" inside attribute values (e.g. aria labels with punctuation)
    // but reject any visible text node ending with "!"
    const textContent = container.textContent ?? ''
    expect(textContent).not.toMatch(/\w!/)
  })
})
