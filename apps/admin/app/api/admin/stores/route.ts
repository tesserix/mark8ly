import { proxyAdminApi } from "@/lib/api/server/proxyAdminApi";

export async function GET(request: Request): Promise<Response> {
  return proxyAdminApi(request, `stores`);
}
