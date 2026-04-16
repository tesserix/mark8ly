// GET /api/admin/stores/:storeId/orders/:id/receipt
//
// Streams a PDF receipt for a paid order. Unlike the invoice route this
// one gates on payment_status — only paid / partially_refunded orders
// can have a receipt issued.

import { NextResponse } from "next/server";
import { pdf } from "@react-pdf/renderer";

import { getBranding, getOrder } from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { InvoicePdf } from "@/lib/invoices/InvoicePdf";
import { receiptNumberFromOrder } from "@/lib/invoices/numbering";
import { buildDocument } from "@/lib/invoices/build";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const PAID_STATUSES = new Set(["paid", "captured", "partially_refunded"]);

export async function GET(
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

  const [order, branding] = await Promise.all([
    getOrder(storeId, id, { userId: session.userId, tenantId: session.tenantId }),
    getBranding(storeId, { userId: session.userId, tenantId: session.tenantId }),
  ]);
  if (!order) {
    return NextResponse.json({ error: "not_found" }, { status: 404 });
  }
  if (!PAID_STATUSES.has(order.payment_status)) {
    return NextResponse.json(
      { error: "not_paid", message: "Receipts are only issued for paid orders." },
      { status: 409 },
    );
  }

  const doc = buildDocument({
    kind: "receipt",
    order,
    branding,
    store: session.currentStore,
    documentNumber: receiptNumberFromOrder(order.order_number),
    contactEmail: session.email,
    // The payment timestamp isn't on AdminOrder yet; use updated_at as a
    // reasonable proxy for "when this was last reconciled to paid".
    paymentDate: order.updated_at,
  });

  const buffer = await pdf(InvoicePdf({ doc })).toBuffer();

  return new Response(buffer as unknown as BodyInit, {
    status: 200,
    headers: {
      "Content-Type": "application/pdf",
      "Content-Disposition": `attachment; filename="${doc.document_number}.pdf"`,
      "Cache-Control": "private, no-store",
    },
  });
}
