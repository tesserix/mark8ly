// POST /api/admin/stores/:storeId/orders/:id/receipt/email
//
// Same-origin proxy: forwards to marketplace-api's receipt-email endpoint
// which renders the receipt email and dispatches it via SendGrid. An
// optional `note` field on the request body is forwarded as the admin's
// personal message ("Note from {store}" block in the email).

import { NextResponse } from "next/server";

import { sendOrderDocumentEmail } from "@/lib/api/order-doc-api";
import { getServerSessionContext, resolveAuth } from "@/lib/auth/serverSession";

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
  // Defensive resolver — see invoice/email/route.ts for the full
  // rationale. Same surprise-401 close.
  const auth = await resolveAuth();
  if (!auth) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  const session = await getServerSessionContext();
  const ownerStoreId =
    session.currentStore?.id ??
    session.stores.find((s) => s.id === storeId)?.id ??
    null;
  if (ownerStoreId !== storeId) {
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
    { userId: auth.userId, tenantId: auth.tenantId },
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
