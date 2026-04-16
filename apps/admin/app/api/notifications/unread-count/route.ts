// GET /api/notifications/unread-count?storeId=<uuid>
//
// Returns the unread notification count for the bell badge. Polled
// every 30s by the NotificationBell client component.

import { NextResponse } from "next/server";

import { getUnreadCount } from "@/lib/api/settings-tier2-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const storeId = url.searchParams.get("storeId") ?? "";

  const session = await getServerSessionContext();
  if (!session.userId || !session.tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  if (!storeId || !session.currentStore || session.currentStore.id !== storeId) {
    return NextResponse.json({ error: "forbidden" }, { status: 403 });
  }

  try {
    const unread_count = await getUnreadCount(storeId, {
      userId: session.userId,
      tenantId: session.tenantId,
    });
    return NextResponse.json({ unread_count });
  } catch (error: unknown) {
    console.error("notifications unread-count failed", error);
    return NextResponse.json({ unread_count: 0 });
  }
}
