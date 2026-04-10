// components/orders/OrderTotalsCard.tsx
//
// Subtotal / shipping / tax / discount / refunded / grand total summary.
// Right-aligned column, hairline rule above the grand total which is set
// in serif for editorial weight.

import type { AdminOrder } from "@/lib/api/marketplace-api";

interface OrderTotalsCardProps {
  order: AdminOrder;
}

export function OrderTotalsCard({ order }: OrderTotalsCardProps) {
  const refunded = Number.parseFloat(order.refunded_amount);
  const showRefunded = Number.isFinite(refunded) && refunded > 0;
  return (
    <section
      aria-labelledby="order-totals-heading"
      className="flex flex-col gap-4"
    >
      <h2
        id="order-totals-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
      >
        Totals
      </h2>
      <dl className="flex flex-col gap-2 text-sm text-[color:var(--ink-900)]">
        <Row label="Subtotal" value={formatMoney(order.subtotal, order.currency_code)} />
        <Row label="Shipping" value={formatMoney(order.shipping_total, order.currency_code)} />
        <Row label="Tax" value={formatMoney(order.tax_total, order.currency_code)} />
        {Number.parseFloat(order.discount_total) > 0 && (
          <Row label="Discount" value={`− ${formatMoney(order.discount_total, order.currency_code)}`} />
        )}
        {showRefunded && (
          <Row
            label="Refunded"
            value={`− ${formatMoney(order.refunded_amount, order.currency_code)}`}
            tone="muted"
          />
        )}
        <div className="mt-2 flex items-baseline justify-between border-t border-[color:var(--ink-900)] border-opacity-15 pt-3">
          <dt className="text-base text-[color:var(--ink-900)]">Grand total</dt>
          <dd className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-2xl text-[color:var(--ink-900)]">
            {formatMoney(order.grand_total, order.currency_code)}
          </dd>
        </div>
      </dl>
    </section>
  );
}

interface RowProps {
  label: string;
  value: string;
  tone?: "default" | "muted";
}

function Row({ label, value, tone = "default" }: RowProps) {
  const cls = tone === "muted" ? "opacity-70" : "";
  return (
    <div className="flex items-baseline justify-between">
      <dt className={`text-[color:var(--ink-900)] ${cls}`}>{label}</dt>
      <dd className={`text-[color:var(--ink-900)] ${cls}`}>{value}</dd>
    </div>
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
