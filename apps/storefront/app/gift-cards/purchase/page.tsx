import { StorefrontNav } from "@/components/StorefrontNav";
import { GiftCardPurchaseForm } from "@/components/gift-cards/GiftCardPurchaseForm";
import { headers } from "next/headers";
import { slugFromHost } from "@/lib/slug";

// Resolve the store's currency server-side (same pattern as other
// storefront pages). We proxy the branding endpoint instead of a
// dedicated store info endpoint — branding carries the data we need.
async function getStoreCurrency(origin: string): Promise<string> {
  try {
    const res = await fetch(`${origin}/api/storefront/store-currency`, {
      cache: "no-store",
    });
    if (!res.ok) return "INR";
    const body = await res.json();
    return body?.currency ?? "INR";
  } catch {
    return "INR";
  }
}

export default async function GiftCardPurchasePage() {
  const h = await headers();
  const host = h.get("host");
  const proto = h.get("x-forwarded-proto") ?? "https";
  const origin = host ? `${proto}://${host}` : "";
  const storeSlug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "default";
  const currency = await getStoreCurrency(origin);

  return (
    <div className="min-h-screen bg-[color:var(--paper-200)]">
      <StorefrontNav />
      <main id="main" className="mx-auto max-w-2xl px-6 py-16 sm:px-8">
        <header className="space-y-3">
          <p className="text-xs font-medium uppercase tracking-[0.18em] text-ink-500">
            Gift cards
          </p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium tracking-tight text-ink-900">
            Buy a gift card
          </h1>
          <p className="text-base leading-7 text-ink-700">
            Choose an amount and where to send it. After payment clears we'll
            email the code to the recipient instantly.
          </p>
        </header>

        <hr className="my-10 border-ink-200" />

        <GiftCardPurchaseForm storeSlug={storeSlug} currency={currency} origin={origin} />
      </main>
    </div>
  );
}
