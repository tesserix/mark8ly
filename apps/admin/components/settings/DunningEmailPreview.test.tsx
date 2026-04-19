/**
 * DunningEmailPreview unit tests — Vitest + Testing Library.
 *
 * Tests:
 *   1. isPlatformOwner false → returns null (nothing rendered)
 *   2. isPlatformOwner true + daysOverdue=1  → T+1 template
 *   3. isPlatformOwner true + daysOverdue=3  → T+3 template
 *   4. isPlatformOwner true + daysOverdue=8  → T+7 template (4–7 range ceiling)
 *      (daysOverdue=8 crosses into T+14 per spec — verify T+14 subject)
 *   5. isPlatformOwner true + daysOverdue=15 → T+14 template
 *   6. No emoji or urgency language in any rendered output
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

// ---------------------------------------------------------------------------
// Mock useIsPlatformOwner — controlled per-test via `platformOwnerValue`
// ---------------------------------------------------------------------------

let platformOwnerValue = false

vi.mock('@/lib/auth/useIsPlatformOwner', () => ({
  useIsPlatformOwner: () => platformOwnerValue,
}))

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import { DunningEmailPreview } from './DunningEmailPreview'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const STORE_ID = 'store-csm-test'
const RETRY_DATE = '25 April 2026'

function renderPreview(daysOverdue: number) {
  return render(
    <DunningEmailPreview
      storeId={STORE_ID}
      daysOverdue={daysOverdue}
      retryDate={RETRY_DATE}
    />,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('DunningEmailPreview', () => {
  it('returns null when isPlatformOwner is false', () => {
    platformOwnerValue = false
    const { container } = renderPreview(1)
    expect(container.firstChild).toBeNull()
  })

  it('renders T+1 template when daysOverdue=1', () => {
    platformOwnerValue = true
    renderPreview(1)

    expect(screen.getByTestId('dunning-email-preview')).toBeInTheDocument()
    expect(screen.getByTestId('dunning-subject')).toHaveTextContent(
      'A note about your payment',
    )
    expect(screen.getByTestId('dunning-body')).toHaveTextContent(RETRY_DATE)
  })

  it('renders T+3 template when daysOverdue=3', () => {
    platformOwnerValue = true
    renderPreview(3)

    expect(screen.getByTestId('dunning-subject')).toHaveTextContent(
      'Your most recent payment',
    )
    expect(screen.getByTestId('dunning-body')).toHaveTextContent(RETRY_DATE)
  })

  it('renders T+14 template when daysOverdue=8 (past T+7 ceiling)', () => {
    platformOwnerValue = true
    renderPreview(8)

    // daysOverdue=8 is > 7 so it falls into T+14
    expect(screen.getByTestId('dunning-subject')).toHaveTextContent(
      'Final notice before your account moves to read-only',
    )
  })

  it('renders T+7 template when daysOverdue=7', () => {
    platformOwnerValue = true
    renderPreview(7)

    expect(screen.getByTestId('dunning-subject')).toHaveTextContent(
      'Your account is on pause',
    )
    expect(screen.getByTestId('dunning-body')).toHaveTextContent(RETRY_DATE)
  })

  it('renders T+14 template when daysOverdue=15', () => {
    platformOwnerValue = true
    renderPreview(15)

    expect(screen.getByTestId('dunning-subject')).toHaveTextContent(
      'Final notice before your account moves to read-only',
    )
    expect(screen.getByTestId('dunning-body')).toHaveTextContent(RETRY_DATE)
  })

  it('does not contain emoji or urgency language', () => {
    platformOwnerValue = true
    // Check all tiers
    for (const days of [1, 3, 7, 15]) {
      const { unmount } = renderPreview(days)
      const text = document.body.textContent ?? ''

      expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
      expect(text).not.toMatch(/!/g)
      expect(text).not.toMatch(/urgent/i)
      expect(text).not.toMatch(/hurry/i)
      expect(text).not.toMatch(/immediately/i)

      unmount()
    }
  })
})
