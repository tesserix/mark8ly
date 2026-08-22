/**
 * Attestation API client tests.
 *
 * MSW-backed. Tests:
 *   1. 201 signed with US jurisdiction — returns { data: { status: 'signed' } }
 *   2. 201 signed with CA jurisdiction — returns { data: { status: 'signed' } }
 *   3. 400 invalid_request (country not US/CA) — throws ApiError
 *   4. Sends checkbox_text + checkbox_version in request body
 *   5. Zod rejects malformed response
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import { signAttestation } from '@/lib/api/subscription/attestation'
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

const STORE_ID = 'b2c3d4e5-f6a7-8901-bcde-f12345678901'
const ATTESTATION_PATH = `/api/v1/admin/stores/${STORE_ID}/tax/attestation`

const US_BODY = {
  country: 'US' as const,
  checkbox_text:
    'I attest that this store is operated by a registered business entity in my selected jurisdiction.',
  checkbox_version: 'v1',
}

const CA_BODY = {
  country: 'CA' as const,
  checkbox_text:
    'I attest that this store is operated by a registered business entity in my selected jurisdiction.',
  checkbox_version: 'v1',
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('signAttestation', () => {
  it('201 signed with US jurisdiction — returns signed status', async () => {
    server.use(
      http.post(ATTESTATION_PATH, () =>
        HttpResponse.json(
          { data: { status: 'signed' }, error: null },
          { status: 201 },
        ),
      ),
    )

    const result = await signAttestation(STORE_ID, US_BODY)

    expect(result.data.status).toBe('signed')
    expect(result.error).toBeNull()
  })

  it('201 signed with CA jurisdiction — returns signed status', async () => {
    server.use(
      http.post(ATTESTATION_PATH, () =>
        HttpResponse.json(
          { data: { status: 'signed' }, error: null },
          { status: 201 },
        ),
      ),
    )

    const result = await signAttestation(STORE_ID, CA_BODY)

    expect(result.data.status).toBe('signed')
  })

  it('400 invalid_request — throws ApiError when country not US/CA', async () => {
    server.use(
      http.post(ATTESTATION_PATH, () =>
        HttpResponse.json(
          {
            error: 'invalid_request',
            message: 'Key: country - failed on the oneof=US CA tag',
          },
          { status: 400 },
        ),
      ),
    )

    await expect(signAttestation(STORE_ID, US_BODY)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        (err as ApiError).code === 'invalid_request' &&
        (err as ApiError).status === 400,
    )
  })

  it('sends checkbox_text and checkbox_version in request body', async () => {
    let received: Record<string, unknown> | null = null

    server.use(
      http.post(ATTESTATION_PATH, async ({ request }) => {
        received = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(
          { data: { status: 'signed' }, error: null },
          { status: 201 },
        )
      }),
    )

    await signAttestation(STORE_ID, US_BODY)

    expect(received).toHaveProperty('country', 'US')
    expect(received).toHaveProperty(
      'checkbox_text',
      'I attest that this store is operated by a registered business entity in my selected jurisdiction.',
    )
    expect(received).toHaveProperty('checkbox_version', 'v1')
  })

  it('Zod rejects malformed response (missing data.status)', async () => {
    server.use(
      http.post(ATTESTATION_PATH, () =>
        HttpResponse.json({ data: {}, error: null }, { status: 201 }),
      ),
    )

    await expect(signAttestation(STORE_ID, US_BODY)).rejects.toThrow()
  })
})
