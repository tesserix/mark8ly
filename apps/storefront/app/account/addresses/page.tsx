import { headers } from "next/headers";
import { AddressBook } from "@/components/account/AddressBook";
import { fetchBranding } from "@/lib/api/marketplace-api";
import { resolveStoreSlug } from "@/lib/slug";

export const metadata = {
  title: "My Addresses",
};

export const dynamic = "force-dynamic";
export const revalidate = 0;

export default async function AddressesPage() {
  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));
  // The store's own country. It locks the address form and scopes the
  // address lookup, which otherwise searches the whole world.
  const storeCountry =
    (await fetchBranding(storeSlug))?.store?.country_code?.trim().toUpperCase() || undefined;

  return (
    <div className="space-y-6">
      <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
        Addresses
      </h1>
      <div className="border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6">
        <AddressBook storeCountry={storeCountry} />
      </div>
    </div>
  );
}
