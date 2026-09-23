/**
 * Cancellation copy — the final step reads back a date the backend may not
 * have. `cancels_at` is the end of the period Stripe is billing, and a
 * subscription Stripe never billed (a trial that added no card) has none, so
 * FinalConfirmStep passes an empty string through.
 */

import { describe, it, expect } from 'vitest'

import { cancellationCopy } from '@/lib/copy/cancellation'

describe('cancellationCopy.finalStep.body', () => {
  it('names the date when there is one', () => {
    const text = cancellationCopy.finalStep.body('12 October 2026')

    expect(text).toContain('Your plan ends on 12 October 2026.')
    expect(text).toContain("You'll keep full access until then.")
  })

  it('still reads as a sentence when no date is known', () => {
    const text = cancellationCopy.finalStep.body('')

    expect(text).not.toContain('ends on .')
    expect(text).toContain('end of your current billing period')
    expect(text).toContain("You'll keep full access until then.")
  })
})
