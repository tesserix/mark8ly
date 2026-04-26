// apps/storefront/lib/cart.ts
//
// Pure cart helpers. No React, no DOM, no localStorage.
// Every function returns a new array — never mutates.

export type CartItemTaxCategory =
  | "standard"
  | "reduced"
  | "zero_rated"
  | "exempt";

export interface CartItem {
  productId: string;
  variantId: string;
  handle: string;
  title: string;
  priceAmount: string;
  currencyCode: string;
  qty: number;
  imageUrl?: string;
  // Tax classification snapshot taken when the item was added — lets
  // the checkout request carry the right per-product rate/category
  // without a re-fetch. Optional because products created before the
  // tax feature shipped don't have these fields.
  taxCode?: string;
  taxRateOverride?: string; // percentage as decimal string, e.g. "18.00"
  taxCategory?: CartItemTaxCategory;
  // Shipping snapshot — same idea as the tax fields. Lets the
  // /shipping-rates request carry the variant's actual weight +
  // package dimensions instead of falling back to a hardcoded
  // 500 g / 30 × 20 × 10 cm envelope.
  weightGrams?: number;
  lengthCm?: number;
  widthCm?: number;
  heightCm?: number;
}

export function addItem(items: readonly CartItem[], item: CartItem): CartItem[] {
  const idx = items.findIndex(
    (i) => i.productId === item.productId && i.variantId === item.variantId,
  );
  if (idx >= 0) {
    return items.map((i, j) =>
      j === idx ? { ...i, qty: i.qty + item.qty } : i,
    );
  }
  return [...items, item];
}

export function removeItem(
  items: readonly CartItem[],
  productId: string,
  variantId: string,
): CartItem[] {
  return items.filter(
    (i) => !(i.productId === productId && i.variantId === variantId),
  );
}

export function setQty(
  items: readonly CartItem[],
  productId: string,
  variantId: string,
  qty: number,
): CartItem[] {
  if (qty <= 0) return removeItem(items, productId, variantId);
  return items.map((i) =>
    i.productId === productId && i.variantId === variantId
      ? { ...i, qty }
      : i,
  );
}

export function subtotal(items: readonly CartItem[]): number {
  return items.reduce((sum, i) => sum + Number.parseFloat(i.priceAmount) * i.qty, 0);
}

export function count(items: readonly CartItem[]): number {
  return items.reduce((sum, i) => sum + i.qty, 0);
}
