import type { ProductVariant } from "@repo/mobile-shared/api/types";
import type { VariantEditField } from "@/lib/admin-api/variant-quick-edit";
import { formatMoney } from "@/lib/money";

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
 * the label to a single line, so a two-line label would silently truncate
 * instead of wrapping. The variant's current price/stock is still NOT
 * appended here — a suffix is the first thing an ellipsis eats on a narrow
 * device — it goes on the row's own `detail` line instead (see
 * `variantDetail` below).
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

/**
 * The number the merchant is CHOOSING BY, for the picker's second line.
 *
 * A picker that asks which variant to restock while showing no quantities
 * makes the merchant open, read and back out of each variant in turn — five
 * round trips on a five-variant product — and the product row above it shows
 * only the store-wide TOTAL, so there is nowhere else to read it either. On a
 * store whose variants carry no option values the rows are bare SKUs, which
 * makes it worse: nothing on the row distinguishes them at all.
 *
 * It shows the field being edited, and only that: "Adjust stock" answers with
 * the stock, "Edit price" with the price. Both are short enough to survive a
 * narrow device at `accessibility-large` on their own line — a joined
 * "3 in stock · $27.50" would not be.
 *
 * Zero is a real answer here — the variant most likely to need restocking —
 * so it is formatted, never treated as absent.
 */
export function variantDetail(variant: ProductVariant, field: VariantEditField): string {
  if (field === "price") return formatMoney(variant.price, variant.currency_code);
  const quantity = variant.inventory_quantity;
  return quantity === 1 ? "1 in stock" : `${quantity} in stock`;
}
