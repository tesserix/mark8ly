/**
 * ImageLimitGrandfatheredBadge unit tests — Vitest + Testing Library.
 *
 * Tests:
 *   1. imageCount <= planLimit → returns null.
 *   2. imageCount > planLimit → renders badge with expected text.
 *   3. Tooltip content correct on click.
 *   4. No urgency language or warning tone.
 */

import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'
import { ImageLimitGrandfatheredBadge } from '@/components/products/ImageLimitGrandfatheredBadge'

const PRODUCT_ID = 'prod-test-001'

describe('ImageLimitGrandfatheredBadge', () => {
  it('returns null when imageCount equals planLimit', () => {
    const { container } = render(
      <ImageLimitGrandfatheredBadge
        productId={PRODUCT_ID}
        imageCount={5}
        planLimit={5}
      />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('returns null when imageCount is below planLimit', () => {
    const { container } = render(
      <ImageLimitGrandfatheredBadge
        productId={PRODUCT_ID}
        imageCount={3}
        planLimit={5}
      />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders badge text when imageCount exceeds planLimit', () => {
    render(
      <ImageLimitGrandfatheredBadge
        productId={PRODUCT_ID}
        imageCount={8}
        planLimit={5}
      />,
    )

    const badge = screen.getByTestId('grandfathered-badge')
    expect(badge).toBeInTheDocument()
    expect(badge).toHaveTextContent('Grandfathered: 8 images (plan cap 5)')
  })

  it('shows tooltip with correct explanation on click', () => {
    render(
      <ImageLimitGrandfatheredBadge
        productId={PRODUCT_ID}
        imageCount={10}
        planLimit={6}
      />,
    )

    // Tooltip not visible initially
    expect(screen.queryByTestId('grandfathered-tooltip')).not.toBeInTheDocument()

    // Click to reveal
    fireEvent.click(screen.getByTestId('grandfathered-badge'))

    const tooltip = screen.getByTestId('grandfathered-tooltip')
    expect(tooltip).toBeInTheDocument()
    expect(tooltip).toHaveAttribute('role', 'tooltip')
    expect(tooltip).toHaveTextContent(
      'Kept from your previous plan',
    )
    expect(tooltip).toHaveTextContent(
      'cannot add more until you are under the cap',
    )
  })

  it('does not contain urgency language or warning tone', () => {
    render(
      <ImageLimitGrandfatheredBadge
        productId={PRODUCT_ID}
        imageCount={9}
        planLimit={5}
      />,
    )

    // Open the tooltip so all text is rendered
    fireEvent.click(screen.getByTestId('grandfathered-badge'))

    const text = document.body.textContent ?? ''
    expect(text).not.toMatch(/warning/i)
    expect(text).not.toMatch(/urgent/i)
    expect(text).not.toMatch(/!/g)
    expect(text).not.toMatch(/[\u{1F300}-\u{1FAFF}]/u)
  })
})
