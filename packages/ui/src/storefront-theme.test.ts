import { describe, it, expect } from "vitest";

import {
  defaultStorefrontTheme,
  normalizeStorefrontTheme,
  themeCssVariables,
  withBackground,
  withColorOverride,
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

it("puts inputs on the store's surface, not the house white, in dark mode", () => {
  const dark = withBackground(withMode(defaultStorefrontTheme, "dark"), "obsidian");
  expect(themeCssVariables(dark)["--background-elevated"]).toBe(dark.colors.surface);
  expect(themeCssVariables(defaultStorefrontTheme)["--background-elevated"]).toBe("#FFFFFF");
});

// A custom background used a fixed #1A1A1A surface, so cards drew darker
// than any custom bg above it (sugarcoat #16171B, 17 Nov).
describe("custom background surface", () => {
  const darkCustom = (hex: string) =>
    withColorOverride(withMode(defaultStorefrontTheme, "dark"), "background", hex);

  it("lifts the surface from the custom bg, keeping its tint", () => {
    expect(darkCustom("#16171B").colors.surface).toBe("#242529");
    expect(darkCustom("#000000").colors.surface).toBe("#0F0F0F");
  });

  it("keeps white cards in light mode", () => {
    const light = withColorOverride(defaultStorefrontTheme, "background", "#EEE8DD");
    expect(light.colors.surface).toBe("#FFFFFF");
  });

  it("re-derives a stale stored surface on load", () => {
    const stored = { ...darkCustom("#16171B") };
    stored.colors = { ...stored.colors, surface: "#1A1A1A" };
    expect(normalizeStorefrontTheme(stored).colors.surface).toBe("#242529");
  });

  it("still honours a stored surface on named backgrounds", () => {
    const stored = { ...withBackground(withMode(defaultStorefrontTheme, "dark"), "obsidian") };
    stored.colors = { ...stored.colors, surface: "#202020" };
    expect(normalizeStorefrontTheme(stored).colors.surface).toBe("#202020");
  });
});
