import { describe, it, expect, beforeAll, afterEach, afterAll, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import {
  createApiClient,
  ApiError,
  SubscriptionInactiveError,
} from './client'

// ---------------------------------------------------------------------------
// MSW server — per-test handlers added via server.use(...)
// ---------------------------------------------------------------------------

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const BASE = 'http://localhost'

function client(onSubscriptionInactive?: (status: string) => void) {
  return createApiClient({ baseURL: BASE, onSubscriptionInactive })
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('apiClient', () => {
  it('GET returns parsed JSON on 200', async () => {
    server.use(
      http.get(`${BASE}/api/stores`, () =>
        HttpResponse.json({ stores: ['a', 'b'] }),
      ),
    )
    const result = await client().get<{ stores: string[] }>('/api/stores')
    expect(result).toEqual({ stores: ['a', 'b'] })
  })

  it('POST sends JSON body and Content-Type header', async () => {
    let receivedContentType: string | null = null
    let receivedBody: unknown = null

    server.use(
      http.post(`${BASE}/api/orders`, async ({ request }) => {
        receivedContentType = request.headers.get('content-type')
        receivedBody = await request.json()
        return HttpResponse.json({ id: 'ord_1' }, { status: 201 })
      }),
    )

    await client().post('/api/orders', { quantity: 2 })

    expect(receivedContentType).toContain('application/json')
    expect(receivedBody).toEqual({ quantity: 2 })
  })

  it('204 response returns null', async () => {
    server.use(
      http.delete(`${BASE}/api/item/1`, () =>
        new HttpResponse(null, { status: 204 }),
      ),
    )
    const result = await client().del('/api/item/1')
    expect(result).toBeNull()
  })

  it('400 with error envelope throws ApiError with code + status + message', async () => {
    server.use(
      http.post(`${BASE}/api/orders`, () =>
        HttpResponse.json(
          { error: 'validation_failed', message: 'quantity must be > 0' },
          { status: 400 },
        ),
      ),
    )

    await expect(
      client().post('/api/orders', { quantity: -1 }),
    ).rejects.toSatisfy((err: unknown) => {
      if (!(err instanceof ApiError)) return false
      expect(err.code).toBe('validation_failed')
      expect(err.status).toBe(400)
      expect(err.message).toBe('quantity must be > 0')
      return true
    })
  })

  it('402 subscription_inactive throws SubscriptionInactiveError with status "expired"', async () => {
    server.use(
      http.get(`${BASE}/api/admin/products`, () =>
        HttpResponse.json(
          { error: 'subscription_inactive', status: 'expired' },
          { status: 402 },
        ),
      ),
    )

    await expect(
      client().get('/api/admin/products'),
    ).rejects.toBeInstanceOf(SubscriptionInactiveError)

    try {
      await client().get('/api/admin/products')
    } catch (err) {
      expect(err).toBeInstanceOf(SubscriptionInactiveError)
      expect((err as SubscriptionInactiveError).status).toBe('expired')
    }
  })

  it('402 subscription_inactive calls onSubscriptionInactive handler and throws', async () => {
    server.use(
      http.get(`${BASE}/api/admin/stores`, () =>
        HttpResponse.json(
          { error: 'subscription_inactive', status: 'store_closed' },
          { status: 402 },
        ),
      ),
    )

    const handler = vi.fn()
    const c = client(handler)

    await expect(c.get('/api/admin/stores')).rejects.toBeInstanceOf(
      SubscriptionInactiveError,
    )

    expect(handler).toHaveBeenCalledOnce()
    expect(handler).toHaveBeenCalledWith('store_closed')
  })

  it('500 response throws ApiError with status 500', async () => {
    server.use(
      http.get(`${BASE}/api/admin/billing`, () =>
        HttpResponse.json(
          { error: 'internal_error', message: 'something went wrong' },
          { status: 500 },
        ),
      ),
    )

    await expect(
      client().get('/api/admin/billing'),
    ).rejects.toSatisfy((err: unknown) => {
      expect(err).toBeInstanceOf(ApiError)
      expect((err as ApiError).status).toBe(500)
      return true
    })
  })

  it('non-JSON error body throws ApiError with code "network_error"', async () => {
    server.use(
      http.get(`${BASE}/api/admin/crash`, () =>
        new HttpResponse('Internal Server Error', {
          status: 503,
          headers: { 'Content-Type': 'text/plain' },
        }),
      ),
    )

    await expect(
      client().get('/api/admin/crash'),
    ).rejects.toSatisfy((err: unknown) => {
      expect(err).toBeInstanceOf(ApiError)
      expect((err as ApiError).code).toBe('network_error')
      return true
    })
  })

  it('network failure (fetch throws) is wrapped as ApiError("network_error", 0)', async () => {
    server.use(
      http.get(`${BASE}/api/admin/unreachable`, () =>
        HttpResponse.error(),
      ),
    )

    await expect(
      client().get('/api/admin/unreachable'),
    ).rejects.toSatisfy((err: unknown) => {
      expect(err).toBeInstanceOf(ApiError)
      expect((err as ApiError).code).toBe('network_error')
      expect((err as ApiError).status).toBe(0)
      return true
    })
  })
})
