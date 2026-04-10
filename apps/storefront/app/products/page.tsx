// apps/storefront/app/products/page.tsx
//
// Storefront product catalogue. Resolves the store from the host
// (subdomain) or query param fallback (matching the home-page pattern),
// fetches the published-products list from M6's marketplace-api, and
// renders a simple grid. Detail pages live at /products/[handle].

import Link from "next/link";
import type { Metadata } from "next";
import { headers } from "next/headers";

import { fetchStoreBySlug, type PublicStore } from "@/lib/api/platform-api";
import { listProducts, type StorefrontProduct } from "@/lib/api/marketplace-api";
import { slugFromHost } from "@/lib/slug";
import { makeTenantMetadata } from "@/lib/seo";

export const dynamic = "force-dynamic";

interface PageProps {
  searchParams: Promise<{ slug?: string; page?: string }>;
}

async function resolveSlug(query: { slug?: string }): Promise<string> {
  const h = await headers();
  const host = h.get("host");
  return (
    query.slug ||
    slugFromHost(host) ||
    process.env.DEFAULT_STORE_SLUG ||
    ""
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

  const store = slug ? await fetchStoreBySlug(slug) : null;
  if (!store) {
    return <NotFound slug={slug} />;
  }

  const response = await listProducts(slug, { page, pageSize: 24 });
  const products = response?.data ?? [];

  return (
    <main id="main" className="min-h-screen bg-[color:var(--paper-200)]">
      <div className="mx-auto max-w-6xl px-6 py-12 sm:px-8">
        <header className="mb-10 flex flex-col gap-2 border-b border-[color:var(--ink-900)] border-opacity-10 pb-6">
          <Link
            href="/"
            className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            ← {store.name}
          </Link>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl text-[color:var(--ink-900)]">
            Shop
          </h1>
          <p className="text-sm text-[color:var(--ink-900)] opacity-60">
            {products.length === 0
              ? "No products yet — check back soon."
              : `${products.length} ${products.length === 1 ? "product" : "products"}`}
          </p>
        </header>

        {products.length === 0 ? (
          <EmptyCatalogue />
        ) : (
          <ProductGrid products={products} />
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
  const cover = product.media[0];
  const min = formatPrice(product.price_range.min, product.price_range.currency_code);
  const max = formatPrice(product.price_range.max, product.price_range.currency_code);
  const isRange = product.price_range.min !== product.price_range.max;
  return (
    <li>
      <Link
        href={`/products/${encodeURIComponent(product.handle)}`}
        className="group block focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <div className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--paper-200)]">
          {cover ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={cover.url}
              alt={cover.alt ?? product.title}
              className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-[1.02]"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-xs uppercase tracking-widest text-[color:var(--ink-900)] opacity-30">
              No image
            </div>
          )}
        </div>
        <div className="mt-4 flex items-start justify-between gap-3">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg text-[color:var(--ink-900)] group-hover:underline">
            {product.title}
          </h2>
          <span
            className="text-sm text-[color:var(--ink-900)] opacity-80"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {isRange ? `from ${min}` : min}
          </span>
        </div>
      </Link>
    </li>
  );
}

function EmptyCatalogue() {
  return (
    <div className="mx-auto max-w-xl py-16 text-center">
      <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl text-[color:var(--ink-900)]">
        Nothing in the shop yet
      </h2>
      <p className="mt-4 text-[color:var(--ink-900)] opacity-60">
        New arrivals are on their way. Check back soon.
      </p>
    </div>
  );
}

function NotFound({ slug }: { slug: string }) {
  return (
    <main id="main" className="mx-auto flex min-h-screen max-w-2xl flex-col items-start justify-center gap-6 px-6 py-20">
      <p className="text-xs font-semibold uppercase tracking-[0.24em] text-[color:var(--ink-900)] opacity-60">
        Store not found
      </p>
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl text-[color:var(--ink-900)]">
        Nothing here yet
      </h1>
      <p className="max-w-xl text-lg text-[color:var(--ink-900)] opacity-70">
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

// Suppress unused-import warning for PublicStore until detail page lands.
type _Unused = PublicStore;
