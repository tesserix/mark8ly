// apps/storefront/app/products/[handle]/page.tsx
//
// Storefront product detail page. Resolves the store from the host
// (matching the catalogue page), then fetches a single published
// product by its handle from M6's marketplace-api. Renders an
// editorial single-column layout with media gallery, price, options,
// description, and category links.

import Link from "next/link";
import type { Metadata } from "next";
import { headers } from "next/headers";
import { notFound } from "next/navigation";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import {
  getProductByHandle,
  type StorefrontProduct,
} from "@/lib/api/marketplace-api";
import { slugFromHost } from "@/lib/slug";
import { StorefrontNav } from "@/components/StorefrontNav";

export const dynamic = "force-dynamic";

interface PageProps {
  params: Promise<{ handle: string }>;
  searchParams: Promise<{ slug?: string }>;
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
  params,
  searchParams,
}: PageProps): Promise<Metadata> {
  const { handle } = await params;
  const sp = await searchParams;
  const slug = await resolveSlug(sp);
  if (!slug) return { title: "Product" };
  const product = await getProductByHandle(slug, handle);
  if (!product) return { title: "Product not found" };
  return {
    title: product.seo_title ?? product.title,
    description: product.seo_description ?? product.description ?? undefined,
  };
}

export default async function StorefrontProductPage({
  params,
  searchParams,
}: PageProps) {
  const { handle } = await params;
  const sp = await searchParams;
  const slug = await resolveSlug(sp);

  const store = slug ? await fetchStoreBySlug(slug) : null;
  if (!store) notFound();

  const product = await getProductByHandle(slug, handle);
  if (!product) notFound();

  return (
    <main id="main" className="min-h-screen bg-[color:var(--paper-200)]">
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <Link
          href="/products"
          className="mb-8 inline-flex items-center gap-1 text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          ← Shop
        </Link>

        <div className="grid gap-12 lg:grid-cols-2">
          <Gallery product={product} />
          <Details product={product} />
        </div>
      </div>
    </main>
  );
}

function Gallery({ product }: { product: StorefrontProduct }) {
  if (product.media.length === 0) {
    return (
      <div className="aspect-square rounded-md bg-[color:var(--paper-200)]" />
    );
  }
  return (
    <div className="flex flex-col gap-4">
      {product.media.map((m, i) => (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          key={`${m.url}-${i}`}
          src={m.url}
          alt={m.alt ?? product.title}
          className="w-full rounded-md bg-[color:var(--paper-200)] object-cover"
        />
      ))}
    </div>
  );
}

function Details({ product }: { product: StorefrontProduct }) {
  const min = formatPrice(
    product.price_range.min,
    product.price_range.currency_code,
  );
  const max = formatPrice(
    product.price_range.max,
    product.price_range.currency_code,
  );
  const isRange = product.price_range.min !== product.price_range.max;
  const firstVariant = product.variants[0];

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-col gap-2">
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl leading-tight text-[color:var(--ink-900)]">
          {product.title}
        </h1>
        <p
          className="text-2xl text-[color:var(--ink-900)]"
          style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
        >
          {isRange ? `${min} – ${max}` : min}
        </p>
        {firstVariant && !firstVariant.in_stock && (
          <p className="text-sm text-[color:var(--signal,#C23B22)]">
            Out of stock
          </p>
        )}
        {firstVariant && firstVariant.in_stock && firstVariant.low_stock && (
          <p className="text-sm text-[color:var(--signal,#C23B22)]">
            Low stock
          </p>
        )}
      </header>

      {product.description && (
        <p className="whitespace-pre-line text-base leading-7 text-[color:var(--ink-900)] opacity-80">
          {product.description}
        </p>
      )}

      {product.options.length > 0 && (
        <section className="border-t border-[color:var(--ink-900)] border-opacity-10 pt-6">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-[color:var(--ink-900)] opacity-60">
            Options
          </h2>
          <ul className="mt-3 space-y-2">
            {product.options.map((opt) => (
              <li
                key={opt.name}
                className="text-sm text-[color:var(--ink-900)]"
              >
                <span className="font-semibold">{opt.name}:</span>{" "}
                <span className="opacity-70">
                  {opt.values.map((v) => v.value).join(", ")}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <button
        type="button"
        disabled
        className="mt-2 inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-6 py-3 text-sm text-[color:var(--paper-200)] opacity-50"
        title="Cart lands in a follow-up slice"
      >
        Add to cart (coming soon)
      </button>

      {product.categories.length > 0 && (
        <footer className="border-t border-[color:var(--ink-900)] border-opacity-10 pt-6">
          <p className="text-xs uppercase tracking-widest text-[color:var(--ink-900)] opacity-50">
            In
          </p>
          <ul className="mt-2 flex flex-wrap gap-2">
            {product.categories.map((c) => (
              <li key={c.slug}>
                <span className="rounded-full border border-[color:var(--ink-900)] border-opacity-15 px-3 py-1 text-sm text-[color:var(--ink-900)] opacity-80">
                  {c.name}
                </span>
              </li>
            ))}
          </ul>
        </footer>
      )}
    </div>
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
