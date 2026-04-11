import type { CSSProperties } from "react";
import type { Metadata } from "next";
import { headers } from "next/headers";

import { fetchStoreBySlug, type PublicStore } from "@/lib/api/platform-api";
import { listProducts } from "@/lib/api/marketplace-api";
import {
  fontStacks,
  normalizeStorefrontTheme,
  themeRadius,
  themeSpacing,
  type StorefrontTheme,
} from "@repo/ui/storefront-theme";
import { slugFromHost } from "@/lib/slug";
import { makeTenantMetadata } from "@/lib/seo";
import { StorefrontLayoutRenderer } from "@/components/layouts";
import { FeaturedProducts } from "@/components/FeaturedProducts";
import { StorefrontNav } from "@/components/StorefrontNav";

export const dynamic = "force-dynamic";

interface PageProps {
  searchParams: Promise<{ slug?: string }>;
}

export async function generateMetadata({
  searchParams,
}: PageProps): Promise<Metadata> {
  const { slug: slugFromQuery } = await searchParams;
  const h = await headers();
  const host = h.get("host");
  const slug =
    slugFromQuery ||
    slugFromHost(host) ||
    process.env.DEFAULT_STORE_SLUG ||
    "";
  const store = slug ? await fetchStoreBySlug(slug).catch(() => null) : null;
  return makeTenantMetadata(store, slug);
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

  // Probe the catalog so an empty store doesn't render editorial
  // layouts advertising "three pieces we love" when there are zero
  // products. We ask for 1 page of 1 product as a cheap signal.
  const productsResponse = await listProducts(store.slug, { pageSize: 1 });
  const hasProducts = (productsResponse?.data ?? []).length > 0;

  if (!hasProducts) {
    return <StoreComingSoon store={store} />;
  }

  return <StoreLanding store={store} />;
}

function StoreLanding({ store }: { store: PublicStore }) {
  const theme = normalizeStorefrontTheme(store.storefront_theme);
  const style = themeStyle(theme);

  return (
    <main
      id="main"
      className="min-h-screen"
      style={{
        background:
          theme.preset === "midnight"
            ? theme.colors.background
            : `radial-gradient(circle at top left, ${theme.colors.accent}18, transparent 24%), radial-gradient(circle at top right, ${theme.colors.primary}12, transparent 28%), ${theme.colors.background}`,
        color: theme.colors.text,
        fontFamily: "var(--store-body-font)",
        ...style,
      }}
    >
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <TopBar store={store} theme={theme} />
        <StorefrontLayoutRenderer store={store} theme={theme} />
        <FeaturedProducts storeSlug={store.slug} />
      </div>
    </main>
  );
}

/**
 * Empty-store landing. Rendered when the catalog has zero products.
 * Deliberately skips the editorial layouts — otherwise the page ships
 * live to customers claiming "three pieces we love right now" alongside
 * empty "Reserved for product photography" tiles, which reads as
 * broken rather than pre-launch. Here we say "coming soon" plainly and
 * keep the brand voice without lying about the catalog.
 */
function StoreComingSoon({ store }: { store: PublicStore }) {
  const theme = normalizeStorefrontTheme(store.storefront_theme);
  const style = themeStyle(theme);

  return (
    <main
      id="main"
      className="min-h-screen"
      style={{
        background: theme.colors.background,
        color: theme.colors.text,
        fontFamily: "var(--store-body-font)",
        ...style,
      }}
    >
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <section className="mt-24 max-w-2xl">
          <p
            className="text-[11px] font-semibold uppercase tracking-[0.24em]"
            style={{ color: `${theme.colors.primary}99` }}
          >
            {store.name}
          </p>
          <h1
            className="mt-6 text-5xl font-medium tracking-tight sm:text-6xl"
            style={{
              fontFamily: "var(--store-heading-font)",
              color: theme.colors.text,
            }}
          >
            Coming soon.
          </h1>
          <p
            className="mt-6 text-lg leading-8 opacity-75"
            style={{ color: theme.colors.text }}
          >
            {store.name} is getting ready. There&apos;s nothing to buy yet —
            the catalog is still being put together. Check back in a little
            while.
          </p>
        </section>
      </div>
    </main>
  );
}

function TopBar({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <header className="mb-10 flex flex-wrap items-center justify-between gap-4">
      <div>
        <p
          className="text-[11px] font-semibold uppercase tracking-[0.24em]"
          style={{ color: `${theme.colors.primary}CC` }}
        >
          {store.slug}.mark8ly.com
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <MetaChip label="Currency" value={store.currency_code} theme={theme} />
          <MetaChip label="Country" value={store.country_code} theme={theme} />
          <MetaChip label="Timezone" value={store.timezone} theme={theme} />
        </div>
      </div>
      <div
        className="rounded-full border px-4 py-2 text-xs font-semibold uppercase tracking-[0.18em]"
        style={{
          borderColor: `${theme.colors.primary}33`,
          background: `${theme.colors.surface}CC`,
          color: theme.colors.primary,
        }}
      >
        Powered by mark8ly
      </div>
    </header>
  );
}

function MetaChip({
  label,
  value,
  theme,
}: {
  label: string;
  value: string;
  theme: StorefrontTheme;
}) {
  return (
    <span
      className="inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm shadow-sm"
      style={{
        background: `${theme.colors.surface}E6`,
        borderColor: `${theme.colors.primary}22`,
      }}
    >
      <span className="text-[11px] font-semibold uppercase tracking-[0.14em] opacity-55">
        {label}
      </span>
      <span className="font-medium">{value}</span>
    </span>
  );
}

function StoreNotFound({ slug }: { slug: string }) {
  return (
    <main
      id="main"
      className="mx-auto flex min-h-screen max-w-2xl flex-col items-start justify-center gap-6 px-6 py-20"
    >
      <p className="text-xs font-semibold uppercase tracking-[0.24em] text-neutral-500">
        Store not found
      </p>
      <h1 className="text-5xl font-medium tracking-tight text-neutral-900">
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

function themeStyle(theme: StorefrontTheme): CSSProperties {
  return {
    ["--store-heading-font" as string]: fontStacks[theme.typography.headingFont],
    ["--store-body-font" as string]: fontStacks[theme.typography.bodyFont],
    ["--store-radius" as string]: themeRadius(theme),
    ["--store-spacing" as string]: themeSpacing(theme),
  };
}
