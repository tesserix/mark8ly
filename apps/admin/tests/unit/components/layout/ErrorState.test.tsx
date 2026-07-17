/**
 * ErrorState unit tests — Vitest + Testing Library.
 *
 * Covers the version-skew branch: a tab left open across a deploy fires a
 * Server Action ID the running build no longer knows, and Next.js throws
 * UnrecognizedActionError into the route boundary. `reset` re-renders the
 * same stale bundle, so the boundary must offer a reload instead.
 *
 * Tests:
 *   1. Skew error → reload button, and clicking it reloads.
 *   2. Skew copy overrides caller-supplied title/description.
 *   3. Ordinary errors keep "Try again" wired to reset.
 *   4. Ordinary errors still surface their raw message.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'
import { ErrorState } from '@/components/layout/ErrorState'

// Mirrors the class Next.js throws from its server-action reducer.
class UnrecognizedActionError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'UnrecognizedActionError'
  }
}

const SKEW_ERROR = new UnrecognizedActionError(
  'Server Action "602eae52dda5537aa2ea4b262281513310dff44a46" was not found on the server.',
)

const reload = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { ...window.location, reload },
  })
})

describe('ErrorState — version skew', () => {
  it('offers a reload and triggers it on click', () => {
    render(<ErrorState error={SKEW_ERROR} onRetry={vi.fn()} />)

    const button = screen.getByRole('button', { name: /reload page/i })
    fireEvent.click(button)

    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('does not offer the dead "Try again" path', () => {
    const onRetry = vi.fn()
    render(<ErrorState error={SKEW_ERROR} onRetry={onRetry} />)

    expect(screen.queryByRole('button', { name: /try again/i })).toBeNull()
    expect(onRetry).not.toHaveBeenCalled()
  })

  it('overrides caller copy that would misdescribe the failure', () => {
    render(
      <ErrorState
        error={SKEW_ERROR}
        title="We couldn't load these settings"
        description="Something went wrong while fetching your store configuration."
      />,
    )

    expect(screen.queryByText(/couldn't load these settings/i)).toBeNull()
    expect(screen.getByText(/running an older version/i)).toBeTruthy()
  })

  it('hides the raw action-id message from merchants', () => {
    render(<ErrorState error={SKEW_ERROR} />)

    expect(screen.queryByText(/602eae52/)).toBeNull()
  })
})

describe('ErrorState — ordinary errors', () => {
  const PLAIN_ERROR = Object.assign(new Error('Upstream timed out'), {
    digest: 'abc123',
  })

  it('keeps "Try again" wired to reset', () => {
    const onRetry = vi.fn()
    render(<ErrorState error={PLAIN_ERROR} onRetry={onRetry} />)

    fireEvent.click(screen.getByRole('button', { name: /try again/i }))

    expect(onRetry).toHaveBeenCalledTimes(1)
    expect(reload).not.toHaveBeenCalled()
  })

  it('surfaces the raw message and caller copy unchanged', () => {
    render(
      <ErrorState error={PLAIN_ERROR} title="We couldn't load these settings" />,
    )

    expect(screen.getByText('Upstream timed out')).toBeTruthy()
    expect(screen.getByText(/couldn't load these settings/i)).toBeTruthy()
  })
})
