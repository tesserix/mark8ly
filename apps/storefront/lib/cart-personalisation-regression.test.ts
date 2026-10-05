import { describe, it, expect } from "vitest";

import { addItem, lineKey, type CartItem } from "./cart";
import { toCartPersonalisation } from "./personalisation";
import type { StorefrontPersonalisationField } from "./api/marketplace-api";

// Regression for the bug that made the whole custom-products feature a
// no-op in production (#966).
//
// AddToCartButton built its cart item inside a useCallback that read
// `personalisation` but did not list it as a dependency. The callback
// therefore kept the value from the FIRST render — undefined, before the
// buyer had uploaded anything — so every personalised line was added with
// the buyer's answers silently dropped.
//
// It was invisible from the outside. `blockedReason` is a prop and is
// recomputed every render, so the button correctly flipped from "Finish
// the details above" to "Add to cart" the instant the upload landed. Only
// the thing it then added was stale. tsc was happy; the unit suite was
// happy; eslint flagged it as a WARNING among 110 others and CI does not
// fail on warnings.
//
// A rendering test would be the direct way to pin this, but the
// storefront's vitest runs in `node` with no DOM. So these pin the two
// properties a dropped-personalisation bug violates, either of which
// would have failed loudly.

const imageField: StorefrontPersonalisationField = {
  id: "41c4ce32-07f3-436b-9d19-76d3ab27219c",
  key: "upload_your_photo",
  label: "Upload your photo",
  kind: "image",
  required: true,
  position: 0,
  max_images: 1,
  min_px: 1200,
} as StorefrontPersonalisationField;

function line(over: Partial<CartItem> = {}): CartItem {
  return {
    productId: "p1",
    variantId: "v1",
    handle: "car-model",
    title: "Car model",
    priceAmount: "50",
    currencyCode: "AUD",
    qty: 1,
    ...over,
  };
}

describe("a personalised line reaches the cart with its answers", () => {
  it("converts a completed image answer into a cart entry", () => {
    // The exact shape the live product page produces: one required image
    // field, one confirmed upload id.
    const out = toCartPersonalisation([imageField], {
      [imageField.id]: { uploadIds: ["upload-1"] },
    });

    expect(out, "an uploaded image must survive into the cart line").toBeDefined();
    expect(out).toEqual([
      {
        fieldId: imageField.id,
        fieldLabel: "Upload your photo",
        uploadId: "upload-1",
      },
    ]);
  });

  it("gives a personalised line a different identity from a bare one", () => {
    // This is what the stale-closure bug actually broke. Dropping
    // personalisation does not just lose the answers — it collapses the
    // line onto the un-personalised one, so two buyers' different
    // photographs would merge into a single cart line of quantity 2.
    const withArt = line({
      personalisation: [
        { fieldId: imageField.id, fieldLabel: "Upload your photo", uploadId: "upload-1" },
      ],
    });
    const withoutArt = line();

    expect(lineKey(withArt)).not.toBe(lineKey(withoutArt));

    const merged = addItem([withoutArt], withArt);
    expect(merged, "a personalised line must not merge into a bare one").toHaveLength(2);
  });

  it("keeps two different photos on two different lines", () => {
    const asha = line({
      personalisation: [{ fieldId: imageField.id, uploadId: "upload-asha" }],
    });
    const ravi = line({
      personalisation: [{ fieldId: imageField.id, uploadId: "upload-ravi" }],
    });

    expect(lineKey(asha)).not.toBe(lineKey(ravi));
    expect(addItem([asha], ravi)).toHaveLength(2);
  });

  it("still merges two identical personalised lines", () => {
    // The other half: identity must be by CONTENT, so adding the same
    // photo twice is quantity 2, not two rows.
    const first = line({
      personalisation: [{ fieldId: imageField.id, uploadId: "upload-1" }],
    });
    const again = line({
      personalisation: [{ fieldId: imageField.id, uploadId: "upload-1" }],
    });

    const merged = addItem([first], again);
    expect(merged).toHaveLength(1);
    expect(merged[0]!.qty).toBe(2);
  });
});
