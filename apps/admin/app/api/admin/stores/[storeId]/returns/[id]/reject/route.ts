import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { rejectReturn } from "@/lib/api/marketplace-api";

export async function POST(
  req: Request,
  { params }: { params: Promise<{ storeId: string; id: string }> },
): Promise<Response> {
  const { storeId, id } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const email = h.get("x-session-email") ?? undefined;
  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  const body = (await req.json().catch(() => ({}))) as { reason?: string };
  if (!body.reason || !body.reason.trim()) {
    return NextResponse.json(
      { error: "missing_reason", message: "reason is required" },
      { status: 400 },
    );
  }
  const result = await rejectReturn(
    storeId,
    id,
    body.reason.trim(),
    { userId, tenantId, email },
  );
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: 400 },
    );
  }
  return NextResponse.json(result.data);
}
