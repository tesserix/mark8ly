/**
 * Zod schemas for the Pro contact-sales wire types.
 *
 * Endpoint (stubbed — backend not yet shipped):
 *   POST /api/v1/admin/stores/:storeId/subscription/pro-contact
 *   → { status: 'submitted', ticket_id: string }
 *
 * TODO: tighten when the marketing webhook endpoint ships (P16, Task 21).
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

export const migrationTimelineSchema = z.enum([
  'this_quarter',
  'next_quarter',
  'h2',
  'exploring',
])

export type MigrationTimeline = z.infer<typeof migrationTimelineSchema>

export const proContactRequestSchema = z.object({
  company_name: z.string().min(2).max(200),
  contact_name: z.string().min(2).max(100),
  contact_email: z.string().email(),
  // Optional strings: treat empty string the same as absent.
  phone: z.string().optional().transform((v) => v || undefined),
  monthly_orders: z.number().int().nonnegative(),
  current_platform: z.string().optional().transform((v) => v || undefined),
  // HTML select returns "" when no option selected — coerce to undefined.
  migration_timeline: z
    .union([migrationTimelineSchema, z.literal('')])
    .optional()
    .transform((v) => (v === '' ? undefined : v)),
  notes: z.string().max(2000).optional().transform((v) => v || undefined),
})

export type ProContactRequest = z.infer<typeof proContactRequestSchema>

// ---------------------------------------------------------------------------
// Response
// ---------------------------------------------------------------------------

export const proContactResponseSchema = z.object({
  status: z.literal('submitted'),
  ticket_id: z.string(),
})

export type ProContactResponse = z.infer<typeof proContactResponseSchema>
