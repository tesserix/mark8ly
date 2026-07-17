import { proxyAdminApi } from "@/lib/api/server/proxyAdminApi";

// SSE passthrough: proxyAdminApi returns the upstream body as a stream,
// so events flush through as marketplace-api emits them. force-dynamic
// keeps Next from ever trying to cache the response.
export const dynamic = "force-dynamic";

export async function GET(
  request: Request,
  { params }: { params: Promise<{ storeId: string }> },
): Promise<Response> {
  const { storeId } = await params;
  return proxyAdminApi(request, `stores/${storeId}/dashboard/setup-progress/stream`);
}
