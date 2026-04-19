/**
 * App credential upload API client tests.
 *
 * MSW-backed. Exercises uploadAppleCredentials and uploadGoogleCredentials
 * through the real fetch wrapper, verifying multipart dispatch, success (204),
 * and error propagation.
 *
 * Note on MSW + FormData: MSW intercepts the raw fetch. We verify the
 * request body was a FormData instance by checking Content-Type is
 * multipart/form-data (set automatically by the browser/node FormData).
 *
 * Tests:
 *   1. Apple 204 — resolves void
 *   2. Apple sends correct multipart fields
 *   3. Apple 400 invalid_p8_format — throws ApiError(400)
 *   4. Apple 403 add_on_not_active — throws ApiError(403)
 *   5. Google 204 — resolves void
 *   6. Google sends service_account_json field
 *   7. Google 400 invalid_service_account_json — throws ApiError(400)
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import {
  uploadAppleCredentials,
  uploadGoogleCredentials,
} from '@/lib/api/subscription/appCredentials'
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
const APPLE_PATH = `/api/v1/admin/stores/${STORE_ID}/app-credentials/apple`
const GOOGLE_PATH = `/api/v1/admin/stores/${STORE_ID}/app-credentials/google`

const APPLE_FIXTURE = {
  key_id: 'ABCDE12345',
  issuer_id: '57246542-96fe-1a63-e053-0824d011072a',
  p8_contents: '-----BEGIN PRIVATE KEY-----\nMIGTAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBHkwdwIBAQQg\n-----END PRIVATE KEY-----',
}

const GOOGLE_FIXTURE = {
  service_account_json: JSON.stringify({
    type: 'service_account',
    project_id: 'my-project',
    client_email: 'play-console@my-project.iam.gserviceaccount.com',
    private_key: '-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----',
  }),
}

// ---------------------------------------------------------------------------
// Apple tests
// ---------------------------------------------------------------------------

describe('uploadAppleCredentials', () => {
  it('204 — resolves void', async () => {
    server.use(
      http.post(APPLE_PATH, () => new HttpResponse(null, { status: 204 })),
    )

    await expect(uploadAppleCredentials(STORE_ID, APPLE_FIXTURE)).resolves.toBeUndefined()
  })

  it('sends multipart/form-data with issuer_id and key_id fields', async () => {
    let contentType: string | null = null
    let formData: FormData | null = null

    server.use(
      http.post(APPLE_PATH, async ({ request }) => {
        contentType = request.headers.get('content-type')
        formData = await request.formData()
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await uploadAppleCredentials(STORE_ID, APPLE_FIXTURE)

    // multipart/form-data with boundary
    expect(contentType).toMatch(/multipart\/form-data/)
    expect(formData?.get('issuer_id')).toBe(APPLE_FIXTURE.issuer_id)
    expect(formData?.get('key_id')).toBe(APPLE_FIXTURE.key_id)
    // p8 sent as a Blob file named 'key.p8'
    const p8Field = formData?.get('p8')
    expect(p8Field).not.toBeNull()
  })

  it('400 invalid_p8_format — throws ApiError(400)', async () => {
    server.use(
      http.post(APPLE_PATH, () =>
        HttpResponse.json(
          { error: 'invalid_p8_format' },
          { status: 400 },
        ),
      ),
    )

    await expect(uploadAppleCredentials(STORE_ID, APPLE_FIXTURE)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        err.code === 'invalid_p8_format' &&
        err.status === 400,
    )
  })

  it('403 add_on_not_active — throws ApiError(403)', async () => {
    server.use(
      http.post(APPLE_PATH, () =>
        HttpResponse.json(
          { error: 'add_on_not_active' },
          { status: 403 },
        ),
      ),
    )

    await expect(uploadAppleCredentials(STORE_ID, APPLE_FIXTURE)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        err.code === 'add_on_not_active' &&
        err.status === 403,
    )
  })
})

// ---------------------------------------------------------------------------
// Google tests
// ---------------------------------------------------------------------------

describe('uploadGoogleCredentials', () => {
  it('204 — resolves void', async () => {
    server.use(
      http.post(GOOGLE_PATH, () => new HttpResponse(null, { status: 204 })),
    )

    await expect(uploadGoogleCredentials(STORE_ID, GOOGLE_FIXTURE)).resolves.toBeUndefined()
  })

  it('sends service_account_json as a multipart file field', async () => {
    let formData: FormData | null = null

    server.use(
      http.post(GOOGLE_PATH, async ({ request }) => {
        formData = await request.formData()
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await uploadGoogleCredentials(STORE_ID, GOOGLE_FIXTURE)

    const field = formData?.get('service_account_json')
    expect(field).not.toBeNull()
  })

  it('400 invalid_service_account_json — throws ApiError(400)', async () => {
    server.use(
      http.post(GOOGLE_PATH, () =>
        HttpResponse.json(
          { error: 'invalid_service_account_json' },
          { status: 400 },
        ),
      ),
    )

    await expect(uploadGoogleCredentials(STORE_ID, GOOGLE_FIXTURE)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        err.code === 'invalid_service_account_json' &&
        err.status === 400,
    )
  })

  it('403 add_on_not_active — throws ApiError(403)', async () => {
    server.use(
      http.post(GOOGLE_PATH, () =>
        HttpResponse.json(
          { error: 'add_on_not_active' },
          { status: 403 },
        ),
      ),
    )

    await expect(uploadGoogleCredentials(STORE_ID, GOOGLE_FIXTURE)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError &&
        err.code === 'add_on_not_active' &&
        err.status === 403,
    )
  })
})
