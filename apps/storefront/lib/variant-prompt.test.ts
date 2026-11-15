import { describe, expect, it } from "vitest";

import { chooseOptionsLabel } from "./variant-prompt";

describe("chooseOptionsLabel", () => {
  it("names the one option the buyer still has to pick", () => {
    expect(chooseOptionsLabel(["Size"])).toBe("Choose your size");
  });

  it("names every option when there are several", () => {
    expect(chooseOptionsLabel(["Colour", "Size"])).toBe("Choose your colour and size");
    expect(chooseOptionsLabel(["Colour", "Size", "Fit"])).toBe("Choose your colour, size and fit");
  });
});
