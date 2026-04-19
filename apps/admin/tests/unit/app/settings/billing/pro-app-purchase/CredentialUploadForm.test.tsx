/**
 * CredentialUploadForm unit tests — Vitest + Testing Library.
 *
 * All hooks are mocked at module level. No real React Query store needed.
 *
 * Tests:
 *   1. Renders both Apple and Google panels
 *   2. Apple submit without fields → validation errors shown
 *   3. Google submit with invalid JSON → inline error shown
 *   4. Apple mutation fires correctly on valid input
 *   5. Google mutation fires correctly on valid input
 *   6. Apple on success → toast success shown
 *   7. Google on success → toast success shown
 *   8. Apple on error → toast error shown
 *   9. Support link is rendered
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

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

const makeMutation = (overrides: Record<string, unknown> = {}) => ({
  mutate: vi.fn(),
  isPending: false,
  isError: false,
  isSuccess: false,
  data: undefined,
  error: null,
  ...overrides,
})

const mockUseUploadApple = vi.fn(() => makeMutation())
const mockUseUploadGoogle = vi.fn(() => makeMutation())

vi.mock('@/lib/api/subscription/hooks/useAppCredentials', () => ({
  useUploadAppleCredentials: (...args: unknown[]) => mockUseUploadApple(...args),
  useUploadGoogleCredentials: (...args: unknown[]) => mockUseUploadGoogle(...args),
}))

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const STORE_ID = 'f47ac10b-58cc-4372-a567-0e02b2c3d479'

const VALID_P8 =
  '-----BEGIN PRIVATE KEY-----\nMIGTAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBHkwdwIBAQQg\n-----END PRIVATE KEY-----'

const VALID_SERVICE_ACCOUNT = JSON.stringify({
  type: 'service_account',
  project_id: 'test-project',
  client_email: 'play@test-project.iam.gserviceaccount.com',
  private_key: '-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----',
})

// ---------------------------------------------------------------------------
// Import component after mocks
// ---------------------------------------------------------------------------

import { CredentialUploadForm } from '@/app/(admin)/settings/billing/pro-app-purchase/CredentialUploadForm'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderComponent() {
  return render(<CredentialUploadForm storeId={STORE_ID} />)
}

/**
 * Simulate a file being selected on a file input.
 * Sets the file input's files property and fires a change event.
 */
function simulateFileUpload(input: HTMLElement, name: string, content: string, type = 'text/plain') {
  const file = new File([content], name, { type })
  Object.defineProperty(input, 'files', {
    value: [file],
    configurable: true,
  })
  fireEvent.change(input)
}

