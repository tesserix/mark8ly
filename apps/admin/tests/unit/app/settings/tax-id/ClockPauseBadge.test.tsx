/**
 * ClockPauseBadge unit tests — Vitest + Testing Library.
 *
 * Tests:
 *   1. registry_unavailable variant renders "Clock paused: registry"
 *   2. sea_review variant renders "Clock paused: review queue"
 *   3. Both variants use role="status" (info tone, never alert)
 *   4. Neither variant contains warning/danger/urgency language
 *   5. Moss text class applied (info tone, not warning)
 */

import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

import { ClockPauseBadge } from '@/app/(admin)/settings/tax-id/ClockPauseBadge'

describe('ClockPauseBadge', () => {
  it('registry_unavailable variant renders correct label', () => {
    render(<ClockPauseBadge reason="registry_unavailable" />)

    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText('Clock paused: registry')).toBeInTheDocument()
  })

  it('sea_review variant renders correct label', () => {
    render(<ClockPauseBadge reason="sea_review" />)

    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText('Clock paused: review queue')).toBeInTheDocument()
  })

  it('registry_unavailable uses role=status, not role=alert (info tone, not error)', () => {
    render(<ClockPauseBadge reason="registry_unavailable" />)

    // Must be status (info), never alert (error)
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('sea_review uses role=status, not role=alert', () => {
    render(<ClockPauseBadge reason="sea_review" />)

    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('registry_unavailable badge does not contain urgency language', () => {
    const { container } = render(<ClockPauseBadge reason="registry_unavailable" />)
    const text = container.textContent ?? ''

    expect(text).not.toMatch(/error/i)
    expect(text).not.toMatch(/fail/i)
    expect(text).not.toMatch(/problem/i)
    expect(text).not.toMatch(/warning/i)
    expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })

  it('sea_review badge does not contain urgency language', () => {
    const { container } = render(<ClockPauseBadge reason="sea_review" />)
    const text = container.textContent ?? ''

    expect(text).not.toMatch(/error/i)
    expect(text).not.toMatch(/fail/i)
    expect(text).not.toMatch(/warning/i)
    expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })

  it('registry_unavailable applies moss text class (info tone)', () => {
    render(<ClockPauseBadge reason="registry_unavailable" />)

    const badge = screen.getByRole('status')
    // Moss text + background = info tone (not warning/danger)
    expect(badge.className).toContain('text-[var(--moss-700)]')
    expect(badge.className).toContain('bg-[var(--paper-200)]')
  })

  it('sea_review applies moss text class (info tone)', () => {
    render(<ClockPauseBadge reason="sea_review" />)

    const badge = screen.getByRole('status')
    expect(badge.className).toContain('text-[var(--moss-700)]')
    expect(badge.className).toContain('bg-[var(--paper-200)]')
  })
})
