import type { CSSProperties } from "react";
import type { Metadata } from "next";
import { headers } from "next/headers";

import { fetchStoreBySlug, type PublicStore } from "@/lib/api/platform-api";
import {
  fontStacks,
  normalizeStorefrontTheme,
  themeRadius,
  themeSpacing,
  type StorefrontTheme,
} from "@repo/ui/storefront-theme";
import { resolveStoreSlug } from "@/lib/slug";
import { makeTenantMetadata } from "@/lib/seo";
import { StorefrontLayoutRenderer } from "@/components/layouts";
import { FeaturedProducts } from "@/components/FeaturedProducts";
import { StorefrontNav } from "@/components/StorefrontNav";
import { fetchBranding, type HomepageContent } from "@/lib/api/marketplace-api";

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
    await resolveStoreSlug(host);
  const store = slug ? await fetchStoreBySlug(slug).catch(() => null) : null;
  const branding = slug ? await fetchBranding(slug).catch(() => null) : null;
  return makeTenantMetadata(store, slug, branding?.branding ?? null);
}

export default async function StoreHomePage({ searchParams }: PageProps) {
  const { slug: slugFromQuery } = await searchParams;
  const h = await headers();
  const host = h.get("host");

  const slug =
    slugFromQuery ||
    await resolveStoreSlug(host);

  const store = slug ? await fetchStoreBySlug(slug) : null;

  if (!store) {
    return <StoreNotFound slug={slug} />;
  }

  const branding = await fetchBranding(store.slug).catch(() => null);
  const content = branding?.branding?.homepage_content ?? null;

  return <StoreLanding store={store} content={content} />;
}

function StoreLanding({
  store,
  content,
}: {
  store: PublicStore;
  content: HomepageContent | null;
}) {
  const theme = normalizeStorefrontTheme(store.storefront_theme);
  const style = themeStyle(theme);

  return (
    <main
      id="main"
      data-storefront-home=""
      className="min-h-screen"
      style={{
        background: theme.colors.background,
        color: theme.colors.text,
        fontFamily: "var(--storefront-body-font)",
        ...style,
      }}
    >
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <StorefrontLayoutRenderer store={store} theme={theme} content={content} />
        {/*
          Default FeaturedProducts strip only when the merchant hasn't authored
          any homepage sections. Once they start authoring (including their
          own featured_products block), they own what shows below the hero —
          avoids a double-render + double API fetch.
        */}
        {(content?.sections?.length ?? 0) === 0 ? (
          <FeaturedProducts storeSlug={store.slug} />
        ) : null}
      </div>
    </main>
  );
}

function StoreNotFound({ slug }: { slug: string }) {
  return (
    <main
      id="main"
      className="mx-auto flex min-h-screen max-w-2xl flex-col items-start justify-center gap-6 px-6 py-20"
    >
      <p className="text-xs font-semibold uppercase tracking-[0.24em] text-[color:var(--storefront-text,var(--ink-900))]/60">
        Store not found
      </p>
      <h1 className="text-3xl font-medium tracking-tight text-[color:var(--storefront-text,var(--ink-900))] sm:text-4xl md:text-5xl">
        Nothing here yet
      </h1>
      <p className="max-w-xl text-lg leading-8 text-[color:var(--storefront-text,var(--ink-900))]/70">
        {slug
          ? `We couldn't find a store at "${slug}". The URL may be wrong, or the store isn't live yet.`
          : "This domain isn't pointed at a live store yet."}
      </p>
    </main>
  );
}

function themeStyle(theme: StorefrontTheme): CSSProperties {
  return {
    ["--storefront-heading-font" as string]: fontStacks[theme.typography.headingFont],
    ["--storefront-body-font" as string]: fontStacks[theme.typography.bodyFont],
    ["--storefront-radius" as string]: themeRadius(theme),
    ["--storefront-spacing" as string]: themeSpacing(theme),
  };
}
