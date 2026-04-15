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
 *   - `paper` is the Mark8ly house preset (editorial restraint) and
 *     remains the default for new stores. The rest of the presets are
 *     for the merchants' customer-facing storefronts and span a wider
 *     range of moods — warm, cool, bold, pastel, jewel, dark.
 *   - Dark-mode is a merchant-set choice via dark presets. There is no
 *     customer-facing mode toggle. Storefront components should consume
 *     `--storefront-*` CSS variables (not hardcoded light tokens) so
 *     dark presets invert cleanly.
 *   - Legacy preset keys remain accepted by the type guard so existing
 *     tenant records keep rendering — they are hidden from the picker.
 *   - Colors are hex strings so they survive JSON storage.
 */

/* ============================================================
   Type surface
   ============================================================ */

export type StorefrontLayout =
  // Editorial / magazine
  | "editorial"
  | "story-led"
  | "lookbook"
  // Shop / catalog
  | "classic-shop"
  | "catalog-first"
  | "grid-dense"
  | "collection-showcase"
  // Hero-forward
  | "split-hero"
  | "product-spotlight"
  | "bold-promo"
  // Long-form + quiet
  | "landing-story"
  | "minimal"
  | "compact";

export type StorefrontPreset =
  // House
  | "paper"
  // Warm
  | "saffron-dusk"
  | "marigold"
  | "terracotta"
  | "blush-studio"
  | "copper-roast"
  // Cool
  | "arctic"
  | "pacific"
  | "slate-cloud"
  // Bold / pop
  | "citrus-burst"
  | "coral-pop"
  // Natural
  | "greenhouse"
  | "matcha"
  // Pastel
  | "lavender-mist"
  | "sky-linen"
  // Monochrome
  | "noir-pop"
  // Jewel
  | "plum-noir"
  // Heritage
  | "indigo-block"
  // Dark mode
  | "obsidian"
  | "velvet"
  | "ember"
  | "nebula"
  | "aurora-dark"
  // Legacy — kept for backwards compatibility with existing tenant
  // records. Hidden from the picker. Normalise to themselves so a
  // merchant on "midnight" keeps seeing their old palette until they
  // actively pick a new one.
  | "warm"
  | "sand"
  | "forest"
  | "midnight"
  | "linen"
  | "dune"
  | "oat"
  | "clove"
  | "jade"
  | "slate"
  | "indigo"
  | "bloom"
  | "rose"
  | "char";

export type StorefrontFont =
  // Serif — display + body
  | "newsreader"
  | "playfair"
  | "cormorant"
  | "lora"
  | "libre-baskerville"
  | "crimson"
  // Sans — workhorse
  | "source"
  | "inter"
  | "manrope"
  | "dm-sans"
  | "work-sans"
  | "poppins"
  | "ibm-plex-sans"
  // Grotesque / technical
  | "space"
  | "archivo";

export type StorefrontMotion = "none" | "subtle" | "expressive";
export type StorefrontDensity = "compact" | "balanced" | "airy";
export type StorefrontRadius = "sharp" | "soft" | "rounded";
export type StorefrontMode = "light" | "dark";

// Surface controls whether the preset's background wash is used as-is
// ("tinted") or overridden with neutral white ("clean"). Clean mode is the
// modern e-commerce default — the accent and product photography carry
// brand rather than a coloured background. Dark presets ignore this — a
// "clean dark" doesn't exist (the whole point of dark is the coloured bg).
export type StorefrontSurface = "tinted" | "clean";

export interface StorefrontTheme {
  layout: StorefrontLayout;
  preset: StorefrontPreset;
  surface: StorefrontSurface;
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
  // ── House ──────────────────────────────────────────────────
  paper: {
    primary: "#0E0E0C",
    accent: "#2D4A2B",
    background: "#F7F6F2",
    surface: "#FFFFFF",
    text: "#0E0E0C",
  },

