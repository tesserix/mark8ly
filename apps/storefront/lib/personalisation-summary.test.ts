import { describe, it, expect } from "vitest";

import { answerText, hasExpiredUpload } from "./personalisation-summary";
import { emptyPreviewLookup, type PreviewLookup } from "@/lib/personalisation-previews";

// The storefront's vitest runs in `node`, so these cover the decision
// logic rather than the markup: what the buyer READS for each kind of
// answer, and whether a line counts as blocked.

function lookup(unavailable: string[]): PreviewLookup {
  return { previews: {}, unavailable: new Set(unavailable) };
}

describe("answerText", () => {
  it("shows text verbatim", () => {
    expect(answerText({ fieldId: "f", text: "Asha" })).toBe("Asha");
  });

  it("treats whitespace-only text as no answer", () => {
    expect(answerText({ fieldId: "f", text: "   " })).toBeNull();
  });

  it("prefers the option label over the id", () => {
    expect(
      answerText({ fieldId: "f", optionId: "o1", optionLabel: "Walnut" }),
    ).toBe("Walnut");
  });

  it("says nothing rather than printing a bare option uuid", () => {
    // Carts stored before #966 have no optionLabel. A uuid on a cart
    // line is worse than an unlabelled row.
    expect(
      answerText({ fieldId: "f", optionId: "3f1a-not-a-label" }),
    ).toBeNull();
  });

  it("renders a ticked box as Yes", () => {
    expect(answerText({ fieldId: "f", checked: true })).toBe("Yes");
    expect(answerText({ fieldId: "f", checked: false })).toBe("No");
  });

  it("has no text for an image answer — the thumbnail is the answer", () => {
    expect(answerText({ fieldId: "f", uploadId: "u1" })).toBeNull();
  });
});

describe("hasExpiredUpload", () => {
  it("is false when there is no personalisation at all", () => {
    expect(hasExpiredUpload(undefined, emptyPreviewLookup)).toBe(false);
    expect(hasExpiredUpload([], emptyPreviewLookup)).toBe(false);
  });

  it("is false while previews are still loading", () => {
    // Nothing is known yet. Claiming expiry here would flash
    // "your photo has expired" on every cart render.
    expect(
      hasExpiredUpload([{ fieldId: "f", uploadId: "u1" }], emptyPreviewLookup),
    ).toBe(false);
  });

  it("is true only for an id the server reported unavailable", () => {
    const l = lookup(["u2"]);
    expect(hasExpiredUpload([{ fieldId: "f", uploadId: "u1" }], l)).toBe(false);
    expect(hasExpiredUpload([{ fieldId: "f", uploadId: "u2" }], l)).toBe(true);
  });

  it("blocks a line when any one of its answers expired", () => {
    expect(
      hasExpiredUpload(
        [
          { fieldId: "a", uploadId: "ok" },
          { fieldId: "b", uploadId: "gone" },
        ],
        lookup(["gone"]),
      ),
    ).toBe(true);
  });

  it("ignores non-image answers", () => {
    expect(hasExpiredUpload([{ fieldId: "f", text: "Asha" }], lookup(["x"]))).toBe(false);
  });
});
