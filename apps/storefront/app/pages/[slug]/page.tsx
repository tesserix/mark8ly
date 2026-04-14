import { headers } from "next/headers";
import { notFound } from "next/navigation";
import type { Metadata } from "next";

import { fetchPage } from "@/lib/api/marketplace-api";
import { Markdown } from "@/lib/markdown";
import { slugFromHost } from "@/lib/slug";

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
  const page = storeSlug ? await fetchPage(storeSlug, slug) : null;

  if (!page) notFound();

  return (
    <main id="main" className="mx-auto max-w-3xl px-6 py-16 sm:py-20">
      <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
        {page.title}
      </h1>
      <Markdown className="prose mt-8 max-w-none prose-headings:font-serif prose-headings:text-foreground prose-a:text-moss-700">
        {page.body}
      </Markdown>
    </main>
  );
}
