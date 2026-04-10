import type { AdminCustomerAddress } from "@/lib/api/marketplace-api";

interface CustomerAddressesCardProps {
  addresses: AdminCustomerAddress[];
}

export function CustomerAddressesCard({ addresses }: CustomerAddressesCardProps) {
  return (
    <section aria-labelledby="addresses-heading" className="flex flex-col gap-6">
      <h2
        id="addresses-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-2xl text-[color:var(--ink-900)]"
      >
        Addresses
      </h2>

      {addresses.length === 0 ? (
        <p className="text-sm text-[color:var(--ink-900)] opacity-60">
          No saved addresses.
        </p>
      ) : (
        <ul className="flex flex-col gap-4">
          {addresses.map((addr) => (
            <li
              key={addr.id}
              className="border-b border-[color:var(--ink-900)] border-opacity-10 pb-4 last:border-b-0"
            >
              <div className="flex items-start justify-between">
                <div className="flex flex-col gap-1 text-sm text-[color:var(--ink-900)]">
                  <span className="font-medium">{addr.name}</span>
                  <span className="opacity-80">{addr.line1}</span>
                  {addr.line2 && (
                    <span className="opacity-80">{addr.line2}</span>
                  )}
                  <span className="opacity-80">
                    {addr.city}
                    {addr.region ? `, ${addr.region}` : ""}{" "}
                    {addr.postal_code ?? ""}
                  </span>
                  <span className="opacity-60">{addr.country_code}</span>
                  {addr.phone && (
                    <span className="opacity-60">{addr.phone}</span>
                  )}
                </div>
                <div className="flex gap-2">
                  {addr.is_default && (
                    <span className="rounded-sm bg-[color:var(--moss-700)] bg-opacity-10 px-2 py-0.5 text-xs font-medium text-[color:var(--moss-700)]">
                      Default
                    </span>
                  )}
                  {addr.label && (
                    <span className="rounded-sm bg-[color:var(--ink-900)] bg-opacity-[0.06] px-2 py-0.5 text-xs text-[color:var(--ink-900)] opacity-70">
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
