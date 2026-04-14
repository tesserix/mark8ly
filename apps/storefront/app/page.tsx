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
        fontFamily: "var(--storefront-body-font)",
        ...style,
      }}
    >
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <StorefrontLayoutRenderer store={store} theme={theme} />
        <FeaturedProducts storeSlug={store.slug} />
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
    ["--storefront-heading-font" as string]: fontStacks[theme.typography.headingFont],
    ["--storefront-body-font" as string]: fontStacks[theme.typography.bodyFont],
    ["--storefront-radius" as string]: themeRadius(theme),
    ["--storefront-spacing" as string]: themeSpacing(theme),
  };
}
