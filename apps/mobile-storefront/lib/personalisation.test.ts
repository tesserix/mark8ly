import { describe, expect, it } from "vitest";

import type { StorefrontPersonalisationField } from "@repo/mobile-shared/api/storefront-types";
import { lineKey } from "@/lib/cart-store";
import {
  describeCartPersonalisation,
  isBelowMinimumResolution,
  personalisationSurcharge,
  sortFields,
  toCartPersonalisation,
  toCheckoutPersonalisation,
  validateAnswers,
} from "./personalisation";

function field(
  over: Partial<StorefrontPersonalisationField>,
): StorefrontPersonalisationField {
  return {
    id: "f",
    key: "k",
    label: "Label",
    kind: "text",
    required: false,
    position: 0,
    ...over,
  };
}

const photo = field({
  id: "photo",
  key: "photo",
  label: "Your photo",
  kind: "image",
  required: true,
  min_px: 1200,
});
const name = field({
  id: "name",
  key: "name",
  label: "Name to engrave",
  kind: "text",
  required: true,
  max_length: 12,
});
const finish = field({
  id: "finish",
  key: "finish",
  label: "Finish",
  kind: "select",
  required: true,
  options: [
    { id: "matte", label: "Matte", price_delta: "0" },
    { id: "gloss", label: "Gloss", price_delta: "2.50" },
  ],
});
const gift = field({
  id: "gift",
  key: "gift",
  label: "Gift wrap",
  kind: "checkbox",
  price_delta: "4",
});

describe("validateAnswers", () => {
  it("passes a product that asks for nothing", () => {
    expect(validateAnswers(undefined, {})).toEqual([]);
    expect(validateAnswers([], { x: { text: "ignored" } })).toEqual([]);
  });

  it("names every missing required field, not just the first", () => {
    const problems = validateAnswers([photo, name, finish], {});
    expect(problems.map((p) => p.fieldId)).toEqual(["photo", "name", "finish"]);
    expect(problems[0]!.message).toContain("Your photo");
  });

  it("treats whitespace as empty for a required text field", () => {
    expect(validateAnswers([name], { name: { text: "   " } })).toHaveLength(1);
    expect(validateAnswers([name], { name: { text: " Asha " } })).toEqual([]);
  });

  it("enforces max_length on the trimmed value", () => {
    expect(
      validateAnswers([name], { name: { text: "Thirteen chars" } })[0]!.message,
    ).toContain("12 characters");
  });

  it("rejects a select option the product no longer offers", () => {
    expect(
      validateAnswers([finish], { finish: { optionId: "chrome" } })[0]!.message,
    ).toContain("no longer available");
    expect(
      validateAnswers([finish], { finish: { optionId: "gloss" } }),
    ).toEqual([]);
  });

  it("a required checkbox must be ticked; an optional one may be anything", () => {
    const consent = field({
      id: "c",
      kind: "checkbox",
      required: true,
      label: "I agree",
    });
    expect(validateAnswers([consent], { c: { checked: false } })).toHaveLength(
      1,
    );
    expect(validateAnswers([consent], { c: { checked: true } })).toEqual([]);
    expect(validateAnswers([gift], {})).toEqual([]);
  });

  it("caps images at max_images, defaulting to one", () => {
    expect(
      validateAnswers([photo], { photo: { uploadIds: ["a", "b"] } })[0]!
        .message,
    ).toContain("Only one photo");
    const many = field({ ...photo, max_images: 3 });
    expect(
      validateAnswers([many], {
        photo: { uploadIds: ["a", "b", "c", "d"] },
      })[0]!.message,
    ).toContain("At most 3");
  });
});

