export type StorefrontLayout =
  | "editorial"
  | "classic-shop"
  | "split-hero"
  | "catalog-first"
  | "story-led"
  | "minimal"
  | "bold-promo"
  | "compact";

export type StorefrontPreset = "warm" | "sand" | "forest" | "midnight";
export type StorefrontFont = "source" | "inter" | "manrope" | "newsreader" | "space";
export type StorefrontMotion = "none" | "subtle" | "expressive";
export type StorefrontDensity = "compact" | "balanced" | "airy";
export type StorefrontRadius = "sharp" | "soft" | "rounded";

export interface StorefrontTheme {
  layout: StorefrontLayout;
  preset: StorefrontPreset;
  colors: {
    primary: string;
    accent: string;
    background: string;
    surface: string;
    text: string;
  };
  typography: {
    headingFont: StorefrontFont;
    bodyFont: StorefrontFont;
  };
  motion: StorefrontMotion;
  density: StorefrontDensity;
  radius: StorefrontRadius;
}

const presetPalette: Record<
  StorefrontPreset,
  StorefrontTheme["colors"]
> = {
  warm: {
    primary: "#8a6440",
    accent: "#e67a2f",
    background: "#fffaf3",
    surface: "#fffdf9",
    text: "#1f1e1c",
  },
  sand: {
    primary: "#7c6a4d",
    accent: "#bf8f56",
    background: "#f6f0e6",
    surface: "#fffaf4",
    text: "#211b16",
  },
  forest: {
    primary: "#31584a",
    accent: "#7ca067",
    background: "#f4f8f2",
    surface: "#fbfdf9",
    text: "#17211c",
  },
  midnight: {
    primary: "#28334f",
    accent: "#b5854d",
    background: "#eef2fb",
    surface: "#f9fbff",
    text: "#182033",
  },
};

export const storefrontLayoutOptions: Array<{
  value: StorefrontLayout;
  label: string;
  description: string;
}> = [
  { value: "editorial", label: "Editorial", description: "Story-led hero with premium pacing." },
  { value: "classic-shop", label: "Classic Shop", description: "Balanced retail landing with trust cues." },
  { value: "split-hero", label: "Split Hero", description: "Left-right composition with stronger action." },
  { value: "catalog-first", label: "Catalog First", description: "Product-led opening with quick highlights." },
  { value: "story-led", label: "Story-led", description: "Narrative presentation with softer hierarchy." },
  { value: "minimal", label: "Minimal", description: "Quiet storefront with lots of breathing room." },
  { value: "bold-promo", label: "Bold Promo", description: "Campaign-forward layout with stronger contrast." },
  { value: "compact", label: "Compact", description: "Dense storefront for practical browsing." },
];

export const storefrontPresetOptions: Array<{
  value: StorefrontPreset;
  label: string;
}> = [
  { value: "warm", label: "Warm" },
  { value: "sand", label: "Sand" },
  { value: "forest", label: "Forest" },
  { value: "midnight", label: "Midnight" },
];

export const storefrontFontOptions: Array<{ value: StorefrontFont; label: string }> = [
  { value: "source", label: "Source" },
  { value: "inter", label: "Inter" },
  { value: "manrope", label: "Manrope" },
  { value: "newsreader", label: "Newsreader" },
  { value: "space", label: "Space Grotesk" },
];

export const defaultStorefrontTheme: StorefrontTheme = {
  layout: "editorial",
  preset: "warm",
  colors: { ...presetPalette.warm },
  typography: {
    headingFont: "newsreader",
    bodyFont: "source",
  },
  motion: "subtle",
  density: "balanced",
  radius: "soft",
};

export function normalizeStorefrontTheme(
  value: unknown,
): StorefrontTheme {
  const raw = (value ?? {}) as Partial<StorefrontTheme>;
  const preset = isPreset(raw.preset) ? raw.preset : defaultStorefrontTheme.preset;
  const palette = presetPalette[preset];

  return {
    layout: isLayout(raw.layout) ? raw.layout : defaultStorefrontTheme.layout,
    preset,
    colors: {
      primary: sanitizeHex(raw.colors?.primary, palette.primary),
      accent: sanitizeHex(raw.colors?.accent, palette.accent),
      background: sanitizeHex(raw.colors?.background, palette.background),
      surface: sanitizeHex(raw.colors?.surface, palette.surface),
      text: sanitizeHex(raw.colors?.text, palette.text),
    },
    typography: {
      headingFont: isFont(raw.typography?.headingFont)
        ? raw.typography.headingFont
        : defaultStorefrontTheme.typography.headingFont,
      bodyFont: isFont(raw.typography?.bodyFont)
        ? raw.typography.bodyFont
        : defaultStorefrontTheme.typography.bodyFont,
    },
    motion: isMotion(raw.motion) ? raw.motion : defaultStorefrontTheme.motion,
    density: isDensity(raw.density) ? raw.density : defaultStorefrontTheme.density,
    radius: isRadius(raw.radius) ? raw.radius : defaultStorefrontTheme.radius,
  };
}

export function withPresetColors(
  theme: StorefrontTheme,
  preset: StorefrontPreset,
): StorefrontTheme {
  return {
    ...theme,
    preset,
    colors: { ...presetPalette[preset] },
  };
}

function sanitizeHex(input: string | undefined, fallback: string): string {
  return /^#[0-9a-f]{6}$/i.test(input ?? "") ? (input as string) : fallback;
}

function isLayout(value: unknown): value is StorefrontLayout {
  return storefrontLayoutOptions.some((option) => option.value === value);
}

function isPreset(value: unknown): value is StorefrontPreset {
  return storefrontPresetOptions.some((option) => option.value === value);
}

function isFont(value: unknown): value is StorefrontFont {
  return storefrontFontOptions.some((option) => option.value === value);
}

function isMotion(value: unknown): value is StorefrontMotion {
  return value === "none" || value === "subtle" || value === "expressive";
}

function isDensity(value: unknown): value is StorefrontDensity {
  return value === "compact" || value === "balanced" || value === "airy";
}

function isRadius(value: unknown): value is StorefrontRadius {
  return value === "sharp" || value === "soft" || value === "rounded";
}
