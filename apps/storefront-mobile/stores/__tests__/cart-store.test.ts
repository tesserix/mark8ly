import { describe, it, expect, beforeEach } from "vitest";
import { useCartStore } from "../cart-store";

function resetStore() {
  useCartStore.getState().clear();
}

const baseItem = {
  productId: "prod-1",
  variantId: "var-1",
  handle: "test-product",
  title: "Test Product",
  priceAmount: 10.0,
  currencyCode: "USD",
};

describe("cart-store", () => {
  beforeEach(() => {
    resetStore();
  });

  it("starts empty", () => {
    const { items, count, subtotal } = useCartStore.getState();
    expect(items).toHaveLength(0);
    expect(count()).toBe(0);
    expect(subtotal()).toBe("0.00");
  });

  it("adds an item", () => {
    useCartStore.getState().addItem({ ...baseItem, qty: 1 });
    const { items, count } = useCartStore.getState();
    expect(items).toHaveLength(1);
    expect(items[0].qty).toBe(1);
    expect(count()).toBe(1);
  });

  it("merges qty for the same variant", () => {
    useCartStore.getState().addItem({ ...baseItem, qty: 2 });
    useCartStore.getState().addItem({ ...baseItem, qty: 3 });
    const { items, count } = useCartStore.getState();
    expect(items).toHaveLength(1);
    expect(items[0].qty).toBe(5);
    expect(count()).toBe(5);
  });

  it("keeps separate entries for different variants", () => {
    useCartStore.getState().addItem({ ...baseItem, variantId: "var-1", qty: 1 });
    useCartStore.getState().addItem({ ...baseItem, variantId: "var-2", qty: 1 });
    expect(useCartStore.getState().items).toHaveLength(2);
  });

  it("removes an item by variantId", () => {
    useCartStore.getState().addItem({ ...baseItem, qty: 1 });
    useCartStore.getState().removeItem("var-1");
    expect(useCartStore.getState().items).toHaveLength(0);
  });

  it("updates qty for an existing item", () => {
    useCartStore.getState().addItem({ ...baseItem, qty: 1 });
    useCartStore.getState().updateQty("var-1", 4);
    expect(useCartStore.getState().items[0].qty).toBe(4);
  });

  it("removes item when qty updated to 0", () => {
    useCartStore.getState().addItem({ ...baseItem, qty: 2 });
    useCartStore.getState().updateQty("var-1", 0);
    expect(useCartStore.getState().items).toHaveLength(0);
  });

  it("computes correct subtotal", () => {
    useCartStore.getState().addItem({ ...baseItem, priceAmount: 10.0, qty: 2 });
    useCartStore.getState().addItem({
      ...baseItem,
      variantId: "var-2",
      priceAmount: 5.5,
      qty: 3,
    });
    // 2*10.0 + 3*5.5 = 20 + 16.5 = 36.5
    expect(useCartStore.getState().subtotal()).toBe("36.50");
  });

  it("clears all items", () => {
    useCartStore.getState().addItem({ ...baseItem, qty: 3 });
    useCartStore.getState().clear();
    expect(useCartStore.getState().items).toHaveLength(0);
    expect(useCartStore.getState().count()).toBe(0);
  });
});
