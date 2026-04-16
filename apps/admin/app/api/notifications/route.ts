// GET /api/notifications?storeId=<uuid>
//
// Lists recent notifications for the given store. Session is resolved
// from middleware headers; the storeId query param must match the
// current session's store to prevent cross-tenant reads.

import { NextResponse } from "next/server";

import { listNotifications } from "@/lib/api/settings-tier2-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const storeId = url.searchParams.get("storeId") ?? "";
  const unreadOnly = url.searchParams.get("unread_only") === "true";

  const session = await getServerSessionContext();
  if (!session.userId || !session.tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  if (!storeId || !session.currentStore || session.currentStore.id !== storeId) {
    return NextResponse.json({ error: "forbidden" }, { status: 403 });
  }

  try {
    const notifications = await listNotifications(storeId, unreadOnly, {
      userId: session.userId,
      tenantId: session.tenantId,
    });
    return NextResponse.json({ notifications });
  } catch (error: unknown) {
    console.error("notifications list failed", error);
    return NextResponse.json(
      { error: "upstream_error", message: "Failed to load notifications." },
      { status: 502 },
    );
  }
}
