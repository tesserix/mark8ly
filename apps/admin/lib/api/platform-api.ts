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
  country_code: string;
  currency_code: string;
  timezone: string;
  created_at: string;
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
