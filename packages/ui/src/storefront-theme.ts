/**
 * Storefront theme — shared source of truth for the merchant-
 * customizable storefront theme system.
 *
 * Consumed by:
 *   - apps/admin (via StorefrontThemeForm — the editor UI)
 *   - apps/storefront (reads the merchant's theme and renders it)
 *
 * Design contract (v2 — mode · palette · background, orthogonal):
 *   - `mode`        : "light" | "dark"  — flips bg + text globally
 *   - `palette`     : one of 18 bi-modal brand identities; sets accent
 *                     + primary with mode-aware variants
 *   - `background`  : mode-scoped named background (white / paper /
 *                     obsidian / velvet / ...) OR "custom" with a hex
 *   - Optional per-merchant overrides: `customAccent`, `customPrimary`,
 *     `customBackground`. Cleared when the merchant picks a palette.
 *   - `colors`      : resolved render tokens {primary, accent, background,
 *                     surface, text}. Always derived from the above; kept
 *                     on the interface so downstream consumers (preview,
 *                     themeCssVariables) don't need the resolver.
 *
 * Backwards compatibility:
 *   - Legacy `preset` + `surface` fields on stored records are migrated
 *     by normalize(). Dark presets (obsidian/velvet/ember/nebula/aurora-
 *     dark) become mode=dark + background=<the-dark-bg-key> + palette=
 *     <closest-light-palette>. Light presets map 1:1 to palette keys.
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

export type StorefrontMode = "light" | "dark";

export type StorefrontPalette =
  | "paper"
  // Warm
  | "saffron-dusk"
  | "marigold"
  | "terracotta"
  | "blush-studio"
  | "copper-roast"
  | "tangerine"
  | "raspberry"
  | "mocha"
  // Cool
  | "arctic"
  | "pacific"
  | "slate-cloud"
  | "cerulean"
  // Bold / pop
  | "citrus-burst"
  | "coral-pop"
  | "mustard"
  // Natural
  | "greenhouse"
  | "matcha"
  | "spearmint"
  // Pastel
  | "lavender-mist"
  | "sky-linen"
  // Monochrome
  | "noir-pop"
  | "graphite"
  // Jewel
  | "plum-noir"
  // Heritage
  | "indigo-block"
  | "peacock";

export type StorefrontBackground =
  // Light-mode options
  | "white"
  | "off-white"
  | "paper"
  // Dark-mode options — formerly "dark presets" from v1
  | "obsidian"
  | "velvet"
  | "ember"
  | "nebula"
  | "aurora"
  // User-supplied hex via theme.customBackground
  | "custom";

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

/** Semantic status source colours. One hex per tone — the bg/border
 *  variants are derived in CSS via color-mix. Configurable by the
 *  merchant in admin theme settings so stock badges, order/ticket
 *  status chips, and toast tones all follow the brand. */
export interface StatusColors {
  success: string;
  warning: string;
  danger: string;
  info: string;
  neutral: string;
}

export interface StorefrontTheme {
  layout: StorefrontLayout;
  mode: StorefrontMode;
  palette: StorefrontPalette;
  background: StorefrontBackground;
  // Optional overrides from the Advanced colour pickers. Cleared when
  // the merchant picks a different palette.
  customAccent?: string;
  customPrimary?: string;
  // Used when `background === "custom"`. Ignored otherwise.
  customBackground?: string;
  // Optional per-tone status-colour overrides. Any tone left undefined
  // falls back to the mode-scoped default in defaultStatusColors.
  customStatusColors?: Partial<StatusColors>;
  // Resolved render tokens. Populated by normalize() from the structured
  // fields above. Downstream render consumers should read these.
  colors: {
    primary: string;
    accent: string;
    background: string;
    surface: string;
    text: string;
  };
  // Resolved semantic status colours — derived from defaults + customStatusColors.
  statusColors: StatusColors;
  typography: {
    headingFont: StorefrontFont;
    bodyFont: StorefrontFont;
  };
  motion: StorefrontMotion;
  density: StorefrontDensity;
  radius: StorefrontRadius;
}

/* ============================================================
   Palette definitions
   Every palette has a light and dark variant. The accent is the
   merchant's brand colour — same identity, re-tuned for contrast
   against a dark background.
   ============================================================ */

interface PaletteVariant {
  accent: string;
  primary: string;
}

interface PaletteDef {
  light: PaletteVariant;
  dark: PaletteVariant;
}

