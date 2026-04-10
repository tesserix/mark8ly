// components/orders/OrderAddressCard.tsx
//
// Two stacked address blocks (shipping + billing) using simple typography.
// No bordered cards — hairline rule above each label, content underneath.

import type { AdminOrderAddress } from "@/lib/api/marketplace-api";

interface OrderAddressCardProps {
  addresses: AdminOrderAddress[];
}

export function OrderAddressCard({ addresses }: OrderAddressCardProps) {
  const shipping = addresses.find((a) => a.kind === "shipping");
  const billing = addresses.find((a) => a.kind === "billing");
  return (
    <section
      aria-labelledby="order-addresses-heading"
      className="flex flex-col gap-4"
    >
      <h2
        id="order-addresses-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
      >
        Addresses
      </h2>
      <div className="grid grid-cols-1 gap-8 sm:grid-cols-2">
        <AddressBlock title="Shipping to" address={shipping} />
        <AddressBlock title="Billing to" address={billing} />
      </div>
    </section>
  );
}

interface AddressBlockProps {
  title: string;
  address: AdminOrderAddress | undefined;
}

function AddressBlock({ title, address }: AddressBlockProps) {
  return (
    <div className="flex flex-col gap-2 border-t border-[color:var(--ink-900)] border-opacity-15 pt-4">
      <span className="text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
        {title}
      </span>
      {address ? (
        <address className="not-italic text-sm leading-6 text-[color:var(--ink-900)]">
          <div>{address.name}</div>
          <div>{address.line1}</div>
          {address.line2 && <div>{address.line2}</div>}
          <div>
            {address.city}
            {address.region && `, ${address.region}`}
            {address.postal_code && ` ${address.postal_code}`}
          </div>
          <div>{address.country_code}</div>
          {address.phone && <div className="opacity-60">{address.phone}</div>}
        </address>
      ) : (
        <span className="text-sm text-[color:var(--ink-900)] opacity-50">
          Not provided
        </span>
      )}
    </div>
  );
}
