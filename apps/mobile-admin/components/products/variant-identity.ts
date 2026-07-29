import type { ProductVariant } from "@repo/mobile-shared/api/types";

/**
 * Variants in the order a merchant expects to see them.
 *
 * The wire does NOT sort by position — a real product ("Bondi Linen Beach
 * Shirt", verified 2026-07-16) comes back as 2,3,4,0,1 — so a picker that
 * trusts array order lists the same product's variants in a different
 * sequence from the editor, the storefront and every other surface.
 *
 * Copies before sorting: `Array.prototype.sort` is in place, and the array
 * handed in belongs to the react-query cache.
 */
export function sortVariants(variants: readonly ProductVariant[]): ProductVariant[] {
  return [...variants].sort((a, b) => a.position - b.position);
}

/**
 * What one variant is CALLED, for a picker a merchant reads in a hurry.
 *
 * Option values first because that is how a merchant thinks about a variant
 * ("the blue large one"), the SKU after it because that is what is printed on
 * the shelf label. A picker listing three rows that all read "Default" is
 * useless, so the last resort still numbers them rather than repeating one
 * word three times.
 *
 * Kept to ONE line's worth of information on purpose: `ActionSheet` clamps
 * every row to a single line and budgets its height on that, so a two-line
 * label would silently truncate instead of wrapping. The variant's current
 * price/stock is deliberately NOT appended — it would be the first thing
 * truncated away on a narrow device, and it is shown in full, pre-filled and
 * editable, on the sheet this picker opens.
 */
export function variantLabel(variant: ProductVariant): string {
  const values = variant.option_values
    .map((option) => option.value.trim())
    .filter((value) => value.length > 0)
    .join(" / ");
  const sku = variant.sku.trim();

  if (values.length > 0 && sku.length > 0) return `${values} · ${sku}`;
  if (values.length > 0) return values;
  if (sku.length > 0) return sku;
  // `position` is 0-based on the wire; a merchant counts from one.
  return `Variant ${variant.position + 1}`;
}
