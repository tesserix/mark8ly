/**
 * Zod schemas for the Tax-ID submit endpoint (§19.3).
 *
 * Backend: POST /api/v1/admin/stores/:storeId/tax/submit
 *   Body:   { country: string(2), tax_id: string, business_name: string,
 *             billing_address?: string }
 *
 * Responses (exact HTTP status + data shape from tax.go):
 *   200 OK       — { data: { status: 'validated' }, error: null }
 *   202 Accepted — { data: { status: 'manual_review_queued', sla_business_days: 5 }, error: null }
 *                  { data: { status: 'registry_unavailable', message: string }, error: null }
 *   400          — { error: 'invalid_format' | 'invalid_request', message?: string }
 *   422          — { error: 'tax_id_not_found' }
 *   503          — { error: 'validator_disabled', message: string }
 *   500          — { error: 'tax_validation_failed' }
 *
 * The front-end only reads the success 200/202 path; error paths are thrown
 * as ApiError by the shared fetch wrapper.
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

export const taxIdSubmitRequestSchema = z.object({
  /** ISO-3166-1 alpha-2 country code — must be exactly 2 chars. */
  country: z
    .string()
    .length(2, 'Select a country (2-letter code required).')
    .toUpperCase(),

  /** Raw tax-ID value. No format enforced client-side — backend validates. */
  tax_id: z.string().min(1, 'Tax ID is required.'),

  /** Legal registered business name. */
  business_name: z.string().min(1, 'Legal business name is required.'),

  /**
   * Optional billing address line. Passed through to the tax service for
   * registry lookups that require an address match.
   */
  billing_address: z.string().optional(),
})

export type TaxIdSubmitRequest = z.infer<typeof taxIdSubmitRequestSchema>

// ---------------------------------------------------------------------------
// Response data variants
// ---------------------------------------------------------------------------

export const taxIdValidatedDataSchema = z.object({
  status: z.literal('validated'),
})

export const taxIdManualReviewDataSchema = z.object({
  status: z.literal('manual_review_queued'),
  sla_business_days: z.number().int().optional(),
})

export const taxIdRegistryUnavailableDataSchema = z.object({
  status: z.literal('registry_unavailable'),
  message: z.string().optional(),
})

/**
 * Union of all success data shapes. Callers key off `status`.
 */
export const taxIdSubmitDataSchema = z.discriminatedUnion('status', [
  taxIdValidatedDataSchema,
  taxIdManualReviewDataSchema,
  taxIdRegistryUnavailableDataSchema,
])

export type TaxIdSubmitData = z.infer<typeof taxIdSubmitDataSchema>

/**
 * Full response envelope — mirrors the Go `gin.H{"data": ..., "error": nil}` pattern.
 */
export const taxIdSubmitResponseSchema = z.object({
  data: taxIdSubmitDataSchema,
  error: z.null(),
})

export type TaxIdSubmitResponse = z.infer<typeof taxIdSubmitResponseSchema>

// ---------------------------------------------------------------------------
// Tax-ID status derived from useCurrentPlan + GET status (§19.3)
//
// The subscription GET response carries the current tax-ID state. These
// schemas define the shape we expect when that field arrives.
// ---------------------------------------------------------------------------

/**
 * Clock-pause reason — drives the ClockPauseBadge variant.
 *   registry_unavailable — external registry unreachable; retry queued.
 *   sea_review           — in the manual SEA review queue.
 */
export const clockPauseReasonSchema = z.enum([
  'registry_unavailable',
  'sea_review',
])

export type ClockPauseReason = z.infer<typeof clockPauseReasonSchema>

/**
 * Tax-ID validation status stored on the subscription row.
 */
export const taxIdStatusSchema = z.enum([
  'not_submitted',
  'pending',
  'validated',
  'clock_paused',
  'rejected',
])

export type TaxIdStatus = z.infer<typeof taxIdStatusSchema>

/**
 * Embedded tax-ID state carried on the subscription response.
 * All fields are optional — only present when the backend ships them.
 */
export const embeddedTaxIdStateSchema = z.object({
  tax_id_status: taxIdStatusSchema.optional(),
  tax_id_pause_reason: clockPauseReasonSchema.optional(),
  tax_id_validation_code: z.string().optional(),
  tax_id_reviewed_at: z.string().optional(),
})

export type EmbeddedTaxIdState = z.infer<typeof embeddedTaxIdStateSchema>
