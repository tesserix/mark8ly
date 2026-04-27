// GET /api/orders/:id/receipt — customer-facing PDF receipt.
// Issued only once the merchant has marked the shipment as delivered;
// paid-but-pending-delivery orders intentionally don't have a receipt
// yet (use the invoice instead). Render is delegated to the shared
// helper which enforces the delivery gate.

import { cookies, headers } from "next/headers";
import { NextResponse } from "next/server";

import { decodeSessionForScope } from "@/lib/session";
import { resolveStoreSlug } from "@/lib/slug";
import { pdfResponseHeaders, renderOrderDocumentPDF } from "@/lib/invoices/render";

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

  const result = await renderOrderDocumentPDF({
    kind: "receipt",
    storeSlug: slug,
    orderId: id,
    customerEmail: session.email,
  });

  if (!result.ok) {
    return NextResponse.json(
      { error: result.error, message: result.message },
      { status: result.status },
    );
  }

  return new Response(result.pdfBytes as unknown as BodyInit, {
    status: 200,
    headers: pdfResponseHeaders(result.filename),
  });
}