const palettes: Record<StorefrontPalette, PaletteDef> = {
  // House — mark8ly's own editorial moss
  paper: {
    light: { accent: "#2D4A2B", primary: "#0E0E0C" },
    dark:  { accent: "#7FB87A", primary: "#FAFAFA" },
  },
  // Warm
  "saffron-dusk": {
    light: { accent: "#C47C0A", primary: "#3D2208" },
    dark:  { accent: "#F5B342", primary: "#FFFCF5" },
  },
  marigold: {
    light: { accent: "#B86800", primary: "#3B1E00" },
    dark:  { accent: "#F5B800", primary: "#FFFCF5" },
  },
  terracotta: {
    light: { accent: "#C05A28", primary: "#3C1E0E" },
    dark:  { accent: "#F08A55", primary: "#FFF6EF" },
  },
  "blush-studio": {
    light: { accent: "#C03858", primary: "#3E1212" },
    dark:  { accent: "#F06890", primary: "#FFF0F0" },
  },
  "copper-roast": {
    light: { accent: "#9A4A00", primary: "#2E1600" },
    dark:  { accent: "#D9A652", primary: "#FBF5EA" },
  },
  // Cool
  arctic: {
    light: { accent: "#0D7FA5", primary: "#0F2D3A" },
    dark:  { accent: "#5AC8EB", primary: "#F0FAFC" },
  },
  pacific: {
    light: { accent: "#1A5FA8", primary: "#0D2240" },
    dark:  { accent: "#6FAEF0", primary: "#F0F6FC" },
  },
  "slate-cloud": {
    light: { accent: "#3B5EE8", primary: "#1E2436" },
    dark:  { accent: "#8FA8FF", primary: "#F4F6FC" },
  },
  // Bold / pop
  "citrus-burst": {
    light: { accent: "#C88000", primary: "#2E2000" },
    dark:  { accent: "#F5C53A", primary: "#FFFCF0" },
  },
  "coral-pop": {
    light: { accent: "#D94832", primary: "#3A1008" },
    dark:  { accent: "#FF7A5E", primary: "#FFF5F2" },
  },
  // Natural
  greenhouse: {
    light: { accent: "#2A7A3C", primary: "#103018" },
    dark:  { accent: "#7FD890", primary: "#F0FBF2" },
  },
  matcha: {
    light: { accent: "#5A7A28", primary: "#1E2E14" },
    dark:  { accent: "#A8C858", primary: "#F8FBEE" },
  },
  // Pastel
  "lavender-mist": {
    light: { accent: "#6C40C8", primary: "#2A1848" },
    dark:  { accent: "#B090FF", primary: "#F8F5FF" },
  },
  "sky-linen": {
    light: { accent: "#1A78C8", primary: "#122030" },
    dark:  { accent: "#6AB8F0", primary: "#F0F8FE" },
  },
  // Monochrome
  "noir-pop": {
    light: { accent: "#D41A1A", primary: "#0A0A0A" },
    dark:  { accent: "#FF4242", primary: "#FAFAFA" },
  },
  // Jewel
  "plum-noir": {
    light: { accent: "#8B2090", primary: "#2E0E40" },
    dark:  { accent: "#D060D8", primary: "#FBF5FE" },
  },
  // Heritage
  "indigo-block": {
    light: { accent: "#B07800", primary: "#12185A" },
    dark:  { accent: "#F5C040", primary: "#F5F7FC" },
  },
  // Warm — additions
  tangerine: {
    light: { accent: "#E8711A", primary: "#2A1405" },
    dark:  { accent: "#FF9A4A", primary: "#FFF5EC" },
  },
  raspberry: {
    light: { accent: "#C1185E", primary: "#32081A" },
    dark:  { accent: "#F25585", primary: "#FFEEF4" },
  },
  mocha: {
    light: { accent: "#6B3A1F", primary: "#1F120A" },
    dark:  { accent: "#C08860", primary: "#F8EDE0" },
  },
  // Cool — additions
  cerulean: {
    light: { accent: "#1C9BD4", primary: "#0C2435" },
    dark:  { accent: "#5FD0FF", primary: "#F0FAFF" },
  },
  // Bold — additions
  mustard: {
    light: { accent: "#B58A1A", primary: "#241A00" },
    dark:  { accent: "#F5C74A", primary: "#FFF9E5" },
  },
  // Natural — additions
  spearmint: {
    light: { accent: "#1DA684", primary: "#0A2A20" },
    dark:  { accent: "#5CE0B0", primary: "#EBFDF6" },
  },
  // Monochrome — additions
  graphite: {
    light: { accent: "#4A4A48", primary: "#0C0C0A" },
    dark:  { accent: "#C8C8C5", primary: "#FAFAFA" },
  },
  // Heritage — additions
  peacock: {
    light: { accent: "#0F5A68", primary: "#071E24" },
    dark:  { accent: "#5EC0D0", primary: "#EEFAFD" },
  },
};

