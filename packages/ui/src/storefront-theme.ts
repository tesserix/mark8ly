/**
 * Storefront theme — shared source of truth for the merchant-
 * customizable storefront theme system.
 *
 * Consumed by:
 *   - apps/admin (via StorefrontThemeForm — the editor UI)
 *   - apps/storefront (reads the merchant's theme and renders it)
 *
 * Single-source here so drift between the editor and the renderer
 * is impossible. Adding a layout, preset, or font is a one-file
 * change that both apps pick up on the next build.
 *
 * Design contract:
 *   - `paper` is the canonical Mark8ly default preset and maps to
 *     the paper/ink/moss tokens used by the marketing + admin
 *     surfaces. Merchants get the house look unless they opt out.
 *   - Legacy presets (warm, sand, forest, midnight) remain so that
 *     existing tenant theme records keep rendering — merchants can
 *     migrate at their own pace.
 *   - Colors are hex strings so they survive JSON storage in the
 *     platform-api tenant record.
 */

/* ============================================================
   Type surface
   ============================================================ */

export type StorefrontLayout =
  | "editorial"
  | "classic-shop"
  | "split-hero"
  | "catalog-first"
  | "story-led"
  | "minimal"
  | "bold-promo"
  | "compact";

export type StorefrontPreset =
  | "paper"
  | "linen"
  | "dune"
  | "oat"
  | "clove"
  | "jade"
  | "slate"
  | "indigo"
  | "bloom"
  | "rose"
  | "char"
  // Legacy preset keys — kept so existing tenant records still validate.
  // Normalisation re-maps these to the closest modern palette below.
  | "warm"
  | "sand"
  | "forest"
  | "midnight";

export type StorefrontFont =
  | "source"
  | "inter"
  | "manrope"
  | "newsreader"
  | "space";

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

/* ============================================================
   Preset palettes
   ============================================================ */

const presetPalette: Record<StorefrontPreset, StorefrontTheme["colors"]> = {
  // Mark8ly house preset — paper · ink · moss. Default for new stores.
  paper: {
    primary: "#0E0E0C",
    accent: "#2D4A2B",
    background: "#F7F6F2",
    surface: "#FFFFFF",
    text: "#0E0E0C",
  },
  // Softer cream — premium linen/bakery feel.
  linen: {
    primary: "#201C17",
    accent: "#6B4F2A",
    background: "#F4EFE6",
    surface: "#FBF8F1",
    text: "#201C17",
  },
  // Warm desert earthy — terracotta accent.
  dune: {
    primary: "#2A2520",
    accent: "#9A5A32",
    background: "#EFE6D7",
    surface: "#FAF4E8",
    text: "#2A2520",
  },
  // Warm beige + umber — practical warm palette.
  oat: {
    primary: "#2A2317",
    accent: "#6B5436",
    background: "#F0E8D8",
    surface: "#FBF6EB",
    text: "#2A2317",
  },
  // Muted olive — softer forest, editorial.
  clove: {
    primary: "#1C1E1B",
    accent: "#4A5D3F",
    background: "#EFEDE6",
    surface: "#FAF9F4",
    text: "#1C1E1B",
  },
  // Pale mint + deep jade accent.
  jade: {
    primary: "#14201A",
    accent: "#2B5F4D",
    background: "#ECF0ED",
    surface: "#F8FBF9",
    text: "#14201A",
  },
  // Cool neutral — taupe accent.
  slate: {
    primary: "#1A1D21",
    accent: "#5A4A3E",
    background: "#EEEFF1",
    surface: "#FAFBFC",
    text: "#1A1D21",
  },
  // Cool light + deep navy accent — modern editorial.
  indigo: {
    primary: "#1A1D28",
    accent: "#384C6B",
    background: "#ECEFF4",
    surface: "#F8FAFD",
    text: "#1A1D28",
  },
  // Muted blush + burgundy — feminine-editorial.
  bloom: {
    primary: "#2A1E1C",
    accent: "#8C3E45",
    background: "#F5EDEA",
    surface: "#FCF7F5",
    text: "#2A1E1C",
  },
  // Blushed earthy + muted rose.
  rose: {
    primary: "#221917",
    accent: "#A04A4C",
    background: "#F2E9E4",
    surface: "#FBF5F1",
    text: "#221917",
  },
  // Quietest palette — soft off-white, near-black ink, charcoal accent.
  char: {
    primary: "#0C0C0A",
    accent: "#2A2A26",
    background: "#F2F1EE",
    surface: "#FAFAF8",
    text: "#0C0C0A",
  },
  // ── Legacy palettes — merchants on these keys still render, but the
  // picker hides them. Normalisation leaves stored `colors` in place;
  // re-picking a preset is what moves them onto a modern palette.
  warm: {
    primary: "#2A2317",
    accent: "#6B5436",
    background: "#F0E8D8",
    surface: "#FBF6EB",
    text: "#2A2317",
  }, // → oat
  sand: {
    primary: "#201C17",
    accent: "#6B4F2A",
    background: "#F4EFE6",
    surface: "#FBF8F1",
    text: "#201C17",
  }, // → linen
  forest: {
    primary: "#1C1E1B",
    accent: "#4A5D3F",
    background: "#EFEDE6",
    surface: "#FAF9F4",
    text: "#1C1E1B",
  }, // → clove
  midnight: {
    primary: "#1A1D28",
    accent: "#384C6B",
    background: "#ECEFF4",
    surface: "#F8FAFD",
    text: "#1A1D28",
  }, // → indigo
};

