// How an in-store sale was paid and who rang it up (#1109). Renders nothing
// for online orders.

import { PAYMENT_METHODS } from "@/components/instore/ChargeSheet";
import type { AdminOrder } from "@/lib/api/marketplace-api";
import { change } from "@/lib/instore/cart";
import { formatMoney } from "@/lib/format";

// Counter sales are handed over in person: no address, no label to buy.
export function showsShipping(order: AdminOrder): boolean {
  return order.channel !== "in_store";
}

export function InStoreSaleCard({ order }: { order: AdminOrder }) {
  if (order.channel !== "in_store") return null;

  const method =
    order.payment_method === "other"
      ? order.payment_method_label || "Other"
      : PAYMENT_METHODS.find((m) => m.value === order.payment_method)?.label;
  const due = order.cash_tendered ? change(order.cash_tendered, order.grand_total) : null;
  const rows: [string, string][] = [
    ["Paid by", method ?? "—"],
    ...(order.cash_tendered
      ? ([
          ["Cash given", formatMoney(order.cash_tendered, order.currency_code)],
          ["Change", formatMoney(due ?? "0", order.currency_code)],
        ] as [string, string][])
      : []),
    ["Rung up by", order.sold_by_email || "A team member"],
    ...(order.notes ? ([["Note", order.notes]] as [string, string][]) : []),
  ];

  return (
    <section aria-labelledby="in-store-heading" className="flex flex-col gap-4">
      <h2
        id="in-store-heading"
        className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        In-store sale
      </h2>
      <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
        {rows.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-foreground-secondary">{k}</dt>
            <dd className="break-words text-foreground">{v}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
