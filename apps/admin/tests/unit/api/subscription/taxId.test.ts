/**
 * Tax-ID API client tests.
 *
 * MSW-backed. Tests:
 *   1. 200 validated — returns { data: { status: 'validated' }, error: null }
 *   2. 202 clock_paused (registry_unavailable) — returns { data: { status: 'registry_unavailable' } }
 *   3. 202 manual_review_queued — returns { data: { status: 'manual_review_queued' } }
 *   4. 400 invalid_format — throws ApiError with code 'invalid_format'
 *   5. 422 tax_id_not_found — throws ApiError with status 422
 *   6. Sends correct request body fields
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import { submitTaxId } from '@/lib/api/subscription/taxId'
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

const STORE_ID = 'a1b2c3d4-e5f6-7890-abcd-ef1234567890'
const SUBMIT_PATH = `/api/v1/admin/stores/${STORE_ID}/tax/submit`

const VALID_BODY = {
  country: 'GB',
  tax_id: 'GB123456789',
  business_name: 'Acme Ltd',
  billing_address: '123 High Street, London',
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('submitTaxId', () => {
  it('200 validated — returns validated status', async () => {
    server.use(
      http.post(SUBMIT_PATH, () =>
        HttpResponse.json({ data: { status: 'validated' }, error: null }),
      ),
    )

    const result = await submitTaxId(STORE_ID, VALID_BODY)

    expect(result.data.status).toBe('validated')
    expect(result.error).toBeNull()
  })

  it('202 registry_unavailable — returns registry_unavailable status', async () => {
    server.use(
      http.post(SUBMIT_PATH, () =>
        HttpResponse.json(
          {
            data: {
              status: 'registry_unavailable',
              message: 'Please retry; the validation window is paused while the registry is unreachable.',
            },
            error: null,
          },
          { status: 202 },
        ),
      ),
    )

    const result = await submitTaxId(STORE_ID, VALID_BODY)

    expect(result.data.status).toBe('registry_unavailable')
    expect(result.error).toBeNull()
  })

  it('202 manual_review_queued — returns manual_review_queued status with sla', async () => {
    server.use(
      http.post(SUBMIT_PATH, () =>
        HttpResponse.json(
          {
            data: {
              status: 'manual_review_queued',
              sla_business_days: 5,
            },
            error: null,
          },
          { status: 202 },
        ),
      ),
    )

    const result = await submitTaxId(STORE_ID, VALID_BODY)

    expect(result.data.status).toBe('manual_review_queued')
    if (result.data.status === 'manual_review_queued') {
      expect(result.data.sla_business_days).toBe(5)
    }
  })

  it('400 invalid_format — throws ApiError with code invalid_format', async () => {
    server.use(
      http.post(SUBMIT_PATH, () =>
        HttpResponse.json(
          { error: 'invalid_format' },
          { status: 400 },
        ),
      ),
    )

    await expect(submitTaxId(STORE_ID, VALID_BODY)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        (err as ApiError).code === 'invalid_format' &&
        (err as ApiError).status === 400,
    )
  })

  it('422 tax_id_not_found — throws ApiError with status 422', async () => {
    server.use(
      http.post(SUBMIT_PATH, () =>
        HttpResponse.json(
          { error: 'tax_id_not_found' },
          { status: 422 },
        ),
      ),
    )

    await expect(submitTaxId(STORE_ID, VALID_BODY)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        (err as ApiError).code === 'tax_id_not_found' &&
        (err as ApiError).status === 422,
    )
  })

  it('sends correct request body fields to the backend', async () => {
    let received: Record<string, unknown> | null = null

    server.use(
      http.post(SUBMIT_PATH, async ({ request }) => {
        received = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ data: { status: 'validated' }, error: null })
      }),
    )

    await submitTaxId(STORE_ID, VALID_BODY)

    expect(received).toHaveProperty('country', 'GB')
    expect(received).toHaveProperty('tax_id', 'GB123456789')
    expect(received).toHaveProperty('business_name', 'Acme Ltd')
    expect(received).toHaveProperty('billing_address', '123 High Street, London')
  })

  it('Zod rejects malformed response (unknown status)', async () => {
    server.use(
      http.post(SUBMIT_PATH, () =>
        // Unknown status not in the discriminated union
        HttpResponse.json({ data: { status: 'unknown_status_xyz' }, error: null }),
      ),
    )

    await expect(submitTaxId(STORE_ID, VALID_BODY)).rejects.toThrow()
  })
})
