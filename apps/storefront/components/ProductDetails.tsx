"use client";

// apps/storefront/components/ProductDetails.tsx
//
// Client component for the product detail page. Manages the selected
// variant state so the price, stock, and add-to-cart button update
// when the user picks options via the VariantSelector.

import { useCallback, useState } from "react";
import Link from "next/link";
import type {
  StorefrontProduct,
  StorefrontVariant,
} from "@/lib/api/marketplace-api";
import { VariantSelector } from "./VariantSelector";
import { AddToCartButton } from "./AddToCartButton";

interface ProductDetailsProps {
  product: StorefrontProduct;
}

export function ProductDetails({ product }: ProductDetailsProps) {
  const hasOptions = product.options.length > 0;
  const defaultVariant = product.variants[0] ?? null;

  // If there are no options (single variant), pre-select it.
  // Otherwise, null until all options are chosen.
  const [selectedVariant, setSelectedVariant] =
    useState<StorefrontVariant | null>(hasOptions ? null : defaultVariant);

  const handleVariantChange = useCallback(
    (variant: StorefrontVariant | null) => {
      setSelectedVariant(variant);
    },
    [],
  );

  const displayPrice = selectedVariant
    ? formatPrice(selectedVariant.price, selectedVariant.currency_code)
    : formatPriceRange(product.price_range);

  const comparePrice =
    selectedVariant?.compare_at_price &&
    Number.parseFloat(selectedVariant.compare_at_price) >
      Number.parseFloat(selectedVariant.price)
      ? formatPrice(
          selectedVariant.compare_at_price,
          selectedVariant.currency_code,
        )
      : null;

  const inStock = selectedVariant?.in_stock ?? defaultVariant?.in_stock ?? true;
  const lowStock = selectedVariant?.low_stock ?? false;

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-col gap-2">
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl leading-tight text-[color:var(--ink-900)]">
          {product.title}
        </h1>
        <div className="flex items-baseline gap-3">
          <p
            className="text-2xl text-[color:var(--ink-900)]"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {displayPrice}
          </p>
          {comparePrice && (
            <p
              className="text-lg text-[color:var(--ink-900)] opacity-40 line-through"
              style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
            >
              {comparePrice}
            </p>
          )}
        </div>
      </header>

      {product.description && (
        <p className="whitespace-pre-line text-base leading-7 text-[color:var(--ink-900)] opacity-80">
          {product.description}
        </p>
      )}

      {hasOptions && (
        <section className="border-t border-[color:var(--ink-900)]/10 pt-6">
          <VariantSelector
            options={product.options}
            variants={product.variants}
            onVariantChange={handleVariantChange}
          />
        </section>
      )}

      <div aria-live="polite">
        {!inStock && (
          <p className="text-sm text-[color:var(--signal,#C23B22)]" role="status">
            Out of stock
          </p>
        )}
        {inStock && lowStock && (
          <p className="text-sm text-[color:var(--warning,#A67C00)]" role="status">
            Low stock — order soon
          </p>
        )}
      </div>

      <AddToCartButton
        productId={product.id}
        variantId={selectedVariant?.id ?? defaultVariant?.id ?? product.id}
        handle={product.handle}
        title={product.title}
        priceAmount={
          selectedVariant?.price ??
          defaultVariant?.price ??
          product.price_range.min
        }
        currencyCode={
          selectedVariant?.currency_code ??
          defaultVariant?.currency_code ??
          product.price_range.currency_code
        }
        imageUrl={product.media[0]?.url}
        inStock={selectedVariant ? selectedVariant.in_stock : inStock}
        disabled={hasOptions && !selectedVariant}
        disabledReason={
          hasOptions && !selectedVariant
            ? "Select all options"
            : undefined
        }
      />

      {product.categories.length > 0 && (
        <footer className="border-t border-[color:var(--ink-900)]/10 pt-6">
          <p className="text-xs uppercase tracking-widest text-[color:var(--ink-900)] opacity-50">
            In
          </p>
          <ul className="mt-2 flex flex-wrap gap-2">
            {product.categories.map((c) => (
              <li key={c.slug}>
                <Link
                  href={`/categories/${encodeURIComponent(c.slug)}`}
                  className="rounded-full border border-[color:var(--ink-900)]/15 px-3 py-1 text-sm text-[color:var(--ink-900)] opacity-80 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                >
                  {c.name}
                </Link>
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

function formatPriceRange(range: {
  min: string;
  max: string;
  currency_code: string;
}): string {
  const min = formatPrice(range.min, range.currency_code);
  const max = formatPrice(range.max, range.currency_code);
  return range.min === range.max ? min : `${min} – ${max}`;
}
