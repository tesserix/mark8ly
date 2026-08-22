/**
 * Cancellation API client tests.
 *
 * MSW-backed: exercises submitCancellation and revertCancellation through
 * the real apiClient.
 *
 * Tests:
 *   1. submitCancellation — 200 happy path (cancel_scheduled)
 *   2. submitCancellation — 422 bad_reason surfaces ApiError
 *   3. revertCancellation — 200 happy path (active + save_offer_msg)
 *   4. revertCancellation — 409 save_offer_already_accepted surfaces ApiError
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import { submitCancellation, revertCancellation } from '@/lib/api/subscription/cancellation'
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
const CANCEL_PATH = `/api/admin/stores/${STORE_ID}/subscription/cancel`

const CANCEL_SCHEDULED_FIXTURE = {
  status: 'cancel_scheduled',
  cancels_at: '2026-05-01T00:00:00Z',
}

const SAVE_OFFER_ACCEPTED_FIXTURE = {
  status: 'active',
  cancels_at: '',
  save_offer_msg: 'Save offer accepted. A 20% discount applies to your next billing cycle.',
}

// ---------------------------------------------------------------------------
// submitCancellation
// ---------------------------------------------------------------------------

describe('submitCancellation', () => {
  it('200 — returns cancel_scheduled response', async () => {
    server.use(
      http.post(CANCEL_PATH, () => HttpResponse.json(CANCEL_SCHEDULED_FIXTURE)),
    )

    const result = await submitCancellation(STORE_ID, {
      survey_reason: 'too_expensive',
      accept_save_offer: false,
    })

    expect(result.status).toBe('cancel_scheduled')
    expect(result.cancels_at).toBe('2026-05-01T00:00:00Z')
    expect(result.save_offer_msg).toBeUndefined()
  })

  it('200 — works with no body fields (all optional)', async () => {
    server.use(
      http.post(CANCEL_PATH, () => HttpResponse.json(CANCEL_SCHEDULED_FIXTURE)),
    )

    const result = await submitCancellation(STORE_ID, {})
    expect(result.status).toBe('cancel_scheduled')
  })

  it('422 — throws ApiError with status 422', async () => {
    // Use a valid-length reason so Zod passes; the server returns 422
    // (simulating a backend validation failure such as an unknown reason code).
    server.use(
      http.post(CANCEL_PATH, () =>
        HttpResponse.json(
          { error: 'validation_failed', message: 'unknown reason code' },
          { status: 422 },
        ),
      ),
    )

    await expect(
      submitCancellation(STORE_ID, { survey_reason: 'unknown_reason_code' }),
    ).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && (err as ApiError).status === 422,
    )
  })
})

// ---------------------------------------------------------------------------
// revertCancellation
// ---------------------------------------------------------------------------

describe('revertCancellation', () => {
  it('200 — returns active status with save_offer_msg', async () => {
    server.use(
      http.post(CANCEL_PATH, () => HttpResponse.json(SAVE_OFFER_ACCEPTED_FIXTURE)),
    )

    const result = await revertCancellation(STORE_ID)

    expect(result.status).toBe('active')
    expect(result.cancels_at).toBe('')
    expect(result.save_offer_msg).toContain('Save offer accepted')
  })

  it('sends accept_save_offer: true in request body', async () => {
    let receivedBody: Record<string, unknown> | null = null

    server.use(
      http.post(CANCEL_PATH, async ({ request }) => {
        receivedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(SAVE_OFFER_ACCEPTED_FIXTURE)
      }),
    )

    await revertCancellation(STORE_ID)
    expect(receivedBody).toEqual({ accept_save_offer: true })
  })

  it('409 save_offer_already_accepted — throws ApiError with status 409', async () => {
    server.use(
      http.post(CANCEL_PATH, () =>
        HttpResponse.json(
          { error: 'save_offer_already_accepted' },
          { status: 409 },
        ),
      ),
    )

    await expect(revertCancellation(STORE_ID)).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && (err as ApiError).status === 409,
    )
  })
})
