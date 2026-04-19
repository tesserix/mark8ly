/**
 * EmailRampVisualizer unit tests.
 *
 * Tests:
 *   1.  Renders all 4 milestones from the copy.
 *   2.  daysSinceSignup=5  → milestone 0 (Days 1–3) is current.
 *   3.  daysSinceSignup=20 → milestone 1 (Days 4–7) is current.
 *   4.  daysSinceSignup=45 → milestone 2 (Days 8–59) is current.
 *   5.  daysSinceSignup=75 → milestone 3 (Days 60–90) is current.
 *   6.  "Currently" badge appears exactly once.
 *   7.  The current milestone's <li> has aria-current="step".
 *   8.  Non-current milestones do NOT have aria-current.
 *
 * No hook mocking needed — EmailRampVisualizer is a pure presentational
 * component that reads only from subscriptionCopy (static import).
 */

import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { EmailRampVisualizer } from '@/components/shell/banners/EmailRampVisualizer'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderVisualizer(daysSinceSignup: number) {
  return render(<EmailRampVisualizer daysSinceSignup={daysSinceSignup} />)
}

/** Returns all milestone <li> elements inside the <ol role="list">. */
function getMilestoneItems(): HTMLElement[] {
  const list = screen.getByRole('list')
  return within(list).getAllByRole('listitem')
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('EmailRampVisualizer', () => {
  it('renders all 4 milestones', () => {
    renderVisualizer(0)

    const items = getMilestoneItems()
    expect(items).toHaveLength(4)
  })

  it('renders the heading and intro text', () => {
    renderVisualizer(0)

    expect(
      screen.getByText('Email sending limits during your trial'),
    ).toBeInTheDocument()

    expect(
      screen.getByText(
        /We raise your sending limit as you progress through the trial/i,
      ),
    ).toBeInTheDocument()
  })

  it('renders the footnote', () => {
    renderVisualizer(0)

    expect(
      screen.getByText(
        /After signup, sending limits take effect within 24 hours/i,
      ),
    ).toBeInTheDocument()
  })

  it('daysSinceSignup=5 → milestone 0 (Days 1–3) is current', () => {
    renderVisualizer(5)

    const items = getMilestoneItems()

    expect(items[0]).toHaveAttribute('aria-current', 'step')
    expect(items[1]).not.toHaveAttribute('aria-current')
    expect(items[2]).not.toHaveAttribute('aria-current')
    expect(items[3]).not.toHaveAttribute('aria-current')
  })

  it('daysSinceSignup=20 → milestone 1 (Days 4–7) is current', () => {
    renderVisualizer(20)

    const items = getMilestoneItems()

    expect(items[0]).not.toHaveAttribute('aria-current')
    expect(items[1]).toHaveAttribute('aria-current', 'step')
    expect(items[2]).not.toHaveAttribute('aria-current')
    expect(items[3]).not.toHaveAttribute('aria-current')
  })

  it('daysSinceSignup=45 → milestone 2 (Days 8–59) is current', () => {
    renderVisualizer(45)

    const items = getMilestoneItems()

    expect(items[0]).not.toHaveAttribute('aria-current')
    expect(items[1]).not.toHaveAttribute('aria-current')
    expect(items[2]).toHaveAttribute('aria-current', 'step')
    expect(items[3]).not.toHaveAttribute('aria-current')
  })

  it('daysSinceSignup=75 → milestone 3 (Days 60–90) is current', () => {
    renderVisualizer(75)

    const items = getMilestoneItems()

    expect(items[0]).not.toHaveAttribute('aria-current')
    expect(items[1]).not.toHaveAttribute('aria-current')
    expect(items[2]).not.toHaveAttribute('aria-current')
    expect(items[3]).toHaveAttribute('aria-current', 'step')
  })

  it('"Currently" badge appears exactly once', () => {
    renderVisualizer(45)

    const badges = screen.getAllByText('Currently')
    expect(badges).toHaveLength(1)
  })

  it('current milestone item has aria-current="step"', () => {
    renderVisualizer(75)

    const currentItem = screen.getByRole('listitem', { current: 'step' })
    expect(currentItem).toBeInTheDocument()
  })

  it('milestone labels are all rendered', () => {
    renderVisualizer(0)

    // Use precise regex anchors to avoid ambiguous multi-match.
    // Labels: "Days 1–14", "Days 15–30", "Days 31–60", "Days 61–90"
    expect(screen.getByText(/Days 1.14/)).toBeInTheDocument()
    expect(screen.getByText(/Days 15.30/)).toBeInTheDocument()
    expect(screen.getByText(/Days 31.60/)).toBeInTheDocument()
    expect(screen.getByText(/Days 61.90/)).toBeInTheDocument()
  })
})
