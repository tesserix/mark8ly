// Deterministic invoice + receipt numbering derived from order_number.
// Mirror of apps/admin/lib/invoices/numbering.ts so admin and customer
// see identical document IDs for the same order.

const ORDER_RE = /^M-([A-Z]{3})-(\d{6})-(\d{5})$/;

export function invoiceNumberFromOrder(orderNumber: string): string {
  const m = ORDER_RE.exec(orderNumber);
  if (!m) return `INV-${orderNumber}`;
  return `INV-${m[1]}-${m[2]}-${m[3]}`;
}

export function receiptNumberFromOrder(orderNumber: string): string {
  const m = ORDER_RE.exec(orderNumber);
  if (!m) return `RCP-${orderNumber}`;
  return `RCP-${m[1]}-${m[2]}-${m[3]}`;
}
