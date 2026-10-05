// apps/storefront/lib/cart.ts
//
// Pure cart helpers. No React, no DOM, no localStorage.
// Every function returns a new array — never mutates.
//
// # Lines are keyed by identity, not by variant (#964)
//
// These helpers used to merge on (productId, variantId), which assumes one
// variant can appear in a cart only once. Personalisation breaks that: a
// shirt printed "Asha" and the same shirt printed "Ravi" are the same
// variant and must be two lines, each with its own quantity.
//
// So the merge key is lineKey(item) — the variant plus a canonical
// fingerprint of what the buyer filled in. With no personalisation the
// fingerprint is empty and the key reduces to the old pair, which is why
// this change is behaviour-preserving for every product that exists today.

export type CartItemTaxCategory =
  | "standard"
  | "reduced"
  | "zero_rated"
  | "exempt";

/**
 * One answer the buyer gave to a personalisation field.
 *
 * Exactly one value field is meaningful, decided by the field's kind:
 * uploadId for image, text for text/textarea, optionId for select,
 * checked for checkbox. The shape is permissive on purpose — validation
 * belongs to the server (#967), and a cart that cannot represent what the
 * buyer typed is worse than one that carries it unvalidated.
 */
export interface CartItemPersonalisation {
  fieldId: string;
  uploadId?: string;
  text?: string;
  optionId?: string;
  checked?: boolean;
  /**
   * Display-only snapshots taken at add-to-cart (#966).
   *
   * The cart has to render "Name to engrave: Asha" without a network
   * call — it is a client component reading localStorage, and the
   * product's field definitions are not there. So the labels come along
   * for the ride, the same way title and priceAmount already do.
   *
   * DELIBERATELY OUTSIDE personalisationFingerprint. That function is an
   * explicit positional tuple rather than a serialisation of this whole
   * object, which is what makes adding these safe: a label is not part
   * of a line's identity, and folding it in would change every existing
   * key and silently stop quantity merging for carts already in
   * localStorage.
   *
   * Optional because carts stored before this shipped do not have them.
   * The UI renders the value alone in that case rather than inventing a
   * label.
   */
  fieldLabel?: string;
  optionLabel?: string;
}

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
  /**
   * What the buyer filled in, when the product asks for anything (#962).
   * Absent on every product that does not, which is all of them until the
   * storefront form ships (#965).
   */
  personalisation?: CartItemPersonalisation[];
}

/**
 * A line's identity, as an opaque token.
 *
 * Branded deliberately. The key is a string and so is a variant id, so
 * without a brand `updateQty(item.variantId, n)` — the pre-#964 call —
 * compiles cleanly and silently matches nothing, which is the worst
 * possible outcome for a refactor whose entire hazard is "a string that
 * looks like the old string". The brand turns every stale call site into
 * a compile error.
 */
export type CartLineKey = string & { readonly __cartLineKey: unique symbol };

/** The part of a CartItem that decides which line it belongs to. */
export type LineIdentity = Pick<
  CartItem,
  "productId" | "variantId" | "personalisation"
>;

/**
 * Canonical fingerprint of a buyer's answers.
 *
 * Sorted by fieldId and emitted as positional tuples, so the key does not
 * depend on the order the form happened to produce or on JS object key
 * order. Returns "" when there is nothing to fingerprint, which is what
 * makes lineKey collapse to the pre-#964 pair.
 *
 * Deliberately NOT hashed. This is a local merge key, not a security
 * boundary, and a readable key is one you can diagnose from a
 * localStorage dump.
 */
export function personalisationFingerprint(
  entries: readonly CartItemPersonalisation[] | undefined,
): string {
  if (!entries || entries.length === 0) return "";
  const canonical = entries
    .map((e) => [
      e.fieldId,
      e.uploadId ?? "",
      e.text ?? "",
      e.optionId ?? "",
      e.checked === undefined ? "" : String(e.checked),
    ])
    .sort((a, b) => (a[0]! < b[0]! ? -1 : a[0]! > b[0]! ? 1 : 0));
  return JSON.stringify(canonical);
}

/**
 * The identity of a cart line.
 *
 * Accepts anything carrying the identity fields, so a caller holding only
 * a product and variant — AddToCartButton, before any personalisation
 * exists — can compute the same key the stored item will have.
 */
export function lineKey(item: LineIdentity): CartLineKey {
  return JSON.stringify([
    item.productId,
    item.variantId,
    personalisationFingerprint(item.personalisation),
  ]) as CartLineKey;
}

export function addItem(items: readonly CartItem[], item: CartItem): CartItem[] {
  const key = lineKey(item);
  const idx = items.findIndex((i) => lineKey(i) === key);
  if (idx >= 0) {
    return items.map((i, j) =>
      j === idx ? { ...i, qty: i.qty + item.qty } : i,
    );
  }
  return [...items, item];
}

export function removeItem(items: readonly CartItem[], key: CartLineKey): CartItem[] {
  return items.filter((i) => lineKey(i) !== key);
}

export function setQty(
  items: readonly CartItem[],
  key: CartLineKey,
  qty: number,
): CartItem[] {
  if (qty <= 0) return removeItem(items, key);
  return items.map((i) => (lineKey(i) === key ? { ...i, qty } : i));
}

export function subtotal(items: readonly CartItem[]): number {
  return items.reduce((sum, i) => sum + Number.parseFloat(i.priceAmount) * i.qty, 0);
}

export function count(items: readonly CartItem[]): number {
  return items.reduce((sum, i) => sum + i.qty, 0);
}
