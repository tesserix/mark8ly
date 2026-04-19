/**
 * Store-downgrade API client.
 *
 * Wraps three endpoints that may not yet exist on the backend (P15 work):
 *   GET    /api/v1/admin/stores/:storeId/subscription/downgrade-preflight
 *   POST   /api/v1/admin/stores/:storeId/close
 *   DELETE /api/v1/admin/stores/:storeId
 *
 * Fallback strategy for getDowngradeBlockList:
 *   If the preflight endpoint returns 404, derive the block list client-side
 *   by calling GET /api/v1/admin/stores and comparing the count to the
 *   target-plan cap. This is a best-effort approach for frontend-first work;
 *   once P15 ships the real preflight endpoint, remove the fallback path.
 *
 * TODO(P15): remove fallback once /downgrade-preflight is implemented.
 */

import { apiClient, ApiError } from '@/lib/api/client'
import {
  downgradeBlockListSchema,
  storeActionResponseSchema,
  type DowngradeBlockList,
  type StoreActionResponse,
} from './schemas/stores'

// ---------------------------------------------------------------------------
// Target-plan store cap table
// Used only in the client-side fallback when the preflight endpoint is absent.
// TODO(P15): remove once backend ships the preflight endpoint.
// ---------------------------------------------------------------------------

const PLAN_STORE_CAPS: Record<string, number> = {
  starter: 1,
  studio: 3,
  growth: 10,
  scale: 25,
  pro: Infinity,
}

export interface GetDowngradeBlockListParams {
  targetPlan: string
  billingPeriod: string
  overBy: number
}

/**
 * Shape of the existing GET /api/v1/admin/stores response item.
 * Mirrors AdminStoreResponse in services/marketplace-api/internal/handlers/admin/stores.go
 */
interface AdminStoreItem {
  id: string
  name: string
  slug: string
  country_code: string
  currency_code: string
  status: string
  updated_at?: string
}

interface AdminStoresListResponse {
  data: AdminStoreItem[]
}

/**
 * Fetch the list of over-cap stores blocking a downgrade.
 *
 * Tries the dedicated preflight endpoint first. Falls back to deriving
 * the list from GET /admin/stores when the preflight endpoint is absent
 * (404) — frontend-first graceful degradation.
 */
export async function getDowngradeBlockList(
  storeId: string,
  params: GetDowngradeBlockListParams,
): Promise<DowngradeBlockList> {
  const { targetPlan, billingPeriod, overBy } = params
  const preflightUrl = `/api/v1/admin/stores/${storeId}/subscription/downgrade-preflight?target=${encodeURIComponent(targetPlan)}&billing=${encodeURIComponent(billingPeriod)}`

  try {
    const raw = await apiClient.get<unknown>(preflightUrl)
    return downgradeBlockListSchema.parse(raw)
  } catch (err) {
    // If the preflight endpoint doesn't exist yet (404), fall back to
    // deriving the block list from the full store list.
    const isNotFound = err instanceof ApiError && err.status === 404
    if (!isNotFound) {
      throw err
    }
  }

  // ---------------------------------------------------------------------------
  // Fallback: derive over-cap stores from GET /admin/stores
  // TODO(P15): remove this block once /downgrade-preflight is implemented.
  // ---------------------------------------------------------------------------
  const raw = await apiClient.get<AdminStoresListResponse>('/api/v1/admin/stores')
  const allStores = raw.data ?? []
  const cap = PLAN_STORE_CAPS[targetPlan] ?? 0

  // Take the last `overBy` stores (arbitrary selection — the merchant will
  // choose which to close or delete from the UI).
  const overCapStores = allStores.slice(Math.max(0, allStores.length - overBy))

  const derived: DowngradeBlockList = {
    stores: overCapStores.map((s) => ({
      id: s.id,
      name: s.name,
      // in_flight_orders is not available from GET /admin/stores
      // TODO(P15): populate once the backend ships this field.
      in_flight_orders: null,
      updated_at: s.updated_at ?? new Date().toISOString(),
    })),
    over_by: overBy,
    target_plan: targetPlan,
    billing_period: billingPeriod,
  }

  return downgradeBlockListSchema.parse(derived)
}

/**
 * Close a store — freezes the storefront behind a closed page, keeps the slot.
 *
 * TODO(P15): POST /api/v1/admin/stores/:storeId/close is not yet implemented.
 * The backend stub should return { status: 'closed' }.
 */
export async function closeStore(storeId: string): Promise<StoreActionResponse> {
  const raw = await apiClient.post<unknown>(
    `/api/v1/admin/stores/${storeId}/close`,
    null,
  )
  return storeActionResponseSchema.parse(raw)
}

/**
 * Delete a store — removes it and frees the slot. 60-day soft-delete grace
 * period applies; restore is available through support.
 *
 * TODO(P15): DELETE /api/v1/admin/stores/:storeId is not yet implemented.
 * The backend stub should return { status: 'deleted_pending_grace' }.
 */
export async function deleteStore(storeId: string): Promise<StoreActionResponse> {
  const raw = await apiClient.del<unknown>(
    `/api/v1/admin/stores/${storeId}`,
  )
  return storeActionResponseSchema.parse(raw)
}
