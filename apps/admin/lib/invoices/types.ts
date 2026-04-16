// Data shape consumed by the InvoicePdf / ReceiptPdf components.
// Decoupled from the AdminOrder / Order wire types so the same generator
// works from either app (admin or storefront).

export interface DocumentLine {
  description: string;
  sku?: string;
  quantity: number;
  unit_price: string;
  line_total: string;
}

export interface DocumentAddress {
  name: string;
  line1: string;
  line2?: string;
  city: string;
  region?: string;
  postal_code?: string;
  country_code: string;
  phone?: string;
}

export interface DocumentStore {
  name: string;
  slug: string;
  // Optional store-level contact / location info pulled from branding
  // (footer_tagline / footer_copyright are good loose homes for these).
  tagline?: string;
  contact_email?: string;
  // ISO 3166-1 alpha-2 country of the seller. Drives the tax-block
  // heading: "Tax breakdown" generically, "GST breakdown" for IN.
  country_code?: string;
  // Brand surface — used to colour the accent strip and the title.
  logo_url?: string;
  color_accent?: string;
  color_text?: string;
}

// One row in the per-jurisdiction tax breakdown rendered above the
// totals strip (CGST / SGST / IGST / VAT / state+county+city splits).
export interface DocumentTaxLine {
  description: string;
  rate: string; // decimal as string, e.g. "9.00"
  amount: string; // decimal as string
  jurisdiction?: string;
}

export interface DocumentTotals {
  subtotal: string;
  shipping_total: string;
  tax_total: string;
  // tax_lines: when populated, the PDF shows each line and suppresses
  // the aggregate "Tax" row in totals (avoids double-counting on screen).
  tax_lines?: DocumentTaxLine[];
  discount_total: string;
  grand_total: string;
  refunded_amount?: string;
  currency_code: string;
}

export interface InvoiceDocument {
  kind: "invoice" | "receipt";
  document_number: string;
  issue_date: string; // ISO
  payment_date?: string; // ISO — receipt only, when payment cleared
  payment_status: string;
  order_number: string;
  customer_email: string;
  customer_name?: string;
  bill_to: DocumentAddress;
  ship_to?: DocumentAddress;
  lines: DocumentLine[];
  totals: DocumentTotals;
  store: DocumentStore;
  notes?: string;
}
