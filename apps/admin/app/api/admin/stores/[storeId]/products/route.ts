// Same-origin proxy for client-side product search — powers
// the ProductSlugPicker in the homepage sections editor so the
// browser never has to talk to marketplace-api directly (session
// headers are injected by the middleware layer on this side).

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { listProducts } from "@/lib/api/marketplace-api";

export async function GET(
  request: Request,
  { params }: { params: Promise<{ storeId: string }> },
): Promise<Response> {
  const { storeId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const url = new URL(request.url);
  const search = url.searchParams.get("search") ?? undefined;
  const pageSize = Number(url.searchParams.get("page_size")) || 20;

  const result = await listProducts(
    storeId,
    { search, pageSize, status: "active" },
    { userId, tenantId },
  );
  if (result === null) {
    return NextResponse.json({ data: [], meta: { page: 1, page_size: 0, total: 0 } });
  }
  return NextResponse.json(result);
}