  // ── Warm ───────────────────────────────────────────────────
  "saffron-dusk": {
    primary: "#3D2208",
    accent: "#C47C0A",
    background: "#FBF3E2",
    surface: "#FFFDF5",
    text: "#2C1A06",
  },
  marigold: {
    primary: "#3B1E00",
    accent: "#B86800",
    background: "#FFF0D0",
    surface: "#FFFDF5",
    text: "#271600",
  },
  terracotta: {
    primary: "#3C1E0E",
    accent: "#C05A28",
    background: "#FAF0E8",
    surface: "#FFFCF8",
    text: "#2A1208",
  },
  "blush-studio": {
    primary: "#3E1212",
    accent: "#C03858",
    background: "#FDF0F0",
    surface: "#FFFAFA",
    text: "#2A1010",
  },
  "copper-roast": {
    primary: "#2E1600",
    accent: "#9A4A00",
    background: "#F5EDE0",
    surface: "#FBF5EB",
    text: "#1E0E00",
  },

  // ── Cool ───────────────────────────────────────────────────
  arctic: {
    primary: "#0F2D3A",
    accent: "#0D7FA5",
    background: "#EDF4F7",
    surface: "#F8FCFE",
    text: "#0C1E26",
  },
  pacific: {
    primary: "#0D2240",
    accent: "#1A5FA8",
    background: "#EBF0F5",
    surface: "#F6F9FC",
    text: "#0D1B2A",
  },
  "slate-cloud": {
    primary: "#1E2436",
    accent: "#3B5EE8",
    background: "#F0F1F5",
    surface: "#F9FAFC",
    text: "#141820",
  },

  // ── Bold / pop ────────────────────────────────────────────
  "citrus-burst": {
    primary: "#2E2000",
    accent: "#C88000",
    background: "#FFF8D6",
    surface: "#FFFEF0",
    text: "#1E1600",
  },
  "coral-pop": {
    primary: "#3A1008",
    accent: "#D94832",
    background: "#FFF2EE",
    surface: "#FFFCFB",
    text: "#250D08",
  },

  // ── Natural ────────────────────────────────────────────────
  greenhouse: {
    primary: "#103018",
    accent: "#2A7A3C",
    background: "#EDF5EE",
    surface: "#F7FBF7",
    text: "#0C1E10",
  },
  matcha: {
    primary: "#1E2E14",
    accent: "#5A7A28",
    background: "#F2F5EC",
    surface: "#F9FBF6",
    text: "#141A0E",
  },

  // ── Pastel ────────────────────────────────────────────────
  "lavender-mist": {
    primary: "#2A1848",
    accent: "#6C40C8",
    background: "#F3F0FA",
    surface: "#FAF8FE",
    text: "#1A1028",
  },
  "sky-linen": {
    primary: "#122030",
    accent: "#1A78C8",
    background: "#EBF4FC",
    surface: "#F6FBFE",
    text: "#0E1A24",
  },

  // ── Monochrome ────────────────────────────────────────────
  "noir-pop": {
    primary: "#0A0A0A",
    accent: "#D41A1A",
    background: "#EDEDEB",
    surface: "#F8F8F7",
    text: "#080808",
  },

  // ── Jewel ─────────────────────────────────────────────────
  "plum-noir": {
    primary: "#2E0E40",
    accent: "#8B2090",
    background: "#F0E8F5",
    surface: "#FBF6FF",
    text: "#1A0820",
  },

  // ── Heritage ──────────────────────────────────────────────
  "indigo-block": {
    primary: "#12185A",
    accent: "#B07800",
    background: "#EEF0F8",
    surface: "#F7F8FD",
    text: "#0C1030",
  },

  // ── Dark mode ─────────────────────────────────────────────
  // NOTE: dark presets set a dark background. Storefront components
  // must consume --storefront-* tokens (never hardcode --paper-200 /
  // --ink-900) for these to render correctly. Components that still
  // hardcode light tokens will appear broken on these presets.
  obsidian: {
    primary: "#FAFAFA",
    accent: "#F5B800",
    background: "#0A0A0A",
    surface: "#1A1A1A",
    text: "#EDEDED",
  },
  velvet: {
    primary: "#FBF5EA",
    accent: "#D9A652",
    background: "#180B14",
    surface: "#24101C",
    text: "#F0E5D8",
  },
  ember: {
    primary: "#F5EFE3",
    accent: "#E6793E",
    background: "#14100C",
    surface: "#211B14",
    text: "#ECE4D4",
  },
  nebula: {
    primary: "#F8F6FC",
    accent: "#D438C6",
    background: "#0D0A24",
    surface: "#1A1636",
    text: "#E8E6F8",
  },
  "aurora-dark": {
    primary: "#F0FCF5",
    accent: "#36E0A6",
    background: "#081814",
    surface: "#112520",
    text: "#DDEFE5",
  },

