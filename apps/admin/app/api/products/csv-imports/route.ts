// apps/admin/app/api/products/csv-imports/route.ts
//
// Thin proxy for client-side listing of CSV import jobs.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { listCsvImports } from "@/lib/api/csvImports";

export async function GET(request: Request): Promise<Response> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  let storeId = h.get("x-session-store-id") ?? "";
  if (!storeId) {
    const tenantId = h.get("x-session-tenant-id") ?? "";
    if (tenantId) {
      const { listStoresByTenant } = await import("@/lib/api/platform-api");
      const stores = await listStoresByTenant(tenantId).catch(() => []);
      storeId = stores[0]?.id ?? "";
    }
  }

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const url = new URL(request.url);
  const page = Number.parseInt(url.searchParams.get("page") ?? "1", 10);
  const pageSize = Number.parseInt(url.searchParams.get("page_size") ?? "10", 10);

  const result = await listCsvImports({ userId, tenantId }, storeId, page, pageSize);

  if (!result) {
    return NextResponse.json({ data: [], meta: { page, page_size: pageSize, total: 0, total_pages: 0 } });
  }

  return NextResponse.json(result);
}
