/**
 * Tenant SSO configuration client (#820).
 *
 * Wraps the proxy routes under /api/admin/sso, which build the tenant-scoped
 * backend path from the SESSION — a client never names a tenant.
 */
import { apiClient, ApiError } from '@/lib/api/client'
import {
  REDACTED,
  ssoConfigSchema,
  ssoTestResultSchema,
  type SSOConfig,
  type SSOConfigInput,
  type SSOTestResult,
} from './schemas'

const CONFIG_PATH = '/api/admin/sso/config'
const TEST_PATH = '/api/admin/sso/test'

/**
 * The tenant's SSO configuration, or null when none is set up.
 *
 * A 404 is not an error here — "no SSO configured" is the normal state for
 * every tenant that has not set it up, and the screen renders an empty form
 * for it. Every other status still throws.
 */
export async function getSSOConfig(): Promise<SSOConfig | null> {
  try {
    return ssoConfigSchema.parse(await apiClient.get<unknown>(CONFIG_PATH))
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null
    throw err
  }
}

/**
 * Saves the configuration.
 *
 * `client_secret_ref` is omitted when it still holds the server's redaction
 * placeholder: the server returns `[redacted]` in place of a saved reference,
 * and sending that back would overwrite the real path with the literal string
 * — leaving a config that looks correct on screen and cannot resolve a secret.
 */
export async function saveSSOConfig(input: SSOConfigInput): Promise<void> {
  const metadata: Record<string, string> = {
    issuer: input.metadata.issuer.trim(),
    client_id: input.metadata.client_id.trim(),
  }
  const ref = input.metadata.client_secret_ref.trim()
  if (ref && ref !== REDACTED) metadata.client_secret_ref = ref

  const redirect = input.metadata.redirect_uri?.trim()
  if (redirect) metadata.redirect_uri = redirect
  const scopes = input.metadata.scopes?.trim()
  if (scopes) metadata.scopes = scopes

  await apiClient.post<unknown>(CONFIG_PATH, {
    provider: input.provider,
    metadata,
    enabled: input.enabled,
  })
}

/** Removes the configuration and every JIT user mapping under it. */
export async function deleteSSOConfig(): Promise<void> {
  await apiClient.del<unknown>(CONFIG_PATH)
}

/**
 * Asks the server to reach the identity provider.
 *
 * Uses fetch directly rather than apiClient, for one reason: a FAILED test is
 * a 422 whose BODY is the answer — `ok: false` plus a reason the merchant
 * needs to read. apiClient turns any non-2xx into an ApiError that does not
 * carry the parsed body, so the interesting half of this endpoint's contract
 * would be thrown away. "Your IdP could not be reached" is a successful
 * answer to the question the merchant asked.
 *
 * Only a request that could not be made at all throws.
 */
export async function testSSOConnection(): Promise<SSOTestResult> {
  const res = await fetch(TEST_PATH, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
  })

  if (res.status !== 200 && res.status !== 422) {
    throw new ApiError('sso_test_failed', res.status, 'could not run the connection test')
  }
  return ssoTestResultSchema.parse(await res.json())
}
