"use client";

import { formatMoney } from "@/lib/format";

// Receipt buttons (#1106, #1107) join "New sale" here.
export function SaleDone({
  orderNumber,
  grandTotal,
  currencyCode,
  onNewSale,
}: {
  orderNumber: string;
  grandTotal: string;
  currencyCode: string;
  onNewSale: () => void;
}) {
  return (
    <section aria-label="Sale recorded" className="flex flex-col items-start gap-3 rounded-md border border-border p-4">
      <p className="text-2xl font-semibold">Sale recorded</p>
      <p className="text-sm">
        Order {orderNumber} · {formatMoney(grandTotal, currencyCode)}
      </p>
      <button
        type="button"
        onClick={onNewSale}
        autoFocus
        className="min-h-12 rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
      >
        New sale
      </button>
    </section>
  );
}
