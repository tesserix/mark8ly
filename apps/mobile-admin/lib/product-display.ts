import type { Product, ProductVariant } from "@repo/mobile-shared/api/types";

// formatMoney lives in ./money (shared with the customers screens); re-exported
// here so existing product-screen imports keep resolving through product-display.
export { formatMoney } from "./money";

/**
 * Variant/media picking, in one place so `variants[0]` never scatters across
 * screens — and because `variants[0]` is WRONG.
 *
 * The API does not sort variants by position: a real product ("Bondi Linen
 * Beach Shirt", verified 2026-07-16) comes back as positions 2,3,4,0,1, so
 * variants[0] is the M variant, not XS. Every one of these helpers sorts.
 *
 * Multi-variant is not an edge case here: 8 of the store's 12 ACTIVE products
 * have 2-5 variants.
 */

/** Lowest `position` wins. Never mutates the input (`.sort` is in-place). */
export function primaryVariant(product: Product): ProductVariant | undefined {
  if (product.variants.length === 0) return undefined;
  return [...product.variants].sort((a, b) => a.position - b.position)[0];
}

export function productPrice(product: Product): number | undefined {
  return primaryVariant(product)?.price;
}

export function productSku(product: Product): string | undefined {
  return primaryVariant(product)?.sku;
}

export function productCurrency(product: Product): string | undefined {
  return primaryVariant(product)?.currency_code;
}

/**
 * Total stock across every variant — what a merchant means by "how many do I
 * have". The primary variant's count alone would understate a 5-variant
 * product by 80%.
 */
export function productStock(product: Product): number {
  return product.variants.reduce((sum, v) => sum + v.inventory_quantity, 0);
}

/** Lowest-position media URL. One real product has no media at all. */
export function productThumb(product: Product): string | undefined {
  if (product.media.length === 0) return undefined;
  return [...product.media].sort((a, b) => a.position - b.position)[0]!.url;
}

