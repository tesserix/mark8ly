/**
 * Arbitrage appeal API client.
 *
 * Wraps: POST /api/v1/admin/stores/:storeId/arbitrage-appeal
 *
 * The arbitrage_flag state comes from the subscription GET response
 * (arbitrage_flag boolean + latest_arbitrage_audit). There is no dedicated
 * GET endpoint — callers derive flag state from useCurrentPlan().
 */
import { apiClient } from '@/lib/api/client'
import {
  arbitrageAppealResponseSchema,
  type ArbitrageAppealRequest,
  type ArbitrageAppealResponse,
} from './schemas/arbitrage'

/**
 * Submit a geo-pricing arbitrage appeal for a store.
 *
 * @param storeId - The store UUID.
 * @param body    - Jurisdiction (required), justification and document_url (optional).
 * @returns Validated ArbitrageAppealResponse `{ data: { status: 'submitted' }, error: null }`.
 * @throws ApiError with code 'arbitrage_appeal_invalid_jurisdiction' on 400.
 * @throws ApiError with code 'arbitrage_appeal_no_open_flag' on 404.
 */
export async function submitArbitrageAppeal(
  storeId: string,
  body: ArbitrageAppealRequest,
): Promise<ArbitrageAppealResponse> {
  const raw = await apiClient.post<unknown>(
    `/api/admin/stores/${storeId}/arbitrage-appeal`,
    body as Record<string, unknown>,
  )
  return arbitrageAppealResponseSchema.parse(raw)
}
