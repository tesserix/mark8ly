// apps/storefront/lib/cart.ts
//
// Pure cart helpers. No React, no DOM, no localStorage.
// Every function returns a new array — never mutates.

export interface CartItem {
  productId: string;
  variantId: string;
  handle: string;
  title: string;
  priceAmount: string;
  currencyCode: string;
  qty: number;
  imageUrl?: string;
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
