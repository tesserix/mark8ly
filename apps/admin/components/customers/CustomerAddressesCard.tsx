import type { AdminCustomerAddress } from "@/lib/api/marketplace-api";

interface CustomerAddressesCardProps {
  addresses: AdminCustomerAddress[];
}

export function CustomerAddressesCard({ addresses }: CustomerAddressesCardProps) {
  return (
    <section aria-labelledby="addresses-heading" className="flex flex-col gap-6">
      <h2
        id="addresses-heading"
        className="font-serif text-2xl font-medium text-foreground"
      >
        Addresses
      </h2>

      {addresses.length === 0 ? (
        <p className="text-sm text-foreground-tertiary">
          No saved addresses.
        </p>
      ) : (
        <ul className="flex flex-col gap-4">
          {addresses.map((addr) => (
            <li
              key={addr.id}
              className="border-b border-border-subtle pb-4 last:border-b-0"
            >
              <div className="flex items-start justify-between gap-4">
                <div className="flex flex-col gap-1 text-sm text-foreground">
                  <span className="font-medium">{addr.name}</span>
                  <span className="text-foreground-secondary">{addr.line1}</span>
                  {addr.line2 && (
                    <span className="text-foreground-secondary">{addr.line2}</span>
                  )}
                  <span className="text-foreground-secondary">
                    {addr.city}
                    {addr.region ? `, ${addr.region}` : ""}{" "}
                    {addr.postal_code ?? ""}
                  </span>
                  <span className="text-foreground-tertiary">{addr.country_code}</span>
                  {addr.phone && (
                    <span className="text-foreground-tertiary">{addr.phone}</span>
                  )}
                </div>
                <div className="flex gap-2">
                  {addr.is_default && (
                    <span className="rounded-sm bg-[color:var(--accent-tint)] px-2 py-0.5 text-xs font-medium text-[color:var(--moss-700)]">
                      Default
                    </span>
                  )}
                  {addr.label && (
                    <span className="rounded-sm bg-[color:var(--ink-900)]/[0.06] px-2 py-0.5 text-xs text-foreground-secondary">
                      {addr.label}
                    </span>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
