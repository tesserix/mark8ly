import { headers } from "next/headers";
import { notFound } from "next/navigation";
import type { Metadata } from "next";

import { fetchBranding } from "@/lib/api/marketplace-api";
import { resolveStoreSlug } from "@/lib/slug";
import { StorefrontNav } from "@/components/StorefrontNav";

export const metadata: Metadata = {
  title: "Returns",
  // Shopper-facing, not a search target — same treatment as the other
  // transactional pages.
  robots: { index: false, follow: true },
};

async function getStoreSlug(): Promise<string> {
  const h = await headers();
  return resolveStoreSlug(h.get("host"));
}

/**
 * The merchant's return policy.
 *
 * Settings → Branding → Policies tells the merchant this text is "linked
 * from the storefront footer and shown on checkout for regulated regions",
 * and migration 000037 says it is used "by storefront policy pages". It was
 * collected, counted toward the go-live checklist, and rendered nowhere a
 * shopper could reach (#891). This is the page that sentence described.
 */
export default async function ReturnsPolicyPage() {
  const storeSlug = await getStoreSlug();
  const data = storeSlug ? await fetchBranding(storeSlug) : null;

  const policy = data?.branding?.return_policy?.trim();
  // 404 when unset, so this route and the footer link agree: the link only
  // renders when there is text to show.
  if (!policy) notFound();

  return (
    <>
      <StorefrontNav />
      <main className="mx-auto w-full max-w-2xl px-4 py-16">
        <h1 className="font-serif text-3xl text-[color:var(--storefront-text,var(--ink-900))]">
          Returns
        </h1>
        {/* Plain text by contract — the admin field states "no HTML" and the
            service trims it. whitespace-pre-line keeps the merchant's
            paragraph breaks; never rendered as HTML. */}
        <div className="mt-8 whitespace-pre-line text-[color:var(--storefront-text,var(--ink-900))]/80">
          {policy}
        </div>
      </main>
    </>
  );
}
