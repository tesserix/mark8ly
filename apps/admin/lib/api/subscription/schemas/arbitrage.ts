/**
 * Zod schemas for the arbitrage appeal endpoint (§18.8.1).
 *
 * Backend: POST /api/v1/admin/stores/:storeId/arbitrage-appeal
 *   Body:    { jurisdiction: string, justification?: string, document_url?: string }
 *   Success: { data: { status: 'submitted' }, error: null }
 *   Errors:  arbitrage_appeal_invalid_jurisdiction (400)
 *            arbitrage_appeal_no_open_flag (404)
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

export const arbitrageAppealRequestSchema = z.object({
  /** ISO-3166-1 alpha-2 country code — e.g. "US", "GB", "IN". */
  jurisdiction: z
    .string()
    .length(2, 'Jurisdiction must be a 2-letter country code.')
    .toUpperCase(),

  /** Optional merchant explanation of the address mismatch. Max 2000 chars. */
  justification: z
    .string()
    .max(2000, 'Additional context cannot exceed 2000 characters.')
    .optional(),

  /**
   * Optional GCS public URL of a supporting document uploaded via media endpoint.
   * Empty string is normalised to undefined so RHF's empty text input does not
   * trigger a URL validation error.
   */
  document_url: z
    .string()
    .url('Document URL must be a valid URL.')
    .optional()
    .or(z.literal('').transform(() => undefined)),
})

export type ArbitrageAppealRequest = z.infer<typeof arbitrageAppealRequestSchema>

// ---------------------------------------------------------------------------
// Response
// ---------------------------------------------------------------------------

export const arbitrageAppealResponseSchema = z.object({
  data: z.object({
    status: z.literal('submitted'),
  }),
  error: z.null(),
})

export type ArbitrageAppealResponse = z.infer<typeof arbitrageAppealResponseSchema>
