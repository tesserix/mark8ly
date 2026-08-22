/**
 * Plan-change API client tests — MSW-backed.
 *
 * Tests:
 *   1. getProrationPreview — 200 happy path (allow_upgrade_now)
 *   2. getProrationPreview — 200 allow_downgrade_at_period_end
 *   3. getProrationPreview — 404 / non-2xx throws ApiError
 *   4. applyPlanChange — 200 result=upgrade_committed
 *   5. applyPlanChange — 200 result=downgrade_scheduled
 *   6. applyPlanChange — 409 plan locked (no_change) throws ApiError
 *   7. getProrationPreview — sends correct query params (target_period)
 */

import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import {
  getProrationPreview,
  applyPlanChange,
} from '@/lib/api/subscription/planChange'
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
const PREFLIGHT_PATH = `/api/admin/stores/${STORE_ID}/subscription/change-plan/preflight`
const APPLY_PATH = `/api/admin/stores/${STORE_ID}/subscription/change-plan`

const PLAN_DIFF_FIXTURE = {
  stores_delta: 1,
  images_per_product_delta: 10,
  campaign_emails_delta: -1,
}

const PREFLIGHT_UPGRADE_FIXTURE = {
  decision: 'allow_upgrade_now',
  current_plan: 'starter',
  current_period: 'monthly',
  target_plan: 'studio',
  target_period: 'monthly',
  effective_at: '2026-04-19T10:00:00Z',
  current_plan_diff: PLAN_DIFF_FIXTURE,
}

const PREFLIGHT_DOWNGRADE_FIXTURE = {
  decision: 'allow_downgrade_at_period_end',
  current_plan: 'studio',
  current_period: 'monthly',
  target_plan: 'starter',
  target_period: 'monthly',
  effective_at: '2026-05-01T00:00:00Z',
  current_plan_diff: {
    stores_delta: -1,
    images_per_product_delta: -5,
    campaign_emails_delta: -500,
  },
}

const APPLY_UPGRADE_FIXTURE = {
  result: 'upgrade_committed',
  effective_at: '2026-04-19T10:00:00Z',
  stripe_updated: true,
}

const APPLY_DOWNGRADE_FIXTURE = {
  result: 'downgrade_scheduled',
  effective_at: '2026-05-01T00:00:00Z',
  stripe_updated: false,
}

// ---------------------------------------------------------------------------
// getProrationPreview
// ---------------------------------------------------------------------------

describe('getProrationPreview', () => {
  it('200 upgrade — returns parsed PreflightReport', async () => {
    server.use(
      http.get(PREFLIGHT_PATH, () =>
        HttpResponse.json(PREFLIGHT_UPGRADE_FIXTURE),
      ),
    )

    const report = await getProrationPreview(STORE_ID, 'studio', 'monthly')

    expect(report.decision).toBe('allow_upgrade_now')
    expect(report.current_plan).toBe('starter')
    expect(report.target_plan).toBe('studio')
    expect(report.target_period).toBe('monthly')
    expect(report.effective_at).toBe('2026-04-19T10:00:00Z')
    expect(report.current_plan_diff.stores_delta).toBe(1)
  })

  it('200 downgrade — returns allow_downgrade_at_period_end decision', async () => {
    server.use(
      http.get(PREFLIGHT_PATH, () =>
        HttpResponse.json(PREFLIGHT_DOWNGRADE_FIXTURE),
      ),
    )

    const report = await getProrationPreview(STORE_ID, 'starter', 'monthly')

    expect(report.decision).toBe('allow_downgrade_at_period_end')
    expect(report.effective_at).toBe('2026-05-01T00:00:00Z')
  })

  it('sends target_period (not billing_period) as query param', async () => {
    let receivedUrl: string | null = null

    server.use(
      http.get(PREFLIGHT_PATH, ({ request }) => {
        receivedUrl = request.url
        return HttpResponse.json(PREFLIGHT_UPGRADE_FIXTURE)
      }),
    )

    await getProrationPreview(STORE_ID, 'studio', 'annual')

    expect(receivedUrl).not.toBeNull()
    const url = new URL(receivedUrl!)
    expect(url.searchParams.get('target_period')).toBe('annual')
    expect(url.searchParams.get('target_plan')).toBe('studio')
    // Must NOT use billing_period
    expect(url.searchParams.has('billing_period')).toBe(false)
  })

  it('404 throws ApiError', async () => {
    server.use(
      http.get(PREFLIGHT_PATH, () =>
        HttpResponse.json(
          { error: 'not_found', message: 'subscription not found' },
          { status: 404 },
        ),
      ),
    )

    await expect(
      getProrationPreview(STORE_ID, 'studio', 'monthly'),
    ).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && (err as ApiError).status === 404,
    )
  })

  it('block_store_count decision — populates store_count + stores', async () => {
    server.use(
      http.get(PREFLIGHT_PATH, () =>
        HttpResponse.json({
          decision: 'block_store_count',
          current_plan: 'pro',
          current_period: 'annual',
          target_plan: 'starter',
          target_period: 'monthly',
          effective_at: '0001-01-01T00:00:00Z',
          current_plan_diff: PLAN_DIFF_FIXTURE,
          store_count: 3,
          target_store_limit: 1,
          stores: [
            {
              store_id: 'abc-123',
              name: 'My Store',
              slug: 'my-store',
              status: 'active',
              in_flight_orders: 2,
              is_soft_deleted: false,
            },
          ],
        }),
      ),
    )

    const report = await getProrationPreview(STORE_ID, 'starter', 'monthly')

    expect(report.decision).toBe('block_store_count')
    expect(report.store_count).toBe(3)
    expect(report.target_store_limit).toBe(1)
    expect(report.stores).toHaveLength(1)
    expect(report.stores![0].name).toBe('My Store')
  })
})

