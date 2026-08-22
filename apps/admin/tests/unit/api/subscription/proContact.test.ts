/**
 * Pro contact-sales API client tests.
 *
 * MSW-backed. Tests:
 *   1. 200 happy path — returns ProContactResponse with status='submitted'
 *   2. 404 → throws ProContactEndpointMissingError (fallback signal)
 *   3. 400 validation error → throws ApiError with status 400
 *   4. Request body is sent correctly (JSON)
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import { submitProContact, ProContactEndpointMissingError } from '@/lib/api/subscription/proContact'
import { ApiError } from '@/lib/api/client'
import type { ProContactRequest } from '@/lib/api/subscription/schemas/proContact'

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
const ENDPOINT = `/api/admin/stores/${STORE_ID}/subscription/pro-contact`

const VALID_REQUEST: ProContactRequest = {
  company_name: 'Acme Ltd',
  contact_name: 'Jane Smith',
  contact_email: 'jane@acme.com',
  monthly_orders: 5000,
}

const SUCCESS_RESPONSE = {
  status: 'submitted',
  ticket_id: 'ticket_abc123',
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('submitProContact', () => {
  it('200 happy path — returns ProContactResponse', async () => {
    server.use(
      http.post(ENDPOINT, () => HttpResponse.json(SUCCESS_RESPONSE)),
    )

    const result = await submitProContact(STORE_ID, VALID_REQUEST)

    expect(result.status).toBe('submitted')
    expect(result.ticket_id).toBe('ticket_abc123')
  })

  it('sends the request body as JSON', async () => {
    let receivedBody: Record<string, unknown> | null = null

    server.use(
      http.post(ENDPOINT, async ({ request }) => {
        receivedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(SUCCESS_RESPONSE)
      }),
    )

    await submitProContact(STORE_ID, VALID_REQUEST)

    expect(receivedBody).toMatchObject({
      company_name: 'Acme Ltd',
      contact_name: 'Jane Smith',
      contact_email: 'jane@acme.com',
      monthly_orders: 5000,
    })
  })

  it('404 → throws ProContactEndpointMissingError', async () => {
    server.use(
      http.post(ENDPOINT, () =>
        HttpResponse.json(
          { error: 'not_found', message: 'endpoint not found' },
          { status: 404 },
        ),
      ),
    )

    await expect(submitProContact(STORE_ID, VALID_REQUEST)).rejects.toBeInstanceOf(
      ProContactEndpointMissingError,
    )
  })

  it('400 validation error → throws ApiError with status 400', async () => {
    server.use(
      http.post(ENDPOINT, () =>
        HttpResponse.json(
          { error: 'validation_error', message: 'contact_email is invalid' },
          { status: 400 },
        ),
      ),
    )

    await expect(submitProContact(STORE_ID, VALID_REQUEST)).rejects.toSatisfy(
      (err: unknown) =>
        err instanceof ApiError && (err as ApiError).status === 400,
    )
  })
})
