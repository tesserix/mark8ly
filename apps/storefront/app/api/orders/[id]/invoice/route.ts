// GET /api/orders/:id/invoice — customer-facing PDF invoice.
//
// Authenticated via the mp_customer_session cookie. The storefront's
// fetchOrder enforces that the customer can only read their own order
// (server-side check in marketplace-api), so we don't need a separate
// authorisation step here.

import { cookies, headers } from "next/headers";
import { NextResponse } from "next/server";
import { pdf } from "@react-pdf/renderer";

import { fetchOrder } from "@/lib/api/checkout-api";
import { fetchBranding } from "@/lib/api/marketplace-api";
import { resolveStoreSlug } from "@/lib/slug";
import { decodeSessionForScope } from "@/lib/session";
import { InvoicePdf } from "@/lib/invoices/InvoicePdf";
import { invoiceNumberFromOrder } from "@/lib/invoices/numbering";
import { buildDocument } from "@/lib/invoices/build";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(
  _req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const { id } = await ctx.params;
  if (!id) {
    return NextResponse.json({ error: "missing_id" }, { status: 400 });
  }

  const h = await headers();
  const slug = await resolveStoreSlug(h.get("host"));
  if (!slug) {
    return NextResponse.json({ error: "missing_store" }, { status: 400 });
  }

  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value ?? "";
  const session = decodeSessionForScope(sessionCookie, { storeSlug: slug });
  if (!session) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
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
  doc.customer_email = session.email;

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
