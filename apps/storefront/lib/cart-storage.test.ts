import { describe, it, expect } from "vitest";

import {
  CART_SCHEMA_VERSION,
  decodeStoredCart,
  encodeStoredCart,
  isUsableItem,
} from "./cart-storage";
import type { CartItem } from "./cart";

function item(over: Partial<CartItem> = {}): CartItem {
  return {
    productId: "p1",
    variantId: "v1",
    handle: "linen-shirt",
    title: "Linen shirt",
    priceAmount: "25.00",
    currencyCode: "INR",
    qty: 2,
    ...over,
  };
}

describe("decodeStoredCart — the v1 migration", () => {
  it("keeps a cart written by the previous build", () => {
    // This is the test that decides whether shipping #964 empties every
    // basket in the wild. v1 stored a bare array.
    const v1 = JSON.stringify([item(), item({ variantId: "v2" })]);
    const out = decodeStoredCart(v1);
    expect(out).toHaveLength(2);
    expect(out[0]!.productId).toBe("p1");
    expect(out[0]!.qty).toBe(2);
  });

  it("round-trips a v2 cart", () => {
    const out = decodeStoredCart(encodeStoredCart([item()]));
    expect(out).toHaveLength(1);
    expect(out[0]!.title).toBe("Linen shirt");
  });

  it("carries personalisation through a round trip", () => {
    const personalised = item({
      personalisation: [{ fieldId: "f1", text: "Asha" }, { fieldId: "f2", uploadId: "u9" }],
    });
    const out = decodeStoredCart(encodeStoredCart([personalised]));
    expect(out[0]!.personalisation).toEqual([
      { fieldId: "f1", text: "Asha" },
      { fieldId: "f2", uploadId: "u9" },
    ]);
  });

  it("discards a cart from a NEWER build rather than misreading it", () => {
    // Forward rollback: the old build must not interpret a shape it has
    // never seen. Dropping is correct here; guessing is not.
    const future = JSON.stringify({ v: CART_SCHEMA_VERSION + 1, items: [item()] });
    expect(decodeStoredCart(future)).toEqual([]);
  });
});

describe("decodeStoredCart — hostile and broken input", () => {
  it("survives everything that is not a cart", () => {
    for (const raw of [
      null,
      "",
      "not json",
      "null",
      '"a string"',
      "42",
      "{}",
      '{"v":2}',
      '{"items":[]}',
      '{"v":"two","items":[]}',
    ]) {
      expect(decodeStoredCart(raw)).toEqual([]);
    }
  });

  it("drops a malformed line and keeps the rest", () => {
    // Losing one row beats losing the basket — and beats posting a line
    // with no price to checkout, which the unchecked cast used to allow.
    const mixed = JSON.stringify([
      item(),
      { productId: "p2" }, // truncated
      item({ variantId: "v3" }),
    ]);
    expect(decodeStoredCart(mixed)).toHaveLength(2);
  });

  it("rejects lines that would break the checkout payload", () => {
    expect(isUsableItem(item({ qty: 0 }))).toBe(false);
    expect(isUsableItem(item({ qty: Number.NaN }))).toBe(false);
    expect(isUsableItem(item({ productId: "" }))).toBe(false);
    expect(isUsableItem({ ...item(), priceAmount: 25 })).toBe(false);
    expect(isUsableItem({ ...item(), currencyCode: undefined })).toBe(false);
  });

  it("accepts a well-formed line", () => {
    expect(isUsableItem(item())).toBe(true);
  });
});
