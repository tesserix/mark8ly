/**
 * Business-entity attestation API client (§19.3.1).
 *
 * Wraps:
 *   POST /api/v1/admin/stores/:storeId/tax/attestation
 *
 * The attestation row is append-only in the database (UPDATE-blocking trigger,
 * no DELETE). Once signed it cannot be changed via the API — the merchant must
 * contact support.
 */
import { apiClient } from '@/lib/api/client'
import {
  attestationResponseSchema,
  type AttestationRequest,
  type AttestationResponse,
} from './schemas/attestation'

/**
 * Sign the US/CA business-entity attestation.
 *
 * @param storeId - The store UUID.
 * @param body    - `{ country: 'US'|'CA', checkbox_text, checkbox_version }`.
 * @returns       - Validated AttestationResponse `{ data: { status: 'signed' } }`.
 *
 * @throws ApiError(invalid_request, 400)       — body validation failed.
 * @throws ApiError(attestation_write_failed, 500) — DB insert error.
 *
 * The server returns 201 Created on success. The fetch wrapper treats all
 * 2xx codes as success so the Zod parse fires as expected.
 */
export async function signAttestation(
  storeId: string,
  body: AttestationRequest,
): Promise<AttestationResponse> {
  const raw = await apiClient.post<unknown>(
    `/api/v1/admin/stores/${storeId}/tax/attestation`,
    body as Record<string, unknown>,
  )
  return attestationResponseSchema.parse(raw)
}
