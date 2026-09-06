/**
 * Promo-code redemption API client.
 *
 * Wraps:
 *   POST /api/admin/stores/:storeId/subscription/apply-promo
 *
 * See services/marketplace-api/internal/handlers/admin/promo.go for backend
 * behaviour and §7.3 of the subscription model spec for the validation rules.
 */
import { apiClient, ApiError } from '@/lib/api/client'
import {
  applyPromoRequestSchema,
  applyPromoResponseSchema,
  toPromoRejectReason,
  type ApplyPromoResponse,
  type PromoRejectReason,
} from './schemas/promo'

const applyPromoPath = (storeId: string) =>
  `/api/admin/stores/${storeId}/subscription/apply-promo`

/**
 * A refused promo code.
 *
 * Thrown instead of a bare ApiError so a call site cannot render a refusal
 * without deciding which sentence it shows. `reason` is always one of the
 * eight public reasons — `toPromoRejectReason` narrows anything else to
 * `invalid_or_expired`, so there is no "unknown reason" branch to forget.
 */
export class PromoRejectedError extends Error {
  constructor(
    public readonly reason: PromoRejectReason,
    message: string,
  ) {
    super(message)
    this.name = 'PromoRejectedError'
  }
}

/**
 * Redeem a promo code against this store's subscription.
 *
 * The request carries only the code. The per-email redemption cap is counted
 * server-side against the subscription's billing address — passing an email
 * from here would let a merchant aim that cap at someone else.
 *
 * Throws PromoRejectedError for a 422 refusal (a valid request the rules
 * declined), and ApiError for everything else — a 401, a 402 read-only
 * subscription, a network failure. Those are not refusals of the code and
 * must not be shown as if the code were the problem.
 */
export async function applyPromo(
  storeId: string,
  code: string,
): Promise<ApplyPromoResponse> {
  const validated = applyPromoRequestSchema.parse({ code })

  try {
    const raw = await apiClient.post<unknown>(
      applyPromoPath(storeId),
      validated,
    )
    return applyPromoResponseSchema.parse(raw)
  } catch (err) {
    if (err instanceof ApiError && err.status === 422) {
      throw new PromoRejectedError(toPromoRejectReason(err.reason), err.message)
    }
    throw err
  }
}
