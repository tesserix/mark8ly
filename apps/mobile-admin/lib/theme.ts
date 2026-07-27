import { Platform, type TextStyle } from "react-native";

// Mirrors @tesserix/tokens/spacing (4px base scale). Inlined to avoid
// Metro's package-exports resolution requirement for subpath imports.
const tokenSpacing = {
  1: 4,
  2: 8,
  3: 12,
  4: 16,
  5: 20,
  6: 24,
  8: 32,
  12: 48,
} as const;

/**
 * Mark8ly mobile-admin theme — Paper · Ink · Moss editorial.
 *
 * Mirrors the web admin's voice: solid Paper background, no glassmorphism,
 * hairline rules, serif headings, sans body. Tokens come from
 * @tesserix/tokens/spacing; the palette is held here because the web admin
 * also pins these specific Paper/Ink/Moss values rather than the generic
 * default OKLCH theme.
 */

const serif = Platform.select({
  ios: "Georgia",
  android: "serif",
  default: "Georgia, Cambria, 'Times New Roman', Times, serif",
}) as string;

const sans = Platform.select({
  ios: "System",
  android: "Roboto",
  default: "System",
}) as string;

const mono = Platform.select({
  ios: "Menlo",
  android: "monospace",
  default: "Menlo, Monaco, 'Courier New', monospace",
}) as string;

const palette = {
  paper: "#F7F6F2",
  paperWarm: "#FAF8F2",
  bone: "#E2DFD6",
  sink: "#ECEAE3",
  ink: "#0E0E0C",
  inkSoft: "#45433E",
  moss: "#2D4A2B",
  mossSoft: "#3D5F38",
  crimson: "#8B2E20",
  amber: "#B5751F",
  // Amber as a badge TINT + a deep bronze for text on it — the warning
  // equivalent of accentTint/accent (moss). Dark-bronze #7A4A0F on the
  // #F4E6CB tint is ~6:1, comfortably AA, and reads far better than ink on
  // the saturated solid amber (~5:1, perceptually marginal).
  amberTint: "#F4E6CB",
  amberDeep: "#7A4A0F",
  white: "#FFFFFF",
} as const;

