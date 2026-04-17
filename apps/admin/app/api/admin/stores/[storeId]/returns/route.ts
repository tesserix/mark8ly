// GET /api/admin/stores/:storeId/returns — lists the RMA inbox for a store.
// Thin proxy — admin middleware injects session headers; we forward them
// to marketplace-api via the typed client.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { listReturns } from "@/lib/api/marketplace-api";

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ storeId: string }> },
): Promise<Response> {
  const { storeId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const email = h.get("x-session-email") ?? undefined;
  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  const data = await listReturns(storeId, { userId, tenantId, email });
  return NextResponse.json({ data: data ?? [] });
}
