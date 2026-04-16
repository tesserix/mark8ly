// POST /api/admin/stores/:storeId/orders/:id/invoice/email
//
// Same-origin proxy: forwards to marketplace-api's invoice-email endpoint
// which renders the invoice email and dispatches it via SendGrid.

import { NextResponse } from "next/server";

import { sendOrderDocumentEmail } from "@/lib/api/order-doc-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(
  _request: Request,
  { params }: { params: Promise<{ storeId: string; id: string }> },
): Promise<Response> {
  const { storeId, id } = await params;
  const session = await getServerSessionContext();
  if (!session.userId || !session.tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  if (!session.currentStore || session.currentStore.id !== storeId) {
    return NextResponse.json({ error: "forbidden" }, { status: 403 });
  }

  const result = await sendOrderDocumentEmail(storeId, id, "invoice", {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: result.status ?? 502 },
    );
  }
  return NextResponse.json(result.data, { status: 200 });
}
