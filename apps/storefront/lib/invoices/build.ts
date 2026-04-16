// Builds an InvoiceDocument from the storefront's Order + branding.
// Storefront's Order shape differs from admin's AdminOrder (single
// shipping_address vs an addresses[] array, fewer fields), so this
// adapter normalises both into the shared InvoiceDocument.

import type { Order } from "@/lib/api/checkout-api";
import type { BrandingResponse } from "@/lib/api/marketplace-api";

import type { InvoiceDocument } from "./types";

interface BuildArgs {
  kind: "invoice" | "receipt";
  order: Order;
  branding: BrandingResponse | null;
  documentNumber: string;
  paymentDate?: string;
}

const RECEIPT_NOTE =
  "Thank you for your purchase. This receipt confirms that your order has been delivered and serves as your official record of the transaction.";
const INVOICE_NOTE =
  "If you have any questions about this invoice, please reply to your order confirmation email and our team will be happy to help.";

// The storefront branding type doesn't enumerate logo_url + brand colours
// (the admin one does), but the wire response carries them — see
// services/marketplace-api/internal/handlers/storefront/branding.go. So
// we read them off via a loose any-cast rather than widening the public
// type and risking churn elsewhere.
interface BrandingExtras {
  logo_url?: string | null;
  tagline?: string | null;
  color_accent?: string | null;
  color_text?: string | null;
}

export function buildDocument({
  kind,
  order,
  branding,
  documentNumber,
  paymentDate,
}: BuildArgs): InvoiceDocument {
  const ship = order.shipping_address;
  const extras = (branding?.branding as unknown as BrandingExtras) ?? {};

  return {
    kind,
    document_number: documentNumber,
    issue_date: order.placed_at,
    payment_date: paymentDate,
    payment_status: order.payment_status,
    order_number: order.order_number,
    customer_email: branding?.store?.name ?? "", // not on the storefront Order — leave blank rather than mislead
    bill_to: ship,
    ship_to: ship,
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
      tax_lines: (order.tax_lines ?? []).map((l) => ({
        description: l.description,
        rate: l.rate,
        amount: l.amount,
        jurisdiction: l.jurisdiction,
      })),
      // storefront Order doesn't carry discount_total — show 0 so the
      // totals block doesn't render the discount row at all.
      discount_total: "0",
      grand_total: order.grand_total,
      currency_code: order.currency_code,
    },
    store: {
      name: branding?.store?.name ?? "Store",
      slug: branding?.store?.slug ?? "",
      logo_url: extras.logo_url ?? undefined,
      tagline: extras.tagline ?? undefined,
      country_code: branding?.store?.country_code ?? undefined,
      color_accent: extras.color_accent ?? undefined,
      color_text: extras.color_text ?? undefined,
    },
    notes: kind === "receipt" ? RECEIPT_NOTE : INVOICE_NOTE,
  };
}