export const theme = {
  colors: {
    background: palette.paper,
    elevated: palette.white,
    surfaceAlt: palette.paperWarm,
    sink: palette.sink,
    border: palette.bone,
    hairline: "rgba(14, 14, 12, 0.08)",
    overlay: "rgba(14, 14, 12, 0.45)",

    text: palette.ink,
    textSecondary: palette.inkSoft,
    // Canonical --ink-500. Replaced a previous insufficient-contrast value
    // with #5C5953 to clear 4.5:1 WCAG AA on paper (see fix-batch-1-report.md).
    textTertiary: "#5C5953",
    // Decorative/placeholder only — not used as real body/label text
    // anywhere in the app today (grepped 2026-07-17). If a future usage
    // promotes it to real copy, it must be re-checked against AA.
    textMuted: "rgba(14, 14, 12, 0.35)",
    inverse: palette.paper,

    accent: palette.moss,
    accentSoft: palette.mossSoft,
    accentTint: "#E8EEE2",
    success: palette.moss,
    warning: palette.amber,
    warningTint: palette.amberTint,
    warningInk: palette.amberDeep,
    danger: palette.crimson,

    palette,
  },

  spacing: {
    xs: tokenSpacing[1],
    sm: tokenSpacing[2],
    md: tokenSpacing[3],
    lg: tokenSpacing[4],
    xl: tokenSpacing[5],
    xxl: tokenSpacing[6],
    xxxl: tokenSpacing[8],
    huge: tokenSpacing[12],
  },

  radius: 6,
  radii: {
    none: 0,
    sm: 4,
    md: 6,
    lg: 10,
    xl: 14,
    pill: 999,
  },

  /**
   * Native row density. iOS list rows carry their content on a taller,
   * calmer field than a web table row — 64 for a single line, 88 for the
   * two-line stack (17pt primary + 13pt secondary). `touchTarget` still
   * holds at 44; rows exceed it rather than replacing the rule.
   */
  row: {
    minHeightSingle: 64,
    minHeightDouble: 88,
    paddingH: 20,
    paddingV: 14,
    gap: 16,
  },

  thumb: {
    list: 60,
    compact: 38,
  },

  /**
   * Native press feedback vocabulary — the one place every ripple colour
   * and iOS press-opacity value is defined. `Pressable`/`PressableRow`
   * call sites spread `ripple*` into `android_ripple` and reference
   * `opacity*` in their pressed-state style function. Centralised so
   * changing the press language is a one-file edit, not a 20-file sweep.
   */
  press: {
    /** Dark ripple for light/transparent/outline surfaces — the default. */
    rippleInk: { color: "rgba(14, 14, 12, 0.12)" },
    /** Light ripple for a solid ink/moss fill, where a dark ripple would vanish. */
    rippleOnDark: { color: "rgba(247, 246, 242, 0.24)" },
    /** Danger-tinted ripple for destructive controls (outline or icon). */
    rippleDanger: { color: "rgba(139, 46, 32, 0.12)" },
    /** Moss-tinted ripple for the rare single-accent press moment (e.g. a chip or accent text link). */
    rippleAccent: { color: "rgba(45, 74, 43, 0.12)" },
    /** iOS press dim for icon buttons, outline/transparent surfaces, and text links. */
    opacityStandard: 0.55,
    /**
     * iOS press dim for a solid ink/moss fill CTA — gentler than
     * `opacityStandard` so the fill still reads as itself while held
     * (a 45% fade on a filled button looks broken, not pressed).
     */
    opacitySolidFill: 0.85,
  },

  hairline: 0.5,
  touchTarget: 44,

  fonts: {
    sans,
    serif,
    mono,
  },

  text: {
    // Home revenue figure only — kept separate so `display` can stay at 40
    // everywhere it is already used.
    heroNumeral: {
      fontFamily: serif,
      fontSize: 44,
      lineHeight: 48,
      fontWeight: "700",
      letterSpacing: -0.8,
    } satisfies TextStyle,
    display: {
      fontFamily: serif,
      fontSize: 40,
      lineHeight: 46,
      fontWeight: "700",
      letterSpacing: -0.6,
    } satisfies TextStyle,
    h1: {
      fontFamily: serif,
      fontSize: 30,
      lineHeight: 36,
      fontWeight: "700",
      letterSpacing: -0.4,
    } satisfies TextStyle,
    h2: {
      fontFamily: serif,
      fontSize: 24,
      lineHeight: 30,
      fontWeight: "700",
      letterSpacing: -0.25,
    } satisfies TextStyle,
    h3: {
      fontFamily: serif,
      fontSize: 20,
      lineHeight: 26,
      fontWeight: "700",
    } satisfies TextStyle,
    eyebrow: {
      fontFamily: sans,
      fontSize: 12,
      lineHeight: 16,
      fontWeight: "600",
      letterSpacing: 1.2,
      textTransform: "uppercase",
    } satisfies TextStyle,
    bodyLg: {
      fontFamily: sans,
      fontSize: 19,
      lineHeight: 26,
      fontWeight: "400",
    } satisfies TextStyle,
    body: {
      fontFamily: sans,
      fontSize: 17,
      lineHeight: 24,
      fontWeight: "400",
    } satisfies TextStyle,
    bodyEmphasis: {
      fontFamily: sans,
      fontSize: 17,
      lineHeight: 24,
      fontWeight: "600",
    } satisfies TextStyle,
    label: {
      fontFamily: sans,
      fontSize: 15,
      lineHeight: 20,
      fontWeight: "500",
      letterSpacing: 0.1,
    } satisfies TextStyle,
    caption: {
      fontFamily: sans,
      fontSize: 13,
      lineHeight: 18,
      fontWeight: "500",
    } satisfies TextStyle,
    mono: {
      fontFamily: mono,
      fontSize: 15,
      lineHeight: 20,
    } satisfies TextStyle,
  },
} as const;

export type Theme = typeof theme;
export type TextPreset = keyof typeof theme.text;
