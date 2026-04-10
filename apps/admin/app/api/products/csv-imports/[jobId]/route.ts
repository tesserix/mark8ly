// apps/admin/app/api/products/csv-imports/[jobId]/route.ts
//
// Thin proxy for client-side polling of CSV import job status.
// The client page fetches this route every 2s; it forwards to
// marketplace-api with session headers.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { getCsvImportStatus } from "@/lib/api/csvImports";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ jobId: string }> },
): Promise<Response> {
  const { jobId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  // storeId from query param or session — for now, use the session store
  const storeId = h.get("x-session-store-id") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const job = await getCsvImportStatus(
    { userId, tenantId },
    storeId,
    jobId,
  );

  if (!job) {
    return NextResponse.json({ error: "not_found" }, { status: 404 });
  }

  return NextResponse.json(job);
}
