// apps/admin/app/api/admin/stores/[storeId]/orders/[id]/personalisations/download/route.ts
//
// Every buyer-supplied original on one order, as signed links (#968).
// Same shape and same reasoning as the single-item proxy beside this
// one — see its header for why links and not bytes.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { getAllArtworkLinks } from "@/lib/api/marketplace-api";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ storeId: string; id: string }> },
): Promise<Response> {
  const { storeId, id: orderId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const artwork = await getAllArtworkLinks(storeId, orderId, {
    userId,
    tenantId,
  });
  if (!artwork) {
    return NextResponse.json({ error: "not_found" }, { status: 404 });
  }
  return NextResponse.json(
    { artwork },
    { headers: { "Cache-Control": "no-store" } },
  );
}