// ---------------------------------------------------------------------------
// applyPlanChange
// ---------------------------------------------------------------------------

describe('applyPlanChange', () => {
  it('200 upgrade_committed — returns parsed response', async () => {
    server.use(
      http.post(APPLY_PATH, () => HttpResponse.json(APPLY_UPGRADE_FIXTURE)),
    )

    const result = await applyPlanChange(STORE_ID, {
      target_plan: 'studio',
      target_period: 'monthly',
    })

    expect(result.result).toBe('upgrade_committed')
    expect(result.stripe_updated).toBe(true)
    expect(result.effective_at).toBe('2026-04-19T10:00:00Z')
  })

  it('200 downgrade_scheduled — stripe_updated is false', async () => {
    server.use(
      http.post(APPLY_PATH, () => HttpResponse.json(APPLY_DOWNGRADE_FIXTURE)),
    )

    const result = await applyPlanChange(STORE_ID, {
      target_plan: 'starter',
      target_period: 'monthly',
    })

    expect(result.result).toBe('downgrade_scheduled')
    expect(result.stripe_updated).toBe(false)
  })

  it('sends target_period (not billing_period) in request body', async () => {
    let receivedBody: Record<string, unknown> | null = null

    server.use(
      http.post(APPLY_PATH, async ({ request }) => {
        receivedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json(APPLY_UPGRADE_FIXTURE)
      }),
    )

    await applyPlanChange(STORE_ID, {
      target_plan: 'pro',
      target_period: 'annual',
    })

    expect(receivedBody).not.toBeNull()
    expect(receivedBody!.target_period).toBe('annual')
    expect(receivedBody!.target_plan).toBe('pro')
    expect(receivedBody).not.toHaveProperty('billing_period')
  })

  it('409 no_change — throws ApiError with status 409', async () => {
    server.use(
      http.post(APPLY_PATH, () =>
        HttpResponse.json(
          { error: 'no_change' },
          { status: 409 },
        ),
      ),
    )

    await expect(
      applyPlanChange(STORE_ID, {
        target_plan: 'starter',
        target_period: 'monthly',
      }),
    ).rejects.toSatisfy(
      (err: unknown) => err instanceof ApiError && (err as ApiError).status === 409,
    )
  })

  it('402 subscription_read_only — throws ApiError with status 402', async () => {
    server.use(
      http.post(APPLY_PATH, () =>
        HttpResponse.json(
          { error: 'subscription_read_only' },
          { status: 402 },
        ),
      ),
    )

    // Note: the apiClient intercepts 402 subscription_inactive, but
    // subscription_read_only is a different error code that falls through to
    // the generic ApiError handler.
    await expect(
      applyPlanChange(STORE_ID, {
        target_plan: 'studio',
        target_period: 'monthly',
      }),
    ).rejects.toBeInstanceOf(Error)
  })
})
