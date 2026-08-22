/**
 * Store-downgrade API client tests.
 *
 * MSW-backed. Tests cover:
 *   1. getDowngradeBlockList — preflight 200 returns validated block list
 *   2. getDowngradeBlockList — preflight 404 falls back to GET /admin/stores
 *   3. getDowngradeBlockList — preflight 500 throws ApiError (no fallback)
 *   4. closeStore — 200 returns { status: 'closed' }
 *   5. deleteStore — 200 returns { status: 'deleted_pending_grace' }
 *   6. deleteStore — 409 (already deleted) throws ApiError with status 409
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import {
  getDowngradeBlockList,
  closeStore,
  deleteStore,
} from '@/lib/api/subscription/stores'
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

const STORE_ID = 'a1b2c3d4-0000-4000-8000-000000000001'
const STORE_ID_2 = 'a1b2c3d4-0000-4000-8000-000000000002'
const STORE_ID_3 = 'a1b2c3d4-0000-4000-8000-000000000003'

const PREFLIGHT_PATH = `/api/admin/stores/${STORE_ID}/subscription/downgrade-preflight`
const STORES_LIST_PATH = '/api/admin/stores'
const CLOSE_PATH = `/api/admin/stores/${STORE_ID}/close`
const DELETE_PATH = `/api/admin/stores/${STORE_ID}`

const BLOCK_LIST_FIXTURE = {
  stores: [
    {
      id: STORE_ID_2,
      name: 'Paris Pop-up',
      in_flight_orders: 3,
      updated_at: '2026-04-01T00:00:00Z',
    },
    {
      id: STORE_ID_3,
      name: 'Berlin Flagship',
      in_flight_orders: 0,
      updated_at: '2026-03-15T00:00:00Z',
    },
  ],
  over_by: 2,
  target_plan: 'studio',
  billing_period: 'annual',
}

const STORES_LIST_FIXTURE = {
  data: [
    { id: STORE_ID, name: 'Main Store', slug: 'main', country_code: 'GB', currency_code: 'GBP', status: 'active', updated_at: '2026-04-01T00:00:00Z' },
    { id: STORE_ID_2, name: 'Paris Pop-up', slug: 'paris', country_code: 'FR', currency_code: 'EUR', status: 'active', updated_at: '2026-04-01T00:00:00Z' },
    { id: STORE_ID_3, name: 'Berlin Flagship', slug: 'berlin', country_code: 'DE', currency_code: 'EUR', status: 'active', updated_at: '2026-03-15T00:00:00Z' },
  ],
}

const PARAMS = { targetPlan: 'studio', billingPeriod: 'annual', overBy: 2 }

// ---------------------------------------------------------------------------
// getDowngradeBlockList
// ---------------------------------------------------------------------------

describe('getDowngradeBlockList', () => {
  it('preflight 200 — returns validated DowngradeBlockList', async () => {
    server.use(
      http.get(PREFLIGHT_PATH, () => HttpResponse.json(BLOCK_LIST_FIXTURE)),
    )

    const result = await getDowngradeBlockList(STORE_ID, PARAMS)

    expect(result.over_by).toBe(2)
    expect(result.target_plan).toBe('studio')
    expect(result.billing_period).toBe('annual')
    expect(result.stores).toHaveLength(2)
    expect(result.stores[0].name).toBe('Paris Pop-up')
    expect(result.stores[0].in_flight_orders).toBe(3)
    expect(result.stores[1].in_flight_orders).toBe(0)
  })

  it('preflight 404 — falls back to GET /admin/stores and derives block list', async () => {
    server.use(
      http.get(PREFLIGHT_PATH, () =>
        HttpResponse.json({ error: 'not_found', message: 'endpoint not found' }, { status: 404 }),
      ),
      http.get(STORES_LIST_PATH, () => HttpResponse.json(STORES_LIST_FIXTURE)),
    )

    const result = await getDowngradeBlockList(STORE_ID, PARAMS)

    // The fallback takes the last `overBy` stores from the list.
    expect(result.stores).toHaveLength(2)
    expect(result.over_by).toBe(2)
    expect(result.target_plan).toBe('studio')
    // in_flight_orders is null in the fallback path (backend doesn't expose it).
    expect(result.stores[0].in_flight_orders).toBeNull()
  })

  it('preflight 500 — throws ApiError (no fallback)', async () => {
    server.use(
      http.get(PREFLIGHT_PATH, () =>
        HttpResponse.json(
          { error: 'internal_error', message: 'database error' },
          { status: 500 },
        ),
      ),
    )

    await expect(getDowngradeBlockList(STORE_ID, PARAMS)).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && (err as ApiError).status === 500,
    )
  })
})

// ---------------------------------------------------------------------------
// closeStore
// ---------------------------------------------------------------------------

describe('closeStore', () => {
  it('200 — returns { status: "closed" }', async () => {
    server.use(
      http.post(CLOSE_PATH, () =>
        HttpResponse.json({ status: 'closed' }),
      ),
    )

    const result = await closeStore(STORE_ID)
    expect(result.status).toBe('closed')
  })

  it('500 — throws ApiError', async () => {
    server.use(
      http.post(CLOSE_PATH, () =>
        HttpResponse.json(
          { error: 'internal_error', message: 'close failed' },
          { status: 500 },
        ),
      ),
    )

    await expect(closeStore(STORE_ID)).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && (err as ApiError).status === 500,
    )
  })
})

// ---------------------------------------------------------------------------
// deleteStore
// ---------------------------------------------------------------------------

describe('deleteStore', () => {
  it('200 — returns { status: "deleted_pending_grace" }', async () => {
    server.use(
      http.delete(DELETE_PATH, () =>
        HttpResponse.json({ status: 'deleted_pending_grace' }),
      ),
    )

    const result = await deleteStore(STORE_ID)
    expect(result.status).toBe('deleted_pending_grace')
  })

  it('409 already deleted — throws ApiError with status 409', async () => {
    server.use(
      http.delete(DELETE_PATH, () =>
        HttpResponse.json(
          { error: 'already_deleted', message: 'store is already in soft-delete grace period' },
          { status: 409 },
        ),
      ),
    )

    const err = await deleteStore(STORE_ID).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(409)
    expect((err as ApiError).code).toBe('already_deleted')
  })
})
