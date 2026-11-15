// The in-store receipt page's view (#1106): what the customer reads, in the
// order they read it. Kept free of React so it can be tested in node.

export type ReceiptLine = {
  title: string;
  option_summary?: string;
  image_url?: string;
  quantity: number;
  unit_price: string;
  line_total: string;
};

export type Receipt = {
  order_number: string;
  placed_at: string;
  currency_code: string;
  lines: ReceiptLine[];
  subtotal: string;
  discount_total: string;
  tax_total: string;
  grand_total: string;
  tax_label: string;
  time_zone?: string;
  fresh: boolean;
  payment_method?: string;
  payment_method_label?: string;
  receipt_email_masked?: string;
};

const METHOD_LABELS: Record<string, string> = {
  card_external: "Card (EFTPOS)",
  cash: "Cash",
  bank_transfer: "Bank transfer",
};

// A receipt reads in the shop's own convention ("$45.00" in Australia), not
// the viewer's, and must not change with the server's locale.
const CURRENCY_LOCALE: Record<string, string> = { AUD: "en-AU", NZD: "en-NZ", INR: "en-IN", GBP: "en-GB", USD: "en-US" };

function money(amount: string, currency: string): string {
  const n = Number.parseFloat(amount);
  if (!Number.isFinite(n)) return `${currency} ${amount}`;
  try {
    return new Intl.NumberFormat(CURRENCY_LOCALE[currency] ?? "en", { style: "currency", currency }).format(n);
  } catch {
    return `${currency} ${amount}`;
  }
}

export function receiptView(r: Receipt) {
  const m = (a: string) => money(a, r.currency_code);
  const placed = new Intl.DateTimeFormat(CURRENCY_LOCALE[r.currency_code] ?? "en", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: r.time_zone || undefined,
  }).format(
    new Date(r.placed_at),
  );
  const totals: [string, string][] = [["Subtotal", m(r.subtotal)]];
  if (Number.parseFloat(r.discount_total) > 0) totals.push(["Discount", `−${m(r.discount_total)}`]);
  totals.push(["Total", m(r.grand_total)]);
  if (Number.parseFloat(r.tax_total) > 0) totals.push([r.tax_label, m(r.tax_total)]);

  const contact: [string, string][] = [];
  if (r.fresh && r.payment_method) {
    const label = r.payment_method === "other" ? r.payment_method_label || "Other" : METHOD_LABELS[r.payment_method];
    contact.push(["Paid by", label ?? "Other"]);
  }
  if (r.fresh && r.receipt_email_masked) contact.push(["Sent to", r.receipt_email_masked]);

  return {
    orderNumber: r.order_number,
    placed: `In-store purchase · ${placed}`,
    lines: r.lines.map((l) => ({
      title: l.title,
      imageUrl: l.image_url ?? null,
      detail: [l.option_summary, `Qty ${l.quantity}`].filter(Boolean).join(" · "),
      price: m(l.line_total),
    })),
    totals,
    contact,
    offerNewLink: !r.fresh,
  };
}
