import { beforeEach, describe, expect, it } from "vitest";

import {
  type Cart,
  cartReducer,
  change,
  emptyCart,
  loadCart,
  saveCart,
  subtotal,
  toSaleLines,
} from "./cart";

const shirt = { variantId: "v1", title: "Linen Shirt", optionSummary: "M", unitPrice: "149.00" };

function withLines(...actions: Parameters<typeof cartReducer>[1][]): Cart {
  return actions.reduce(cartReducer, emptyCart());
}

describe("cartReducer", () => {
  it("add merges the same variant", () => {
    const cart = withLines({ type: "add", line: shirt }, { type: "add", line: shirt });
    expect(cart.lines).toHaveLength(1);
    expect(cart.lines[0]!.quantity).toBe(2);
  });

  it("custom lines never merge", () => {
    const custom = { title: "Gift wrap", unitPrice: "5.00", custom: true };
    const cart = withLines({ type: "add", line: custom }, { type: "add", line: custom });
    expect(cart.lines).toHaveLength(2);
  });

  it("setQty 0 removes", () => {
    const cart = withLines({ type: "add", line: shirt });
    const next = cartReducer(cart, { type: "setQty", key: cart.lines[0]!.key, quantity: 0 });
    expect(next.lines).toHaveLength(0);
  });

  it("beginCharge creates a key once", () => {
    const first = withLines({ type: "add", line: shirt }, { type: "beginCharge" });
    expect(first.idempotencyKey).toBeTruthy();
    expect(cartReducer(first, { type: "beginCharge" }).idempotencyKey).toBe(first.idempotencyKey);
  });

  it("locks the cart after an uncertain failure", () => {
    const locked = withLines({ type: "add", line: shirt }, { type: "beginCharge" }, { type: "uncertain" });
    expect(locked.locked).toBe(true);
    const key = locked.lines[0]!.key;
    expect(cartReducer(locked, { type: "add", line: shirt })).toBe(locked);
    expect(cartReducer(locked, { type: "setQty", key, quantity: 5 })).toBe(locked);
    expect(cartReducer(locked, { type: "remove", key })).toBe(locked);
    expect(cartReducer(locked, { type: "discount", value: "10" })).toBe(locked);

    const reset = cartReducer(locked, { type: "reset" });
    expect(reset).toEqual(emptyCart());
  });

  it("refused clears the key and unlocks", () => {
    const cart = withLines({ type: "add", line: shirt }, { type: "beginCharge" }, { type: "uncertain" }, { type: "refused" });
    expect(cart.idempotencyKey).toBeNull();
    expect(cart.locked).toBe(false);
    expect(cart.lines).toHaveLength(1);
  });
});

describe("money", () => {
  it("subtotal adds lines in cents", () => {
    const cart = withLines(
      { type: "add", line: { ...shirt, unitPrice: "0.10" } },
      { type: "add", line: { ...shirt, unitPrice: "0.10" } },
      { type: "add", line: { title: "Bag", unitPrice: "0.20", custom: true } },
    );
    expect(subtotal(cart)).toBe("0.40");
  });

  it("change", () => {
    expect(change("50", "38.50")).toBe("11.50");
    expect(change("38.50", "38.50")).toBe("0.00");
    expect(change("30", "38.50")).toBeNull();
    expect(change("abc", "1")).toBeNull();
    expect(change("", "1")).toBeNull();
  });
});

describe("storage", () => {
  beforeEach(() => localStorage.clear());

  it("persists and restores", () => {
    const cart = withLines({ type: "add", line: shirt }, { type: "beginCharge" }, { type: "uncertain" });
    saveCart("s1", cart);
    expect(loadCart("s1")).toEqual(cart);
    expect(loadCart("s2")).toEqual(emptyCart());

    localStorage.setItem("mark8ly:instore-cart:s1", "{not json");
    expect(loadCart("s1")).toEqual(emptyCart());
  });
});

describe("toSaleLines", () => {
  it("maps custom lines and never sends a price for catalogue lines", () => {
    const cart = withLines(
      { type: "add", line: shirt },
      { type: "add", line: { title: "Gift wrap", unitPrice: "5.00", custom: true } },
    );
    expect(toSaleLines(cart)).toEqual([
      { variant_id: "v1", quantity: 1 },
      { custom_title: "Gift wrap", custom_price: "5.00", quantity: 1 },
    ]);
  });
});
