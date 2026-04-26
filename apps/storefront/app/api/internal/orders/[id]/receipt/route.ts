// GET /api/internal/orders/:id/receipt — service-to-service PDF render.
// Same idea as the invoice variant — gated by X-Internal-Auth so the
// marketplace-api receipt mailer can attach the PDF directly.
// Receipt is only issued after delivery; same gating as the customer
// route.

import { NextResponse } from "next/server";
import { pdf } from "@react-pdf/renderer";

import { fetchOrder } from "@/lib/api/checkout-api";
import { fetchBranding } from "@/lib/api/marketplace-api";
import { InvoicePdf } from "@/lib/invoices/InvoicePdf";
import { receiptNumberFromOrder } from "@/lib/invoices/numbering";
import { buildDocument } from "@/lib/invoices/build";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const INTERNAL_SECRET = process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

export async function GET(
  req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const provided = req.headers.get("x-internal-auth") ?? "";
  if (!INTERNAL_SECRET || provided !== INTERNAL_SECRET) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const { id } = await ctx.params;
  if (!id) return NextResponse.json({ error: "missing_id" }, { status: 400 });

  const url = new URL(req.url);
  const slug = url.searchParams.get("slug") ?? "";
  const customerEmail = url.searchParams.get("customer_email") ?? "";
  if (!slug) {
    return NextResponse.json({ error: "missing_slug" }, { status: 400 });
  }

  const [order, branding] = await Promise.all([
    fetchOrder(slug, id),
    fetchBranding(slug),
  ]);
  if (!order) {
    return NextResponse.json({ error: "not_found" }, { status: 404 });
  }
  if (!order.shipment || order.shipment.status !== "delivered") {
    return NextResponse.json(
      { error: "not_delivered", message: "Receipt is issued after delivery." },
      { status: 409 },
    );
  }

  const doc = buildDocument({
    kind: "receipt",
    order,
    branding,
    documentNumber: receiptNumberFromOrder(order.order_number),
  });
  doc.customer_email = customerEmail || (order as { customer_email?: string }).customer_email || "";

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
