// apps/admin/app/api/admin/stores/[storeId]/orders/[orderId]/shipments/[shipmentId]/label/route.ts
//
// Proxy for downloading the shipping-label PDF from marketplace-api.
// The backend holds the Delhivery API token — we can't deep-link to
// the carrier's packing-slip URL because it requires an Authorization
// header. Instead the browser hits this route (same origin as the
// admin app, so cookies flow naturally), we forward the session
// headers to marketplace-api, and stream the PDF bytes back as a file
// download.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { downloadShipmentLabel } from "@/lib/api/shipping-api";

export async function GET(
  _request: Request,
  {
    params,
  }: {
    params: Promise<{ storeId: string; orderId: string; shipmentId: string }>;
  },
): Promise<Response> {
  const { storeId, orderId, shipmentId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const result = await downloadShipmentLabel(storeId, orderId, shipmentId, {
    userId,
    tenantId,
  });
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: 502 },
    );
  }
  return new Response(result.data.bytes, {
    headers: {
      "Content-Type": result.data.contentType,
      "Content-Disposition": `attachment; filename="shipping-label-${shipmentId}.pdf"`,
    },
  });
}
