// Same-origin proxy for the browser-side finalize-media call.
// See ./upload-url/route.ts for why this proxy is necessary.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import {
  finalizeMedia,
  type FinalizeMediaInput,
} from "@/lib/api/marketplace-api";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ storeId: string; productId: string }> },
): Promise<Response> {
  const { storeId, productId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  let body: FinalizeMediaInput;
  try {
    body = (await request.json()) as FinalizeMediaInput;
  } catch {
    return NextResponse.json({ error: "invalid_json" }, { status: 400 });
  }

  const result = await finalizeMedia(storeId, productId, body, {
    userId,
    tenantId,
  });
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: 502 },
    );
  }
  return NextResponse.json(result.data, { status: 201 });
}
