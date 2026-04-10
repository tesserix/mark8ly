// components/orders/OrderItemsTable.tsx
//
// Hairline-rule table of order line items. Mirrors the OrdersList visual
// language: column header rule, item rows separated by hairlines, totals
// right-aligned in serif.

import type { AdminOrderItem } from "@/lib/api/marketplace-api";

interface OrderItemsTableProps {
  items: AdminOrderItem[];
  currencyCode: string;
}

export function OrderItemsTable({ items, currencyCode }: OrderItemsTableProps) {
  return (
    <section
      aria-labelledby="order-items-heading"
      className="flex flex-col gap-4"
    >
      <h2
        id="order-items-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
      >
        Items
      </h2>
      <div className="flex flex-col">
        <div
          role="presentation"
          className="grid grid-cols-[minmax(0,3fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)] items-end gap-6 border-b border-[color:var(--ink-900)] border-opacity-15 pb-3 text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60"
        >
          <span>Item</span>
          <span className="text-right">Unit price</span>
          <span className="text-right">Qty</span>
          <span className="text-right">Line total</span>
        </div>
        <ul role="list" className="flex flex-col">
          {items.map((it) => (
            <li
              key={it.id}
              className="grid grid-cols-[minmax(0,3fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)] items-center gap-6 border-b border-[color:var(--ink-900)] border-opacity-10 py-4"
            >
              <span className="flex flex-col gap-1">
                <span className="text-base text-[color:var(--ink-900)]">
                  {it.title_snapshot}
                </span>
                <span className="text-xs text-[color:var(--ink-900)] opacity-60">
                  SKU {it.sku_snapshot}
                  {it.option_summary && ` · ${it.option_summary}`}
                </span>
              </span>
              <span className="text-right text-sm text-[color:var(--ink-900)] opacity-80">
                {formatMoney(it.unit_price, it.currency_code)}
              </span>
              <span className="text-right text-sm text-[color:var(--ink-900)] opacity-80">
                {it.quantity}
              </span>
              <span className="text-right font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-base text-[color:var(--ink-900)]">
                {formatMoney(it.line_total, it.currency_code)}
              </span>
            </li>
          ))}
        </ul>
      </div>
      <span className="sr-only">Currency: {currencyCode}</span>
    </section>
  );
}

function formatMoney(amount: string, currency: string): string {
  const n = Number.parseFloat(amount);
  if (Number.isNaN(n)) return `${currency} ${amount}`;
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(n);
  } catch {
    return `${currency} ${amount}`;
  }
}
