/**
 * BannerShell unit tests.
 *
 * Tests:
 *  1. Renders heading and body text.
 *  2. Dismiss button calls onDismiss when dismissible.
 *  3. Warning tone applies data-tone="warning".
 *  4. Danger tone applies data-tone="danger".
 *  5. Info tone applies data-tone="info".
 *  6. CTA button triggers onClick when no href is provided.
 *  7. CTA renders as an anchor when href is provided.
 *  8. Dismiss button has accessible aria-label "Dismiss".
 *  9. No dismiss button when dismissible is false (default).
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BannerShell } from '@/components/shell/banners/BannerShell'

describe('BannerShell', () => {
  it('renders heading and body text', () => {
    render(
      <BannerShell
        tone="info"
        heading="Your bank needs to confirm this payment"
        body="Review the invoice to complete authentication."
      />,
    )
    expect(
      screen.getByText('Your bank needs to confirm this payment'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('Review the invoice to complete authentication.'),
    ).toBeInTheDocument()
  })

  it('calls onDismiss when dismiss button is clicked', async () => {
    const onDismiss = vi.fn()
    render(
      <BannerShell
        tone="warning"
        heading="Heading"
        body="Body"
        dismissible
        onDismiss={onDismiss}
      />,
    )
    const dismissBtn = screen.getByRole('button', { name: 'Dismiss' })
    await userEvent.click(dismissBtn)
    expect(onDismiss).toHaveBeenCalledOnce()
  })

  it('applies data-tone="warning" for warning tone', () => {
    render(<BannerShell tone="warning" heading="H" body="B" />)
    expect(screen.getByTestId('banner-shell')).toHaveAttribute(
      'data-tone',
      'warning',
    )
  })

  it('applies data-tone="danger" for danger tone', () => {
    render(<BannerShell tone="danger" heading="H" body="B" />)
    expect(screen.getByTestId('banner-shell')).toHaveAttribute(
      'data-tone',
      'danger',
    )
  })

  it('applies data-tone="info" for info tone', () => {
    render(<BannerShell tone="info" heading="H" body="B" />)
    expect(screen.getByTestId('banner-shell')).toHaveAttribute(
      'data-tone',
      'info',
    )
  })

  it('renders CTA button and calls onClick', async () => {
    const onClick = vi.fn()
    render(
      <BannerShell
        tone="info"
        heading="H"
        body="B"
        cta={{ label: 'Review the invoice', onClick }}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Review the invoice' }))
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('renders CTA as an anchor when href is provided', () => {
    render(
      <BannerShell
        tone="info"
        heading="H"
        body="B"
        cta={{ label: 'Review the invoice', href: 'https://stripe.com/inv' }}
      />,
    )
    const link = screen.getByRole('link', { name: 'Review the invoice' })
    expect(link).toHaveAttribute('href', 'https://stripe.com/inv')
  })

  it('dismiss button has aria-label "Dismiss"', () => {
    render(
      <BannerShell
        tone="warning"
        heading="H"
        body="B"
        dismissible
        onDismiss={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: 'Dismiss' })).toBeInTheDocument()
  })

  it('does not render dismiss button when dismissible is false', () => {
    render(<BannerShell tone="info" heading="H" body="B" />)
    expect(
      screen.queryByRole('button', { name: 'Dismiss' }),
    ).not.toBeInTheDocument()
  })
})
