/**
 * Pro contact-sales API client.
 *
 * Endpoint (marketing webhook — backend not yet shipped):
 *   POST /api/v1/admin/stores/:storeId/subscription/pro-contact
 *
 * On 404, throws ProContactEndpointMissingError so the hook can surface the
 * mailto fallback instead of a generic error.
 *
 * TODO: remove the ProContactEndpointMissingError branch once the backend
 * ships (P16, Task 21 — Notion + Slack marketing webhook endpoint).
 */
import { apiClient, ApiError } from '@/lib/api/client'
import {
  proContactRequestSchema,
  proContactResponseSchema,
  type ProContactRequest,
  type ProContactResponse,
} from './schemas/proContact'

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

/**
 * Thrown when the backend endpoint does not yet exist (404).
 * The hook catches this to surface the mailto fallback.
 */
export class ProContactEndpointMissingError extends Error {
  constructor() {
    super('pro-contact endpoint not yet available')
    this.name = 'ProContactEndpointMissingError'
  }
}

// ---------------------------------------------------------------------------
// API function
// ---------------------------------------------------------------------------

/**
 * Submit a Pro contact-sales enquiry for a store.
 *
 * Validates the request body with Zod before sending so caller errors surface
 * early, not as a backend 400.
 *
 * Throws:
 *   - ProContactEndpointMissingError on 404
 *   - ApiError on any other non-2xx
 *   - ZodError if the request body fails validation (programming error)
 */
export async function submitProContact(
  storeId: string,
  body: ProContactRequest,
): Promise<ProContactResponse> {
  // Validate before sending — catches programming errors early.
  const validated = proContactRequestSchema.parse(body)

  let raw: unknown
  try {
    raw = await apiClient.post<unknown>(
      `/api/v1/admin/stores/${storeId}/subscription/pro-contact`,
      validated as Record<string, unknown>,
    )
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      throw new ProContactEndpointMissingError()
    }
    throw err
  }

  return proContactResponseSchema.parse(raw)
}
