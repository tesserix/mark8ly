import { AddressBook } from "@/components/account/AddressBook";

export const metadata = {
  title: "My Addresses",
};

export const dynamic = "force-dynamic";
export const revalidate = 0;

export default function AddressesPage() {
  return (
    <div className="space-y-6">
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
        Addresses
      </h1>
      <div className="border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6">
        <AddressBook />
      </div>
    </div>
  );
}
