// apps/storefront/lib/cart-storage.ts
//
// Reading and writing the cart to localStorage, extracted from
// CartProvider so the part that decides whether a shopper keeps their
// basket across a deploy can be tested directly (#964).
//
// # Why there is a version now
//
// The stored value used to be a bare array, read with an unchecked
// `as CartItem[]`. That is fine until the item shape changes, and #964 is
// the first change to it. A version tag introduced at the moment it is
// first needed is one release too late, so it goes in now — while a v1
// cart is still trivially a valid v2 cart and the migration is lossless.

import type { CartItem } from "./cart";

export const CART_SCHEMA_VERSION = 2;

export interface StoredCart {
  v: number;
  items: CartItem[];
}

export function cartStorageKey(storeSlug: string): string {
  return `mark8ly.cart.${storeSlug}`;
}

function isStoredCart(value: unknown): value is StoredCart {
  if (typeof value !== "object" || value === null) return false;
  const c = value as Record<string, unknown>;
  return typeof c.v === "number" && Array.isArray(c.items);
}

/**
 * Minimal shape check for one stored line.
 *
 * This is not validation of the buyer's data — the server does that at
 * checkout. It exists because the read used to be an unchecked cast, so a
 * truncated or hand-edited entry flowed straight into the cart and into
 * the checkout request body. A malformed line is dropped rather than
 * failing the whole cart: losing one row beats losing the basket.
 */
export function isUsableItem(value: unknown): value is CartItem {
  if (typeof value !== "object" || value === null) return false;
  const i = value as Record<string, unknown>;
  return (
    typeof i.productId === "string" &&
    i.productId.length > 0 &&
    typeof i.variantId === "string" &&
    typeof i.priceAmount === "string" &&
    typeof i.currencyCode === "string" &&
    typeof i.qty === "number" &&
    Number.isFinite(i.qty) &&
    i.qty > 0
  );
}

/**
 * Decodes whatever is in storage into usable cart lines.
 *
 * MIGRATES rather than drops. A v1 cart is a bare array whose items are
 * already valid under v2 — personalisation is optional — so a shopper who
 * left something in their basket keeps it across this deploy. Dropping
 * would have been one line shorter and would have emptied every cart in
 * the wild.
 *
 * A version NEWER than this build's is treated as unreadable and discarded:
 * a forward rollback must not have the old build misinterpret a shape it
 * has never seen.
 */
export function decodeStoredCart(raw: string | null): CartItem[] {
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }

  const candidates: unknown[] = Array.isArray(parsed)
    ? parsed
    : isStoredCart(parsed) && parsed.v <= CART_SCHEMA_VERSION
      ? parsed.items
      : [];

  return candidates.filter(isUsableItem);
}

export function encodeStoredCart(items: CartItem[]): string {
  const payload: StoredCart = { v: CART_SCHEMA_VERSION, items };
  return JSON.stringify(payload);
}

export function readCart(storeSlug: string): CartItem[] {
  if (typeof window === "undefined") return [];
  try {
    return decodeStoredCart(localStorage.getItem(cartStorageKey(storeSlug)));
  } catch {
    return [];
  }
}

export function writeCart(storeSlug: string, items: CartItem[]): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(cartStorageKey(storeSlug), encodeStoredCart(items));
  } catch {
    // Storage full or blocked — silently degrade.
  }
}
