/**
 * Billing API client integration tests.
 *
 * MSW-backed: every test spins up a real MSW interceptor, exercises the
 * getSubscription / openPortal functions through the apiClient, and
 * verifies the response shape + error handling.
 *
 * Tests:
 *   1. GET subscription — happy path returns a valid CurrentPlan
 *   2. GET subscription — 402 subscription_inactive throws SubscriptionInactiveError
 *   3. GET subscription — 500 throws ApiError
 *   4. openPortal — happy path returns PortalResponse with url
 *   5. openPortal — 500 throws ApiError
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import { getSubscription, openPortal } from '@/lib/api/subscription/billing'
import { ApiError, SubscriptionInactiveError } from '@/lib/api/client'

// ---------------------------------------------------------------------------
// MSW server
// ---------------------------------------------------------------------------

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

// ---------------------------------------------------------------------------
// Wire fixtures
// ---------------------------------------------------------------------------

const STORE_ID = 'f47ac10b-58cc-4372-a567-0e02b2c3d479'
const SUBSCRIPTION_PATH = `/api/v1/admin/stores/${STORE_ID}/subscription`
const PORTAL_PATH = `/api/v1/admin/stores/${STORE_ID}/subscription/portal`

const SUBSCRIPTION_FIXTURE = {
  id: 'sub_1',
  store_id: STORE_ID,
  plan: 'growth',
  status: 'active',
  current_period_start: '2026-04-01T00:00:00Z',
  current_period_end: '2027-04-01T00:00:00Z',
  cancel_at_period_end: false,
  stripe_subscription_id: 'sub_stripe_1',
  created_at: '2025-01-01T00:00:00Z',
  arbitrage_flag: false,
  latest_arbitrage_audit: null,
}

const PORTAL_FIXTURE = {
  url: 'https://billing.stripe.com/session/test_session_id',
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('getSubscription', () => {
  it('happy path — returns a valid CurrentPlan', async () => {
    server.use(
      http.get(SUBSCRIPTION_PATH, () => HttpResponse.json(SUBSCRIPTION_FIXTURE)),
    )

    const plan = await getSubscription(STORE_ID)

    expect(plan.id).toBe('sub_1')
    expect(plan.storeId).toBe(STORE_ID)
    expect(plan.plan).toBe('growth')
    expect(plan.status).toBe('active')
    expect(plan.periodEnd).toBe('2027-04-01T00:00:00Z')
    expect(plan.cancelAtPeriodEnd).toBe(false)
    expect(plan.arbitrageFlag).toBe(false)
    expect(plan.billingCurrency).toBe('USD') // safe default
    expect(plan.addOns).toEqual([])
  })

  it('maps add_ons and billing_currency when present', async () => {
    server.use(
      http.get(SUBSCRIPTION_PATH, () =>
        HttpResponse.json({
          ...SUBSCRIPTION_FIXTURE,
          billing_currency: 'GBP',
          add_ons: ['white_label_app'],
        }),
      ),
    )

    const plan = await getSubscription(STORE_ID)
    expect(plan.billingCurrency).toBe('GBP')
    expect(plan.addOns).toContain('white_label_app')
  })

  it('maps payment method fields when present', async () => {
    server.use(
      http.get(SUBSCRIPTION_PATH, () =>
        HttpResponse.json({
          ...SUBSCRIPTION_FIXTURE,
          payment_method_brand: 'visa',
          payment_method_last4: '4242',
        }),
      ),
    )

    const plan = await getSubscription(STORE_ID)
    expect(plan.paymentMethodBrand).toBe('visa')
    expect(plan.paymentMethodLast4).toBe('4242')
  })

  it('402 subscription_inactive throws SubscriptionInactiveError', async () => {
    server.use(
      http.get(SUBSCRIPTION_PATH, () =>
        HttpResponse.json(
          { error: 'subscription_inactive', status: 'expired' },
          { status: 402 },
        ),
      ),
    )

    await expect(getSubscription(STORE_ID)).rejects.toBeInstanceOf(
      SubscriptionInactiveError,
    )
  })

  it('500 error throws ApiError', async () => {
    server.use(
      http.get(SUBSCRIPTION_PATH, () =>
        HttpResponse.json(
          { error: 'internal_error', message: 'database unavailable' },
          { status: 500 },
        ),
      ),
    )

    await expect(getSubscription(STORE_ID)).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && (err as ApiError).status === 500,
    )
  })

  // A 404 here is not a real error — the store simply has no
  // store_subscriptions row yet (pre-v2.3 signup, fresh test store).
  // Returning null lets the shell banner stack degrade to "no banner"
  // on every admin page instead of logging a red 404 in the browser
  // console each navigation. Regressing this would make every refresh
  // on /settings/shipping (and everywhere else) noisy again.
  it('404 resolves to null instead of throwing', async () => {
    server.use(
      http.get(SUBSCRIPTION_PATH, () =>
        HttpResponse.json(
          { error: 'not_found', message: 'subscription not configured' },
          { status: 404 },
        ),
      ),
    )

    await expect(getSubscription(STORE_ID)).resolves.toBeNull()
  })
})

describe('openPortal', () => {
  it('happy path — returns portal response with url', async () => {
    server.use(
      http.post(PORTAL_PATH, () => HttpResponse.json(PORTAL_FIXTURE)),
    )

    const result = await openPortal(STORE_ID)
    expect(result.url).toBe(PORTAL_FIXTURE.url)
  })

  it('sends return_url in the request body', async () => {
    let receivedBody: Record<string, unknown> | null = null
    server.use(
      http.post(PORTAL_PATH, async ({ request }) => {
        receivedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(PORTAL_FIXTURE)
      }),
    )

    await openPortal(STORE_ID)
    expect(receivedBody).toHaveProperty('return_url')
    expect(typeof receivedBody?.return_url).toBe('string')
  })

  it('500 error throws ApiError', async () => {
    server.use(
      http.post(PORTAL_PATH, () =>
        HttpResponse.json(
          { error: 'internal_error', message: 'stripe unavailable' },
          { status: 500 },
        ),
      ),
    )

    await expect(openPortal(STORE_ID)).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && (err as ApiError).status === 500,
    )
  })
})
