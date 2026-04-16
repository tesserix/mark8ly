// PATCH /api/notifications/read-all?storeId=<uuid>
//
// Marks every unread notification for the store as read. Invoked from
// the "Mark all read" action in NotificationBell.

import { NextResponse } from "next/server";

import { markAllNotificationsRead } from "@/lib/api/settings-tier2-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function PATCH(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const storeId = url.searchParams.get("storeId") ?? "";

  const session = await getServerSessionContext();
  if (!session.userId || !session.tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  if (!storeId || !session.currentStore || session.currentStore.id !== storeId) {
    return NextResponse.json({ error: "forbidden" }, { status: 403 });
  }

  const result = await markAllNotificationsRead(storeId, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: 502 },
    );
  }
  return NextResponse.json({ marked_all: true });
}
