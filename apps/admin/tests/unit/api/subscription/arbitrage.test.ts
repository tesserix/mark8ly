/**
 * Arbitrage appeal API client tests.
 *
 * MSW-backed. Tests:
 *   1. Submit 200 — returns { data: { status: 'submitted' }, error: null }
 *   2. Submit 400 invalid_jurisdiction — throws ApiError with code
 *   3. Submit 404 no_open_flag — throws ApiError with code
 *   4. Zod rejects malformed response (missing data.status)
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import { submitArbitrageAppeal } from '@/lib/api/subscription/arbitrage'
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
const APPEAL_PATH = `/api/admin/stores/${STORE_ID}/arbitrage-appeal`

const VALID_BODY = {
  jurisdiction: 'GB',
  justification: 'We operate primarily from the UK but have EU customers.',
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('submitArbitrageAppeal', () => {
  it('200 — returns submitted status', async () => {
    server.use(
      http.post(APPEAL_PATH, () =>
        HttpResponse.json({ data: { status: 'submitted' }, error: null }),
      ),
    )

    const result = await submitArbitrageAppeal(STORE_ID, VALID_BODY)

    expect(result.data.status).toBe('submitted')
    expect(result.error).toBeNull()
  })

  it('200 — sends correct jurisdiction in request body', async () => {
    let received: Record<string, unknown> | null = null

    server.use(
      http.post(APPEAL_PATH, async ({ request }) => {
        received = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ data: { status: 'submitted' }, error: null })
      }),
    )

    await submitArbitrageAppeal(STORE_ID, VALID_BODY)

    expect(received).toHaveProperty('jurisdiction', 'GB')
    expect(received).toHaveProperty('justification')
  })

  it('400 arbitrage_appeal_invalid_jurisdiction — throws ApiError with code', async () => {
    server.use(
      http.post(APPEAL_PATH, () =>
        HttpResponse.json(
          {
            error: 'arbitrage_appeal_invalid_jurisdiction',
            message: 'jurisdiction must be a valid ISO-3166-1 alpha-2 code',
          },
          { status: 400 },
        ),
      ),
    )

    await expect(
      submitArbitrageAppeal(STORE_ID, VALID_BODY),
    ).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        (err as ApiError).code === 'arbitrage_appeal_invalid_jurisdiction',
    )
  })

  it('404 arbitrage_appeal_no_open_flag — throws ApiError with code', async () => {
    server.use(
      http.post(APPEAL_PATH, () =>
        HttpResponse.json(
          {
            error: 'arbitrage_appeal_no_open_flag',
            message: 'no open arbitrage flag found for this store',
          },
          { status: 404 },
        ),
      ),
    )

    await expect(
      submitArbitrageAppeal(STORE_ID, VALID_BODY),
    ).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        (err as ApiError).code === 'arbitrage_appeal_no_open_flag' &&
        (err as ApiError).status === 404,
    )
  })

  it('Zod rejects malformed response (missing data.status)', async () => {
    server.use(
      http.post(APPEAL_PATH, () =>
        // Missing data.status field — should fail Zod validation
        HttpResponse.json({ data: {}, error: null }),
      ),
    )

    await expect(
      submitArbitrageAppeal(STORE_ID, VALID_BODY),
    ).rejects.toThrow()
  })
})
