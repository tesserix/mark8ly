/**
 * Tax-ID API client.
 *
 * Wraps:
 *   POST /api/v1/admin/stores/:storeId/tax/submit
 *
 * Tax-ID status is derived from the subscription GET response (useCurrentPlan).
 * There is no dedicated GET endpoint — callers read `tax_id_status` and
 * `tax_id_pause_reason` from the embedded fields on the subscription row.
 */
import { apiClient } from '@/lib/api/client'
import {
  taxIdSubmitResponseSchema,
  type TaxIdSubmitRequest,
  type TaxIdSubmitResponse,
} from './schemas/taxId'

/**
 * Submit (or resubmit) a tax ID for verification.
 *
 * @param storeId - The store UUID.
 * @param body    - `{ country, tax_id, business_name, billing_address? }`.
 * @returns       - Validated TaxIdSubmitResponse.
 *
 * @throws ApiError(invalid_format, 400)     — bad tax-ID format.
 * @throws ApiError(invalid_request, 400)    — malformed body.
 * @throws ApiError(tax_id_not_found, 422)   — not in registry.
 * @throws ApiError(validator_disabled, 503) — country not yet supported.
 * @throws ApiError(tax_validation_failed, 500) — unexpected server error.
 *
 * Note: HTTP 202 is treated as a success by the fetch wrapper (it is 2xx).
 * The caller must inspect `data.status` to distinguish 'validated' from
 * 'manual_review_queued' / 'registry_unavailable'.
 */
export async function submitTaxId(
  storeId: string,
  body: TaxIdSubmitRequest,
): Promise<TaxIdSubmitResponse> {
  const raw = await apiClient.post<unknown>(
    `/api/admin/stores/${storeId}/tax/submit`,
    body as Record<string, unknown>,
  )
  return taxIdSubmitResponseSchema.parse(raw)
}
