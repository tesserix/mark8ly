// Same-origin proxy for the browser-side mediaUploadClient.
//
// Why this exists: components/products/media/mediaUploadClient.ts runs
// in the browser and falls back to a relative URL when
// NEXT_PUBLIC_MARKETPLACE_API_URL is unset (it always is in prod, since
// the marketplace-api is in-cluster only). Without this route the
// upload-url request returns 404 and every media upload silently
// errors out in the MediaTab progress list.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import {
  requestMediaUploadUrl,
  type RequestMediaUploadUrlInput,
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

  let body: RequestMediaUploadUrlInput;
  try {
    body = (await request.json()) as RequestMediaUploadUrlInput;
  } catch {
    return NextResponse.json({ error: "invalid_json" }, { status: 400 });
  }

  const result = await requestMediaUploadUrl(
    storeId,
    productId,
    body,
    { userId, tenantId },
  );
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: 502 },
    );
  }

  // The mediaUploadClient expects the wire shape { url, storage_key,
  // expires_at } (it does the same rename internally). Map back so the
  // proxy is transparent.
  return NextResponse.json({
    url: result.data.upload_url,
    storage_key: result.data.storage_key,
    expires_at: result.data.expires_at,
  });
}