describe("toCartPersonalisation", () => {
  it("keeps only answers that mean something, and snapshots labels", () => {
    const out = toCartPersonalisation([photo, name, finish, gift], {
      photo: { uploadIds: ["u1"] },
      name: { text: "  Asha " },
      finish: { optionId: "gloss" },
      gift: { checked: false },
    });
    expect(out).toEqual([
      { fieldId: "photo", fieldLabel: "Your photo", uploadId: "u1" },
      { fieldId: "name", fieldLabel: "Name to engrave", text: "Asha" },
      {
        fieldId: "finish",
        fieldLabel: "Finish",
        optionId: "gloss",
        optionLabel: "Gloss",
      },
    ]);
  });

  it("is undefined, not empty, when nothing was answered", () => {
    expect(toCartPersonalisation([gift], {})).toBeUndefined();
    expect(
      toCartPersonalisation([gift], { gift: { checked: false } }),
    ).toBeUndefined();
    expect(toCartPersonalisation(undefined, {})).toBeUndefined();
  });

  it("labels do not change a line's identity", () => {
    // The guard that makes adding fieldLabel/optionLabel safe: lineKey is
    // a positional tuple, so two lines differing only in snapshots merge.
    const a = toCartPersonalisation([finish], {
      finish: { optionId: "gloss" },
    })!;
    const b = a.map((e) => ({
      ...e,
      fieldLabel: "Renamed",
      optionLabel: "Shiny",
    }));
    expect(
      lineKey({ productId: "p", variantId: "v", personalisation: a }),
    ).toBe(lineKey({ productId: "p", variantId: "v", personalisation: b }));
  });
});

describe("personalisationSurcharge", () => {
  it("adds ticked checkbox deltas and the chosen option's delta", () => {
    expect(
      personalisationSurcharge([finish, gift], {
        finish: { optionId: "gloss" },
        gift: { checked: true },
      }),
    ).toBe(6.5);
    expect(
      personalisationSurcharge([finish, gift], {
        finish: { optionId: "matte" },
      }),
    ).toBe(0);
    expect(personalisationSurcharge(undefined, {})).toBe(0);
  });
});

describe("isBelowMinimumResolution", () => {
  it("compares the short edge", () => {
    expect(isBelowMinimumResolution(photo, { width: 4000, height: 600 })).toBe(
      true,
    );
    expect(isBelowMinimumResolution(photo, { width: 1200, height: 1600 })).toBe(
      false,
    );
  });
  it("stays quiet when there is no minimum or no measurement", () => {
    expect(isBelowMinimumResolution({}, { width: 10, height: 10 })).toBe(false);
    expect(isBelowMinimumResolution(photo, null)).toBe(false);
    expect(isBelowMinimumResolution(photo, { width: 0, height: 0 })).toBe(
      false,
    );
  });
});

describe("toCheckoutPersonalisation", () => {
  it("strips the display-only labels down to the wire contract", () => {
    expect(
      toCheckoutPersonalisation([
        { fieldId: "photo", fieldLabel: "Your photo", uploadId: "u1" },
        {
          fieldId: "finish",
          fieldLabel: "Finish",
          optionId: "gloss",
          optionLabel: "Gloss",
        },
      ]),
    ).toEqual([
      {
        field_id: "photo",
        upload_id: "u1",
        text: undefined,
        option_id: undefined,
        checked: undefined,
      },
      {
        field_id: "finish",
        upload_id: undefined,
        text: undefined,
        option_id: "gloss",
        checked: undefined,
      },
    ]);
    expect(toCheckoutPersonalisation(undefined)).toBeUndefined();
    expect(toCheckoutPersonalisation([])).toBeUndefined();
  });
});

describe("describeCartPersonalisation", () => {
  it("renders from snapshots alone", () => {
    expect(
      describeCartPersonalisation([
        { fieldId: "photo", fieldLabel: "Your photo", uploadId: "u1" },
        { fieldId: "name", fieldLabel: "Name", text: "Asha" },
        {
          fieldId: "finish",
          fieldLabel: "Finish",
          optionId: "gloss",
          optionLabel: "Gloss",
        },
        { fieldId: "gift", fieldLabel: "Gift wrap", checked: true },
        { fieldId: "old", optionId: "x" },
      ]),
    ).toEqual([
      { label: "Your photo", value: "Photo added" },
      { label: "Name", value: "Asha" },
      { label: "Finish", value: "Gloss" },
      { label: "Gift wrap", value: "Yes" },
      { label: "Personalisation", value: "Selected" },
    ]);
  });
});

describe("sortFields", () => {
  it("orders by position, then key, without mutating the input", () => {
    const input = [
      field({ id: "b", key: "b", position: 1 }),
      field({ id: "a", key: "a", position: 1 }),
      field({ id: "z", key: "z", position: 0 }),
    ];
    expect(sortFields(input).map((f) => f.id)).toEqual(["z", "a", "b"]);
    expect(input[0]!.id).toBe("b");
  });
});
