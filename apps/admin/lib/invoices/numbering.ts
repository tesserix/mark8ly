// Deterministic invoice + receipt numbering derived from order_number.
//
// order_number format: M-{STORE3}-{YYMMDD}-{SEQ5}   e.g. M-PLA-260416-01154
// invoice number:      INV-{STORE3}-{YYMMDD}-{SEQ5}
// receipt number:      RCP-{STORE3}-{YYMMDD}-{SEQ5}
//
// One order = one invoice + one receipt. The mapping is 1:1 and stable,
// so regenerating the PDF always yields the same document number — no
// DB writes needed.

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
