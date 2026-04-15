// Same-origin proxy for branding image uploads (logo, favicon, hero,
// aside, section). Mirrors the products-media upload-url proxy in
// ../products/[productId]/media/upload-url/route.ts.
//
// Why this exists: apps/admin/lib/brandingUpload.ts runs in the browser
// and cannot call marketplace-api directly in prod — the service is
// in-cluster only, and even where exposed via the ingress the cross-
// origin POST trips the istio authz policy (RBAC: access denied). By
// routing through a same-origin /api/... path in the admin Next.js
// server we pick up the session cookie via the session-user headers
// set by middleware and proxy internally to marketplace-api.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import {
  requestBrandingUploadUrl,
  type RequestBrandingUploadUrlInput,
} from "@/lib/api/marketplace-api";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ storeId: string }> },
): Promise<Response> {
  const { storeId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  let body: RequestBrandingUploadUrlInput;
  try {
    body = (await request.json()) as RequestBrandingUploadUrlInput;
  } catch {
    return NextResponse.json({ error: "invalid_json" }, { status: 400 });
  }

  const result = await requestBrandingUploadUrl(storeId, body, {
    userId,
    tenantId,
  });
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: 502 },
    );
  }

  return NextResponse.json(result.data);
}
