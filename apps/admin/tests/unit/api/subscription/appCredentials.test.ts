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
 * Note on the FormData/Blob/File globals (see #857): this suite runs in the
 * jsdom environment, which installs jsdom's own FormData, Blob and File over
 * the Node built-ins. Node's bundled undici -- which implements fetch, and so
 * both serialises and parses the multipart body -- constructs file parts with
 * `new File(...)` off the live global but brand-checks them against the `File`
 * it captured at its own module init, the built-in one. Under jsdom those are
 * two different classes, with two consequences: serialising a jsdom File loses
 * its filename (it degrades to a plain Blob part named "blob"), and parsing
 * the body back fails an internal assertion outright, so
 * `request.formData()` throws.
 *
 * Neither is MSW's doing and neither is a product defect: a bare
 * `new Request(url, { method: 'POST', body: fd }).formData()` throws the same
 * assertion under jsdom with no MSW in the picture, and succeeds once the
 * built-ins are restored. Browsers use the native FormData/Blob/File, so
 * restoring them here makes this file exercise the same primitives production
 * does. jsdom is still what resolves the relative request URLs the API client
 * sends, so the environment stays jsdom; only these three globals are swapped,
 * and they are put back afterwards.
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
import { Blob as NodeBlob, File as NodeFile } from 'node:buffer'

import {
  uploadAppleCredentials,
  uploadGoogleCredentials,
} from '@/lib/api/subscription/appCredentials'
import { ApiError } from '@/lib/api/client'

// ---------------------------------------------------------------------------
// MSW server
// ---------------------------------------------------------------------------

const server = setupServer()

// ---------------------------------------------------------------------------
// Restore the built-in multipart primitives over jsdom's -- see header note
// ---------------------------------------------------------------------------

const jsdomGlobals = {
  FormData: globalThis.FormData,
  Blob: globalThis.Blob,
  File: globalThis.File,
}

/**
 * Node exposes Blob and File from `node:buffer` but FormData only as a global,
 * which jsdom has already overwritten by the time this file loads. Round-trip
 * a trivial urlencoded body through the built-in fetch types to get an
 * instance of the built-in FormData back, and read its constructor off it.
 */
async function builtinFormData(): Promise<typeof FormData> {
  const parsed = await new Response('_=1', {
    headers: { 'content-type': 'application/x-www-form-urlencoded' },
  }).formData()
  return parsed.constructor as typeof FormData
}

beforeAll(async () => {
  globalThis.FormData = await builtinFormData()
  globalThis.Blob = NodeBlob as unknown as typeof globalThis.Blob
  globalThis.File = NodeFile as unknown as typeof globalThis.File
  server.listen({ onUnhandledRequest: 'error' })
})
afterEach(() => server.resetHandlers())
afterAll(() => {
  server.close()
  globalThis.FormData = jsdomGlobals.FormData
  globalThis.Blob = jsdomGlobals.Blob
  globalThis.File = jsdomGlobals.File
})

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const STORE_ID = 'f47ac10b-58cc-4372-a567-0e02b2c3d479'
const APPLE_PATH = `/api/admin/stores/${STORE_ID}/app-credentials/apple`
const GOOGLE_PATH = `/api/admin/stores/${STORE_ID}/app-credentials/google`

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
    // p8 sent as a file part named 'key.p8', carrying the .p8 text verbatim --
    // this is what the Go handler reads via c.Request.FormFile("p8").
    const p8Field = formData?.get('p8')
    expect(p8Field).toBeInstanceOf(NodeFile)
    expect((p8Field as File).name).toBe('key.p8')
    expect(await (p8Field as File).text()).toBe(APPLE_FIXTURE.p8_contents)
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

    // Sent as a file part named 'service-account.json' carrying the JSON
    // verbatim, not as a plain string field.
    const field = formData?.get('service_account_json')
    expect(field).toBeInstanceOf(NodeFile)
    expect((field as File).name).toBe('service-account.json')
    expect(await (field as File).text()).toBe(GOOGLE_FIXTURE.service_account_json)
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
