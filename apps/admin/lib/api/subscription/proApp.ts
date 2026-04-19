/**
 * Pro+App add-on purchase API client.
 *
 * Endpoint:
 *   POST /api/v1/admin/stores/:storeId/subscription/add-on/white-label-app
 *
 * Note: There is no GET "quote" endpoint on the backend. Proration context
 * (periodEnd, currency) is derived client-side from the CurrentPlan returned
 * by getSubscription(). The purchaseProApp function is the only API call.
 */
import { apiClient } from '@/lib/api/client'
import {
  purchaseProAppRequestSchema,
  purchaseProAppResponseSchema,
  type PurchaseProAppRequest,
  type PurchaseProAppResponse,
} from './schemas/proApp'

/**
 * Purchase the White-label App add-on for the given store.
 *
 * On success (HTTP 201) returns a PurchaseProAppResponse containing the
 * Stripe hosted invoice URL and the prorated amount charged.
 *
 * The `apple_4_2_6_acknowledged` field must be `true` — the handler returns
 * 400 `apple_4_2_6_ack_required` if it is false or missing.
 *
 * @throws ApiError for 400 (validation), 403 (not Pro / already active), 500 (server)
 */
export async function purchaseProApp(
  storeId: string,
  body: PurchaseProAppRequest,
): Promise<PurchaseProAppResponse> {
  // Validate request shape before sending
  const validated = purchaseProAppRequestSchema.parse(body)

  const raw = await apiClient.post<unknown>(
    `/api/admin/stores/${storeId}/subscription/add-on/white-label-app`,
    validated as Record<string, unknown>,
  )

  return purchaseProAppResponseSchema.parse(raw)
}
