/**
 * Cancellation API client.
 *
 * Wraps:
 *   POST /api/v1/admin/stores/:storeId/subscription/cancel
 *
 * Two call shapes:
 *   submitCancellation(storeId, body) — schedules cancellation (accept_save_offer: false, or omitted).
 *   revertCancellation(storeId)       — accepts the save offer (accept_save_offer: true), reverting
 *                                       cancel_scheduled → active.
 *
 * See services/marketplace-api/internal/subscription/cancel/ for backend behaviour.
 * See §15, §15.1 of the subscription model spec for the product rules.
 */
import { apiClient } from '@/lib/api/client'
import {
  cancelRequestSchema,
  cancelResponseSchema,
  type CancelRequest,
  type CancelResponse,
} from './schemas/cancellation'

const cancelPath = (storeId: string) =>
  `/api/admin/stores/${storeId}/subscription/cancel`

/**
 * Submit a cancellation (or save-offer response).
 *
 * Pass `accept_save_offer: false` (or omit) to schedule cancellation.
 * Pass `accept_save_offer: true` to accept the save offer — but prefer
 * `revertCancellation` for that call site to keep intent explicit.
 */
export async function submitCancellation(
  storeId: string,
  body: CancelRequest,
): Promise<CancelResponse> {
  const validated = cancelRequestSchema.parse(body)
  const raw = await apiClient.post<unknown>(cancelPath(storeId), validated)
  return cancelResponseSchema.parse(raw)
}

/**
 * Accept the save offer — reverses cancel_scheduled → active.
 *
 * Internally sends `{ accept_save_offer: true }`. The Go service
 * requires the subscription to already be in `cancel_scheduled` state;
 * a 409 is returned if the offer was already accepted.
 */
export async function revertCancellation(
  storeId: string,
): Promise<CancelResponse> {
  const raw = await apiClient.post<unknown>(cancelPath(storeId), {
    accept_save_offer: true,
  })
  return cancelResponseSchema.parse(raw)
}
