import { headers } from "next/headers";

import { fetchStoreBySlug, type PublicStore } from "@/lib/api/platform-api";
import { slugFromHost } from "@/lib/slug";

/**
 * Phase S — storefront home page.
 *
 * Resolves the requested store by:
 *   1. ?slug= query param (dev override — the test harness and
 *      local curl both use this)
 *   2. Host header subdomain (e.g. acme.mark8ly.com → "acme")
 *   3. DEFAULT_STORE_SLUG env var (dev fallback when neither of
 *      the above work — useful when running on bare localhost)
 *
 * Renders a minimal "store landing" for the first real customer-
 * facing page on the platform. No product catalog, no cart — just
 * confirming the brand, currency, and location so the storefront
 * can be pointed at a real domain during infra bring-up.
 */
export const dynamic = "force-dynamic";

interface PageProps {
  searchParams: Promise<{ slug?: string }>;
}

export default async function StoreHomePage({ searchParams }: PageProps) {
  const { slug: slugFromQuery } = await searchParams;
  const h = await headers();
  const host = h.get("host");

  const slug =
    slugFromQuery ||
    slugFromHost(host) ||
    process.env.DEFAULT_STORE_SLUG ||
    "";

  const store = slug ? await fetchStoreBySlug(slug) : null;

  if (!store) {
    return <StoreNotFound slug={slug} />;
  }

  return <StoreLanding store={store} />;
}

function StoreLanding({ store }: { store: PublicStore }) {
  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col items-start justify-center gap-8 px-6 py-20">
      <header>
        <p className="text-xs font-semibold uppercase tracking-[0.24em] text-neutral-500">
          {store.slug}.mark8ly.com
        </p>
        <h1 className="mt-4 font-serif text-5xl font-medium tracking-tight text-neutral-900 sm:text-6xl">
          {store.name}
        </h1>
        <p className="mt-4 max-w-xl text-lg leading-8 text-neutral-600">
          We&apos;re getting the shop ready. Check back soon for the full
          catalogue.
        </p>
      </header>

      <div className="flex flex-wrap items-center gap-3">
        <Chip label="Currency" value={store.currency_code} />
        <Chip label="Country" value={store.country_code} />
        <Chip label="Timezone" value={store.timezone} />
      </div>

      <footer className="mt-auto border-t border-neutral-200 pt-6 text-sm text-neutral-500">
        Powered by{" "}
        <strong className="font-semibold text-neutral-700">Mark8ly</strong>
      </footer>
    </main>
  );
}

function Chip({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-neutral-200 bg-white px-3 py-1.5 text-sm text-neutral-700 shadow-sm">
      <span className="text-[11px] font-semibold uppercase tracking-[0.14em] text-neutral-400">
        {label}
      </span>
      <span className="font-medium text-neutral-900">{value}</span>
    </span>
  );
}

function StoreNotFound({ slug }: { slug: string }) {
  return (
    <main className="mx-auto flex min-h-screen max-w-2xl flex-col items-start justify-center gap-6 px-6 py-20">
      <p className="text-xs font-semibold uppercase tracking-[0.24em] text-neutral-500">
        Store not found
      </p>
      <h1 className="font-serif text-5xl font-medium tracking-tight text-neutral-900">
        Nothing here yet
      </h1>
      <p className="max-w-xl text-lg leading-8 text-neutral-600">
        {slug
          ? `We couldn't find a store at "${slug}". The URL may be wrong, or the store isn't live yet.`
          : "This domain isn't pointed at a live store yet."}
      </p>
    </main>
  );
}
