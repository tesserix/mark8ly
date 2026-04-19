/**
 * Zod schemas for the store-downgrade block surface.
 *
 * Source of truth (backend, not yet shipped):
 *   GET  /api/v1/admin/stores/:storeId/subscription/downgrade-preflight
 *   POST /api/v1/admin/stores/:storeId/close
 *   DELETE /api/v1/admin/stores/:storeId
 *
 * TODO(P15): once the backend ships in_flight_orders on the store model,
 * tighten in_flight_orders from nullable to z.number().int().nonnegative().
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Individual store in the downgrade block list
// ---------------------------------------------------------------------------

export const downgradeBlockStoreSchema = z.object({
  /** UUID of the store. */
  id: z.string(),
  /** Display name of the store. */
  name: z.string(),
  /**
   * Number of orders currently in flight (e.g. pending / processing).
   *
   * TODO(P15): in_flight_orders not yet populated by the Go store model.
   * Null when the backend hasn't shipped the field; the UI renders "–".
   */
  in_flight_orders: z.number().int().nonnegative().nullable().optional(),
  /** ISO date string for when the store was last updated. */
  updated_at: z.string(),
})

export type DowngradeBlockStore = z.infer<typeof downgradeBlockStoreSchema>

// ---------------------------------------------------------------------------
// GET /api/v1/admin/stores/:storeId/subscription/downgrade-preflight
// (or derived client-side from GET /api/v1/admin/stores when endpoint is absent)
// ---------------------------------------------------------------------------

export const downgradeBlockListSchema = z.object({
  /** Stores that exceed the target plan's cap and must be acted on. */
  stores: z.array(downgradeBlockStoreSchema),
  /** How many stores over the cap the tenant is. */
  over_by: z.number().int().positive(),
  /** Target plan slug, e.g. "studio". */
  target_plan: z.string(),
  /** Target billing period, e.g. "annual" | "monthly". */
  billing_period: z.string(),
})

export type DowngradeBlockList = z.infer<typeof downgradeBlockListSchema>

// ---------------------------------------------------------------------------
// POST /api/v1/admin/stores/:storeId/close
// DELETE /api/v1/admin/stores/:storeId
// ---------------------------------------------------------------------------

export const storeActionResponseSchema = z.object({
  status: z.enum(['closed', 'deleted_pending_grace']),
})

export type StoreActionResponse = z.infer<typeof storeActionResponseSchema>
