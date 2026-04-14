// Same-origin proxy: PATCH shipment status.
// Admin BFF session headers are read from middleware-set cookies and
// forwarded to marketplace-api. Drives the customer-facing order
// timeline (see services/marketplace-api/.../shipments.go UpdateStatus).

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { updateShipmentStatus } from "@/lib/api/shipping-api";

export async function PATCH(
  request: Request,
  {
    params,
  }: { params: Promise<{ storeId: string; id: string; shipmentId: string }> },
): Promise<Response> {
  const { storeId, id, shipmentId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  let body: { status: string; description?: string };
  try {
    body = (await request.json()) as { status: string; description?: string };
  } catch {
    return NextResponse.json({ error: "invalid_json" }, { status: 400 });
  }
  if (!body.status) {
    return NextResponse.json({ error: "missing_status" }, { status: 400 });
  }

  const result = await updateShipmentStatus(storeId, id, shipmentId, body, {
    userId,
    tenantId,
  });
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: 502 },
    );
  }
  return NextResponse.json(result.data, { status: 200 });
}
