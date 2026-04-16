import { StorefrontNav } from "@/components/StorefrontNav";
import { GiftCardBalanceForm } from "@/components/gift-cards/GiftCardBalanceForm";
import { headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";

export default async function GiftCardBalancePage() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    await resolveStoreSlug(host);

  return (
    <div className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <StorefrontNav />
      <main id="main" className="mx-auto max-w-2xl px-6 py-16 sm:px-8">
        <header className="space-y-3">
          <p className="text-xs font-medium uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))]/60">
            Gift cards
          </p>
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-4xl font-medium tracking-tight text-[color:var(--storefront-text,var(--ink-900))]">
            Check your balance
          </h1>
          <p className="text-base leading-7 text-[color:var(--storefront-text,var(--ink-900))]/80">
            Enter your gift card code below. Dashes and spaces are fine — we'll
            sort the rest out.
          </p>
        </header>

        <hr className="my-10 border-[color:var(--storefront-text,var(--ink-900))]/20" />

        <GiftCardBalanceForm storeSlug={storeSlug} />
      </main>
    </div>
  );
}
