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

export const fontStacks: Record<StorefrontFont, string> = {
  source: '"Source Sans 3", "Inter", "Segoe UI", sans-serif',
  inter: '"Inter", "Segoe UI", sans-serif',
  manrope: '"Manrope", "Avenir Next", sans-serif',
  newsreader: '"Newsreader", "Iowan Old Style", "Times New Roman", serif',
  space: '"Space Grotesk", "Inter", sans-serif',
};

export function normalizeStorefrontTheme(value: unknown): StorefrontTheme {
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

export function themeRadius(theme: StorefrontTheme): string {
  if (theme.radius === "sharp") return "1.1rem";
  if (theme.radius === "rounded") return "2rem";
  return "1.5rem";
}

export function themeSpacing(theme: StorefrontTheme): string {
  if (theme.density === "compact") return "0.88";
  if (theme.density === "airy") return "1.12";
  return "1";
}

function sanitizeHex(input: string | undefined, fallback: string): string {
  return /^#[0-9a-f]{6}$/i.test(input ?? "") ? (input as string) : fallback;
}

function isLayout(value: unknown): value is StorefrontLayout {
  return (
    value === "editorial" ||
    value === "classic-shop" ||
    value === "split-hero" ||
    value === "catalog-first" ||
    value === "story-led" ||
    value === "minimal" ||
    value === "bold-promo" ||
    value === "compact"
  );
}

function isPreset(value: unknown): value is StorefrontPreset {
  return value === "warm" || value === "sand" || value === "forest" || value === "midnight";
}

function isFont(value: unknown): value is StorefrontFont {
  return value === "source" || value === "inter" || value === "manrope" || value === "newsreader" || value === "space";
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
