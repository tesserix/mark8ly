"use client";

import { ReceiptActions } from "@/components/instore/ReceiptActions";

// Email or reprint the receipt for an in-store sale from its order page (#1107).
export function InStoreReceiptCard({
  storeId,
  orderId,
  channel,
}: {
  storeId: string;
  orderId: string;
  channel?: string;
}) {
  if (channel !== "in_store") return null;
  return (
    <section aria-labelledby="receipt-heading" className="flex flex-col gap-4">
      <h2
        id="receipt-heading"
        className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Receipt
      </h2>
      <ReceiptActions storeId={storeId} orderId={orderId} />
    </section>
  );
}
