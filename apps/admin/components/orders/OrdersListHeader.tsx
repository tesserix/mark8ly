// components/orders/OrdersListHeader.tsx
//
// Page header for /orders. No "create" CTA — the admin surface does
// not normally create orders manually; orders flow in via storefront
// checkout. Matches the same header rhythm as `AdminPage` so list and
// settings pages read consistently.

export function OrdersListHeader() {
  return (
    <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
      <div className="min-w-0 flex-1 space-y-3">
        <p className="eyebrow">Operations</p>
        <h1
          id="orders-heading"
          className="font-serif text-4xl font-medium tracking-tight text-foreground text-balance sm:text-5xl"
        >
          Orders
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          Every order placed in this store, with payment, fulfillment, and
          refund state at a glance.
        </p>
      </div>
    </header>
  );
}
