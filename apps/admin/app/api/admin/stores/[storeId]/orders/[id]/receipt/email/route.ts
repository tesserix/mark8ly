// POST /api/admin/stores/:storeId/orders/:id/receipt/email
//
// Same-origin proxy: forwards to marketplace-api's receipt-email endpoint
// which renders the receipt email and dispatches it via SendGrid. An
// optional `note` field on the request body is forwarded as the admin's
// personal message ("Note from {store}" block in the email).

import { NextResponse } from "next/server";

import { sendOrderDocumentEmail } from "@/lib/api/order-doc-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

interface RequestBody {
  note?: string;
}

export async function POST(
  request: Request,
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

  let note = "";
  try {
    const body = (await request.json().catch(() => ({}))) as RequestBody;
    note = (body.note ?? "").trim();
  } catch {
    note = "";
  }

  const result = await sendOrderDocumentEmail(
    storeId,
    id,
    "receipt",
    { userId: session.userId, tenantId: session.tenantId },
    { note },
  );
  if (!result.ok) {
    return NextResponse.json(
      { error: result.error.code, message: result.error.message },
      { status: result.status ?? 502 },
    );
  }
  return NextResponse.json(result.data, { status: 200 });
}
