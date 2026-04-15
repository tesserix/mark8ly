// apps/storefront/app/categories/page.tsx
//
// Storefront category index page. Lists every active category in the
// store as a grid of links. The site's not-found.tsx advertises
// "Browse categories" as a fallback CTA, so this route must exist —
// otherwise the 404 page loops back to another 404.

import type { Metadata } from "next";
import Link from "next/link";
import { headers } from "next/headers";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { listCategories } from "@/lib/api/marketplace-api";
import { slugFromHost } from "@/lib/slug";
import { StorefrontNav } from "@/components/StorefrontNav";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Categories",
};

export default async function CategoriesIndexPage() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    slugFromHost(host) || process.env.DEFAULT_STORE_SLUG || "default";

  const store = await fetchStoreBySlug(storeSlug).catch(() => null);
  const categories = await listCategories(storeSlug);

  return (
    <div className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <StorefrontNav storeName={store?.name} />
      <main
        id="main"
        className="mx-auto max-w-6xl px-6 py-12 sm:px-8"
      >
        <header className="max-w-2xl">
          <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-[color:var(--storefront-text,var(--ink-900))] opacity-55">
            Browse
          </p>
          <h1 className="mt-4 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium text-[color:var(--storefront-text,var(--ink-900))] sm:text-5xl">
            Categories
          </h1>
          <p className="mt-4 text-base leading-7 text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
            Every section of {store?.name ?? "the store"}, at a glance.
          </p>
        </header>

        {categories.length === 0 ? (
          <div className="mt-12 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-12">
            <p className="max-w-xl text-base leading-7 text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
              No categories yet — the catalog is still being put together.
            </p>
            <Link
              href="/"
              className="mt-4 inline-block text-sm font-semibold text-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
            >
              Back home →
            </Link>
          </div>
        ) : (
          <ul className="mt-12 grid gap-x-6 gap-y-10 sm:grid-cols-2 lg:grid-cols-3">
            {categories.map((c) => (
              <li key={c.slug}>
                <Link
                  href={`/categories/${encodeURIComponent(c.slug)}`}
                  className="group block border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-4 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
                >
                  <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-55">
                    Collection
                  </p>
                  <h2 className="mt-2 font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--storefront-text,var(--ink-900))] transition-opacity group-hover:opacity-80">
                    {c.name}
                  </h2>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </main>
    </div>
  );
}
