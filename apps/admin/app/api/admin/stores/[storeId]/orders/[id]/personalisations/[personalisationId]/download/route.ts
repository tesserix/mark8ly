// apps/admin/app/api/admin/stores/[storeId]/orders/[id]/personalisations/[personalisationId]/download/route.ts
//
// Proxy that mints a signed link to one buyer-supplied original (#968).
//
// Returns the LINK rather than the bytes, unlike the shipping-label
// proxy next door. The label has to be streamed because the carrier
// demands an Authorization header the browser cannot send; artwork lives
// behind a signed GCS URL that the browser can fetch directly, and
// streaming a 20 MB photograph through two Node processes to achieve the
// same download would be waste with a timeout attached.
//
// The marketplace-api call is audited, so there is no "quiet" path to a
// buyer's photograph: every link minted is a row.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import { getArtworkLink } from "@/lib/api/marketplace-api";

export async function GET(
  _request: Request,
  {
    params,
  }: {
    params: Promise<{
      storeId: string;
      id: string;
      personalisationId: string;
    }>;
  },
): Promise<Response> {
  const { storeId, id: orderId, personalisationId } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const link = await getArtworkLink(storeId, orderId, personalisationId, {
    userId,
    tenantId,
  });
  if (!link) {
    return NextResponse.json({ error: "not_found" }, { status: 404 });
  }
  return NextResponse.json(link, {
    // The link expires in minutes; a cached copy is a broken copy.
    headers: { "Cache-Control": "no-store" },
  });
}
