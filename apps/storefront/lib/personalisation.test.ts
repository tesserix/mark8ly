import { describe, it, expect } from "vitest";

import {
  isBelowMinimumResolution,
  personalisationSurcharge,
  toCartPersonalisation,
  validateAnswers,
  type PersonalisationAnswers,
} from "./personalisation";
import { lineKey } from "./cart";
import type { StorefrontPersonalisationField } from "./api/marketplace-api";

function field(
  over: Partial<StorefrontPersonalisationField> = {},
): StorefrontPersonalisationField {
  return {
    id: "f1",
    key: "name",
    label: "Name",
    kind: "text",
    required: false,
    position: 0,
    ...over,
  };
}

describe("validateAnswers", () => {
  it("has nothing to say about a product that asks for nothing", () => {
    expect(validateAnswers(undefined, {})).toEqual([]);
    expect(validateAnswers([], {})).toEqual([]);
  });

  it("names the field in every message, so the form can place it", () => {
    const problems = validateAnswers(
      [field({ required: true, label: "Name to engrave" })],
      {},
    );
    expect(problems).toHaveLength(1);
    expect(problems[0]!.fieldId).toBe("f1");
    expect(problems[0]!.message).toContain("Name to engrave");
  });

  it("treats whitespace as empty for a required text field", () => {
    expect(validateAnswers([field({ required: true })], { f1: { text: "   " } })).toHaveLength(1);
    expect(validateAnswers([field({ required: true })], { f1: { text: "Asha" } })).toHaveLength(0);
  });

  it("enforces the character limit", () => {
    const f = field({ max_length: 5 });
    expect(validateAnswers([f], { f1: { text: "12345" } })).toHaveLength(0);
    expect(validateAnswers([f], { f1: { text: "123456" } })).toHaveLength(1);
  });

  it("requires an image when the merchant said so, and caps the count", () => {
    const img = field({ kind: "image", required: true, max_images: 2, label: "Your photo" });
    expect(validateAnswers([img], {})).toHaveLength(1);
    expect(validateAnswers([img], { f1: { uploadIds: ["u1"] } })).toHaveLength(0);
    expect(validateAnswers([img], { f1: { uploadIds: ["u1", "u2", "u3"] } })).toHaveLength(1);
  });

  it("does not say “images” when only one is allowed", () => {
    const img = field({ kind: "image", max_images: 1 });
    const [problem] = validateAnswers([img], { f1: { uploadIds: ["a", "b"] } });
    expect(problem!.message).toContain("Only one image");
  });

  it("rejects a choice the product no longer offers", () => {
    // A stale tab or a tampered form. Catching it here means the buyer is
    // told rather than bounced at checkout.
    const sel = field({
      kind: "select",
      options: [{ id: "o1", label: "Matte", price_delta: "0" }],
    });
    expect(validateAnswers([sel], { f1: { optionId: "o1" } })).toHaveLength(0);
    expect(validateAnswers([sel], { f1: { optionId: "gone" } })).toHaveLength(1);
  });

  it("treats a required checkbox as “must tick”, not “must answer”", () => {
    const box = field({ kind: "checkbox", required: true });
    expect(validateAnswers([box], {})).toHaveLength(1);
    expect(validateAnswers([box], { f1: { checked: false } })).toHaveLength(1);
    expect(validateAnswers([box], { f1: { checked: true } })).toHaveLength(0);
  });

  it("collects a problem per field rather than stopping at the first", () => {
    const problems = validateAnswers(
      [
        field({ id: "a", required: true }),
        field({ id: "b", kind: "image", required: true }),
        field({ id: "c", kind: "checkbox", required: true }),
      ],
      {},
    );
    expect(problems.map((p) => p.fieldId)).toEqual(["a", "b", "c"]);
  });
});

