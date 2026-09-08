// Proxy for the tenant SSO configuration (#820).
//
// The tenant id comes from the SESSION, never from the client. marketplace-api
// re-checks that the path tenant matches the authenticated one and refuses a
// mismatch, but building the path from the session means a client cannot even
// ask about another tenant's IdP configuration.
import { resolveAuth } from "@/lib/auth/serverSession";
import { proxyAdminApi } from "@/lib/api/server/proxyAdminApi";

async function backendPath(): Promise<string | null> {
  const auth = await resolveAuth();
  return auth ? `tenants/${auth.tenantId}/sso/config` : null;
}

function unauthorized(): Response {
  return Response.json(
    { error: "unauthorized", message: "session headers missing" },
    { status: 401 },
  );
}

export async function GET(request: Request): Promise<Response> {
  const path = await backendPath();
  return path ? proxyAdminApi(request, path) : unauthorized();
}

export async function POST(request: Request): Promise<Response> {
  const path = await backendPath();
  return path ? proxyAdminApi(request, path) : unauthorized();
}

export async function DELETE(request: Request): Promise<Response> {
  const path = await backendPath();
  return path ? proxyAdminApi(request, path) : unauthorized();
}
