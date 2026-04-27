import type { Metadata } from "next";
import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { SecurityClient } from "./SecurityClient";

export const metadata: Metadata = {
  title: "Security",
  robots: { index: false, follow: false },
};

export default async function SecurityPage() {
  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));
  const store = await fetchStoreBySlug(storeSlug).catch(() => null);

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-[color:var(--storefront-text,var(--ink-900))] opacity-55">
          {store?.name ?? "Account"}
        </p>
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          Security
        </h1>
        <p className="text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
          Manage how you sign in to {store?.name ?? "this store"}.
        </p>
      </header>

      <SecurityClient storeSlug={storeSlug} />
    </div>
  );
}
