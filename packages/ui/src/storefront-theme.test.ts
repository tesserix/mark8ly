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

  it("mixes dark-mode status surfaces against the dark background", () => {
    const dark = withBackground(withMode(defaultStorefrontTheme, "dark"), "obsidian");
    const vars = themeCssVariables(dark);
    expect(vars["--storefront-success-bg"]).toBe(
      `color-mix(in oklab, ${dark.statusColors.success} 18%, ${dark.colors.background})`,
    );
    expect(vars["--storefront-neutral-bg"]).toContain(dark.colors.background);
    // Chip text must be the light status colour, not the light-mode dark hex.
    expect(vars["--storefront-success-fg"]).toBe(dark.statusColors.success);
    expect(vars["--storefront-neutral-fg"]).toBe(dark.colors.text);
  });

  it("leaves light-mode status surfaces to globals.css", () => {
    expect(themeCssVariables(defaultStorefrontTheme)["--storefront-success-bg"]).toBeUndefined();
    expect(themeCssVariables(defaultStorefrontTheme)["--storefront-success-fg"]).toBeUndefined();
  });
});
