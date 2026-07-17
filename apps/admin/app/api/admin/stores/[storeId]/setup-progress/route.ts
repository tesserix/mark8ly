import { proxyAdminApi } from "@/lib/api/server/proxyAdminApi";

export async function GET(
  request: Request,
  { params }: { params: Promise<{ storeId: string }> },
): Promise<Response> {
  const { storeId } = await params;
  return proxyAdminApi(request, `stores/${storeId}/dashboard/setup-progress`);
}