  // ── Legacy (hidden from picker, still validated) ──────────
  warm: {
    primary: "#2A2317",
    accent: "#6B5436",
    background: "#F0E8D8",
    surface: "#FBF6EB",
    text: "#2A2317",
  },
  sand: {
    primary: "#201C17",
    accent: "#6B4F2A",
    background: "#F4EFE6",
    surface: "#FBF8F1",
    text: "#201C17",
  },
  forest: {
    primary: "#1C1E1B",
    accent: "#4A5D3F",
    background: "#EFEDE6",
    surface: "#FAF9F4",
    text: "#1C1E1B",
  },
  midnight: {
    primary: "#1A1D28",
    accent: "#384C6B",
    background: "#ECEFF4",
    surface: "#F8FAFD",
    text: "#1A1D28",
  },
  linen: {
    primary: "#201C17",
    accent: "#6B4F2A",
    background: "#F4EFE6",
    surface: "#FBF8F1",
    text: "#201C17",
  },
  dune: {
    primary: "#2A2520",
    accent: "#9A5A32",
    background: "#EFE6D7",
    surface: "#FAF4E8",
    text: "#2A2520",
  },
  oat: {
    primary: "#2A2317",
    accent: "#6B5436",
    background: "#F0E8D8",
    surface: "#FBF6EB",
    text: "#2A2317",
  },
  clove: {
    primary: "#1C1E1B",
    accent: "#4A5D3F",
    background: "#EFEDE6",
    surface: "#FAF9F4",
    text: "#1C1E1B",
  },
  jade: {
    primary: "#14201A",
    accent: "#2B5F4D",
    background: "#ECF0ED",
    surface: "#F8FBF9",
    text: "#14201A",
  },
  slate: {
    primary: "#1A1D21",
    accent: "#5A4A3E",
    background: "#EEEFF1",
    surface: "#FAFBFC",
    text: "#1A1D21",
  },
  indigo: {
    primary: "#1A1D28",
    accent: "#384C6B",
    background: "#ECEFF4",
    surface: "#F8FAFD",
    text: "#1A1D28",
  },
  bloom: {
    primary: "#2A1E1C",
    accent: "#8C3E45",
    background: "#F5EDEA",
    surface: "#FCF7F5",
    text: "#2A1E1C",
  },
  rose: {
    primary: "#221917",
    accent: "#A04A4C",
    background: "#F2E9E4",
    surface: "#FBF5F1",
    text: "#221917",
  },
  char: {
    primary: "#0C0C0A",
    accent: "#2A2A26",
    background: "#F2F1EE",
    surface: "#FAFAF8",
    text: "#0C0C0A",
  },
};

/** Presets that render with a dark background. Used to derive the
 *  `--storefront-mode` CSS var so downstream components can branch on
 *  `body[data-theme-mode="dark"]` if they must.
 */
const darkPresets: ReadonlySet<StorefrontPreset> = new Set([
  "obsidian",
  "velvet",
  "ember",
  "nebula",
  "aurora-dark",
]);

export function presetMode(preset: StorefrontPreset): StorefrontMode {
  return darkPresets.has(preset) ? "dark" : "light";
}

/** Does this preset actually honour the surface toggle? Dark presets
 *  always use their own (dark) background — the toggle is a no-op there.
 */
export function presetSupportsSurfaceToggle(preset: StorefrontPreset): boolean {
  return presetMode(preset) === "light";
}

/** Neutral surface colours used when a preset is rendered in "clean"
 *  surface mode. Chosen to feel modern and let product photography carry
 *  the visual weight. Accent / text / primary still come from the preset.
 */
const cleanSurfaceColors = {
  background: "#FCFCFC",
  surface: "#FFFFFF",
} as const;

/** Merges preset colours with the surface choice. Dark presets and
 *  explicit "tinted" surfaces return the palette as-is. "clean" on a
 *  light preset swaps only background + surface, leaving accent/text/
 *  primary untouched so the preset's identity survives.
 */
