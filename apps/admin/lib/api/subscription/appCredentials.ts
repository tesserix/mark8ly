/**
 * App credential upload API client.
 *
 * Endpoints:
 *   POST /api/v1/admin/stores/:storeId/app-credentials/apple  → 204 No Content
 *   POST /api/v1/admin/stores/:storeId/app-credentials/google → 204 No Content
 *
 * Both endpoints accept multipart/form-data — NOT JSON. The API client's
 * post() sends JSON by default, so these functions build FormData and use
 * a raw fetch call via the shared apiClient internals.
 *
 * Credentials are sent as file content in FormData blobs so they travel
 * through the existing multipart handler on the backend. They are never
 * stored in localStorage, sessionStorage, or any persistent browser store.
 */
import { ApiError } from '@/lib/api/client'
import type { AppleCredentialsForm, GoogleCredentialsForm } from './schemas/appCredentials'

// ---------------------------------------------------------------------------
// Internal multipart sender
// ---------------------------------------------------------------------------

async function postMultipart(url: string, formData: FormData): Promise<void> {
  let res: Response
  try {
    res = await fetch(url, {
      method: 'POST',
      credentials: 'include',
      body: formData,
      // Do NOT set Content-Type — the browser sets it with the correct boundary.
    })
  } catch (err) {
    throw new ApiError('network_error', 0, (err as Error).message)
  }

  if (res.status === 204) {
    return
  }

  let errorCode = 'network_error'
  let errorMessage: string | undefined
  try {
    const body = (await res.json()) as { error?: string; message?: string; detail?: string }
    errorCode = body.error ?? 'network_error'
    errorMessage = body.message ?? body.detail
  } catch {
    // Non-JSON body — fall through with generic code
  }

  throw new ApiError(errorCode, res.status, errorMessage)
}

// ---------------------------------------------------------------------------
// Apple
// ---------------------------------------------------------------------------

/**
 * Upload Apple App Store Connect credentials for a store.
 *
 * Sends `p8` (file blob), `issuer_id`, and `key_id` as multipart/form-data.
 * Returns void on success (HTTP 204).
 *
 * @throws ApiError on validation failure (400) or gate failure (403/401).
 */
export async function uploadAppleCredentials(
  storeId: string,
  fields: AppleCredentialsForm,
): Promise<void> {
  const formData = new FormData()

  // Convert the .p8 text content to a Blob so the backend receives it as a
  // file field (matching c.Request.FormFile("p8") in the Go handler).
  const p8Blob = new Blob([fields.p8_contents], { type: 'application/octet-stream' })
  formData.append('p8', p8Blob, 'key.p8')
  formData.append('issuer_id', fields.issuer_id)
  formData.append('key_id', fields.key_id)

  return postMultipart(
    `/api/v1/admin/stores/${storeId}/app-credentials/apple`,
    formData,
  )
}

// ---------------------------------------------------------------------------
// Google
// ---------------------------------------------------------------------------

/**
 * Upload Google Play service account JSON credentials for a store.
 *
 * Sends `service_account_json` as a file blob in multipart/form-data.
 * Returns void on success (HTTP 204).
 *
 * @throws ApiError on validation failure (400) or gate failure (403/401).
 */
export async function uploadGoogleCredentials(
  storeId: string,
  fields: GoogleCredentialsForm,
): Promise<void> {
  const formData = new FormData()

  const jsonBlob = new Blob([fields.service_account_json], { type: 'application/json' })
  formData.append('service_account_json', jsonBlob, 'service-account.json')

  return postMultipart(
    `/api/v1/admin/stores/${storeId}/app-credentials/google`,
    formData,
  )
}
