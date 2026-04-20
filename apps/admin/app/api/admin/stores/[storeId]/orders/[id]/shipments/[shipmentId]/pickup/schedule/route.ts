// Same-origin proxy: POST shipment pickup/schedule.
// Forwards the merchant's "Reschedule pickup" click (or an omitted
// body, which means "use defaults") to marketplace-api's
// SchedulePickup handler. That handler type-asserts
// shipping.PickupScheduler on the carrier, so a 422 comes back for
// carriers that don't implement pickup scheduling.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { schedulePickup } from "@/lib/api/shipping-api";

export async function POST(
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

  // Body is optional — the "use the carrier-config defaults" case sends
  // an empty POST. Tolerate invalid JSON the same way the backend does.
  let body: { date?: string; slot_start?: string } = {};
  try {
    body = (await request.json()) as { date?: string; slot_start?: string };
  } catch {
    body = {};
  }

  const result = await schedulePickup(storeId, id, shipmentId, body, {
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
