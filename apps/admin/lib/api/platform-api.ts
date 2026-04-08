/**
 * platform-api client used by the admin server components. Only the
 * endpoints admin actually needs — nothing speculative. Right now
 * that's just the tenant lookup used by the dashboard.
 */

const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";

export interface Tenant {
  id: string;
  slug: string;
  name: string;
  owner_user_id: string;
  owner_email: string;
  country_code: string;
  currency_code: string;
  timezone: string;
  status: string;
  created_at: string;
  updated_at: string;
}

interface TenantResponse {
  data: Tenant;
}

/**
 * Fetches a tenant row by ID. Hits the internal route at
 * /internal/tenants/:id which platform-api exposes for auth-bff + admin
 * use. Returns null on 404 or network failure so callers can branch
 * cleanly on "not found" without try/catching around every call site.
 */
export async function fetchTenant(id: string): Promise<Tenant | null> {
  if (!id) return null;
  try {
    const res = await fetch(`${PLATFORM_API_URL}/internal/tenants/${id}`, {
      cache: "no-store",
    });
    if (res.status === 404) return null;
    if (!res.ok) return null;
    const body = (await res.json()) as TenantResponse;
    return body.data;
  } catch {
    return null;
  }
}

/**
 * Patches the editable subset of a tenant row. Phase N ships with only
 * `name` editable; additional fields will be added as follow-up slices
 * (timezone picker, currency change with billing warning, etc).
 *
 * Hits the internal route at PATCH /internal/tenants/:id which platform-
 * api exposes for trusted in-cluster callers. The admin server action
 * is the only caller today and has already validated the session's
 * tenant id against the path param before invoking this.
 *
 * Throws PlatformApiError on any non-2xx so callers can map the error
 * code to a user-friendly message inline in the form.
 */
export async function updateTenant(
  id: string,
  patch: { name?: string },
): Promise<Tenant> {
  const res = await fetch(`${PLATFORM_API_URL}/internal/tenants/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
    cache: "no-store",
  });
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new PlatformApiError(
      res.status,
      body.error ?? "platform_api_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const body = (await res.json()) as TenantResponse;
  return body.data;
}

export class PlatformApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

interface TenantSummary {
  id: string;
  slug: string;
  name: string;
  owner_user_id: string;
  owner_email: string;
}

/**
 * Looks up a workspace tenant by GIP UID. Used by the /login server
 * action to bridge a freshly minted GIP id_token to the workspace_tenant
 * field auth-bff /auth/auto-login requires.
 *
 * Throws on any non-2xx so the caller can map the error code to a
 * user-friendly message ("no store found for this account" on 404, etc).
 */
export async function getTenantByOwner(uid: string): Promise<TenantSummary> {
  const res = await fetch(
    `${PLATFORM_API_URL}/api/v1/tenants/by-owner?uid=${encodeURIComponent(uid)}`,
    { cache: "no-store" },
  );
  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    throw new PlatformApiError(
      res.status,
      body.error ?? "platform_api_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }
  const body = (await res.json()) as { data: TenantSummary };
  return body.data;
}
