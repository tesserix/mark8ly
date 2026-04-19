/**
 * ArbitrageBanner unit tests.
 *
 * Tests:
 *   1. Returns null when not flagged.
 *   2. Renders editorial heading "We've noted a discrepancy" when flagged.
 *   3. CTA href is /admin/settings/arbitrage.
 *   4. No urgency language or emoji in rendered output.
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ArbitrageBanner } from '@/components/shell/banners/ArbitrageBanner'

// ---------------------------------------------------------------------------
// Mock hooks
// ---------------------------------------------------------------------------

const mockUseArbitrageFlag = vi.fn()

vi.mock('@/lib/api/subscription/hooks/useArbitrage', () => ({
  useArbitrageFlag: () => mockUseArbitrageFlag(),
}))

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

const STORE_ID = 'store-abc-123'

describe('ArbitrageBanner', () => {
  it('returns null when not flagged', () => {
    mockUseArbitrageFlag.mockReturnValue({
      flagged: false,
      isLoading: false,
      firstFlaggedAt: null,
      severity: null,
      latestAudit: null,
    })

    const { container } = render(<ArbitrageBanner storeId={STORE_ID} />)

    expect(container.firstChild).toBeNull()
  })

  it('returns null while loading', () => {
    mockUseArbitrageFlag.mockReturnValue({
      flagged: false,
      isLoading: true,
      firstFlaggedAt: null,
      severity: null,
      latestAudit: null,
    })

    const { container } = render(<ArbitrageBanner storeId={STORE_ID} />)

    expect(container.firstChild).toBeNull()
  })

  it('renders the editorial heading when flagged', () => {
    mockUseArbitrageFlag.mockReturnValue({
      flagged: true,
      isLoading: false,
      firstFlaggedAt: null,
      severity: 'material',
      latestAudit: null,
    })

    render(<ArbitrageBanner storeId={STORE_ID} />)

    expect(
      screen.getByText(/We've noted a discrepancy/i),
    ).toBeInTheDocument()
  })

  it('CTA href is /admin/settings/arbitrage when flagged', () => {
    mockUseArbitrageFlag.mockReturnValue({
      flagged: true,
      isLoading: false,
      firstFlaggedAt: null,
      severity: 'material',
      latestAudit: null,
    })

    render(<ArbitrageBanner storeId={STORE_ID} />)

    const link = screen.getByRole('link', { name: /Resolve/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/admin/settings/arbitrage')
  })

  it('does not contain urgency language when flagged', () => {
    mockUseArbitrageFlag.mockReturnValue({
      flagged: true,
      isLoading: false,
      firstFlaggedAt: null,
      severity: 'material',
      latestAudit: null,
    })

    render(<ArbitrageBanner storeId={STORE_ID} />)

    const bannerText = screen.getByTestId('banner-shell').textContent ?? ''

    // No urgency language per editorial voice spec
    expect(bannerText).not.toMatch(/fraud/i)
    expect(bannerText).not.toMatch(/verify now/i)
    expect(bannerText).not.toMatch(/detected/i)
    expect(bannerText).not.toMatch(/immediately/i)
    expect(bannerText).not.toMatch(/!/i)
    // No emoji (any character in emoji unicode range)
    expect(bannerText).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })
})
