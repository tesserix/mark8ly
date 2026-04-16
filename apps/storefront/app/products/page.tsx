// apps/storefront/app/products/page.tsx
//
// Storefront product catalogue. Resolves the store from the host
// (subdomain) or query param fallback, fetches published products
// and categories, renders a grid with search, category filter, and
// pagination.

import Link from "next/link";
import type { Metadata } from "next";
import { headers } from "next/headers";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import {
  listProducts,
  listCategories,
  type StorefrontProduct,
} from "@/lib/api/marketplace-api";
import { resolveStoreSlug } from "@/lib/slug";
import { makeTenantMetadata } from "@/lib/seo";
import { StorefrontNav } from "@/components/StorefrontNav";
import { ProductSearchInput } from "@/components/ProductSearchInput";
import { CategoryFilter } from "@/components/CategoryFilter";
import { Pagination } from "@/components/Pagination";
import { ProductCardMedia } from "@/components/ProductCardMedia";

export const dynamic = "force-dynamic";

const PAGE_SIZE = 24;

interface PageProps {
  searchParams: Promise<{ slug?: string; page?: string; search?: string }>;
}

async function resolveSlug(query: { slug?: string }): Promise<string> {
  const h = await headers();
  const host = h.get("host");
  return (
    query.slug ||
    await resolveStoreSlug(host)
  );
}

export async function generateMetadata({
  searchParams,
}: PageProps): Promise<Metadata> {
  const params = await searchParams;
  const slug = await resolveSlug(params);
  const store = slug ? await fetchStoreBySlug(slug).catch(() => null) : null;
  return makeTenantMetadata(store, slug);
}

export default async function StoreProductsPage({ searchParams }: PageProps) {
  const params = await searchParams;
  const slug = await resolveSlug(params);
  const page = params.page ? Number.parseInt(params.page, 10) || 1 : 1;
  const search = params.search?.trim() || undefined;

  const store = slug ? await fetchStoreBySlug(slug) : null;
  if (!store) {
    return <NotFound slug={slug} />;
  }

  const [response, categories] = await Promise.all([
    listProducts(slug, { page, pageSize: PAGE_SIZE, search }),
    listCategories(slug),
  ]);
  const products = response?.data ?? [];

  // Build extra params to preserve across pagination links
  const extraParams: Record<string, string> = {};
  if (params.slug) extraParams.slug = params.slug;
  if (search) extraParams.search = search;

  return (
    <main id="main" className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <header className="mb-10 flex flex-col gap-4 border-b border-[color:var(--storefront-text,var(--ink-900))]/10 pb-6">
          <Link
            href="/"
            className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
          >
            ← {store.name}
          </Link>
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-5xl text-[color:var(--storefront-text,var(--ink-900))]">
            Shop
          </h1>
          <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
            {products.length === 0
              ? search
                ? (
                    <span>
                      No matches for &quot;{search}&quot;.{" "}
                      <a
                        href="/products"
                        className="text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-2 transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
                      >
                        Clear search
                      </a>
                    </span>
                  )
                : "No products yet — check back soon."
              : `${products.length} ${products.length === 1 ? "product" : "products"}${search ? ` matching "${search}"` : ""}`}
          </p>
          <div className="mt-2">
            <ProductSearchInput />
          </div>
          {!search && categories.length > 0 && (
            <CategoryFilter categories={categories} />
          )}
        </header>

        {products.length === 0 ? (
          <EmptyCatalogue />
        ) : (
          <>
            <ProductGrid products={products} />
            <Pagination
              currentPage={page}
              pageSize={PAGE_SIZE}
              itemCount={products.length}
              basePath="/products"
              extraParams={extraParams}
            />
          </>
        )}
      </div>
    </main>
  );
}

function ProductGrid({ products }: { products: StorefrontProduct[] }) {
  return (
    <ul className="grid grid-cols-1 gap-x-6 gap-y-12 sm:grid-cols-2 lg:grid-cols-3">
      {products.map((p) => (
        <ProductCard key={p.id} product={p} />
      ))}
    </ul>
  );
}

function ProductCard({ product }: { product: StorefrontProduct }) {
  const min = formatPrice(product.price_range.min, product.price_range.currency_code);
  const max = formatPrice(product.price_range.max, product.price_range.currency_code);
  const isRange = product.price_range.min !== product.price_range.max;
  const anyInStock = product.variants.some((v) => v.in_stock);
  const anyLowStock = product.variants.some((v) => v.in_stock && v.low_stock);
  const stockLabel = !anyInStock
    ? { text: "Out of stock", tone: "danger" as const }
    : anyLowStock
      ? { text: "Low stock", tone: "warn" as const }
      : { text: "In stock", tone: "ok" as const };
  return (
    <li>
      <Link
        href={`/products/${encodeURIComponent(product.handle)}`}
        className="group block focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
      >
        <ProductCardMedia media={product.media} alt={product.title} />
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
        <StockBadge tone={stockLabel.tone} text={stockLabel.text} />
      </Link>
    </li>
  );
}

function StockBadge({ tone, text }: { tone: "ok" | "warn" | "danger"; text: string }) {
  const toneClass =
    tone === "ok"
      ? "bg-[color:var(--storefront-accent,var(--moss-700))]/10 text-[color:var(--storefront-accent,var(--moss-700))]"
      : tone === "warn"
        ? "bg-amber-50 text-amber-700"
        : "bg-red-50 text-red-700";
  const dotClass =
    tone === "ok"
      ? "bg-[color:var(--storefront-accent,var(--moss-700))]"
      : tone === "warn"
        ? "bg-amber-500"
        : "bg-red-500";
  return (
    <span className={`mt-2 inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${toneClass}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${dotClass}`} aria-hidden />
      {text}
    </span>
  );
}

function EmptyCatalogue() {
  return (
    <div className="mx-auto max-w-xl py-16 text-center">
      <h2 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl text-[color:var(--storefront-text,var(--ink-900))]">
        Nothing in the shop yet
      </h2>
      <p className="mt-4 text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        New arrivals are on their way. Check back soon or browse our categories.
      </p>
    </div>
  );
}

function NotFound({ slug }: { slug: string }) {
  return (
    <main id="main" className="mx-auto flex min-h-screen max-w-2xl flex-col items-start justify-center gap-6 px-6 py-20">
      <p className="text-xs font-semibold uppercase tracking-[0.24em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        Store not found
      </p>
      <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-5xl text-[color:var(--storefront-text,var(--ink-900))]">
        Nothing here yet
      </h1>
      <p className="max-w-xl text-lg text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
        {slug
          ? `We couldn't find a store at "${slug}". The URL may be wrong, or the store isn't live yet.`
          : "This domain isn't pointed at a live store yet."}
      </p>
    </main>
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
