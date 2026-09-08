// Proxy for "Test connection" (#820). See ../config/route.ts for why the
// tenant comes from the session.
import { resolveAuth } from "@/lib/auth/serverSession";
import { proxyAdminApi } from "@/lib/api/server/proxyAdminApi";

export async function POST(request: Request): Promise<Response> {
  const auth = await resolveAuth();
  if (!auth) {
    return Response.json(
      { error: "unauthorized", message: "session headers missing" },
      { status: 401 },
    );
  }
  return proxyAdminApi(request, `tenants/${auth.tenantId}/sso/test`);
}
