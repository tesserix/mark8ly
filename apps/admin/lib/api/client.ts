/**
 * Shared fetch wrapper for admin client components.
 * Runs in the browser — no server-only imports.
 *
 * Handles:
 *   - 204 → null
 *   - 402 subscription_inactive → SubscriptionInactiveError + optional handler
 *   - Other non-2xx with JSON body → ApiError
 *   - Non-JSON error body → ApiError('network_error')
 *   - Network failure → re-throws with wrapping
 */

type SubscriptionInactiveStatus = 'expired' | 'store_closed' | 'pending_hard_delete'

export class SubscriptionInactiveError extends Error {
  constructor(public readonly status: SubscriptionInactiveStatus) {
    super(`subscription is ${status}`)
    this.name = 'SubscriptionInactiveError'
  }
}

export class ApiError extends Error {
  constructor(
    public readonly code: string,
    public readonly status: number,
    message?: string,
    /**
     * Machine-readable sub-reason from the error body's `reason` field,
     * when the endpoint sends one.
     *
     * `code` is coarse and stable — it names the class of failure and UI
     * branches on it. `reason` is the detail within that class, so an
     * endpoint can add a case without every existing caller having to learn
     * a new `code`. apply-promo is the first user: one `code`
     * (`promo_invalid_or_expired`), eight reasons, each a different sentence
     * the merchant can act on (#770).
     *
     * Undefined for every endpoint that does not send one, which is most of
     * them — always fall back to `message` rather than assuming it is here.
     */
    public readonly reason?: string,
  ) {
    super(message ?? code)
    this.name = 'ApiError'
  }
}

export interface ApiClientOptions {
  baseURL?: string
  onSubscriptionInactive?: (status: string) => void
}

// Module-level handler for the default apiClient instance.
let _globalInactiveHandler: ((status: string) => void) | undefined

export function registerSubscriptionInactiveHandler(
  handler: (status: string) => void,
): void {
  _globalInactiveHandler = handler
}

type RequestBody = Record<string, unknown> | unknown[] | null

function buildHeaders(withBody: boolean): Record<string, string> {
  const headers: Record<string, string> = {}
  if (withBody) {
    headers['Content-Type'] = 'application/json'
  }
  return headers
}

interface ErrorBody {
  error?: string
  message?: string
  status?: string
  reason?: string
}

async function parseErrorBody(res: Response): Promise<ErrorBody> {
  try {
    return (await res.json()) as ErrorBody
  } catch {
    return {}
  }
}

async function handleResponse<T>(
  res: Response,
  onSubscriptionInactive?: (status: string) => void,
): Promise<T> {
  if (res.status === 402) {
    const body = await parseErrorBody(res)
    if (body.error === 'subscription_inactive') {
      const inactiveStatus = body.status ?? 'expired'
      if (onSubscriptionInactive) {
        onSubscriptionInactive(inactiveStatus)
      }
      throw new SubscriptionInactiveError(
        inactiveStatus as SubscriptionInactiveStatus,
      )
    }
  }

  if (res.status === 204) {
    return null as T
  }

  if (res.ok) {
    return res.json() as Promise<T>
  }

  const body = await parseErrorBody(res)
  throw new ApiError(
    body.error ?? 'network_error',
    res.status,
    body.message,
    body.reason,
  )
}

export function createApiClient(opts: ApiClientOptions = {}) {
  const base = opts.baseURL ?? ''
  const onInactive = opts.onSubscriptionInactive

  function resolveUrl(url: string): string {
    return url.startsWith('http') ? url : `${base}${url}`
  }

  async function request<T>(url: string, init: RequestInit): Promise<T> {
    try {
      const res = await fetch(resolveUrl(url), {
        credentials: 'include',
        ...init,
      })
      return handleResponse<T>(res, onInactive)
    } catch (err) {
      if (err instanceof SubscriptionInactiveError || err instanceof ApiError) {
        throw err
      }
      throw new ApiError('network_error', 0, (err as Error).message)
    }
  }

  return {
    get<T>(url: string, init?: RequestInit): Promise<T> {
      return request<T>(url, {
        method: 'GET',
        ...init,
        headers: { ...buildHeaders(false), ...init?.headers },
      })
    },
    post<T>(url: string, body: RequestBody, init?: RequestInit): Promise<T> {
      return request<T>(url, {
        method: 'POST',
        body: JSON.stringify(body),
        ...init,
        headers: { ...buildHeaders(true), ...init?.headers },
      })
    },
    patch<T>(url: string, body: RequestBody, init?: RequestInit): Promise<T> {
      return request<T>(url, {
        method: 'PATCH',
        body: JSON.stringify(body),
        ...init,
        headers: { ...buildHeaders(true), ...init?.headers },
      })
    },
    put<T>(url: string, body: RequestBody, init?: RequestInit): Promise<T> {
      return request<T>(url, {
        method: 'PUT',
        body: JSON.stringify(body),
        ...init,
        headers: { ...buildHeaders(true), ...init?.headers },
      })
    },
    del<T>(url: string, init?: RequestInit): Promise<T> {
      return request<T>(url, {
        method: 'DELETE',
        ...init,
        headers: { ...buildHeaders(false), ...init?.headers },
      })
    },
  }
}

// Default instance — wires onSubscriptionInactive to the module-level handler
// so app-level router context can register a redirect on init via
// registerSubscriptionInactiveHandler().
export const apiClient = createApiClient({
  onSubscriptionInactive: (status) => {
    _globalInactiveHandler?.(status)
  },
})
