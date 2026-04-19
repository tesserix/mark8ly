/**
 * Pro+App purchase API client tests.
 *
 * MSW-backed. Exercises the purchaseProApp function through the real apiClient
 * and verifies response parsing, error propagation, and Zod validation.
 *
 * Tests:
 *   1. 201 purchased — returns a valid PurchaseProAppResponse
 *   2. 201 includes invoice URL and amount_due_cents
 *   3. 400 apple_4_2_6_ack_required — throws ApiError with correct code
 *   4. 403 pro_plan_required — throws ApiError(403)
 *   5. 409 already_active — throws ApiError(409)
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import { purchaseProApp } from '@/lib/api/subscription/proApp'
import { ApiError } from '@/lib/api/client'

// ---------------------------------------------------------------------------
// MSW server
// ---------------------------------------------------------------------------

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const STORE_ID = 'f47ac10b-58cc-4372-a567-0e02b2c3d479'
const PURCHASE_PATH = `/api/v1/admin/stores/${STORE_ID}/subscription/add-on/white-label-app`

const VALID_REQUEST = {
  apple_4_2_6_acknowledged: true as const,
  attestation_text: 'Merchant acknowledged Apple App Store guideline 4.2.6 at purchase time.',
}

const PURCHASE_FIXTURE = {
  hosted_invoice_url: 'https://invoice.stripe.com/i/test_invoice_123',
  stripe_invoice_id: 'in_test_123',
  amount_due_cents: 150000,
  currency: 'usd',
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('purchaseProApp', () => {
  it('201 — returns a valid PurchaseProAppResponse', async () => {
    server.use(
      http.post(PURCHASE_PATH, () =>
        HttpResponse.json(PURCHASE_FIXTURE, { status: 201 }),
      ),
    )

    const result = await purchaseProApp(STORE_ID, VALID_REQUEST)

    expect(result.hosted_invoice_url).toBe(PURCHASE_FIXTURE.hosted_invoice_url)
    expect(result.stripe_invoice_id).toBe(PURCHASE_FIXTURE.stripe_invoice_id)
    expect(result.amount_due_cents).toBe(150000)
    expect(result.currency).toBe('usd')
  })

  it('sends the correct request body', async () => {
    let receivedBody: Record<string, unknown> | null = null

    server.use(
      http.post(PURCHASE_PATH, async ({ request }) => {
        receivedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(PURCHASE_FIXTURE, { status: 201 })
      }),
    )

    await purchaseProApp(STORE_ID, VALID_REQUEST)

    expect(receivedBody).toMatchObject({
      apple_4_2_6_acknowledged: true,
      attestation_text: VALID_REQUEST.attestation_text,
    })
  })

  it('400 apple_4_2_6_ack_required — throws ApiError with code and status 400', async () => {
    server.use(
      http.post(PURCHASE_PATH, () =>
        HttpResponse.json(
          { error: 'apple_4_2_6_ack_required' },
          { status: 400 },
        ),
      ),
    )

    await expect(purchaseProApp(STORE_ID, VALID_REQUEST)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        err.code === 'apple_4_2_6_ack_required' &&
        err.status === 400,
    )
  })

  it('403 pro_plan_required — throws ApiError(403)', async () => {
    server.use(
      http.post(PURCHASE_PATH, () =>
        HttpResponse.json(
          { error: 'pro_plan_required' },
          { status: 403 },
        ),
      ),
    )

    await expect(purchaseProApp(STORE_ID, VALID_REQUEST)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        err.code === 'pro_plan_required' &&
        err.status === 403,
    )
  })

  it('409 already_active — throws ApiError(409)', async () => {
    server.use(
      http.post(PURCHASE_PATH, () =>
        HttpResponse.json(
          { error: 'already_active' },
          { status: 409 },
        ),
      ),
    )

    await expect(purchaseProApp(STORE_ID, VALID_REQUEST)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        err.code === 'already_active' &&
        err.status === 409,
    )
  })

  it('500 subscription_inactive — throws ApiError(500)', async () => {
    server.use(
      http.post(PURCHASE_PATH, () =>
        HttpResponse.json(
          { error: 'stripe_invoice_failed' },
          { status: 500 },
        ),
      ),
    )

    await expect(purchaseProApp(STORE_ID, VALID_REQUEST)).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && err.status === 500,
    )
  })
})
