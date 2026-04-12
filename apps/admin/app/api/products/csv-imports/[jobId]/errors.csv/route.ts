// apps/admin/app/api/products/csv-imports/[jobId]/errors.csv/route.ts
//
// Proxy for downloading the error CSV from marketplace-api.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { downloadErrorCsv } from "@/lib/api/csvImports";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ jobId: string }> },
): Promise<Response> {
  const { jobId } = await params;
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

  try {
    const blob = await downloadErrorCsv({ userId, tenantId }, storeId, jobId);
    return new Response(blob, {
      headers: {
        "Content-Type": "text/csv",
        "Content-Disposition": `attachment; filename="import-errors-${jobId}.csv"`,
      },
    });
  } catch {
    return NextResponse.json({ error: "download_failed" }, { status: 502 });
  }
}
