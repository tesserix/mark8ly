// GET /api/admin/stores/:storeId/orders/:id/invoice
//
// Streams a PDF invoice for the given order. The document number is
// deterministically derived from the order_number (see numbering.ts), so
// hitting this endpoint repeatedly always produces the same INV-... id.

import { NextResponse } from "next/server";
import { pdf } from "@react-pdf/renderer";

import { getBranding, getOrder } from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { InvoicePdf } from "@/lib/invoices/InvoicePdf";
import { invoiceNumberFromOrder } from "@/lib/invoices/numbering";
import { buildDocument } from "@/lib/invoices/build";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

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

  const doc = buildDocument({
    kind: "invoice",
    order,
    branding,
    store: session.currentStore,
    documentNumber: invoiceNumberFromOrder(order.order_number),
    contactEmail: session.email,
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
