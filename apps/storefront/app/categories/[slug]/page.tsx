// apps/storefront/app/categories/[slug]/page.tsx
//
// Storefront category landing page. Reuses the same store-by-host
// resolution and pricing helpers as the catalogue page; filters
// products by category slug and includes pagination.

import Image from "next/image";
import Link from "next/link";
import type { Metadata } from "next";
import { headers } from "next/headers";
import { notFound } from "next/navigation";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import {
  listProducts,
  listCategories,
  type StorefrontProduct,
} from "@/lib/api/marketplace-api";
import { resolveStoreSlug } from "@/lib/slug";
import { StorefrontNav } from "@/components/StorefrontNav";
import { CategoryFilter } from "@/components/CategoryFilter";
import { Pagination } from "@/components/Pagination";

export const dynamic = "force-dynamic";

const PAGE_SIZE = 24;

interface PageProps {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ slug?: string; page?: string }>;
}

async function getStoreSlug(query: { slug?: string }): Promise<string> {
  if (query.slug) return query.slug;
  const h = await headers();
  return resolveStoreSlug(h.get("host"));
}

export async function generateMetadata({
  params,
}: PageProps): Promise<Metadata> {
  const { slug: categorySlug } = await params;
  return {
    title: `${prettify(categorySlug)} — Shop`,
  };
}

export default async function CategoryLandingPage({
  params,
  searchParams,
}: PageProps) {
  const { slug: categorySlug } = await params;
  const sp = await searchParams;
  const storeSlug = await getStoreSlug(sp);
  const page = sp.page ? Number.parseInt(sp.page, 10) || 1 : 1;

  const store = storeSlug ? await fetchStoreBySlug(storeSlug) : null;
  if (!store) notFound();

  const [response, categories] = await Promise.all([
    listProducts(storeSlug, { categorySlug, page, pageSize: PAGE_SIZE }),
    listCategories(storeSlug),
  ]);
  const products = response?.data ?? [];

  const extraParams: Record<string, string> = {};
  if (sp.slug) extraParams.slug = sp.slug;

  return (
    <main id="main" className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <header className="mb-10 flex flex-col gap-4 border-b border-[color:var(--storefront-text,var(--ink-900))]/10 pb-6">
          <Link
            href="/products"
            className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
          >
            ← Shop all
          </Link>
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-5xl text-[color:var(--storefront-text,var(--ink-900))]">
            {prettify(categorySlug)}
          </h1>
          <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
            {products.length === 0
              ? "Nothing in this category yet. Try browsing all products instead."
              : `${products.length} ${products.length === 1 ? "product" : "products"}`}
          </p>
          {categories.length > 0 && (
            <CategoryFilter
              categories={categories}
              activeCategorySlug={categorySlug}
            />
          )}
        </header>

        {products.length > 0 && (
          <>
            <ul className="grid grid-cols-1 gap-x-6 gap-y-12 sm:grid-cols-2 lg:grid-cols-3">
              {products.map((p) => (
                <ProductCard key={p.id} product={p} />
              ))}
            </ul>
            <Pagination
              currentPage={page}
              pageSize={PAGE_SIZE}
              itemCount={products.length}
              basePath={`/categories/${encodeURIComponent(categorySlug)}`}
              extraParams={extraParams}
            />
          </>
        )}
      </div>
    </main>
  );
}

function ProductCard({ product }: { product: StorefrontProduct }) {
  const cover = product.media[0];
  const min = formatPrice(
    product.price_range.min,
    product.price_range.currency_code,
  );
  const isRange = product.price_range.min !== product.price_range.max;
  return (
    <li>
      <Link
        href={`/products/${encodeURIComponent(product.handle)}`}
        className="group block focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
      >
        <div className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--storefront-background,var(--paper-200))]">
          {cover ? (
            <Image
              src={cover.url}
              alt={cover.alt ?? product.title}
              fill
              sizes="(min-width: 1024px) 33vw, (min-width: 640px) 50vw, 100vw"
              className="object-cover transition-transform duration-500 group-hover:scale-[1.02]"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-xs uppercase tracking-widest text-[color:var(--storefront-text,var(--ink-900))] opacity-30">
              No image
            </div>
          )}
        </div>
        <div className="mt-4 flex items-start justify-between gap-3">
          <h2 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-lg text-[color:var(--storefront-text,var(--ink-900))] group-hover:underline">
            {product.title}
          </h2>
          <span
            className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-80"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {isRange ? `from ${min}` : min}
          </span>
        </div>
      </Link>
    </li>
  );
}

function formatPrice(amount: string, currencyCode: string): string {
  const n = Number.parseFloat(amount);
  if (!Number.isFinite(n)) return `${currencyCode} ${amount}`;
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
    }).format(n);
  } catch {
    return `${currencyCode} ${amount}`;
  }
}

function prettify(slug: string): string {
  return slug
    .split("-")
    .map((s) => (s.length > 0 ? s[0]!.toUpperCase() + s.slice(1) : s))
    .join(" ");
}
