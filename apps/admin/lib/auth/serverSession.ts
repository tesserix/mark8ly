import { headers } from "next/headers";

import { fetchTenant, type Tenant } from "@/lib/api/platform-api";

/**
 * Helper used by every authenticated server component that needs to
 * render the admin chrome. Pulls the session headers that middleware
 * set, fetches the tenant, and returns the combined shape the shell
 * expects.
 *
 * Since middleware has already gated the request, the session headers
 * are guaranteed to be present by the time this runs. `fetchTenant`
 * may still return null if platform-api is unreachable — callers
 * should render gracefully in that case rather than crash.
 */
export interface ServerSessionContext {
  userId: string;
  email: string;
  tenantId: string;
  tenant: Tenant | null;
  tenantName: string;
}

export async function getServerSessionContext(): Promise<ServerSessionContext> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const email = h.get("x-session-email") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const tenant = await fetchTenant(tenantId);
  const tenantName = tenant?.name ?? "your store";

  return { userId, email, tenantId, tenant, tenantName };
}