/* ============================================================
   Background definitions
   Each named background has a background + surface pair. "surface"
   is the slightly-lighter-or-darker companion used for cards,
   popovers, and elevated chrome.
   ============================================================ */

interface BackgroundDef {
  mode: StorefrontMode;
  background: string;
  surface: string;
}

const backgrounds: Record<Exclude<StorefrontBackground, "custom">, BackgroundDef> = {
  // Light
  white:       { mode: "light", background: "#FCFCFC", surface: "#FFFFFF" },
  "off-white": { mode: "light", background: "#F7F6F2", surface: "#FFFFFF" },
  paper:       { mode: "light", background: "#F4EFE6", surface: "#FBF8F1" },
  // Dark (the former "dark presets" — now just background choices)
  obsidian:    { mode: "dark",  background: "#0A0A0A", surface: "#1A1A1A" },
  velvet:      { mode: "dark",  background: "#180B14", surface: "#24101C" },
  ember:       { mode: "dark",  background: "#14100C", surface: "#211B14" },
  nebula:      { mode: "dark",  background: "#0D0A24", surface: "#1A1636" },
  aurora:      { mode: "dark",  background: "#081814", surface: "#112520" },
};

/** Light-mode default background per palette — used on first theme
 *  setup and as a fallback when switching modes. */
const defaultLightBackground: StorefrontBackground = "white";

/** Dark-mode default background per palette — thematic pairings. */
const defaultDarkBackground: Record<StorefrontPalette, StorefrontBackground> = {
  paper:          "obsidian",
  "saffron-dusk": "ember",
  marigold:       "ember",
  terracotta:     "ember",
  "blush-studio": "velvet",
  "copper-roast": "ember",
  arctic:         "obsidian",
  pacific:        "nebula",
  "slate-cloud":  "nebula",
  "citrus-burst": "obsidian",
  "coral-pop":    "ember",
  greenhouse:     "aurora",
  matcha:         "aurora",
  "lavender-mist": "nebula",
  "sky-linen":    "nebula",
  "noir-pop":     "obsidian",
  "plum-noir":    "velvet",
  "indigo-block": "nebula",
  tangerine:      "ember",
  raspberry:      "velvet",
  mocha:          "ember",
  cerulean:       "nebula",
  mustard:        "ember",
  spearmint:      "aurora",
  graphite:       "obsidian",
  peacock:        "aurora",
};

/** Returns the sensible default background for (mode, palette). */
export function defaultBackgroundFor(
  mode: StorefrontMode,
  palette: StorefrontPalette,
): StorefrontBackground {
  return mode === "dark" ? defaultDarkBackground[palette] : defaultLightBackground;
}

/* ============================================================
   Semantic status defaults
   ------------------------------------------------------------
   Sensible conventional signals per mode. Light-mode tones are
   saturated for contrast on pale paper; dark-mode tones are
   brightened so they read on near-black surfaces. Merchants
   override any subset via theme.customStatusColors.
   ============================================================ */

const defaultStatusColors: Record<StorefrontMode, StatusColors> = {
  light: {
    success: "#15603D",
    warning: "#B77A00",
    danger: "#C23B22",
    info: "#3457B2",
    neutral: "#5A5A58",
  },
  dark: {
    success: "#7FD494",
    warning: "#F5C74A",
    danger: "#F08670",
    info: "#8FAFF0",
    neutral: "#A8A8A5",
  },
};

/** Returns the default status colours for a given mode. */
export function defaultStatusColorsFor(mode: StorefrontMode): StatusColors {
  return { ...defaultStatusColors[mode] };
}

/** Resolve the final status palette by merging mode defaults with
 *  any merchant overrides. */
export function resolveStatusColors(
  mode: StorefrontMode,
  overrides?: Partial<StatusColors>,
): StatusColors {
  const base = defaultStatusColors[mode];
  return {
    success: overrides?.success ?? base.success,
    warning: overrides?.warning ?? base.warning,
    danger: overrides?.danger ?? base.danger,
    info: overrides?.info ?? base.info,
    neutral: overrides?.neutral ?? base.neutral,
  };
}

/* ============================================================
   Theme resolver — mode × palette × background → colors
   ============================================================ */

interface ResolvedColors {
  primary: string;
  accent: string;
  background: string;
  surface: string;
  text: string;
}

const LIGHT_TEXT = "#EDEDED";
const DARK_TEXT = "#0E0E0C";

