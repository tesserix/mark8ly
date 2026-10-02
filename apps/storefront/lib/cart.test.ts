import { describe, it, expect } from "vitest";

import {
  addItem,
  count,
  lineKey,
  personalisationFingerprint,
  removeItem,
  setQty,
  subtotal,
  type CartItem,
} from "./cart";

function item(over: Partial<CartItem> = {}): CartItem {
  return {
    productId: "p1",
    variantId: "v1",
    handle: "linen-shirt",
    title: "Linen shirt",
    priceAmount: "25.00",
    currencyCode: "INR",
    qty: 1,
    ...over,
  };
}

describe("lineKey", () => {
  it("reduces to the variant when nothing was personalised", () => {
    // The behaviour-preservation claim of #964: every product that exists
    // today has no personalisation, so its key is the old pair.
    expect(lineKey(item())).toBe(lineKey({ productId: "p1", variantId: "v1" }));
  });

  it("separates two personalisations of one variant", () => {
    const asha = item({ personalisation: [{ fieldId: "f1", text: "Asha" }] });
    const ravi = item({ personalisation: [{ fieldId: "f1", text: "Ravi" }] });
    expect(lineKey(asha)).not.toBe(lineKey(ravi));
  });

  it("separates a personalised line from a plain one", () => {
    expect(lineKey(item())).not.toBe(
      lineKey(item({ personalisation: [{ fieldId: "f1", text: "Asha" }] })),
    );
  });

  it("does not depend on the order the form produced the answers", () => {
    // The form can emit fields in any order; the same cart line must not
    // become two because of it.
    const a = item({
      personalisation: [
        { fieldId: "b", text: "two" },
        { fieldId: "a", text: "one" },
      ],
    });
    const b = item({
      personalisation: [
        { fieldId: "a", text: "one" },
        { fieldId: "b", text: "two" },
      ],
    });
    expect(lineKey(a)).toBe(lineKey(b));
  });

  it("distinguishes different uploads of the same field", () => {
    const one = item({ personalisation: [{ fieldId: "f1", uploadId: "u1" }] });
    const two = item({ personalisation: [{ fieldId: "f1", uploadId: "u2" }] });
    expect(lineKey(one)).not.toBe(lineKey(two));
  });

  it("does not confuse a value in one slot with the same value in another", () => {
    const asText = item({ personalisation: [{ fieldId: "f1", text: "x" }] });
    const asOption = item({ personalisation: [{ fieldId: "f1", optionId: "x" }] });
    expect(lineKey(asText)).not.toBe(lineKey(asOption));
  });

  it("treats checkbox false as an answer, not as absence", () => {
    const unticked = item({ personalisation: [{ fieldId: "f1", checked: false }] });
    expect(lineKey(unticked)).not.toBe(lineKey(item()));
    expect(lineKey(unticked)).not.toBe(
      lineKey(item({ personalisation: [{ fieldId: "f1", checked: true }] })),
    );
  });

  it("keeps different variants and different products apart", () => {
    expect(lineKey(item())).not.toBe(lineKey(item({ variantId: "v2" })));
    expect(lineKey(item())).not.toBe(lineKey(item({ productId: "p2" })));
  });
});

describe("personalisationFingerprint", () => {
  it("is empty for nothing, which is what collapses the key", () => {
    expect(personalisationFingerprint(undefined)).toBe("");
    expect(personalisationFingerprint([])).toBe("");
  });
});

describe("addItem", () => {
  it("merges quantities for the same line", () => {
    const out = addItem([item({ qty: 2 })], item({ qty: 3 }));
    expect(out).toHaveLength(1);
    expect(out[0]!.qty).toBe(5);
  });

  it("keeps two personalisations of one variant as two lines", () => {
    // The whole point of #964.
    const asha = item({ personalisation: [{ fieldId: "f1", text: "Asha" }] });
    const ravi = item({ personalisation: [{ fieldId: "f1", text: "Ravi" }] });
    const out = addItem(addItem([], asha), ravi);
    expect(out).toHaveLength(2);
    expect(count(out)).toBe(2);
  });

  it("does not mutate the input", () => {
    const start = [item({ qty: 1 })];
    addItem(start, item({ qty: 1 }));
    expect(start[0]!.qty).toBe(1);
  });
});

describe("removeItem", () => {
  it("removes only the line whose key matches", () => {
    const asha = item({ personalisation: [{ fieldId: "f1", text: "Asha" }] });
    const ravi = item({ personalisation: [{ fieldId: "f1", text: "Ravi" }] });
    const out = removeItem([asha, ravi], lineKey(asha));
    expect(out).toHaveLength(1);
    expect(out[0]!.personalisation?.[0]!.text).toBe("Ravi");
  });

  it("leaves the cart alone for a key that is not in it", () => {
    const items = [item()];
    expect(removeItem(items, lineKey(item({ variantId: "nope" })))).toHaveLength(1);
  });
});

describe("setQty", () => {
  it("changes only the matching line", () => {
    const asha = item({ personalisation: [{ fieldId: "f1", text: "Asha" }], qty: 1 });
    const ravi = item({ personalisation: [{ fieldId: "f1", text: "Ravi" }], qty: 1 });
    const out = setQty([asha, ravi], lineKey(ravi), 4);
    expect(out.find((i) => i.personalisation?.[0]!.text === "Asha")!.qty).toBe(1);
    expect(out.find((i) => i.personalisation?.[0]!.text === "Ravi")!.qty).toBe(4);
  });

  it("removes the line at zero or below", () => {
    expect(setQty([item()], lineKey(item()), 0)).toHaveLength(0);
    expect(setQty([item()], lineKey(item()), -1)).toHaveLength(0);
  });
});

describe("totals", () => {
  it("sums across separate personalised lines", () => {
    const asha = item({ personalisation: [{ fieldId: "f1", text: "Asha" }], qty: 2 });
    const ravi = item({ personalisation: [{ fieldId: "f1", text: "Ravi" }], qty: 1 });
    expect(count([asha, ravi])).toBe(3);
    expect(subtotal([asha, ravi])).toBeCloseTo(75);
  });
});
