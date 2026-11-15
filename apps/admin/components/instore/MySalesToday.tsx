import type { MySale } from "@/lib/api/instore-api";
import { formatMoney } from "@/lib/format";

import { PAYMENT_METHODS } from "@/lib/instore/payment-methods";


// The seller's own sales today (#1104): how a cashier, who can't open
// Orders, checks whether a sale went through.
// This renders on the server, so the time zone must be the store's.
export function MySalesToday({
  sales,
  currencyCode,
  timeZone,
}: {
  sales: MySale[] | null;
  currencyCode: string;
  timeZone?: string;
}) {
  const time = new Intl.DateTimeFormat("en-AU", { hour: "numeric", minute: "2-digit", timeZone: timeZone || undefined });
  const newestFirst = sales ? [...sales].sort((a, b) => b.placed_at.localeCompare(a.placed_at)) : null;
  return (
    <section aria-label="Your sales today" className="flex flex-col gap-3">
      <h2 className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">Your sales today</h2>
      {newestFirst === null ? (
        <p className="text-sm text-foreground-secondary">Couldn’t load your sales. Reload the page to try again.</p>
      ) : newestFirst.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No sales yet today.</p>
      ) : (
        <ul className="divide-y divide-border-subtle text-sm">
          {newestFirst.map((s) => (
            <li key={s.id} className="flex items-center justify-between gap-4 py-2">
              <span className="flex min-w-0 flex-col">
                <span className="font-medium">{s.order_number}</span>
                <span className="text-xs text-foreground-secondary">
                  {time.format(new Date(s.placed_at))} ·{" "}
                  {PAYMENT_METHODS.find((m) => m.value === s.payment_method)?.label ?? "Other"}
                </span>
              </span>
              <span className="tabular-nums">{formatMoney(s.grand_total, s.currency_code || currencyCode)}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
