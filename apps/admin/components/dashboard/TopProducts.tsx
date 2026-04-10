"use client";

import Link from "next/link";

import type { TopProduct } from "@/lib/api/marketplace-api";

interface TopProductsProps {
  products: TopProduct[];
}

export function TopProducts({ products }: TopProductsProps) {
  if (products.length === 0) {
    return (
      <div className="py-8 text-center">
        <p className="text-sm text-foreground-secondary">
          No product data yet. Add products and make some sales to see your top
          sellers here.
        </p>
      </div>
    );
  }

  return (
    <div>
      <h3 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-foreground">
        Top products
      </h3>
      <div className="overflow-x-auto">
      <ul className="mt-4 divide-y divide-border-subtle" role="list">
        {products.map((product) => (
          <li key={product.id}>
            <Link
              href={`/products/${product.id}`}
              className="flex min-h-[44px] items-center gap-4 py-3 transition-colors hover:bg-paper-100"
            >
              <div className="h-8 w-8 shrink-0 overflow-hidden rounded bg-paper-100">
                {product.image_url ? (
                  <img
                    src={product.image_url}
                    alt={product.title}
                    className="h-full w-full object-cover"
                    width={32}
                    height={32}
                  />
                ) : (
                  <div className="flex h-full w-full items-center justify-center text-[10px] text-foreground-tertiary">
                    --
                  </div>
                )}
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-foreground">
                  {product.title}
                </p>
                <p className="text-xs text-foreground-secondary">
                  {product.units_sold} units sold
                </p>
              </div>
              <p
                className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-sm font-medium text-foreground"
                style={{ fontFeatureSettings: '"tnum" 1' }}
              >
                {product.revenue}
              </p>
            </Link>
          </li>
        ))}
      </ul>
      </div>
    </div>
  );
}
