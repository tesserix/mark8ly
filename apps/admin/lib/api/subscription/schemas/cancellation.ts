/**
 * Zod schemas for the cancellation wire types.
 *
 * Source of truth:
 *   services/marketplace-api/internal/subscription/cancel/handler.go
 *   services/marketplace-api/internal/subscription/cancel/service.go
 *
 * Request:  POST /api/v1/admin/stores/:storeId/subscription/cancel
 *   Body:   { survey_reason?: string; accept_save_offer?: boolean }
 *
 * Response: { status: "cancel_scheduled" | "active"; cancels_at: string; save_offer_msg?: string }
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

export const cancelRequestSchema = z.object({
  /**
   * Optional exit-survey reason from the exit survey step.
   * The backend truncates to 256 chars — we mirror that constraint.
   */
  survey_reason: z.string().max(256).optional(),
  /**
   * true  → accept the save offer, reverse cancel_scheduled → active.
   * false → schedule cancellation at period end (default path).
   */
  accept_save_offer: z.boolean().optional(),
})

export type CancelRequest = z.infer<typeof cancelRequestSchema>

// ---------------------------------------------------------------------------
// Response
// ---------------------------------------------------------------------------

export const cancelResponseSchema = z.object({
  /**
   * "cancel_scheduled" — cancellation scheduled at period end.
   * "active"           — save offer accepted; subscription restored.
   */
  status: z.enum(['cancel_scheduled', 'active']),
  /**
   * RFC3339 of current_period_end.
   * Empty string ("") when the save offer was accepted and no cancellation
   * is scheduled.
   */
  cancels_at: z.string(),
  /**
   * Human-readable message when the save offer was accepted.
   * Present only on the save-offer-accepted path.
   */
  save_offer_msg: z.string().optional(),
})

export type CancelResponse = z.infer<typeof cancelResponseSchema>
