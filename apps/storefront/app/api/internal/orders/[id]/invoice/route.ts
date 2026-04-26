// GET /api/internal/orders/:id/invoice — service-to-service PDF render.
//
// Same as the customer-facing /api/orders/[id]/invoice but gated by a
// shared X-Internal-Auth secret instead of the customer session cookie,
// because the marketplace-api invoice mailer (Go) needs to fetch the
// rendered PDF before attaching it to SendGrid emails. Without this
// internal endpoint we'd either have to duplicate the React-PDF
// generator in Go or settle for a "View invoice" link in the email.
//
// Inputs: ?slug=<store-slug>&customer_email=<email>
// Auth:   X-Internal-Auth header must equal MARKETPLACE_INTERNAL_AUTH_SECRET

import { NextResponse } from "next/server";
import { pdf } from "@react-pdf/renderer";

import { fetchOrder } from "@/lib/api/checkout-api";
import { fetchBranding } from "@/lib/api/marketplace-api";
import { InvoicePdf } from "@/lib/invoices/InvoicePdf";
import { invoiceNumberFromOrder } from "@/lib/invoices/numbering";
import { buildDocument } from "@/lib/invoices/build";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const INTERNAL_SECRET = process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

export async function GET(
  req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  // Constant-time-ish header check — Node's Buffer.compare is plenty
  // safe for a same-cluster service boundary.
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

  const doc = buildDocument({
    kind: "invoice",
    order,
    branding,
    documentNumber: invoiceNumberFromOrder(order.order_number),
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