function resolveSurfaceColors(
  preset: StorefrontPreset,
  surface: StorefrontSurface,
  palette: StorefrontTheme["colors"],
): StorefrontTheme["colors"] {
  if (surface === "tinted" || presetMode(preset) === "dark") return palette;
  return {
    ...palette,
    background: cleanSurfaceColors.background,
    surface: cleanSurfaceColors.surface,
  };
}

/* ============================================================
   Option lists for the admin editor UI
   ============================================================ */

export const storefrontLayoutOptions: Array<{
  value: StorefrontLayout;
  label: string;
  description: string;
}> = [
  { value: "editorial",          label: "Editorial",          description: "Story-led hero with premium pacing." },
  { value: "story-led",          label: "Story-led",          description: "Narrative presentation with softer hierarchy." },
  { value: "lookbook",           label: "Lookbook",           description: "Big-image tiles, editorial spreads — ideal for fashion." },
  { value: "classic-shop",       label: "Classic Shop",       description: "Balanced retail landing with trust cues." },
  { value: "catalog-first",      label: "Catalog First",      description: "Product-led opening with quick highlights." },
  { value: "grid-dense",         label: "Grid Dense",         description: "Tight product grid — maximum SKUs above the fold." },
  { value: "collection-showcase", label: "Collection Showcase", description: "Categories as featured tiles above the product grid." },
  { value: "split-hero",         label: "Split Hero",         description: "Left-right composition with stronger action." },
  { value: "product-spotlight",  label: "Product Spotlight",  description: "Single hero product above a lean catalog." },
  { value: "bold-promo",         label: "Bold Promo",         description: "Campaign-forward layout with stronger contrast." },
  { value: "landing-story",      label: "Landing Story",      description: "Long-scroll brand narrative with modular sections." },
  { value: "minimal",            label: "Minimal",            description: "Quiet storefront with lots of breathing room." },
  { value: "compact",            label: "Compact",            description: "Dense storefront for practical browsing." },
];

// Presets shown in the admin picker. Legacy keys are accepted by the
// type guard but intentionally absent from this list.
export const storefrontPresetOptions: Array<{
  value: StorefrontPreset;
  label: string;
  description: string;
  mode: StorefrontMode;
}> = [
  // House
  { value: "paper",         label: "Paper",         description: "Mark8ly house — warm paper, near-black ink, moss accent.", mode: "light" },
  // Warm
  { value: "saffron-dusk",  label: "Saffron Dusk",  description: "Warm Indian marketplace — ceramics, artisans, handcrafted goods.", mode: "light" },
  { value: "marigold",      label: "Marigold",      description: "Festive saffron with tobacco ink — Indian DTC and gifting.", mode: "light" },
  { value: "terracotta",    label: "Terracotta",    description: "Mediterranean clay — artisan food, pottery, handmade goods.", mode: "light" },
  { value: "blush-studio",  label: "Blush Studio",  description: "Rose-tinted premium — skincare, beauty, wellness, bridal.", mode: "light" },
  { value: "copper-roast",  label: "Copper Roast",  description: "Amber-brown with espresso depth — coffee roasters, whisky, craft beer.", mode: "light" },
  // Cool
  { value: "arctic",        label: "Arctic",        description: "Glacier wash with deep teal — skincare, homeware, tech brands.", mode: "light" },
  { value: "pacific",       label: "Pacific",       description: "Navy with sea-glass — coffee, spirits, outdoor gear.", mode: "light" },
  { value: "slate-cloud",   label: "Slate Cloud",   description: "Cool grey with electric blue — gadgets, electronics, tools.", mode: "light" },
  // Bold
  { value: "citrus-burst",  label: "Citrus Burst",  description: "Loud lemon with amber pop — snacks, kids, stationery.", mode: "light" },
  { value: "coral-pop",     label: "Coral Pop",     description: "Red-coral punch — Gen Z DTC, lifestyle, streetwear.", mode: "light" },
  // Natural
  { value: "greenhouse",    label: "Greenhouse",    description: "Mint wash with saturated green — plants, garden, outdoor.", mode: "light" },
  { value: "matcha",        label: "Matcha",        description: "Earthy olive-sage — wellness, yoga, supplements.", mode: "light" },
  // Pastel
  { value: "lavender-mist", label: "Lavender Mist", description: "Lilac with jewel-violet accent — baby, maternity, gifting.", mode: "light" },
  { value: "sky-linen",     label: "Sky Linen",     description: "Pale sky with friendly warmth — pets, children's toys, books.", mode: "light" },
  // Monochrome
  { value: "noir-pop",      label: "Noir Pop",      description: "Near-monochrome charcoal with electric red — streetwear, sneakers.", mode: "light" },
  // Jewel
  { value: "plum-noir",     label: "Plum Noir",     description: "Plum wash with amethyst — jewellery, wine, perfume.", mode: "light" },
  // Heritage
  { value: "indigo-block",  label: "Indigo Block",  description: "Indigo with gold — block-print textiles, heritage crafts.", mode: "light" },
  // Dark mode
  { value: "obsidian",      label: "Obsidian",      description: "Near-black with amber — luxury fashion, watches, tech premium.", mode: "dark" },
  { value: "velvet",        label: "Velvet",        description: "Deep plum-black with gold — wine, perfume, fine jewellery.", mode: "dark" },
  { value: "ember",         label: "Ember",         description: "Dark espresso with warm copper — craft coffee, whisky bars.", mode: "dark" },
  { value: "nebula",        label: "Nebula",        description: "Dark violet with electric magenta — gaming, music, creative tech.", mode: "dark" },
  { value: "aurora-dark",   label: "Aurora Dark",   description: "Deep forest-teal with neon mint — outdoor gear, modern streetwear.", mode: "dark" },
];

