import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SubscriptionStatusBadge } from './SubscriptionStatusBadge'
import type { SubscriptionStatus } from './SubscriptionStatusBadge'

describe('SubscriptionStatusBadge', () => {
  it.each<[SubscriptionStatus, string, string]>([
    ['signup', 'Signup', 'neutral'],
    ['trialing', 'Trialing', 'neutral'],
    ['active', 'Active', 'success'],
    ['past_due', 'Past due', 'warning'],
    ['payment_action_required', 'Action required', 'warning'],
    ['cancel_scheduled', 'Ending', 'neutral'],
    ['expired', 'Expired', 'danger'],
    ['store_closed', 'Store closed', 'danger'],
    ['pending_hard_delete', 'Pending deletion', 'danger'],
  ])('renders %s as label "%s" with variant %s', (status, label, variant) => {
    render(<SubscriptionStatusBadge status={status} />)
    expect(screen.getByText(label)).toBeInTheDocument()
    expect(screen.getByTestId('status-badge')).toHaveAttribute('data-variant', variant)
  })

  it('uses moss only for active (editorial rule: one accent per view)', () => {
    render(<SubscriptionStatusBadge status="active" />)
    expect(screen.getByTestId('status-badge')).toHaveClass(/moss/)
  })

  it('never uses moss for warning/danger states', () => {
    render(<SubscriptionStatusBadge status="expired" />)
    expect(screen.getByTestId('status-badge')).not.toHaveClass(/moss/)
  })

  it('never uses moss for past_due (warning state)', () => {
    render(<SubscriptionStatusBadge status="past_due" />)
    expect(screen.getByTestId('status-badge')).not.toHaveClass(/moss/)
  })

  it('never uses moss for store_closed (danger state)', () => {
    render(<SubscriptionStatusBadge status="store_closed" />)
    expect(screen.getByTestId('status-badge')).not.toHaveClass(/moss/)
  })

  it('aria-label matches the visible label', () => {
    render(<SubscriptionStatusBadge status="active" />)
    const badge = screen.getByRole('status')
    expect(badge).toHaveAttribute('aria-label', 'Active')
  })

  it('aria-label matches for past_due', () => {
    render(<SubscriptionStatusBadge status="past_due" />)
    expect(screen.getByRole('status')).toHaveAttribute('aria-label', 'Past due')
  })

  it('aria-label matches for payment_action_required', () => {
    render(<SubscriptionStatusBadge status="payment_action_required" />)
    expect(screen.getByRole('status')).toHaveAttribute('aria-label', 'Action required')
  })

  it('accepts an optional className', () => {
    const { container } = render(
      <SubscriptionStatusBadge status="active" className="custom-class" />,
    )
    expect(container.firstChild).toHaveClass('custom-class')
  })
})
