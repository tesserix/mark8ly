// Shared invoice / receipt PDF render helper.
//
// The storefront has four routes that all render the same PDF:
//   • /api/orders/[id]/invoice          — customer self-download
//   • /api/orders/[id]/receipt          — customer self-download
//   • /api/internal/orders/[id]/invoice — service-to-service for email attach
//   • /api/internal/orders/[id]/receipt — service-to-service for email attach
//
// They differ only in auth, in whether they enforce the delivery gate
// for receipts, and in how they resolve the buyer's email. Everything
// else — fetch order + branding, build doc, render PDF, set headers —
// is identical, so it lives here.

import { pdf } from "@react-pdf/renderer";

import { fetchOrder } from "@/lib/api/checkout-api";
import { fetchBranding } from "@/lib/api/marketplace-api";

import { buildDocument } from "./build";
import { InvoicePdf } from "./InvoicePdf";
import { invoiceNumberFromOrder, receiptNumberFromOrder } from "./numbering";

export type DocumentKind = "invoice" | "receipt";

interface RenderArgs {
  kind: DocumentKind;
  storeSlug: string;
  orderId: string;
  customerEmail?: string;
}

// pdf().toBuffer() is typed as ReadableStream in @react-pdf/renderer's
// d.ts but actually returns a Node Buffer in the Node.js runtime. We
// take the value through `unknown` so the cast doesn't fight TS, then
// hand it straight to the Response which accepts a Buffer at runtime.
export type RenderedPDF = unknown;

export type RenderResult =
  | { ok: true; status: 200; pdfBytes: RenderedPDF; filename: string }
  | { ok: false; status: number; error: string; message?: string };

// renderOrderDocumentPDF performs the shared work for all four routes.
// Returns a RenderResult that callers wrap into a Response (or NextResponse
// for error cases). Splitting it from the route file keeps the routes
// thin and avoids duplicated PDF-render plumbing.
export async function renderOrderDocumentPDF({
  kind,
  storeSlug,
  orderId,
  customerEmail,
}: RenderArgs): Promise<RenderResult> {
  const [order, branding] = await Promise.all([
    fetchOrder(storeSlug, orderId),
    fetchBranding(storeSlug),
  ]);
  if (!order) {
    return { ok: false, status: 404, error: "not_found" };
  }
  if (kind === "receipt") {
    if (!order.shipment || order.shipment.status !== "delivered") {
      return {
        ok: false,
        status: 409,
        error: "not_delivered",
        message: "Receipts are issued once your order has been delivered.",
      };
    }
  }

  const documentNumber =
    kind === "receipt"
      ? receiptNumberFromOrder(order.order_number)
      : invoiceNumberFromOrder(order.order_number);

  // Receipt: prefer the real shipment.delivered_at moment over the
  // order's updated_at, so the PDF stamps the actual delivery time
  // rather than the proxy.
  const paymentDate =
    kind === "receipt"
      ? order.shipment?.delivered_at
      : undefined;

  const doc = buildDocument({
    kind,
    order,
    branding,
    documentNumber,
    customerEmail,
    paymentDate,
  });

  const pdfBytes = (await pdf(InvoicePdf({ doc })).toBuffer()) as unknown;

  return {
    ok: true,
    status: 200,
    pdfBytes,
    filename: `${documentNumber}.pdf`,
  };
}

// pdfResponseHeaders returns the standard headers for serving a generated
// invoice/receipt PDF as an attachment download.
export function pdfResponseHeaders(filename: string): HeadersInit {
  return {
    "Content-Type": "application/pdf",
    "Content-Disposition": `attachment; filename="${filename}"`,
    "Cache-Control": "private, no-store",
  };
}
