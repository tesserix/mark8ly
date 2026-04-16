// apps/storefront/components/FeaturedProducts.tsx
//
// Home-page section that fetches the first page of published products
// and renders up to 8 cards as a "Featured" grid. Degrades to nothing
// when the store has no products yet.

import Image from "next/image";
import Link from "next/link";

import { listProducts, type StorefrontProduct } from "@/lib/api/marketplace-api";

export interface FeaturedProductsProps {
  storeSlug: string;
  max?: number;
}

export async function FeaturedProducts({ storeSlug, max = 8 }: FeaturedProductsProps) {
  const response = await listProducts(storeSlug, { pageSize: max });
  const products = response?.data ?? [];
  if (products.length === 0) return null;

  return (
    <section
      aria-label="Featured products"
      className="mt-16 border-t border-[color:var(--storefront-text,var(--ink-900))] border-opacity-10 pt-12"
    >
      <header className="mb-8 flex flex-wrap items-end justify-between gap-4">
        <h2 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-3xl text-[color:var(--storefront-text,var(--ink-900))]">
          Featured
        </h2>
        <Link
          href="/products"
          className="text-sm text-[color:var(--storefront-accent,var(--moss-700))] underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        >
          Shop all →
        </Link>
      </header>
      <ul className="grid grid-cols-2 gap-x-4 gap-y-10 sm:grid-cols-3 lg:grid-cols-4">
        {products.map((p) => (
          <ProductCard key={p.id} product={p} />
        ))}
      </ul>
    </section>
  );
}

function ProductCard({ product }: { product: StorefrontProduct }) {
  const cover = product.media[0];
  const min = formatPrice(
    product.price_range.min,
    product.price_range.currency_code,
  );
  const isRange = product.price_range.min !== product.price_range.max;
  const anyInStock = product.variants.some((v) => v.in_stock);
  const anyLowStock = product.variants.some((v) => v.in_stock && v.low_stock);
  const stockTone: "ok" | "warn" | "danger" = !anyInStock
    ? "danger"
    : anyLowStock
      ? "warn"
      : "ok";
  const stockText = !anyInStock ? "Out of stock" : anyLowStock ? "Low stock" : "In stock";
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
              sizes="(min-width: 1024px) 25vw, (min-width: 640px) 33vw, 50vw"
              className="object-cover transition-transform duration-500 group-hover:scale-[1.02]"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-[10px] uppercase tracking-widest text-[color:var(--storefront-text,var(--ink-900))] opacity-30">
              No image
            </div>
          )}
        </div>
        <div className="mt-3 flex items-start justify-between gap-2">
          <h3 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-sm text-[color:var(--storefront-text,var(--ink-900))] group-hover:underline">
            {product.title}
          </h3>
          <span
            className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-80"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {isRange ? `from ${min}` : min}
          </span>
        </div>
        <FeaturedStockBadge tone={stockTone} text={stockText} />
      </Link>
    </li>
  );
}

function FeaturedStockBadge({ tone, text }: { tone: "ok" | "warn" | "danger"; text: string }) {
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
    <span className={`mt-2 inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-medium ${toneClass}`}>
      <span className={`h-1 w-1 rounded-full ${dotClass}`} aria-hidden />
      {text}
    </span>
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
