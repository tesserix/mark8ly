// GET /api/internal/orders/:id/receipt — service-to-service PDF render.
// Same idea as the invoice variant — gated by X-Internal-Auth so the
// marketplace-api receipt mailer can attach the PDF directly. The
// shared render helper enforces the post-delivery gate.

import { NextResponse } from "next/server";

import { pdfResponseHeaders, renderOrderDocumentPDF } from "@/lib/invoices/render";

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

  const result = await renderOrderDocumentPDF({
    kind: "receipt",
    storeSlug: slug,
    orderId: id,
    customerEmail: customerEmail || undefined,
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
