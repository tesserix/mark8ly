// components/orders/OrdersListHeader.tsx
//
// Page header for /orders. No "create" CTA — the admin surface does
// not normally create orders manually; orders flow in via storefront
// checkout. The header is intentionally quiet (editorial spirit per
// .impeccable.md) — left-aligned serif title, brief muted subtitle.

export function OrdersListHeader() {
  return (
    <header className="flex flex-col gap-2">
      <h1
        id="orders-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl text-[color:var(--ink-900)]"
      >
        Orders
      </h1>
      <p className="max-w-prose text-[color:var(--ink-900)] opacity-70">
        Every order placed in this store, with payment, fulfillment, and
        refund state at a glance.
      </p>
    </header>
  );
}
