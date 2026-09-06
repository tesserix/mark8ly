/**
 * Promo-code redemption tests (#770).
 *
 * Two halves, and the second is the point of the issue:
 *
 *   1. MSW-backed transport — applyPromo's request shape, the parsed 200,
 *      and the 422 → PromoRejectedError translation carrying the reason.
 *   2. The copy mapping — every rejection reason renders a DISTINCT sentence
 *      a merchant can act on. A test that only checked the happy path would
 *      pass while all eight said "invalid or expired".
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import { applyPromo, PromoRejectedError } from '@/lib/api/subscription/promo'
import {
  PROMO_REJECT_REASONS,
  toPromoRejectReason,
  type PromoRejectReason,
} from '@/lib/api/subscription/schemas/promo'
import { promoRejectionMessage } from '@/lib/copy/promoRejection'
import { promoAppliedMessage } from '@/lib/copy/promoApplied'
import { ApiError } from '@/lib/api/client'

// ---------------------------------------------------------------------------
// MSW server
// ---------------------------------------------------------------------------

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const STORE_ID = 'f47ac10b-58cc-4372-a567-0e02b2c3d479'
const APPLY_PATH = `/api/admin/stores/${STORE_ID}/subscription/apply-promo`

const APPLIED_FIXTURE = {
  stripe_coupon_id: 'co_test_winback',
  effective_minor: 1520,
  currency: 'usd',
  percent_off_bps: 2000,
  max_duration_months: 6,
}

function rejection(reason: string) {
  return HttpResponse.json(
    {
      error: 'promo_invalid_or_expired',
      message: 'the promo code is invalid or has expired',
      reason,
    },
    { status: 422 },
  )
}

// ---------------------------------------------------------------------------
// applyPromo — transport
// ---------------------------------------------------------------------------

describe('applyPromo', () => {
  it('200 — parses the applied response', async () => {
    server.use(http.post(APPLY_PATH, () => HttpResponse.json(APPLIED_FIXTURE)))

    const result = await applyPromo(STORE_ID, 'WINBACK20OFF6MONTHS')

    expect(result.effective_minor).toBe(1520)
    expect(result.currency).toBe('usd')
    expect(result.percent_off_bps).toBe(2000)
    expect(result.max_duration_months).toBe(6)
  })

  // The per-email redemption cap is counted server-side against the
  // subscription's billing address. A body carrying an email would let a
  // merchant aim that cap at somebody else's account, so the request must
  // not have one — asserted against the wire, not against the module's own
  // types, because a type cannot stop an extra field being sent.
  it('sends only the code — never an email', async () => {
    let sent: unknown = null
    server.use(
      http.post(APPLY_PATH, async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json(APPLIED_FIXTURE)
      }),
    )

    await applyPromo(STORE_ID, 'WINBACK20OFF6MONTHS')

    expect(sent).toEqual({ code: 'WINBACK20OFF6MONTHS' })
  })

  it('422 — throws PromoRejectedError carrying the reason', async () => {
    server.use(http.post(APPLY_PATH, () => rejection('below_absolute_floor')))

    await expect(applyPromo(STORE_ID, 'HALFOFF')).rejects.toBeInstanceOf(
      PromoRejectedError,
    )

    const err = await applyPromo(STORE_ID, 'HALFOFF').catch((e) => e)
    expect(err.reason).toBe('below_absolute_floor')
  })

  it('422 with an unrecognised reason — falls back to invalid_or_expired', async () => {
    server.use(http.post(APPLY_PATH, () => rejection('reason_from_the_future')))

    const err = await applyPromo(STORE_ID, 'ANYCODE').catch((e) => e)
    expect(err).toBeInstanceOf(PromoRejectedError)
    expect(err.reason).toBe('invalid_or_expired')
  })

  // A 402 is the read-only-subscription gate, not a judgement on the code.
  // It must stay an ApiError so the panel says "we could not reach billing"
  // rather than telling the merchant their working code is invalid.
  it('non-422 failures stay ApiError, not PromoRejectedError', async () => {
    server.use(
      http.post(APPLY_PATH, () =>
        HttpResponse.json(
          { error: 'internal_error', message: 'boom' },
          { status: 500 },
        ),
      ),
    )

    const err = await applyPromo(STORE_ID, 'ANYCODE').catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).not.toBeInstanceOf(PromoRejectedError)
  })
})

describe('toPromoRejectReason', () => {
  it('narrows anything unrecognised to invalid_or_expired', () => {
    expect(toPromoRejectReason(undefined)).toBe('invalid_or_expired')
    expect(toPromoRejectReason('')).toBe('invalid_or_expired')
    expect(toPromoRejectReason(42)).toBe('invalid_or_expired')
    expect(toPromoRejectReason('wrong_plan')).toBe('wrong_plan')
  })

  // not_found and expired are merged SERVER-side and must never appear as
  // client reasons. A client that knew them would be one copy change away
  // from telling a code-guesser which strings are real.
  it('does not recognise the server-internal not_found / expired reasons', () => {
    expect(toPromoRejectReason('not_found')).toBe('invalid_or_expired')
    expect(toPromoRejectReason('expired')).toBe('invalid_or_expired')
  })
})

// ---------------------------------------------------------------------------
// The copy mapping — this is the feature
// ---------------------------------------------------------------------------

const CTX = { plan: 'starter' }

describe('promoRejectionMessage', () => {
  it.each(PROMO_REJECT_REASONS)('%s renders a non-empty sentence', (reason) => {
    const message = promoRejectionMessage(reason, CTX)
    expect(message.length).toBeGreaterThan(0)
  })

  // The mutation guard. Getting these sentences right IS the issue: two
  // reasons sharing one string puts the merchant back where #770 found
  // them, told something that does not describe their situation.
  it('renders a DISTINCT sentence for every reason', () => {
    const seen = new Map<string, PromoRejectReason>()
    for (const reason of PROMO_REJECT_REASONS) {
      const message = promoRejectionMessage(reason, CTX)
      const previous = seen.get(message)
      expect(
        previous,
        `${previous} and ${reason} render the same sentence — a merchant cannot tell what happened`,
      ).toBeUndefined()
      seen.set(message, reason)
    }
    expect(seen.size).toBe(PROMO_REJECT_REASONS.length)
  })

  // The five reasons where the code is FINE. Telling a merchant their code
  // is "invalid or expired" for any of these sends them to support holding
  // a code that works — the exact failure #770 was filed for.
  it.each([
    'max_redemptions_reached',
    'max_per_email_reached',
    'wrong_plan',
    'annual_only',
    'below_absolute_floor',
  ] as const)('%s never claims the code is invalid or expired', (reason) => {
    const message = promoRejectionMessage(reason, CTX).toLowerCase()
    expect(message).not.toContain('invalid')
    expect(message).not.toContain('expired')
  })

  // The two actionable ones must name the change that would work, or the
  // merchant is told what failed and not what to do about it.
  it('wrong_plan names the merchant plan and points at changing it', () => {
    const message = promoRejectionMessage('wrong_plan', { plan: 'starter' })
    expect(message).toContain('starter')
    expect(message.toLowerCase()).toContain('change plan')
  })

  it('annual_only names annual billing as the fix', () => {
    const message = promoRejectionMessage('annual_only', CTX).toLowerCase()
    expect(message).toContain('annual')
  })

  // below_absolute_floor is our pricing rule biting, not the merchant's
  // mistake, and unknown_discount_type is our own bad data. Neither may
  // read as something they did wrong.
  it('below_absolute_floor says the code is valid', () => {
    const message = promoRejectionMessage('below_absolute_floor', CTX)
    // "is valid", not "valid" — the latter is satisfied by the word
    // "invalid", which is the sentence this reason must never render.
    expect(message.toLowerCase()).toContain('is valid')
    expect(message).toContain('starter')
  })

  it('unknown_discount_type blames our side, not the merchant', () => {
    const message = promoRejectionMessage('unknown_discount_type', CTX).toLowerCase()
    expect(message).toContain('our side')
  })
})

// ---------------------------------------------------------------------------
// The confirmation sentence
// ---------------------------------------------------------------------------

describe('promoAppliedMessage', () => {
  it('states the discount, its duration and the new price when the response carries all three', () => {
    const message = promoAppliedMessage(APPLIED_FIXTURE)
    expect(message).toContain('20%')
    expect(message).toContain('6 months')
    // The amount, not the symbol: Intl renders USD as "$15.20" or
    // "USD 15.20" depending on the ambient locale, and jsdom has none.
    expect(message).toContain('15.20')
    expect(message).toContain('next invoice')
  })

  // max_duration_months: 0 means the row states no bound. Rendering it as a
  // duration would tell the merchant the discount lasts zero months.
  it('states no duration when the response gives no month bound', () => {
    const message = promoAppliedMessage({
      ...APPLIED_FIXTURE,
      max_duration_months: 0,
    })
    expect(message).toContain('15.20')
    expect(message).not.toContain('month')
  })

  // A flat-amount code has no percentage to quote.
  it('states no percentage for a flat-amount code', () => {
    const message = promoAppliedMessage({
      ...APPLIED_FIXTURE,
      percent_off_bps: 0,
    })
    expect(message).toContain('15.20')
    expect(message).not.toContain('%')
  })

  // Without a currency the amount cannot be formatted. Defaulting to USD is
  // exactly the bug this avoids: an INR price printed with a dollar sign.
  it('quotes no price at all when the response carries no currency', () => {
    const message = promoAppliedMessage({ ...APPLIED_FIXTURE, currency: '' })
    expect(message).not.toContain('15.20')
    expect(message).not.toContain('USD')
    expect(message).toContain('next invoice')
  })

  it('formats a zero-decimal currency without inventing cents', () => {
    const message = promoAppliedMessage({
      ...APPLIED_FIXTURE,
      effective_minor: 1520,
      currency: 'jpy',
    })
    expect(message).toContain('1,520')
    expect(message).not.toContain('15.20')
  })

  it('never claims the discount applies today', () => {
    const message = promoAppliedMessage(APPLIED_FIXTURE).toLowerCase()
    expect(message).not.toContain('immediately')
    expect(message).not.toContain('today')
    expect(message).toContain('next invoice')
  })
})
