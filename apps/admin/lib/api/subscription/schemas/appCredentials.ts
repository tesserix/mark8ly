/**
 * Zod schemas for Apple and Google app credential uploads.
 *
 * Source of truth:
 *   services/marketplace-api/internal/handlers/admin/app_credentials.go
 *
 * Key deviations from initial spec guess:
 *   - Both endpoints use multipart/form-data (NOT JSON bodies).
 *   - Apple fields: `p8` (file), `issuer_id` (string), `key_id` (string).
 *     There is NO `bundle_id` field in the Go handler.
 *   - Google field: `service_account_json` (file/blob).
 *   - Both endpoints return HTTP 204 No Content on success — there is no
 *     `uploaded_at` or `status` in the response.
 *
 * The client-side schemas here represent what the *form* collects, not a
 * JSON wire format. The actual multipart encoding is handled by the API
 * client functions in appCredentials.ts.
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Apple ASC credentials — form shape
// ---------------------------------------------------------------------------

/**
 * Client-side form schema for Apple App Store Connect credentials.
 *
 * `p8_contents` holds the raw text read from the .p8 file. The file is
 * never kept in state beyond the read — cleared on unmount.
 */
export const appleCredentialsFormSchema = z.object({
  /** App Store Connect API Key ID (e.g. "ABCDE12345") */
  key_id: z.string().min(1, 'Key ID is required'),
  /** App Store Connect Issuer ID (UUID format) */
  issuer_id: z.string().min(1, 'Issuer ID is required'),
  /** Raw text content of the .p8 private key file */
  p8_contents: z
    .string()
    .min(1, 'Private key file is required')
    .refine(
      (v) => v.includes('-----BEGIN PRIVATE KEY-----') || v.includes('-----BEGIN EC PRIVATE KEY-----'),
      'File does not look like a valid .p8 private key',
    ),
})

export type AppleCredentialsForm = z.infer<typeof appleCredentialsFormSchema>

// ---------------------------------------------------------------------------
// Google Play service account — form shape
// ---------------------------------------------------------------------------

/**
 * Client-side form schema for Google Play service account JSON.
 *
 * `service_account_json` holds the raw text read from the JSON file.
 * Client-side validation checks for valid JSON with a `client_email` field.
 */
export const googleCredentialsFormSchema = z.object({
  service_account_json: z
    .string()
    .min(1, 'Service account JSON is required')
    .refine((v) => {
      try {
        const parsed = JSON.parse(v) as Record<string, unknown>
        return typeof parsed.client_email === 'string' && parsed.client_email.length > 0
      } catch {
        return false
      }
    }, 'That does not look like a valid service account JSON.'),
})

export type GoogleCredentialsForm = z.infer<typeof googleCredentialsFormSchema>

// ---------------------------------------------------------------------------
// Error codes the handler can return
// ---------------------------------------------------------------------------

export type AppCredentialErrorCode =
  | 'invalid_payload'
  | 'invalid_p8_format'
  | 'invalid_service_account_json'
  | 'store_failed'
  | 'add_on_not_active'
  | 'invalid_store_id'
  | 'unauthorized'
