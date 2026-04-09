// apps/admin/components/products/ProductsList.tsx
//
// The products data-table. Hairline rules, 56px row height, columns per
// spec §7.2: checkbox (disabled — bulk actions land in M7d), image
// (Paper placeholder), product (serif title + muted handle), status
// (StatusDot), stock (muted for drafts, signal vermillion for low),
// price (PriceDisplay with "from" prefix for variant-priced products),
// overflow menu trigger (no menu yet — M7d wires it).

import Link from "next/link";
import Image from "next/image";
import type { ReactNode } from "react";
import { MoreHorizontal } from "lucide-react";

import { StatusDot } from "@repo/ui/status-dot";
import { PriceDisplay } from "@repo/ui/price-display";

import type {
  AdminProduct,
  AdminMediaResponse,
} from "@/lib/api/marketplace-api";

export interface ProductsListProps {
  products: AdminProduct[];
}

export function ProductsList({ products }: ProductsListProps) {
  return (
    <div className="w-full">
      <table
        className="w-full border-collapse text-sm"
        aria-label="Products"
      >
        <thead>
          <tr className="border-b border-[color:var(--ink-900)] border-opacity-10 text-left text-xs font-medium uppercase tracking-wide text-[color:var(--ink-900)] opacity-60">
            <th scope="col" className="w-10 py-3" aria-hidden="true" />
            <th scope="col" className="w-14 py-3" aria-hidden="true" />
            <th scope="col" className="py-3">
              Product
            </th>
            <th scope="col" className="w-32 py-3">
              Status
            </th>
            <th scope="col" className="w-40 py-3">
              Stock
            </th>
            <th scope="col" className="w-32 py-3">
              Price
            </th>
            <th scope="col" className="w-10 py-3" aria-hidden="true" />
          </tr>
        </thead>
        <tbody>
          {products.map((p) => (
            <ProductRow key={p.id} product={p} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ProductRow({ product }: { product: AdminProduct }) {
  const firstMedia = product.media[0];
  const variantCount = product.variants.length;

  const priceMin = product.variants.reduce<number | null>((acc, v) => {
    const n = Number.parseFloat(v.price);
    if (!Number.isFinite(n)) return acc;
    if (acc === null || n < acc) return n;
    return acc;
  }, null);
  const priceMax = product.variants.reduce<number | null>((acc, v) => {
    const n = Number.parseFloat(v.price);
    if (!Number.isFinite(n)) return acc;
    if (acc === null || n > acc) return n;
    return acc;
  }, null);
  const currency = product.variants[0]?.currency_code ?? "USD";
  const isVariantRange =
    priceMin !== null && priceMax !== null && priceMin !== priceMax;

  const stock = product.variants.reduce(
    (sum, v) => sum + v.inventory_quantity,
    0,
  );
  const isDraft = product.status === "draft";
  const isLowStock = stock > 0 && stock <= 5;
  const isOutOfStock = stock === 0;

  return (
    <tr className="h-14 border-b border-[color:var(--ink-900)] border-opacity-5 transition-colors hover:bg-[color:var(--ink-900)] hover:bg-opacity-[0.02]">
      <td className="py-3">
        <input
          type="checkbox"
          aria-label={`Select ${product.title}`}
          disabled
          title="Bulk actions land in M7d"
          className="h-4 w-4 rounded border-[color:var(--ink-900)] border-opacity-30 disabled:opacity-30"
        />
      </td>
      <td className="py-3">
        <MediaThumb media={firstMedia} productTitle={product.title} />
      </td>
      <td className="py-3">
        <Link
          href={`/products/${product.id}`}
          className="group inline-flex flex-col focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <span className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-base text-[color:var(--ink-900)] group-hover:underline">
            {product.title}
          </span>
          <span className="text-xs text-[color:var(--ink-900)] opacity-50">
            /{product.handle}
          </span>
        </Link>
      </td>
      <td className="py-3">
        <StatusDot status={product.status} />
      </td>
      <td className="py-3">
        <StockCell
          stock={stock}
          variantCount={variantCount}
          isDraft={isDraft}
          isLowStock={isLowStock}
          isOutOfStock={isOutOfStock}
        />
      </td>
      <td className="py-3">
        <PriceCell
          priceMin={priceMin}
          isVariantRange={isVariantRange}
          currency={currency}
        />
      </td>
      <td className="py-3 text-right">
        <button
          type="button"
          aria-label={`More actions for ${product.title}`}
          className="inline-flex h-8 w-8 items-center justify-center rounded-md text-[color:var(--ink-900)] opacity-60 hover:bg-[color:var(--ink-900)] hover:bg-opacity-5 hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        </button>
      </td>
    </tr>
  );
}

function MediaThumb({
  media,
  productTitle,
}: {
  media: AdminMediaResponse | undefined;
  productTitle: string;
}) {
  if (!media) {
    return (
      <div
        className="h-10 w-10 rounded border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-200)]"
        aria-hidden="true"
      />
    );
  }
  return (
    <div className="relative h-10 w-10 overflow-hidden rounded border border-[color:var(--ink-900)] border-opacity-10">
      {/* unoptimized to avoid the Next Image domain allowlist requirement on dev */}
      <Image
        src={media.url}
        alt={media.alt ?? productTitle}
        fill
        sizes="40px"
        unoptimized
      />
    </div>
  );
}

function StockCell({
  stock,
  variantCount,
  isDraft,
  isLowStock,
  isOutOfStock,
}: {
  stock: number;
  variantCount: number;
  isDraft: boolean;
  isLowStock: boolean;
  isOutOfStock: boolean;
}): ReactNode {
  if (isDraft) {
    return (
      <span className="text-[color:var(--ink-900)] opacity-40">Draft</span>
    );
  }
  if (isOutOfStock) {
    return (
      <span className="text-[color:var(--signal,#C23B22)]">Out of stock</span>
    );
  }
  if (isLowStock) {
    return (
      <span className="text-[color:var(--signal,#C23B22)]">
        {stock} in stock
      </span>
    );
  }
  return (
    <span className="text-[color:var(--ink-900)]">
      {stock} {variantCount > 1 ? `across ${variantCount} variants` : "in stock"}
    </span>
  );
}

function PriceCell({
  priceMin,
  isVariantRange,
  currency,
}: {
  priceMin: number | null;
  isVariantRange: boolean;
  currency: string;
}): ReactNode {
  if (priceMin === null) {
    return <span className="text-[color:var(--ink-900)] opacity-40">—</span>;
  }
  if (isVariantRange) {
    return (
      <span className="text-[color:var(--ink-900)]">
        from <PriceDisplay amount={String(priceMin)} currencyCode={currency} />
      </span>
    );
  }
  return <PriceDisplay amount={String(priceMin)} currencyCode={currency} />;
}