describe("toCartPersonalisation", () => {
  it("omits untouched optional fields entirely", () => {
    // Two buyers who both skipped an optional field must land on the same
    // cart line, not two lines differing by an empty string.
    const fields = [field({ id: "a" }), field({ id: "b", kind: "checkbox" })];
    const skipped: PersonalisationAnswers = {};
    const blanked: PersonalisationAnswers = { a: { text: "  " }, b: { checked: false } };

    expect(toCartPersonalisation(fields, skipped)).toBeUndefined();
    expect(toCartPersonalisation(fields, blanked)).toBeUndefined();

    const base = { productId: "p", variantId: "v" };
    expect(lineKey({ ...base, personalisation: toCartPersonalisation(fields, skipped) })).toBe(
      lineKey({ ...base, personalisation: toCartPersonalisation(fields, blanked) }),
    );
  });

  it("records an unticked box as absent and a ticked one as true", () => {
    const box = [field({ kind: "checkbox" })];
    expect(toCartPersonalisation(box, { f1: { checked: false } })).toBeUndefined();
    // fieldLabel is snapshotted for display (#966): the cart renders
    // from localStorage and has no field definitions to look it up in.
    expect(toCartPersonalisation(box, { f1: { checked: true } })).toEqual([
      { fieldId: "f1", fieldLabel: "Name", checked: true },
    ]);
  });

  it("emits one entry per uploaded image", () => {
    const img = [field({ kind: "image", max_images: 3 })];
    expect(toCartPersonalisation(img, { f1: { uploadIds: ["u1", "u2"] } })).toEqual([
      { fieldId: "f1", fieldLabel: "Name", uploadId: "u1" },
      { fieldId: "f1", fieldLabel: "Name", uploadId: "u2" },
    ]);
  });

  it("produces different line keys for different answers", () => {
    // The whole reason #964 exists.
    const fields = [field()];
    const base = { productId: "p", variantId: "v" };
    const asha = lineKey({ ...base, personalisation: toCartPersonalisation(fields, { f1: { text: "Asha" } }) });
    const ravi = lineKey({ ...base, personalisation: toCartPersonalisation(fields, { f1: { text: "Ravi" } }) });
    expect(asha).not.toBe(ravi);
  });
});

describe("personalisationSurcharge", () => {
  it("adds up what the buyer actually chose", () => {
    const fields = [
      field({ id: "wrap", kind: "checkbox", price_delta: "4.00" }),
      field({
        id: "finish",
        kind: "select",
        options: [
          { id: "matte", label: "Matte", price_delta: "0" },
          { id: "gloss", label: "Gloss", price_delta: "2.50" },
        ],
      }),
    ];
    expect(personalisationSurcharge(fields, {})).toBe(0);
    expect(personalisationSurcharge(fields, { wrap: { checked: true } })).toBe(4);
    expect(
      personalisationSurcharge(fields, { wrap: { checked: true }, finish: { optionId: "gloss" } }),
    ).toBe(6.5);
    expect(personalisationSurcharge(fields, { finish: { optionId: "matte" } })).toBe(0);
  });

  it("treats an unparseable delta as zero rather than NaN", () => {
    // A NaN would propagate into the displayed price and read as a bug to
    // the shopper. The server prices the line regardless.
    const fields = [field({ kind: "checkbox", price_delta: "not a number" })];
    expect(personalisationSurcharge(fields, { f1: { checked: true } })).toBe(0);
  });
});

describe("isBelowMinimumResolution", () => {
  it("measures the SHORT edge, because that is what limits print size", () => {
    const f = { min_px: 1200 };
    // A wide panorama has plenty of pixels and still cannot fill a square.
    expect(isBelowMinimumResolution(f, { width: 4000, height: 600 })).toBe(true);
    expect(isBelowMinimumResolution(f, { width: 1200, height: 1200 })).toBe(false);
    expect(isBelowMinimumResolution(f, { width: 1199, height: 4000 })).toBe(true);
  });

  it("stays quiet when there is nothing to say", () => {
    // No minimum set, or the browser could not measure it. A warning
    // nobody can act on is worse than none.
    expect(isBelowMinimumResolution({ min_px: undefined }, { width: 1, height: 1 })).toBe(false);
    expect(isBelowMinimumResolution({ min_px: 1200 }, null)).toBe(false);
    expect(isBelowMinimumResolution({ min_px: 1200 }, { width: 0, height: 0 })).toBe(false);
  });
});