// Testing Library's fireEvent — imported directly for the file simulation
import { fireEvent } from '@testing-library/react'

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('CredentialUploadForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseUploadApple.mockReturnValue(makeMutation())
    mockUseUploadGoogle.mockReturnValue(makeMutation())
  })

  it('renders both Apple and Google panels', () => {
    renderComponent()

    expect(screen.getByText(/apple app store connect/i)).toBeInTheDocument()
    expect(screen.getByText(/google play console/i)).toBeInTheDocument()
  })

  it('renders support link', () => {
    renderComponent()

    expect(screen.getByRole('link', { name: /contact support/i })).toBeInTheDocument()
  })

  it('Apple submit without fields — shows validation errors', async () => {
    renderComponent()

    const appleSubmit = screen.getByRole('button', { name: /save apple credentials/i })
    await userEvent.click(appleSubmit)

    await waitFor(() => {
      // At least one validation error should appear
      const alerts = screen.getAllByRole('alert')
      expect(alerts.length).toBeGreaterThan(0)
    })
  })

  it('Google submit with invalid JSON — shows inline error', async () => {
    renderComponent()

    // Simulate uploading an invalid JSON file
    const googleFileInput = document.querySelector(
      'input[accept=".json,application/json"]',
    ) as HTMLInputElement
    expect(googleFileInput).not.toBeNull()

    simulateFileUpload(googleFileInput, 'bad.json', 'this is not json')

    const googleSubmit = screen.getByRole('button', { name: /save google credentials/i })
    await userEvent.click(googleSubmit)

    await waitFor(() => {
      expect(
        screen.getByText(/valid service account json/i),
      ).toBeInTheDocument()
    })
  })

  it('Apple mutation fires on valid input', async () => {
    const appleMutation = {
      ...makeMutation(),
      mutate: vi.fn(),
    }
    mockUseUploadApple.mockReturnValue(appleMutation)

    renderComponent()

    // Fill text fields
    await userEvent.type(screen.getByLabelText(/key id/i), 'ABCDE12345')
    await userEvent.type(screen.getByLabelText(/issuer id/i), '57246542-96fe-1a63-e053-0824d011072a')

    // Simulate p8 file selection
    const p8Input = document.querySelector('input[accept=".p8"]') as HTMLInputElement
    simulateFileUpload(p8Input, 'key.p8', VALID_P8)

    await waitFor(() => {
      // After file change, form should have p8_contents set
    })

    const appleSubmit = screen.getByRole('button', { name: /save apple credentials/i })
    await userEvent.click(appleSubmit)

    await waitFor(() => {
      expect(appleMutation.mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          key_id: 'ABCDE12345',
          issuer_id: '57246542-96fe-1a63-e053-0824d011072a',
        }),
        expect.any(Object),
      )
    })
  })

  it('Google mutation fires on valid JSON file', async () => {
    const googleMutation = {
      ...makeMutation(),
      mutate: vi.fn(),
    }
    mockUseUploadGoogle.mockReturnValue(googleMutation)

    renderComponent()

    const googleFileInput = document.querySelector(
      'input[accept=".json,application/json"]',
    ) as HTMLInputElement
    simulateFileUpload(googleFileInput, 'service-account.json', VALID_SERVICE_ACCOUNT, 'application/json')

    await waitFor(() => {
      // form state updated
    })

    const googleSubmit = screen.getByRole('button', { name: /save google credentials/i })
    await userEvent.click(googleSubmit)

    await waitFor(() => {
      expect(googleMutation.mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          service_account_json: VALID_SERVICE_ACCOUNT,
        }),
        expect.any(Object),
      )
    })
  })

  it('Apple on success — shows success toast', async () => {
    const appleMutation = {
      ...makeMutation(),
      mutate: vi.fn((_vars: unknown, opts: { onSuccess?: () => void }) => {
        opts.onSuccess?.()
      }),
    }
    mockUseUploadApple.mockReturnValue(appleMutation)

    renderComponent()

    await userEvent.type(screen.getByLabelText(/key id/i), 'ABCDE12345')
    await userEvent.type(screen.getByLabelText(/issuer id/i), '57246542-96fe-1a63-e053-0824d011072a')

    const p8Input = document.querySelector('input[accept=".p8"]') as HTMLInputElement
    simulateFileUpload(p8Input, 'key.p8', VALID_P8)

    await waitFor(() => {})

    await userEvent.click(screen.getByRole('button', { name: /save apple credentials/i }))

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith('Credentials saved.')
    })
  })

  it('Apple on error — shows error toast', async () => {
    const appleMutation = {
      ...makeMutation(),
      mutate: vi.fn((_vars: unknown, opts: { onError?: (err: unknown) => void }) => {
        opts.onError?.(new Error('store_failed'))
      }),
    }
    mockUseUploadApple.mockReturnValue(appleMutation)

    renderComponent()

    await userEvent.type(screen.getByLabelText(/key id/i), 'ABCDE12345')
    await userEvent.type(screen.getByLabelText(/issuer id/i), '57246542-96fe-1a63-e053-0824d011072a')

    const p8Input = document.querySelector('input[accept=".p8"]') as HTMLInputElement
    simulateFileUpload(p8Input, 'key.p8', VALID_P8)

    await waitFor(() => {})

    await userEvent.click(screen.getByRole('button', { name: /save apple credentials/i }))

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalled()
    })
  })

  it('Google on success — shows success toast', async () => {
    const googleMutation = {
      ...makeMutation(),
      mutate: vi.fn((_vars: unknown, opts: { onSuccess?: () => void }) => {
        opts.onSuccess?.()
      }),
    }
    mockUseUploadGoogle.mockReturnValue(googleMutation)

    renderComponent()

    const googleFileInput = document.querySelector(
      'input[accept=".json,application/json"]',
    ) as HTMLInputElement
    simulateFileUpload(googleFileInput, 'service-account.json', VALID_SERVICE_ACCOUNT, 'application/json')

    await waitFor(() => {})

    await userEvent.click(screen.getByRole('button', { name: /save google credentials/i }))

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith('Credentials saved.')
    })
  })
})