export const storefrontFontOptions: Array<{
  value: StorefrontFont;
  label: string;
  description: string;
}> = [
  // Serifs
  { value: "newsreader",        label: "Newsreader",        description: "Editorial serif. Strong display, good body." },
  { value: "playfair",          label: "Playfair Display",  description: "High-contrast display serif — premium fashion, beauty." },
  { value: "cormorant",         label: "Cormorant",         description: "Elegant low-contrast serif — luxury editorial." },
  { value: "lora",              label: "Lora",              description: "Warm, readable serif — long-form storytelling." },
  { value: "libre-baskerville", label: "Libre Baskerville", description: "Classic body serif — dependable, literary feel." },
  { value: "crimson",           label: "Crimson Pro",       description: "Old-style serif — book-like, warm." },
  // Sans workhorses
  { value: "source",            label: "Source Sans 3",     description: "House sans. Neutral workhorse." },
  { value: "inter",             label: "Inter",             description: "Geometric modern sans — the safe, clean choice." },
  { value: "manrope",           label: "Manrope",           description: "Friendly rounded sans — approachable DTC." },
  { value: "dm-sans",           label: "DM Sans",           description: "Geometric with warmth — balanced, versatile." },
  { value: "work-sans",         label: "Work Sans",         description: "Functional sans — editorial and retail both." },
  { value: "poppins",           label: "Poppins",           description: "Round geometric sans — lively, popular DTC." },
  { value: "ibm-plex-sans",     label: "IBM Plex Sans",     description: "Technical-editorial hybrid — confident." },
  // Technical grotesques
  { value: "space",             label: "Space Grotesk",     description: "Technical grotesque — tech, gaming, modern." },
  { value: "archivo",           label: "Archivo",           description: "Industrial grotesque — streetwear, bold brands." },
];

export const fontStacks: Record<StorefrontFont, string> = {
  // Serifs
  newsreader:        '"Newsreader", "Iowan Old Style", "Times New Roman", serif',
  playfair:          '"Playfair Display", "Iowan Old Style", Georgia, serif',
  cormorant:         '"Cormorant Garamond", "Iowan Old Style", Georgia, serif',
  lora:              '"Lora", Georgia, "Times New Roman", serif',
  "libre-baskerville": '"Libre Baskerville", Baskerville, Georgia, serif',
  crimson:           '"Crimson Pro", "Crimson Text", Georgia, serif',
  // Sans
  source:            '"Source Sans 3", "Inter", "Segoe UI", sans-serif',
  inter:             '"Inter", "Segoe UI", sans-serif',
  manrope:           '"Manrope", "Avenir Next", sans-serif',
  "dm-sans":         '"DM Sans", "Inter", "Segoe UI", sans-serif',
  "work-sans":       '"Work Sans", "Inter", "Segoe UI", sans-serif',
  poppins:           '"Poppins", "Inter", "Segoe UI", sans-serif',
  "ibm-plex-sans":   '"IBM Plex Sans", "Inter", "Segoe UI", sans-serif',
  space:             '"Space Grotesk", "Inter", sans-serif',
  archivo:           '"Archivo", "Inter", "Segoe UI", sans-serif',
};

