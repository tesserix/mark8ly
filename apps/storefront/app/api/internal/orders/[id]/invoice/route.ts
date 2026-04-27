// GET /api/internal/orders/:id/invoice — service-to-service PDF render.
//
// Same render path as /api/orders/[id]/invoice but gated by a shared
// X-Internal-Auth secret instead of the customer session cookie. The
// marketplace-api invoice mailer (Go) calls this so it can attach the
// rendered PDF to SendGrid emails without duplicating the React-PDF
// generator in Go. Without this internal endpoint we'd either ship two
// separate generators or settle for a "View invoice" link-only email.
//
// Inputs: ?slug=<store-slug>&customer_email=<email>
// Auth:   X-Internal-Auth header must equal MARKETPLACE_INTERNAL_AUTH_SECRET

import { NextResponse } from "next/server";

import { pdfResponseHeaders, renderOrderDocumentPDF } from "@/lib/invoices/render";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const INTERNAL_SECRET = process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

export async function GET(
  req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  // Constant-time-ish header check — Node's strict equality is plenty
  // safe for a same-cluster service boundary protected by Istio mTLS.
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
    kind: "invoice",
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
