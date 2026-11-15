"use client";

import { formatMoney } from "@/lib/format";

import { ReceiptActions } from "./ReceiptActions";

export function SaleDone({
  storeId,
  orderId,
  orderNumber,
  grandTotal,
  currencyCode,
  onNewSale,
}: {
  storeId: string;
  orderId: string;
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
      {orderId ? <ReceiptActions storeId={storeId} orderId={orderId} /> : null}
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
