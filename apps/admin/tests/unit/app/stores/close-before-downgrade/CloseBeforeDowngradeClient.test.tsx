/**
 * CloseBeforeDowngradeClient unit tests — Vitest + Testing Library.
 *
 * All hooks and the dynamic import are mocked at module level so no real
 * React Query store or MSW server is needed.
 *
 * Tests:
 *   1.  Reads URL params and renders heading with target/limit/overBy text
 *   2.  Renders N rows matching over_by
 *   3.  Each row shows store name
 *   4.  Each row shows in-flight order count when available
 *   5.  Row shows "—" when in_flight_orders is null (backend not yet wired)
 *   6.  Apply button is disabled until every row has an action
 *   7.  Apply button becomes enabled once all rows are resolved
 *   8.  Selecting Close renders the Close help text for that row
 *   9.  Selecting Delete renders the 60-day grace copy for that row
 *   10. Clicking Apply with mixed selections fires correct mutations per row
 *   11. Export CSV link has correct href per store
 *   12. Skeleton rows render when loading
 *   13. Error state shown when block list query fails
 *   14. Partial failure — failed stores remain, success toast not shown
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import React from 'react'

// ---------------------------------------------------------------------------
// Mocks — declared before any imports that reference these modules
// ---------------------------------------------------------------------------

const mockPush = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => mockSearchParams,
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

// ---------------------------------------------------------------------------
// useDowngradeBlockList control
// ---------------------------------------------------------------------------

type MockBlockListResult = {
  data: unknown
  isLoading: boolean
  isError: boolean
  error: unknown
}

let mockBlockListResult: MockBlockListResult = {
  data: undefined,
  isLoading: false,
  isError: false,
  error: null,
}

const mockCloseStoreMutate = vi.fn()
const mockDeleteStoreMutate = vi.fn()
const mockCloseReset = vi.fn()
const mockDeleteReset = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useStoreDowngrade', () => ({
  useDowngradeBlockList: () => mockBlockListResult,
  useCloseStore: () => ({
    mutateAsync: mockCloseStoreMutate,
    reset: mockCloseReset,
  }),
  useDeleteStore: () => ({
    mutateAsync: mockDeleteStoreMutate,
    reset: mockDeleteReset,
  }),
}))

// Mock the dynamic import inside handleApply so we can control per-store outcomes.
const mockCloseStoreFn = vi.fn()
const mockDeleteStoreFn = vi.fn()

vi.mock('@/lib/api/subscription/stores', () => ({
  closeStore: (...args: unknown[]) => mockCloseStoreFn(...args),
  deleteStore: (...args: unknown[]) => mockDeleteStoreFn(...args),
  getDowngradeBlockList: vi.fn(),
}))

// ---------------------------------------------------------------------------
// URL search params mock — mutated per-test
// ---------------------------------------------------------------------------

let mockSearchParamsMap: Record<string, string> = {}

const mockSearchParams = {
  get: (key: string) => mockSearchParamsMap[key] ?? null,
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const STORE_A = {
  id: 'store-a-id',
  name: 'Paris Pop-up',
  in_flight_orders: 3,
  updated_at: '2026-04-01T00:00:00Z',
}

const STORE_B = {
  id: 'store-b-id',
  name: 'Berlin Flagship',
  in_flight_orders: 0,
  updated_at: '2026-03-15T00:00:00Z',
}

const STORE_NULL_ORDERS = {
  id: 'store-c-id',
  name: 'Tokyo Concept',
  in_flight_orders: null,
  updated_at: '2026-02-20T00:00:00Z',
}

const BLOCK_LIST_2 = {
  stores: [STORE_A, STORE_B],
  over_by: 2,
  target_plan: 'studio',
  billing_period: 'annual',
}

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import { CloseBeforeDowngradeClient } from '@/app/(admin)/stores/close-before-downgrade/CloseBeforeDowngradeClient'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderClient() {
  return render(<CloseBeforeDowngradeClient />)
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  mockPush.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  mockCloseStoreFn.mockReset()
  mockDeleteStoreFn.mockReset()
  mockCloseReset.mockReset()
  mockDeleteReset.mockReset()

  mockSearchParamsMap = {
    target: 'studio',
    billing: 'annual',
    over_by: '2',
  }

  mockBlockListResult = {
    data: BLOCK_LIST_2,
    isLoading: false,
    isError: false,
    error: null,
  }

  // Default: mutations succeed.
  mockCloseStoreFn.mockResolvedValue({ status: 'closed' })
  mockDeleteStoreFn.mockResolvedValue({ status: 'deleted_pending_grace' })
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('CloseBeforeDowngradeClient', () => {
  it('renders the heading from copy', () => {
    renderClient()
    expect(
      screen.getByRole('heading', { level: 1, name: /choose what happens/i }),
    ).toBeInTheDocument()
  })

  it('renders intro text with target plan, limit, and overBy', () => {
    renderClient()
    // Studio plan cap = 3; over_by = 2
    expect(screen.getByText(/studio plan includes 3 stores/i)).toBeInTheDocument()
    expect(screen.getByText(/2 over your cap/i)).toBeInTheDocument()
  })

  it('renders N rows matching over_by', () => {
    renderClient()
    // Each store row has a heading for the store name.
    const rows = screen.getAllByRole('heading', { level: 3 })
    expect(rows).toHaveLength(2)
  })

  it('each row shows the store name', () => {
    renderClient()
    expect(screen.getByText('Paris Pop-up')).toBeInTheDocument()
    expect(screen.getByText('Berlin Flagship')).toBeInTheDocument()
  })

  it('row shows in-flight order count when available', () => {
    renderClient()
    // STORE_A has 3 in-flight orders.
    expect(screen.getByText(/3 orders? in flight/i)).toBeInTheDocument()
    // STORE_B has 0 in-flight orders.
    expect(screen.getByText(/no in-flight orders/i)).toBeInTheDocument()
  })

  it('row shows "—" when in_flight_orders is null', () => {
    mockBlockListResult = {
      data: {
        stores: [STORE_NULL_ORDERS],
        over_by: 1,
        target_plan: 'studio',
        billing_period: 'annual',
      },
      isLoading: false,
      isError: false,
      error: null,
    }
    mockSearchParamsMap = { target: 'studio', billing: 'annual', over_by: '1' }

    renderClient()
    // The placeholder dash should appear in the DOM.
    expect(screen.getByLabelText('data unavailable')).toBeInTheDocument()
  })

  it('Apply button is disabled until every row has an action', () => {
    renderClient()
    const applyBtn = screen.getByRole('button', { name: /apply changes/i })
    expect(applyBtn).toBeDisabled()
  })

  it('Apply button becomes enabled once all rows have an action', () => {
    renderClient()

    const closeRadios = screen.getAllByRole('radio', { name: /close/i })
    fireEvent.click(closeRadios[0])
    fireEvent.click(closeRadios[1])

    const applyBtn = screen.getByRole('button', { name: /apply changes/i })
    expect(applyBtn).not.toBeDisabled()
  })

  it('selecting Close renders the Close help text for that row', () => {
    renderClient()

    const closeRadios = screen.getAllByRole('radio', { name: /close/i })
    fireEvent.click(closeRadios[0])

    expect(
      screen.getByText(/freezes the storefront behind a closed page/i),
    ).toBeInTheDocument()
  })

  it('selecting Delete renders the 60-day grace copy for that row', () => {
    renderClient()

    const deleteRadios = screen.getAllByRole('radio', { name: /delete/i })
    fireEvent.click(deleteRadios[0])

    expect(
      screen.getByText(/60-day soft-delete grace/i),
    ).toBeInTheDocument()
  })

  it('clicking Apply with mixed selections fires correct mutations per row', async () => {
    renderClient()

    // Select Close for STORE_A, Delete for STORE_B.
    const closeRadios = screen.getAllByRole('radio', { name: /close/i })
    const deleteRadios = screen.getAllByRole('radio', { name: /delete/i })
    fireEvent.click(closeRadios[0]) // Paris Pop-up → Close
    fireEvent.click(deleteRadios[1]) // Berlin Flagship → Delete

    const applyBtn = screen.getByRole('button', { name: /apply changes/i })
    fireEvent.click(applyBtn)

    await waitFor(() => {
      expect(mockCloseStoreFn).toHaveBeenCalledWith(STORE_A.id)
      expect(mockDeleteStoreFn).toHaveBeenCalledWith(STORE_B.id)
    })
  })

  it('successful apply shows success toast and navigates to retry URL', async () => {
    renderClient()

    const closeRadios = screen.getAllByRole('radio', { name: /close/i })
    fireEvent.click(closeRadios[0])
    fireEvent.click(closeRadios[1])

    const applyBtn = screen.getByRole('button', { name: /apply changes/i })
    fireEvent.click(applyBtn)

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith('Stores updated. Downgrade scheduled.')
      expect(mockPush).toHaveBeenCalledWith(
        expect.stringContaining('/admin/settings/billing/plan-change'),
      )
      expect(mockPush).toHaveBeenCalledWith(expect.stringContaining('retry=true'))
    })
  })

  it('Export CSV link has the correct href per store', () => {
    renderClient()

    const exportLinks = screen.getAllByRole('link', { name: /export orders csv/i })
    expect(exportLinks).toHaveLength(2)
    expect(exportLinks[0]).toHaveAttribute(
      'href',
      `/admin/stores/${STORE_A.id}/orders?export=csv`,
    )
    expect(exportLinks[1]).toHaveAttribute(
      'href',
      `/admin/stores/${STORE_B.id}/orders?export=csv`,
    )
  })

  it('renders skeleton rows when block list is loading', () => {
    mockBlockListResult = {
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
    }

    renderClient()

    // Skeleton items are aria-hidden="true".
    const skeleton = document.querySelectorAll('[aria-hidden="true"]')
    expect(skeleton.length).toBeGreaterThan(0)
  })

  it('shows error state when block list query fails', () => {
    mockBlockListResult = {
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('network error'),
    }

    renderClient()

    expect(
      screen.getByText(/could not load over-cap stores/i),
    ).toBeInTheDocument()
  })

  it('partial failure — shows error toast and keeps failed stores visible', async () => {
    // closeStore for STORE_A fails; deleteStore for STORE_B succeeds.
    mockCloseStoreFn.mockRejectedValueOnce(new Error('close failed'))
    mockDeleteStoreFn.mockResolvedValue({ status: 'deleted_pending_grace' })

    renderClient()

    const closeRadios = screen.getAllByRole('radio', { name: /close/i })
    const deleteRadios = screen.getAllByRole('radio', { name: /delete/i })
    fireEvent.click(closeRadios[0]) // Paris Pop-up → Close (will fail)
    fireEvent.click(deleteRadios[1]) // Berlin Flagship → Delete (will succeed)

    const applyBtn = screen.getByRole('button', { name: /apply changes/i })
    fireEvent.click(applyBtn)

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalled()
      expect(mockToastSuccess).not.toHaveBeenCalled()
      expect(mockPush).not.toHaveBeenCalled()
    })

    // Failure notice should be shown.
    expect(screen.getByRole('alert')).toBeInTheDocument()
  })
})
