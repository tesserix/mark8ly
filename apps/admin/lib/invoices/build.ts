// Builds the InvoiceDocument shape from the admin AdminOrder + branding
// + store records. Shared between the invoice and receipt routes.

import type { AdminOrder, StoreBranding } from "@/lib/api/marketplace-api";
import type { Store } from "@/lib/api/platform-api";

import type { InvoiceDocument } from "./types";

interface BuildArgs {
  kind: "invoice" | "receipt";
  order: AdminOrder;
  branding: StoreBranding | null;
  store: Store;
  documentNumber: string;
  contactEmail?: string;
  paymentDate?: string;
}

const RECEIPT_NOTE =
  "Thank you for your purchase. This receipt confirms that your order has been delivered and serves as your official record of the transaction.";
const INVOICE_NOTE =
  "If you have any questions about this invoice, please reply to your order confirmation email and our team will be happy to help.";

export function buildDocument({
  kind,
  order,
  branding,
  store,
  documentNumber,
  contactEmail,
  paymentDate,
}: BuildArgs): InvoiceDocument {
  const shipping = order.addresses.find((a) => a.kind === "shipping");
  const billing = order.addresses.find((a) => a.kind === "billing") ?? shipping;
  if (!billing) {
    // Fall back to a name-only stub so we never blow up at render time.
    // This shouldn't happen for real orders — addresses are required at
    // checkout — but guards us against malformed test data.
    const stub = {
      kind: "billing",
      name: order.customer_name ?? order.customer_email,
      line1: "—",
      city: "—",
      country_code: "",
    };
    return finalize(kind, order, stub, undefined, branding, store, documentNumber, contactEmail, paymentDate);
  }
  return finalize(kind, order, billing, shipping, branding, store, documentNumber, contactEmail, paymentDate);
}

function finalize(
  kind: "invoice" | "receipt",
  order: AdminOrder,
  billing: NonNullable<ReturnType<AdminOrder["addresses"]["find"]>>,
  shipping: ReturnType<AdminOrder["addresses"]["find"]>,
  branding: StoreBranding | null,
  store: Store,
  documentNumber: string,
  contactEmail?: string,
  paymentDate?: string,
): InvoiceDocument {
  return {
    kind,
    document_number: documentNumber,
    issue_date: order.placed_at,
    payment_date: paymentDate,
    payment_status: order.payment_status,
    order_number: order.order_number,
    customer_email: order.customer_email,
    customer_name: order.customer_name,
    bill_to: billing,
    ship_to: shipping,
    lines: order.items.map((it) => ({
      description: it.title_snapshot + (it.option_summary ? ` — ${it.option_summary}` : ""),
      sku: it.sku_snapshot || undefined,
      quantity: it.quantity,
      unit_price: it.unit_price,
      line_total: it.line_total,
    })),
    totals: {
      subtotal: order.subtotal,
      shipping_total: order.shipping_total,
      tax_total: order.tax_total,
      discount_total: order.discount_total,
      grand_total: order.grand_total,
      refunded_amount: order.refunded_amount,
      currency_code: order.currency_code,
    },
    store: {
      name: store.name,
      slug: store.slug,
      logo_url: branding?.logo_url ?? store.logo_url ?? undefined,
      tagline: branding?.tagline ?? undefined,
      contact_email: contactEmail,
      color_accent: branding?.color_accent ?? undefined,
      color_text: branding?.color_text ?? undefined,
    },
    notes: kind === "receipt" ? RECEIPT_NOTE : INVOICE_NOTE,
  };
}