/** Derive a surface colour from an arbitrary hex bg. Nudges white-ish
 *  backgrounds lighter and dark-ish ones slightly lighter for contrast. */
function derivedSurface(bgHex: string, mode: StorefrontMode): string {
  // Cheap heuristic — no need to go perceptual. Custom bgs are rare.
  return mode === "dark" ? "#1A1A1A" : "#FFFFFF";
}

export function resolveThemeColors(
  theme: Pick<
    StorefrontTheme,
    "mode" | "palette" | "background" | "customAccent" | "customPrimary" | "customBackground"
  >,
): ResolvedColors {
  const variant = palettes[theme.palette][theme.mode];

  let bg: string;
  let surface: string;
  if (theme.background === "custom") {
    bg = theme.customBackground ?? (theme.mode === "dark" ? "#0A0A0A" : "#FCFCFC");
    surface = derivedSurface(bg, theme.mode);
  } else {
    const def = backgrounds[theme.background];
    // If somehow the saved background is mis-aligned with mode (e.g.
    // mode=light + background=obsidian from a buggy client write), fall
    // back to the default for this mode+palette so render doesn't break.
    if (def.mode !== theme.mode) {
      const fallback = backgrounds[defaultBackgroundFor(theme.mode, theme.palette) as Exclude<StorefrontBackground, "custom">];
      bg = fallback.background;
      surface = fallback.surface;
    } else {
      bg = def.background;
      surface = def.surface;
    }
  }

  return {
    primary: theme.customPrimary ?? variant.primary,
    accent: theme.customAccent ?? variant.accent,
    background: bg,
    surface,
    text: theme.mode === "dark" ? LIGHT_TEXT : DARK_TEXT,
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

export const storefrontPaletteOptions: Array<{
  value: StorefrontPalette;
  label: string;
  description: string;
}> = [
  { value: "paper",         label: "Paper",         description: "Mark8ly house — near-black ink with moss accent." },
  { value: "saffron-dusk",  label: "Saffron Dusk",  description: "Warm amber-gold — ceramics, artisans, handcraft." },
  { value: "marigold",      label: "Marigold",      description: "Festive saffron — Indian DTC, gifting, festive goods." },
  { value: "terracotta",    label: "Terracotta",    description: "Mediterranean clay — pottery, artisan food." },
  { value: "blush-studio",  label: "Blush Studio",  description: "Deep rose — skincare, beauty, bridal." },
  { value: "copper-roast",  label: "Copper Roast",  description: "Espresso amber — coffee, whisky, craft beer." },
  { value: "arctic",        label: "Arctic",        description: "Deep teal — skincare, homeware, modern tech." },
  { value: "pacific",       label: "Pacific",       description: "Navy blue — coffee, spirits, outdoor gear." },
  { value: "slate-cloud",   label: "Slate Cloud",   description: "Electric indigo — gadgets, electronics, tools." },
  { value: "citrus-burst",  label: "Citrus Burst",  description: "Loud amber — snacks, kids, stationery." },
  { value: "coral-pop",     label: "Coral Pop",     description: "Red-coral — Gen Z DTC, lifestyle, streetwear." },
  { value: "greenhouse",    label: "Greenhouse",    description: "Saturated green — plants, garden, outdoor." },
  { value: "matcha",        label: "Matcha",        description: "Olive-sage — wellness, yoga, supplements." },
  { value: "lavender-mist", label: "Lavender Mist", description: "Jewel violet — baby, maternity, gifting." },
  { value: "sky-linen",     label: "Sky Linen",     description: "Cornflower blue — pets, children's toys, books." },
  { value: "noir-pop",      label: "Noir Pop",      description: "Electric red — streetwear, sneakers, limited drops." },
  { value: "plum-noir",     label: "Plum Noir",     description: "Amethyst — jewellery, wine, perfume." },
  { value: "indigo-block",  label: "Indigo Block",  description: "Heritage gold — block-print textiles, handloom." },
  { value: "tangerine",     label: "Tangerine",     description: "Bright orange — fresh, juice, energetic DTC." },
  { value: "raspberry",     label: "Raspberry",     description: "Warm pink-red — cheerful, feminine DTC." },
  { value: "mocha",          label: "Mocha",         description: "Warm dark brown — editorial luxe, leather goods." },
  { value: "cerulean",      label: "Cerulean",      description: "Vivid sky blue — summery, fresh, travel." },
  { value: "mustard",       label: "Mustard",       description: "Muted yellow-brown — editorial fashion." },
  { value: "spearmint",     label: "Spearmint",     description: "Cool mint-teal — oral care, fresh, minty brands." },
  { value: "graphite",      label: "Graphite",      description: "Grey-black — industrial, architectural, tech tools." },
  { value: "peacock",       label: "Peacock",       description: "Deep teal + gold — classic Indian wedding & heritage." },
];

export const storefrontBackgroundOptions: Record<StorefrontMode, Array<{
  value: StorefrontBackground;
  label: string;
  description: string;
}>> = {
  light: [
    { value: "white",     label: "White",     description: "Clean neutral — product photography carries the brand." },
    { value: "off-white", label: "Off-White", description: "Mark8ly paper — warm neutral, gentle on the eye." },
    { value: "paper",     label: "Paper",     description: "Cream tint — editorial, boutique, artisan." },
    { value: "custom",    label: "Custom",    description: "Pick your own hex." },
  ],
  dark: [
    { value: "obsidian",  label: "Obsidian",  description: "Near-black — premium, flexible." },
    { value: "velvet",    label: "Velvet",    description: "Deep plum — wine, perfume, jewellery." },
    { value: "ember",     label: "Ember",     description: "Dark espresso — coffee, whisky, warmth." },
    { value: "nebula",    label: "Nebula",    description: "Violet-black — gaming, music, creative tech." },
    { value: "aurora",    label: "Aurora",    description: "Deep forest-teal — outdoor, modern streetwear." },
    { value: "custom",    label: "Custom",    description: "Pick your own hex." },
  ],
};

export const storefrontFontOptions: Array<{
  value: StorefrontFont;
  label: string;
  description: string;
}> = [
  // Serifs
  { value: "newsreader",        label: "Newsreader",        description: "Editorial serif. Strong display, good body." },
  { value: "playfair",          label: "Playfair Display",  description: "High-contrast display serif — fashion, beauty." },
  { value: "cormorant",         label: "Cormorant",         description: "Elegant low-contrast — luxury editorial." },
  { value: "lora",              label: "Lora",              description: "Warm readable — long-form storytelling." },
  { value: "libre-baskerville", label: "Libre Baskerville", description: "Classic body serif — literary feel." },
  { value: "crimson",           label: "Crimson Pro",       description: "Old-style serif — book-like, warm." },
  // Sans
  { value: "source",            label: "Source Sans 3",     description: "House sans. Neutral workhorse." },
  { value: "inter",             label: "Inter",             description: "Geometric modern — the safe, clean choice." },
  { value: "manrope",           label: "Manrope",           description: "Friendly rounded — approachable DTC." },
  { value: "dm-sans",           label: "DM Sans",           description: "Geometric with warmth — balanced." },
  { value: "work-sans",         label: "Work Sans",         description: "Functional sans — editorial and retail." },
  { value: "poppins",           label: "Poppins",           description: "Round geometric — lively DTC." },
  { value: "ibm-plex-sans",     label: "IBM Plex Sans",     description: "Technical-editorial hybrid." },
  // Grotesques
  { value: "space",             label: "Space Grotesk",     description: "Technical grotesque — tech, gaming." },
  { value: "archivo",           label: "Archivo",           description: "Industrial grotesque — streetwear, bold." },
];

export const fontStacks: Record<StorefrontFont, string> = {
  newsreader:          '"Newsreader", "Iowan Old Style", "Times New Roman", serif',
  playfair:            '"Playfair Display", "Iowan Old Style", Georgia, serif',
  cormorant:           '"Cormorant Garamond", "Iowan Old Style", Georgia, serif',
  lora:                '"Lora", Georgia, "Times New Roman", serif',
  "libre-baskerville": '"Libre Baskerville", Baskerville, Georgia, serif',
  crimson:             '"Crimson Pro", "Crimson Text", Georgia, serif',
  source:              '"Source Sans 3", "Inter", "Segoe UI", sans-serif',
  inter:               '"Inter", "Segoe UI", sans-serif',
  manrope:             '"Manrope", "Avenir Next", sans-serif',
  "dm-sans":           '"DM Sans", "Inter", "Segoe UI", sans-serif',
  "work-sans":         '"Work Sans", "Inter", "Segoe UI", sans-serif',
  poppins:             '"Poppins", "Inter", "Segoe UI", sans-serif',
  "ibm-plex-sans":     '"IBM Plex Sans", "Inter", "Segoe UI", sans-serif',
  space:               '"Space Grotesk", "Inter", sans-serif',
  archivo:             '"Archivo", "Inter", "Segoe UI", sans-serif',
};

/* ============================================================
   Defaults + normalization
   ============================================================ */

export const defaultStorefrontTheme: StorefrontTheme = {
  layout: "editorial",
  mode: "light",
  palette: "paper",
  background: "off-white",
  colors: {
    primary: "#0E0E0C",
    accent: "#2D4A2B",
    background: "#F7F6F2",
    surface: "#FFFFFF",
    text: "#0E0E0C",
  },
  statusColors: defaultStatusColors.light,
  typography: {
    headingFont: "newsreader",
    bodyFont: "source",
  },
  motion: "subtle",
  density: "balanced",
  radius: "soft",
};

/** v1 → v2 preset migration. Former "dark presets" become mode+bg
 *  pairs; light presets 1:1 map to palette keys. */
const legacyPresetMigration: Record<
  string,
  { mode: StorefrontMode; palette: StorefrontPalette; background: StorefrontBackground }
> = {
  // v1 dark "presets" → mode=dark + corresponding background
  obsidian:      { mode: "dark", palette: "paper",        background: "obsidian" },
  velvet:        { mode: "dark", palette: "blush-studio", background: "velvet"   },
  ember:         { mode: "dark", palette: "copper-roast", background: "ember"    },
  nebula:        { mode: "dark", palette: "plum-noir",    background: "nebula"   },
  "aurora-dark": { mode: "dark", palette: "greenhouse",   background: "aurora"   },
  // v1 legacy light presets from even earlier (linen/dune/oat/clove/etc)
  linen:  { mode: "light", palette: "paper",        background: "paper"    },
  dune:   { mode: "light", palette: "terracotta",   background: "paper"    },
  oat:    { mode: "light", palette: "copper-roast", background: "off-white" },
  clove:  { mode: "light", palette: "matcha",       background: "off-white" },
  jade:   { mode: "light", palette: "greenhouse",   background: "off-white" },
  slate:  { mode: "light", palette: "slate-cloud",  background: "off-white" },
  indigo: { mode: "light", palette: "indigo-block", background: "off-white" },
  bloom:  { mode: "light", palette: "blush-studio", background: "off-white" },
  rose:   { mode: "light", palette: "blush-studio", background: "off-white" },
  char:   { mode: "light", palette: "noir-pop",     background: "off-white" },
  warm:   { mode: "light", palette: "copper-roast", background: "paper"    },
  sand:   { mode: "light", palette: "saffron-dusk", background: "paper"    },
  forest: { mode: "light", palette: "matcha",       background: "off-white" },
  midnight: { mode: "dark", palette: "slate-cloud", background: "nebula"   },
};

function isMode(v: unknown): v is StorefrontMode {
  return v === "light" || v === "dark";
}

function isPalette(v: unknown): v is StorefrontPalette {
  return typeof v === "string" && v in palettes;
}

function isBackground(v: unknown): v is StorefrontBackground {
  return (
    v === "custom" ||
    (typeof v === "string" && v in backgrounds)
  );
}

function isLayout(value: unknown): value is StorefrontLayout {
  return storefrontLayoutOptions.some((option) => option.value === value);
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

function sanitizeHex(input: unknown, fallback: string): string {
  return typeof input === "string" && /^#[0-9a-f]{6}$/i.test(input) ? input : fallback;
}

function optionalHex(input: unknown): string | undefined {
  return typeof input === "string" && /^#[0-9a-f]{6}$/i.test(input) ? input : undefined;
}

/** Safe constructor for theme values coming from untrusted sources
 *  (platform-api JSON, form submissions, legacy records). */
export function normalizeStorefrontTheme(value: unknown): StorefrontTheme {
  const raw = (value ?? {}) as Record<string, unknown> & Partial<StorefrontTheme>;

  // ── Migration path: legacy `preset` field (v1) ────────────
  let mode: StorefrontMode = defaultStorefrontTheme.mode;
  let palette: StorefrontPalette = defaultStorefrontTheme.palette;
  let background: StorefrontBackground = defaultStorefrontTheme.background;

  if (isMode(raw.mode) && isPalette(raw.palette) && isBackground(raw.background)) {
    // v2 record — use as-is.
    mode = raw.mode;
    palette = raw.palette;
    background = raw.background;
  } else if (typeof raw.preset === "string" && raw.preset in legacyPresetMigration) {
    // v1 record — migrate.
    const mig = legacyPresetMigration[raw.preset];
    if (mig) {
      mode = mig.mode;
      palette = mig.palette;
      background = mig.background;
    }
  } else if (typeof raw.preset === "string" && isPalette(raw.preset)) {
    // v1 record on a preset key that's still a valid palette.
    palette = raw.preset;
    mode = "light";
    background = defaultBackgroundFor("light", palette);
  }

  // Ensure background is compatible with the chosen mode. If someone
  // saved a mode+background pair that doesn't match, snap to the default.
  if (background !== "custom" && backgrounds[background].mode !== mode) {
    background = defaultBackgroundFor(mode, palette);
  }

  const customAccent = optionalHex(raw.customAccent);
  const customPrimary = optionalHex(raw.customPrimary);
  const customBackground = optionalHex(raw.customBackground);

  const colors = resolveThemeColors({
    mode,
    palette,
    background,
    customAccent,
    customPrimary,
    customBackground,
  });

  // ── Status colour overrides ───────────────────────────────
  // Accept individual tone overrides from untrusted input; keep
  // only valid hex values, drop the rest.
  const rawStatus = (raw.customStatusColors ?? {}) as Record<string, unknown>;
  const customStatusColors: Partial<StatusColors> = {};
  (["success", "warning", "danger", "info", "neutral"] as const).forEach((key) => {
    const hex = optionalHex(rawStatus[key]);
    if (hex) customStatusColors[key] = hex;
  });
  const statusColors = resolveStatusColors(
    mode,
    Object.keys(customStatusColors).length > 0 ? customStatusColors : undefined,
  );

  return {
    layout: isLayout(raw.layout) ? raw.layout : defaultStorefrontTheme.layout,
    mode,
    palette,
    background,
    customAccent,
    customPrimary,
    customBackground: background === "custom" ? customBackground : undefined,
    customStatusColors:
      Object.keys(customStatusColors).length > 0 ? customStatusColors : undefined,
    colors: {
      // Still accept hard-overrides from legacy records via top-level `colors`.
      primary: sanitizeHex((raw.colors as Record<string, unknown> | undefined)?.primary, colors.primary),
      accent: sanitizeHex((raw.colors as Record<string, unknown> | undefined)?.accent, colors.accent),
      background: sanitizeHex((raw.colors as Record<string, unknown> | undefined)?.background, colors.background),
      surface: sanitizeHex((raw.colors as Record<string, unknown> | undefined)?.surface, colors.surface),
      text: sanitizeHex((raw.colors as Record<string, unknown> | undefined)?.text, colors.text),
    },
    statusColors,
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

/** Apply a single status-tone colour override. Passing null clears
 *  the override so the tone reverts to its mode-scoped default. */
export function withStatusColorOverride(
  theme: StorefrontTheme,
  tone: keyof StatusColors,
  value: string | null,
): StorefrontTheme {
  const nextOverrides: Partial<StatusColors> = { ...(theme.customStatusColors ?? {}) };
  if (value === null || value === "") {
    delete nextOverrides[tone];
  } else {
    nextOverrides[tone] = value;
  }
  const hasOverrides = Object.keys(nextOverrides).length > 0;
  return {
    ...theme,
    customStatusColors: hasOverrides ? nextOverrides : undefined,
    statusColors: resolveStatusColors(theme.mode, hasOverrides ? nextOverrides : undefined),
  };
}

/** Apply a new palette to a theme. Clears custom overrides so the
 *  merchant sees the new palette's colours immediately. Background is
 *  preserved — switching palette shouldn't reset the bg. */
export function withPalette(
  theme: StorefrontTheme,
  palette: StorefrontPalette,
): StorefrontTheme {
  const next: StorefrontTheme = {
    ...theme,
    palette,
    customAccent: undefined,
    customPrimary: undefined,
  };
  next.colors = resolveThemeColors(next);
  return next;
}

/** Switch between light and dark mode. Picks a sensible default
 *  background for the new mode+palette combo. Clears custom bg (it was
 *  hex'd for the old mode; mode changes likely invalidate it). Status
 *  overrides are cleared on mode flip — tones that read well on paper
 *  rarely read well on obsidian, so we fall back to the mode defaults. */
export function withMode(
  theme: StorefrontTheme,
  mode: StorefrontMode,
): StorefrontTheme {
  const background =
    theme.background === "custom"
      ? defaultBackgroundFor(mode, theme.palette)
      : backgrounds[theme.background].mode === mode
        ? theme.background
        : defaultBackgroundFor(mode, theme.palette);
  const next: StorefrontTheme = {
    ...theme,
    mode,
    background,
    customBackground: undefined,
    customStatusColors: undefined,
    statusColors: resolveStatusColors(mode),
  };
  next.colors = resolveThemeColors(next);
  return next;
}

/** Apply a new background selection (named or "custom"). */
export function withBackground(
  theme: StorefrontTheme,
  background: StorefrontBackground,
  customHex?: string,
): StorefrontTheme {
  const next: StorefrontTheme = {
    ...theme,
    background,
    customBackground:
      background === "custom" ? (customHex ?? theme.customBackground ?? theme.colors.background) : undefined,
  };
  next.colors = resolveThemeColors(next);
  return next;
}

/** Apply an advanced colour override (accent or primary). Passing null
 *  clears the override. */
export function withColorOverride(
  theme: StorefrontTheme,
  key: "accent" | "primary" | "background",
  value: string | null,
): StorefrontTheme {
  const next: StorefrontTheme = { ...theme };
  if (key === "accent") next.customAccent = value ?? undefined;
  else if (key === "primary") next.customPrimary = value ?? undefined;
  else if (key === "background") {
    next.customBackground = value ?? undefined;
    if (value) next.background = "custom";
  }
  next.colors = resolveThemeColors(next);
  return next;
}

/* ============================================================
   Style helpers
   ============================================================ */

export function themeRadius(theme: StorefrontTheme): string {
  // Editorial scale — sharp is visibly crisp, soft is a restrained
  // modern default, rounded is confidently round without tipping into
  // cartoon territory.
  if (theme.radius === "sharp") return "0.125rem";
  if (theme.radius === "rounded") return "0.875rem";
  return "0.375rem";
}

/**
 * Card-level radius scale applied to Tailwind `.rounded-*` utilities
 * inside the storefront. Multiplies the default Tailwind radius so
 * choosing "sharp" / "soft" / "rounded" flows through every product
 * card, image, button, and chip without touching component code.
 * Tightened alongside themeRadius() — soft = 1× baseline Tailwind,
 * sharp pulls everything crisper, rounded lifts modestly instead of
 * doubling.
 */
export function themeRadiusScale(theme: StorefrontTheme): number {
  if (theme.radius === "sharp") return 0.25;
  if (theme.radius === "rounded") return 1.5;
  return 1;
}

export function themeDensityScale(theme: StorefrontTheme): number {
  if (theme.density === "compact") return 0.88;
  if (theme.density === "airy") return 1.12;
  return 1;
}

/**
 * Motion scale. 0 kills all transitions/animations via CSS. Subtle is
 * the default (1x). Expressive lengthens transitions (1.4x) for a
 * slower, more luxurious feel.
 */
export function themeMotionScale(theme: StorefrontTheme): number {
  if (theme.motion === "none") return 0;
  if (theme.motion === "expressive") return 1.4;
  return 1;
}

export function themeSpacing(theme: StorefrontTheme): string {
  return String(themeDensityScale(theme));
}

export function themeCssVariables(
  theme: StorefrontTheme,
): Record<string, string> {
  const { primary, accent, background, surface, text } = theme.colors;
  const { success, warning, danger, info, neutral } = theme.statusColors;
  // "On-X" contrast colours — used by buttons / chips where bg is the
  // brand colour and text needs legible contrast. Light mode: white
  // text on saturated accent; dark mode: dark text because dark-mode
  // accents are designed brighter for visibility against the dark bg.
  const onAccent = theme.mode === "dark" ? "#0E0E0C" : "#FFFFFF";
  const onPrimary = theme.mode === "dark" ? "#0E0E0C" : "#FFFFFF";
  return {
    "--storefront-primary": primary,
    "--storefront-accent": accent,
    "--storefront-background": background,
    "--storefront-surface": surface,
    "--storefront-text": text,
    "--storefront-on-accent": onAccent,
    "--storefront-on-primary": onPrimary,
    // Semantic status sources — bg/border variants are derived from
    // these in the storefront's globals.css via color-mix, so merchants
    // only configure five values.
    "--storefront-success": success,
    "--storefront-warning": warning,
    "--storefront-danger": danger,
    "--storefront-info": info,
    "--storefront-neutral": neutral,
    "--storefront-heading-font": fontStacks[theme.typography.headingFont],
    "--storefront-body-font": fontStacks[theme.typography.bodyFont],
    "--storefront-radius": themeRadius(theme),
    "--storefront-radius-scale": String(themeRadiusScale(theme)),
    "--storefront-density-scale": String(themeDensityScale(theme)),
    "--storefront-motion-scale": String(themeMotionScale(theme)),
    "--storefront-mode": theme.mode,
    // Narrow house-token overrides — only the three foreground colours,
    // which always need to flip with mode. We intentionally DO NOT
    // override --ink-900 / --paper-200 / --moss-700 — those are used
    // as button backgrounds and borders where flipping breaks contrast.
    "--foreground": text,
    "--foreground-secondary": `${text}B3`,
    "--foreground-tertiary": `${text}80`,
  };
}
