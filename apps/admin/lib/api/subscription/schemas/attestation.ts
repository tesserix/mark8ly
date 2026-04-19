/**
 * Zod schemas for the US/CA business-entity attestation endpoint (§19.3.1).
 *
 * Backend: POST /api/v1/admin/stores/:storeId/tax/attestation
 *   Body:    { country: 'US'|'CA', checkbox_text: string, checkbox_version: string }
 *
 *   NOTE: The backend field is `country` (not `jurisdiction`). It accepts only
 *   "US" or "CA" per the `binding:"oneof=US CA"` tag in tax.go:attestationBody.
 *   `checkbox_text` must be the exact attestation copy the user acknowledged.
 *   `checkbox_version` is a client-version token to allow future copy changes
 *   to be traced in the audit table.
 *
 * Success: 201 Created — { data: { status: 'signed' }, error: null }
 * Errors:
 *   400 invalid_request — malformed body or country not US/CA
 *   500 attestation_write_failed — DB insert error
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

/**
 * Jurisdiction — only US and Canada require the attestation per §19.3.1.
 */
export const attestationJurisdictionSchema = z.enum(['US', 'CA'])

export type AttestationJurisdiction = z.infer<typeof attestationJurisdictionSchema>

export const attestationRequestSchema = z.object({
  /**
   * ISO-3166-1 alpha-2 country code, restricted to US|CA.
   * Maps to `country` in the Go request body.
   */
  country: attestationJurisdictionSchema,

  /**
   * Verbatim text of the checkbox the user ticked.
   * Stored immutably in business_entity_attestations for audit trail (§18.8).
   */
  checkbox_text: z.string().min(1, 'Checkbox text is required.'),

  /**
   * Version token for the attestation copy, e.g. "v1".
   * Allows the audit table to distinguish which copy revision was shown.
   */
  checkbox_version: z.string().min(1, 'Checkbox version is required.'),
})

export type AttestationRequest = z.infer<typeof attestationRequestSchema>

// ---------------------------------------------------------------------------
// Response
// ---------------------------------------------------------------------------

export const attestationResponseSchema = z.object({
  data: z.object({
    status: z.literal('signed'),
  }),
  error: z.null(),
})

export type AttestationResponse = z.infer<typeof attestationResponseSchema>

// ---------------------------------------------------------------------------
// Form-layer type
//
// The form collects `country` (select) + `is_business_entity` (checkbox).
// The checkbox drives whether the form is submittable; `checkbox_text` and
// `checkbox_version` are resolved from the copy constants at submit time, not
// from the form fields.
// ---------------------------------------------------------------------------

export const attestationFormSchema = z.object({
  country: attestationJurisdictionSchema,
  is_business_entity: z.literal(true, {
    errorMap: () => ({
      message: 'You must confirm this store is operated by a registered business entity.',
    }),
  }),
})

export type AttestationFormValues = z.infer<typeof attestationFormSchema>
