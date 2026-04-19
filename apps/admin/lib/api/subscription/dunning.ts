/**
 * Dunning API client.
 *
 * Wraps:
 *   GET /api/v1/admin/stores/:storeId/subscription/complete-action
 *
 * The complete-action endpoint issues a 302 redirect to the Stripe-hosted
 * invoice URL. We use `redirect: 'manual'` to capture the Location header
 * without following it, so the merchant can be sent there in a new tab.
 *
 * Note: `hosted_invoice_url`, `retry_at`, `last_retry_failed_at`, and
 * `first_flagged_at` are NOT exposed on the GET subscription wire DTO.
 * The dunning state hooks derive from the fields that ARE available
 * (status, current_period_end). See useDunning.ts for the full TODO notes.
 */

/**
 * Fetch the Stripe hosted-invoice URL for a pending payment action.
 *
 * The Go handler (CompleteActionHandler.Redirect) responds with:
 *   - 302  → hosted_invoice_url set on the subscription row (follow to get URL)
 *   - 404  → no pending action (hosted_invoice_url is null/empty)
 *   - 401/403 → auth/tenant failure
 *
 * We call with `redirect: 'manual'` so the browser doesn't auto-follow the
 * redirect; we then read `response.url` (the redirect target) directly.
 *
 * Throws an Error when:
 *   - The server returns 404 (no pending action) — callers should guard with
 *     the `isPending` flag from `usePaymentActionRequiredState` before calling.
 *   - Network or unexpected HTTP errors.
 */
export async function getCompleteActionUrl(
  storeId: string,
): Promise<{ url: string }> {
  const endpoint = `/api/v1/admin/stores/${storeId}/subscription/complete-action`

  // Use fetch directly with redirect:'manual' so we capture the Location
  // header without navigating. The apiClient does not support manual-redirect
  // mode, so we bypass it here.
  const response = await fetch(endpoint, {
    method: 'GET',
    credentials: 'include',
    redirect: 'manual',
  })

  // fetch with redirect:'manual' returns an opaque-redirect response (type
  // 'opaqueredirect') whose `.url` is empty in some environments. Fall back
  // to reading the Location header from the initial (non-followed) response.
  if (response.type === 'opaqueredirect' || response.status === 302) {
    const location = response.headers.get('Location')
    if (location) {
      return { url: location }
    }
    // opaqueredirect hides headers in the browser — try response.url instead.
    if (response.url) {
      return { url: response.url }
    }
  }

  if (response.status === 404) {
    throw new Error('no_pending_action')
  }

  if (!response.ok) {
    throw new Error(`complete-action: unexpected status ${response.status}`)
  }

  // Fallback: if the response was somehow followed and resolved to the
  // Stripe URL, use the final URL.
  if (response.url) {
    return { url: response.url }
  }

  throw new Error('complete-action: could not determine redirect URL')
}
