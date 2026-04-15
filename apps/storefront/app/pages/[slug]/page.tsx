import { headers } from "next/headers";
import { notFound } from "next/navigation";
import type { Metadata } from "next";

import { fetchPage } from "@/lib/api/marketplace-api";
import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { Markdown } from "@/lib/markdown";
import { slugFromHost } from "@/lib/slug";
import { StorefrontNav } from "@/components/StorefrontNav";

interface Props {
  params: Promise<{ slug: string }>;
}

async function resolveStoreSlug(): Promise<string> {
  const h = await headers();
  const storeSlug = slugFromHost(h.get("host")) ?? process.env.DEFAULT_STORE_SLUG ?? "";
  if (!storeSlug) {
    console.warn("[storefront] /pages/[slug]: no store context (host + DEFAULT_STORE_SLUG both empty)");
  }
  return storeSlug;
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params;
  const storeSlug = await resolveStoreSlug();
  const page = storeSlug ? await fetchPage(storeSlug, slug) : null;
  if (!page) return { title: "Page not found" };
  return {
    title: page.seo_title ?? page.title,
    description: page.seo_description ?? undefined,
  };
}

export default async function PageView({ params }: Props) {
  const { slug } = await params;
  const storeSlug = await resolveStoreSlug();
  if (!storeSlug) notFound();

  const [page, store] = await Promise.all([
    fetchPage(storeSlug, slug),
    fetchStoreBySlug(storeSlug).catch(() => null),
  ]);

  if (!page) notFound();

  return (
    <main id="main" className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store?.name} />
        <article className="mx-auto max-w-3xl py-10 sm:py-14">
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-serif))] text-4xl font-medium tracking-tight text-[color:var(--storefront-text,var(--ink-900))]">
            {page.title}
          </h1>
          <Markdown className="prose mt-8 max-w-none prose-headings:font-[family-name:var(--storefront-heading-font,var(--font-serif))] prose-headings:text-[color:var(--storefront-text,var(--ink-900))] prose-a:text-[color:var(--storefront-accent,var(--moss-700))]">
            {page.body}
          </Markdown>
        </article>
      </div>
    </main>
  );
}
