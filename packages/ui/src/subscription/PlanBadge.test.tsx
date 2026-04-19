import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PlanBadge } from './PlanBadge'

describe('PlanBadge', () => {
  it.each([
    ['starter', 'Starter'],
    ['studio', 'Studio'],
    ['pro', 'Pro'],
  ])('renders %s plan as %s', (plan, label) => {
    render(<PlanBadge plan={plan as 'starter' | 'studio' | 'pro'} />)
    expect(screen.getByText(label)).toBeInTheDocument()
  })

  it('renders trial plan as Trial', () => {
    render(<PlanBadge plan="trial" />)
    expect(screen.getByText('Trial')).toBeInTheDocument()
  })

  it('appends +White-label App when addon present', () => {
    render(<PlanBadge plan="studio" addon="white_label_app" />)
    expect(screen.getByText('Studio')).toBeInTheDocument()
    expect(screen.getByText(/White-label App/)).toBeInTheDocument()
  })

  it('aria-label includes the add-on suffix', () => {
    render(<PlanBadge plan="studio" addon="white_label_app" />)
    const badge = screen.getByRole('status')
    expect(badge).toHaveAttribute('aria-label', 'Studio · +White-label App')
  })

  it('uses moss accent only for Pro', () => {
    const { container } = render(<PlanBadge plan="pro" />)
    expect(container.firstChild).toHaveClass(/moss/)
  })

  it('does not use moss for non-Pro plans', () => {
    const { container } = render(<PlanBadge plan="studio" />)
    expect(container.firstChild).not.toHaveClass(/moss/)
  })

  it('is a semantic element with role="status" for screen readers', () => {
    render(<PlanBadge plan="studio" />)
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('falls back to the literal plan string for unknown plans without crashing', () => {
    render(<PlanBadge plan={'enterprise' as 'pro'} />)
    expect(screen.getByText('enterprise')).toBeInTheDocument()
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('accepts an optional className', () => {
    const { container } = render(<PlanBadge plan="starter" className="custom-class" />)
    expect(container.firstChild).toHaveClass('custom-class')
  })
})