/* ============================================================
   Defaults + normalization
   ============================================================ */

export const defaultStorefrontTheme: StorefrontTheme = {
  layout: "editorial",
  preset: "paper",
  // Paper is the house preset — keeps its warm tinted bg as the default.
  // New tenants who pick any other light preset will land on "clean".
  surface: "tinted",
  colors: { ...presetPalette.paper },
  typography: {
    headingFont: "newsreader",
    bodyFont: "source",
  },
  motion: "subtle",
  density: "balanced",
  radius: "soft",
};

/** Default surface mode for a given preset. Paper keeps tinted (house
 *  look). Dark presets keep tinted (the toggle is a no-op there). Every
 *  other light preset defaults to clean so new tenants land on the
 *  modern e-commerce default.
 */
function defaultSurfaceFor(preset: StorefrontPreset): StorefrontSurface {
  if (preset === "paper") return "tinted";
  if (presetMode(preset) === "dark") return "tinted";
  return "clean";
}

export function normalizeStorefrontTheme(value: unknown): StorefrontTheme {
  const raw = (value ?? {}) as Partial<StorefrontTheme>;
  const preset = isPreset(raw.preset) ? raw.preset : defaultStorefrontTheme.preset;
  const palette = presetPalette[preset];
  // Records stored before the surface toggle existed don't have the
  // field — fall back to "tinted" so existing storefronts don't visually
  // change overnight. Merchants who pick a new preset get the clean
  // default via withPresetColors instead.
  const surface: StorefrontSurface = isSurface(raw.surface)
    ? raw.surface
    : "tinted";

  return {
    layout: isLayout(raw.layout) ? raw.layout : defaultStorefrontTheme.layout,
    preset,
    surface,
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

export function withPresetColors(
  theme: StorefrontTheme,
  preset: StorefrontPreset,
): StorefrontTheme {
  return {
    ...theme,
    preset,
    // Switching presets resets surface to that preset's sensible default
    // so a merchant clicking (say) Marigold lands on its modern clean look
    // unless they explicitly toggle back to tinted.
    surface: defaultSurfaceFor(preset),
    colors: { ...presetPalette[preset] },
  };
}

export function withSurface(
  theme: StorefrontTheme,
  surface: StorefrontSurface,
): StorefrontTheme {
  return { ...theme, surface };
}

/* ============================================================
   Style helpers
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

export function themeSpacing(theme: StorefrontTheme): string {
  return String(themeDensityScale(theme));
}

export function themeCssVariables(
  theme: StorefrontTheme,
): Record<string, string> {
  // Apply the surface toggle to background + surface only. Accent, text,
  // and primary always come through as the preset author intended.
  const resolved = resolveSurfaceColors(theme.preset, theme.surface, theme.colors);
  return {
    "--storefront-primary": resolved.primary,
    "--storefront-accent": resolved.accent,
    "--storefront-background": resolved.background,
    "--storefront-surface": resolved.surface,
    "--storefront-text": resolved.text,
    "--storefront-heading-font": fontStacks[theme.typography.headingFont],
    "--storefront-body-font": fontStacks[theme.typography.bodyFont],
    "--storefront-radius": themeRadius(theme),
    "--storefront-density-scale": String(themeDensityScale(theme)),
    "--storefront-mode": presetMode(theme.preset),
    "--storefront-surface-mode": theme.surface,
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
  // Validate against the palette map so legacy keys (hidden from the
  // picker) still normalise to themselves.
  return typeof value === "string" && value in presetPalette;
}

function isFont(value: unknown): value is StorefrontFont {
  return storefrontFontOptions.some((option) => option.value === value);
}

function isSurface(value: unknown): value is StorefrontSurface {
  return value === "tinted" || value === "clean";
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
