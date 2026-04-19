import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Money } from './Money'

describe('Money', () => {
  it('renders USD with cents as two decimals', () => {
    render(<Money amount={1188_00} currency="USD" />)
    expect(screen.getByText('$1,188.00')).toBeInTheDocument()
  })

  it('renders CAD explicitly with the CA$ symbol (success #42)', () => {
    render(<Money amount={1498_00} currency="CAD" />)
    expect(screen.getByText('CA$1,498.00')).toBeInTheDocument()
  })

  it('renders INR with ₹ symbol and no thousands separator in Indian grouping', () => {
    render(<Money amount={9900_00} currency="INR" />)
    expect(screen.getByText('₹9,900.00')).toBeInTheDocument()
  })

  it('accepts a locale override', () => {
    render(<Money amount={1188_00} currency="EUR" locale="de-DE" />)
    expect(screen.getByText(/1\.188,00/)).toBeInTheDocument()
  })

  it('handles zero', () => {
    render(<Money amount={0} currency="USD" />)
    expect(screen.getByText('$0.00')).toBeInTheDocument()
  })

  it('strips cents when showCents=false', () => {
    render(<Money amount={1188_00} currency="USD" showCents={false} />)
    expect(screen.getByText('$1,188')).toBeInTheDocument()
  })
})