/* ============================================================
   Option lists for the admin editor UI
   ============================================================ */

export const storefrontLayoutOptions: Array<{
  value: StorefrontLayout;
  label: string;
  description: string;
}> = [
  {
    value: "editorial",
    label: "Editorial",
    description: "Story-led hero with premium pacing.",
  },
  {
    value: "classic-shop",
    label: "Classic Shop",
    description: "Balanced retail landing with trust cues.",
  },
  {
    value: "split-hero",
    label: "Split Hero",
    description: "Left-right composition with stronger action.",
  },
  {
    value: "catalog-first",
    label: "Catalog First",
    description: "Product-led opening with quick highlights.",
  },
  {
    value: "story-led",
    label: "Story-led",
    description: "Narrative presentation with softer hierarchy.",
  },
  {
    value: "minimal",
    label: "Minimal",
    description: "Quiet storefront with lots of breathing room.",
  },
  {
    value: "bold-promo",
    label: "Bold Promo",
    description: "Campaign-forward layout with stronger contrast.",
  },
  {
    value: "compact",
    label: "Compact",
    description: "Dense storefront for practical browsing.",
  },
];

// Presets shown in the admin picker. Legacy keys (warm/sand/forest/
// midnight) remain accepted by the type guard so historical tenant
// records still validate, but they are intentionally absent from this
// list — the picker only surfaces modern palettes.
export const storefrontPresetOptions: Array<{
  value: StorefrontPreset;
  label: string;
  description: string;
}> = [
  {
    value: "paper",
    label: "Paper",
    description: "The Mark8ly house preset — warm paper, near-black ink, moss accent.",
  },
  { value: "linen", label: "Linen", description: "Soft cream with espresso accent." },
  { value: "dune",  label: "Dune",  description: "Warm desert with terracotta accent." },
  { value: "oat",   label: "Oat",   description: "Warm beige with umber accent." },
  { value: "clove", label: "Clove", description: "Muted olive with grey-green base." },
  { value: "jade",  label: "Jade",  description: "Pale mint with deep jade accent." },
  { value: "slate", label: "Slate", description: "Cool neutral with taupe accent." },
  { value: "indigo", label: "Indigo", description: "Cool light with deep navy accent." },
  { value: "bloom", label: "Bloom", description: "Muted blush with burgundy accent." },
  { value: "rose",  label: "Rose",  description: "Blushed neutral with muted rose accent." },
  { value: "char",  label: "Char",  description: "Quietest palette — near-monochrome editorial." },
];

