import { describe, expect, it } from "vitest";

import type { StorefrontPersonalisationField } from "./api/marketplace-api";
import { personalisationFingerprint } from "./cart";
import {
  resolveTextStyle,
  toCartPersonalisation,
  toCheckoutPersonalisation,
} from "./personalisation";

const styled: StorefrontPersonalisationField = {
  id: "f-name",
  key: "name",
  label: "Name on the mug",
  kind: "text",
  required: true,
  position: 0,
  text_fonts: ["script", "classic"],
  text_colours: [
    { label: "Gold foil", hex: "#C9A227" },
    { label: "White", hex: "#FFFFFF" },
  ],
};

describe("resolveTextStyle", () => {
  it("defaults to the first of each, which is what checkout records", () => {
    expect(resolveTextStyle(styled, {})).toEqual({
      font: "script",
      colour: { label: "Gold foil", hex: "#C9A227" },
    });
  });

  it("takes the buyer's picks while they are still on offer", () => {
    expect(resolveTextStyle(styled, { font: "classic", colour: "White" })).toEqual({
      font: "classic",
      colour: { label: "White", hex: "#FFFFFF" },
    });
  });

  it("falls back when a pick has been withdrawn since the page loaded", () => {
    expect(resolveTextStyle(styled, { font: "block", colour: "Neon" }).font).toBe("script");
  });

  it("offers nothing on a plain text field", () => {
    expect(resolveTextStyle({ text_fonts: undefined, text_colours: undefined }, {})).toEqual({
      font: undefined,
      colour: undefined,
    });
  });
});

describe("text style through the cart", () => {
  it("records the resolved style on the line and sends font + colour label to checkout", () => {
    const cart = toCartPersonalisation([styled], { "f-name": { text: " Asha ", colour: "White" } });
    expect(cart).toEqual([
      {
        fieldId: "f-name",
        fieldLabel: "Name on the mug",
        text: "Asha",
        font: "script",
        colour: "White",
        colourHex: "#FFFFFF",
      },
    ]);
    expect(toCheckoutPersonalisation(cart)).toEqual([
      {
        field_id: "f-name",
        upload_id: undefined,
        text: "Asha",
        option_id: undefined,
        checked: undefined,
        font: "script",
        colour: "White",
      },
    ]);
  });

  it("keeps the line key of every cart stored before styling existed", () => {
    const legacy = [{ fieldId: "f-name", text: "Asha" }];
    expect(personalisationFingerprint(legacy)).toBe(JSON.stringify([["f-name", "", "Asha", "", ""]]));
  });

  it("treats the same words in a different colour as a different line", () => {
    const gold = personalisationFingerprint([{ fieldId: "f", text: "Asha", font: "script", colour: "Gold foil" }]);
    const white = personalisationFingerprint([{ fieldId: "f", text: "Asha", font: "script", colour: "White" }]);
    expect(gold).not.toBe(white);
  });
});
