/**
 * Zod schemas for dunning-related wire types.
 *
 * Source of truth: services/marketplace-api/internal/handlers/admin/
 *   subscription.go (GET subscription) +
 *   subscription_complete_action.go (GET complete-action → 302 redirect).
 *
 * IMPORTANT — known field gaps (TODO: remove .optional() once backend ships):
 *   - `hosted_invoice_url`: present on the DB model (StoreSubscription) but NOT
 *     included in the SubscriptionResponse DTO. Use CompleteActionURL instead.
 *   - `retry_at`: not on SubscriptionResponse DTO. Derived from periodEnd as
 *     a best-effort approximation; mark as TODO when real field lands.
 *   - `last_retry_failed_at`: not on SubscriptionResponse DTO.
 *   - `first_flagged_at` (payment_action_required): not on SubscriptionResponse DTO.
 *
 * The complete-action endpoint (GET .../subscription/complete-action) performs
 * a 302 redirect — it does NOT return JSON. Frontend must follow the redirect
 * or open the redirect URL in a new tab. `getCompleteActionUrl` fetches the
 * endpoint and captures the resolved URL from the final response.
 */
import { z } from 'zod'

/**
 * Response schema for the complete-action URL helper.
 *
 * Note: the Go handler issues a 302 redirect. This schema is used for
 * the frontend's parsed representation after we resolve the redirect URL
 * from the response URL, not a JSON parse.
 */
export const completeActionUrlResponseSchema = z.object({
  url: z.string().url(),
})

export type CompleteActionUrlResponse = z.infer<
  typeof completeActionUrlResponseSchema
>
