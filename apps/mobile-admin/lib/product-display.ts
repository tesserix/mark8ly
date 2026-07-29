import type { Product, ProductVariant } from "@repo/mobile-shared/api/types";
// Type-only: no runtime edge from this display module into the mutation hook.
import type { ProductStatus } from "./admin-api/product-status";

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

/**
 * The ONE display string for a product's status — badge copy and screen-reader
 * copy both, so the two can never disagree.
 *
 * They did: the badge read `titleize(status)` while the row's
 * `accessibilityLabel` interpolated the raw value, so VoiceOver announced
 * "archived" beside a chip reading "Archived".
 *
 * The wire schema types `status` as a bare `z.string()` on purpose (a server
 * that grows a fourth status must not blank the catalogue), so this must cope
 * with a value it has never seen. `titleize` alone rendered one verbatim —
 * "out_of_stock" became "Out_of_stock" on screen and in the announcement. The
 * fallback humanises instead of leaking the wire token.
 */
// Keyed on the union rather than `string`, so a status added to the backend
// enum is a compile error here instead of a silently humanised fallback.
const STATUS_LABELS: Record<ProductStatus, string> = {
  draft: "Draft",
  active: "Active",
  archived: "Archived",
};

export function productStatusLabel(status: string): string {
  const known = STATUS_LABELS[status as ProductStatus];
  if (known) return known;
  const words = status.replace(/[_-]+/g, " ").trim();
  if (words.length === 0) return "Unknown";
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** Lowest-position media URL. One real product has no media at all. */
export function productThumb(product: Product): string | undefined {
  if (product.media.length === 0) return undefined;
  return [...product.media].sort((a, b) => a.position - b.position)[0]!.url;
}

