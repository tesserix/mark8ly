import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { approveReturn } from "@/lib/api/marketplace-api";

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
  const body = (await req.json().catch(() => ({}))) as {
    pickup_details?: string;
  };
  const result = await approveReturn(
    storeId,
    id,
    body.pickup_details ?? "",
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
