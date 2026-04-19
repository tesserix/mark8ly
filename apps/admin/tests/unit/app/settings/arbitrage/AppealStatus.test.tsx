/**
 * AppealStatus unit tests — Vitest + Testing Library.
 *
 * Tests:
 *   1.  Not flagged → returns null.
 *   2.  Flagged, no appeal → renders Submit CTA.
 *   3.  Pending appeal, 2 business days old → stepper step 1 active, SLA "3 day(s) remaining".
 *   4.  Under-review appeal, 4 days old → stepper step 2 active, SLA "1 day(s) remaining".
 *   5.  Resolved appeal → step 3 active, no SLA countdown.
 *   6.  Rejected appeal → step 3 shown with declined label + editorial decline message.
 *   7.  Overdue (6+ days, still pending) → "pending longer than usual" message.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'
import type { ArbitrageAuditSummary } from '@/lib/api/subscription/schemas/billing'

// ---------------------------------------------------------------------------
// Mock next/link
// ---------------------------------------------------------------------------

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

// ---------------------------------------------------------------------------
// Mock useArbitrageFlag — controlled per test
// ---------------------------------------------------------------------------

interface ArbitrageFlagOverride {
  flagged: boolean
  isLoading: boolean
  latestAudit: ArbitrageAuditSummary | null
}

let flagOverride: ArbitrageFlagOverride = {
  flagged: false,
  isLoading: false,
  latestAudit: null,
}

vi.mock('@/lib/api/subscription/hooks/useArbitrage', () => ({
  useArbitrageFlag: () => flagOverride,
}))

// ---------------------------------------------------------------------------
// Mock businessDaysElapsed — fixed value per test
// ---------------------------------------------------------------------------

let businessDaysMock = 0

vi.mock('@/lib/format/businessDays', () => ({
  businessDaysElapsed: () => businessDaysMock,
}))

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import { AppealStatus } from '@/app/(admin)/settings/arbitrage/AppealStatus'

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const STORE_ID = 'store-appeal-test'

function makeAudit(
  overrides: Partial<ArbitrageAuditSummary> = {},
): ArbitrageAuditSummary {
  return {
    card_country: 'US',
    billing_country: 'IN',
    ip_country: 'US',
    resolution: '',
    flagged_at: '2026-04-10T00:00:00Z',
    mismatch_reason: 'card_country_mismatch',
    appeal_status: null,
    appeal_submitted_at: null,
    ...overrides,
  }
}

function renderComponent() {
  return render(<AppealStatus storeId={STORE_ID} />)
}

// ---------------------------------------------------------------------------
// beforeEach
// ---------------------------------------------------------------------------

beforeEach(() => {
  flagOverride = { flagged: false, isLoading: false, latestAudit: null }
  businessDaysMock = 0
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('AppealStatus', () => {
  it('returns null when store is not flagged and has no audit', () => {
    const { container } = renderComponent()
    expect(container.firstChild).toBeNull()
  })

  it('renders Submit CTA when flagged but no appeal in progress', () => {
    flagOverride = {
      flagged: true,
      isLoading: false,
      latestAudit: makeAudit({ appeal_status: null }),
    }
    renderComponent()

    expect(
      screen.getByRole('link', { name: /Submit an appeal/i }),
    ).toBeInTheDocument()
    expect(
      screen.queryByTestId('appeal-status-tracker'),
    ).not.toBeInTheDocument()
  })

  it('shows step 1 active and SLA "3 day(s) remaining" for pending appeal 2 days old', () => {
    businessDaysMock = 2
    flagOverride = {
      flagged: true,
      isLoading: false,
      latestAudit: makeAudit({
        appeal_status: 'pending',
        appeal_submitted_at: '2026-04-15T00:00:00Z',
      }),
    }
    renderComponent()

    expect(screen.getByTestId('appeal-status-tracker')).toBeInTheDocument()
    expect(screen.getByTestId('appeal-sla-message')).toHaveTextContent(
      '3 day',
    )
    expect(screen.getByTestId('appeal-sla-message')).toHaveTextContent(
      'remaining',
    )
    // Step 1 "Submitted" should be present
    expect(screen.getByText('Submitted')).toBeInTheDocument()
  })

  it('shows step 2 active and SLA "1 day(s) remaining" for under_review appeal 4 days old', () => {
    businessDaysMock = 4
    flagOverride = {
      flagged: true,
      isLoading: false,
      latestAudit: makeAudit({
        appeal_status: 'under_review',
        appeal_submitted_at: '2026-04-14T00:00:00Z',
      }),
    }
    renderComponent()

    expect(screen.getByText('Under review')).toBeInTheDocument()
    expect(screen.getByTestId('appeal-sla-message')).toHaveTextContent(
      '1 day',
    )
  })

  it('shows step 3 active and no SLA countdown for resolved appeal', () => {
    flagOverride = {
      flagged: true,
      isLoading: false,
      latestAudit: makeAudit({
        appeal_status: 'resolved',
        appeal_submitted_at: '2026-04-10T00:00:00Z',
      }),
    }
    renderComponent()

    expect(screen.getByTestId('appeal-resolved-message')).toBeInTheDocument()
    expect(
      screen.queryByTestId('appeal-sla-message'),
    ).not.toBeInTheDocument()
  })

  it('shows declined step 3 with editorial decline message for rejected appeal', () => {
    flagOverride = {
      flagged: true,
      isLoading: false,
      latestAudit: makeAudit({
        appeal_status: 'rejected',
        appeal_submitted_at: '2026-04-10T00:00:00Z',
      }),
    }
    renderComponent()

    expect(screen.getByText('Declined')).toBeInTheDocument()
    expect(screen.getByTestId('appeal-rejected-message')).toBeInTheDocument()
    // No SLA when rejected
    expect(
      screen.queryByTestId('appeal-sla-message'),
    ).not.toBeInTheDocument()
  })

  it('shows "pending longer than usual" message when overdue (6+ business days, still pending)', () => {
    businessDaysMock = 6
    flagOverride = {
      flagged: true,
      isLoading: false,
      latestAudit: makeAudit({
        appeal_status: 'pending',
        appeal_submitted_at: '2026-04-08T00:00:00Z',
      }),
    }
    renderComponent()

    expect(screen.getByTestId('appeal-sla-message')).toHaveTextContent(
      'pending longer than usual',
    )
  })

  it('does not contain urgency language', () => {
    flagOverride = {
      flagged: true,
      isLoading: false,
      latestAudit: makeAudit({ appeal_status: 'pending', appeal_submitted_at: '2026-04-15T00:00:00Z' }),
    }
    renderComponent()
    const text = document.body.textContent ?? ''
    expect(text).not.toMatch(/!/g)
    expect(text).not.toMatch(/urgent/i)
    expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })
})
