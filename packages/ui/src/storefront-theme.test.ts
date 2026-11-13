import { describe, it, expect } from "vitest";

import {
  defaultStorefrontTheme,
  themeCssVariables,
  withBackground,
  withMode,
} from "./storefront-theme";

// Hero CTAs render `bg-foreground text-background`, so the two tokens must
// flip together. Only --foreground flipping drew light text on a light
// button in dark stores (sugarcoat, 13 Nov).
describe("themeCssVariables", () => {
  it("flips --background with --foreground in dark mode", () => {
    const dark = withBackground(withMode(defaultStorefrontTheme, "dark"), "obsidian");
    const vars = themeCssVariables(dark);
    expect(vars["--background"]).toBe(dark.colors.background);
    expect(vars["--foreground"]).toBe(dark.colors.text);
    expect(vars["--background"]).not.toBe(vars["--foreground"]);
  });

  it("keeps the light pair in light mode", () => {
    const vars = themeCssVariables(defaultStorefrontTheme);
    expect(vars["--background"]).toBe(defaultStorefrontTheme.colors.background);
    expect(vars["--foreground"]).toBe(defaultStorefrontTheme.colors.text);
  });
});