export const storefrontFontOptions: Array<{
  value: StorefrontFont;
  label: string;
  description: string;
}> = [
  {
    value: "source",
    label: "Source Sans 3",
    description: "House sans. Neutral, workhorse.",
  },
  {
    value: "inter",
    label: "Inter",
    description: "Geometric modern sans.",
  },
  {
    value: "manrope",
    label: "Manrope",
    description: "Friendly rounded sans.",
  },
  {
    value: "newsreader",
    label: "Newsreader",
    description: "Editorial serif for display.",
  },
  {
    value: "space",
    label: "Space Grotesk",
    description: "Technical grotesk.",
  },
];

export const fontStacks: Record<StorefrontFont, string> = {
  source: '"Source Sans 3", "Inter", "Segoe UI", sans-serif',
  inter: '"Inter", "Segoe UI", sans-serif',
  manrope: '"Manrope", "Avenir Next", sans-serif',
  newsreader:
    '"Newsreader", "Iowan Old Style", "Times New Roman", serif',
  space: '"Space Grotesk", "Inter", sans-serif',
};

/* ============================================================
   Defaults + normalization
   ============================================================ */

export const defaultStorefrontTheme: StorefrontTheme = {
  layout: "editorial",
  preset: "paper",
  colors: { ...presetPalette.paper },
  typography: {
    headingFont: "newsreader",
    bodyFont: "source",
  },
  motion: "subtle",
  density: "balanced",
  radius: "soft",
};

/**
 * normalizeStorefrontTheme — safe constructor for theme values
 * coming in from untrusted sources (platform-api JSON, form
 * submissions, legacy records). Any field that fails validation
 * falls back to the preset's default.
 */
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
    density: isDensity(raw.density)
      ? raw.density
      : defaultStorefrontTheme.density,
    radius: isRadius(raw.radius) ? raw.radius : defaultStorefrontTheme.radius,
  };
}

/**
 * withPresetColors — swap to a preset's canonical colors while
 * preserving the rest of the theme (layout, typography, motion,
 * density, radius). Used by the admin editor when a merchant
 * clicks a preset swatch.
 */
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

/* ============================================================
   Style helpers — translate a theme into computed values the
   storefront can consume in its layout and CSS.
   ============================================================ */

export function themeRadius(theme: StorefrontTheme): string {
  if (theme.radius === "sharp") return "1.1rem";
  if (theme.radius === "rounded") return "2rem";
  return "1.5rem";
}

export function themeDensityScale(theme: StorefrontTheme): number {
  if (theme.density === "compact") return 0.88;
  if (theme.density === "airy") return 1.12;
  return 1;
}

/**
 * themeSpacing — legacy string form of themeDensityScale, kept
 * for existing storefront render code that passes the scale
 * through CSS as a unitless multiplier. New code should prefer
 * themeDensityScale which returns a proper number.
 */
export function themeSpacing(theme: StorefrontTheme): string {
  return String(themeDensityScale(theme));
}

/**
 * themeCssVariables — returns a Record suitable for spreading
 * into a JSX `style` prop or stringifying into a <style> block.
 * Every storefront layout should consume these vars so the
 * merchant's choices cascade through the whole render.
 */
export function themeCssVariables(
  theme: StorefrontTheme,
): Record<string, string> {
  return {
    "--storefront-primary": theme.colors.primary,
    "--storefront-accent": theme.colors.accent,
    "--storefront-background": theme.colors.background,
    "--storefront-surface": theme.colors.surface,
    "--storefront-text": theme.colors.text,
    "--storefront-heading-font": fontStacks[theme.typography.headingFont],
    "--storefront-body-font": fontStacks[theme.typography.bodyFont],
    "--storefront-radius": themeRadius(theme),
    "--storefront-density-scale": String(themeDensityScale(theme)),
  };
}

/* ============================================================
   Type guards
   ============================================================ */

function sanitizeHex(input: string | undefined, fallback: string): string {
  return /^#[0-9a-f]{6}$/i.test(input ?? "") ? (input as string) : fallback;
}

function isLayout(value: unknown): value is StorefrontLayout {
  return storefrontLayoutOptions.some((option) => option.value === value);
}

function isPreset(value: unknown): value is StorefrontPreset {
  // Validate against the palette map rather than the picker options —
  // the picker hides legacy keys (warm/sand/forest/midnight) but those
  // records must still normalise to themselves, not fall back to paper.
  return typeof value === "string" && value in presetPalette;
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
