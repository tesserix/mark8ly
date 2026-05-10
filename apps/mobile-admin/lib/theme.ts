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
  paperWarm: "#FAFAF6",
  bone: "#E5E4DF",
  ink: "#0E0E0C",
  inkSoft: "#3A3A36",
  moss: "#2D4A2B",
  mossSoft: "#3F6A3D",
  crimson: "#8B2020",
  amber: "#B08A30",
  white: "#FFFFFF",
} as const;

export const theme = {
  colors: {
    background: palette.paper,
    elevated: palette.white,
    surfaceAlt: palette.paperWarm,
    border: palette.bone,
    hairline: "rgba(14, 14, 12, 0.08)",
    overlay: "rgba(14, 14, 12, 0.45)",

    text: palette.ink,
    textSecondary: palette.inkSoft,
    textTertiary: "rgba(14, 14, 12, 0.5)",
    textMuted: "rgba(14, 14, 12, 0.35)",
    inverse: palette.paper,

    accent: palette.moss,
    accentSoft: palette.mossSoft,
    success: palette.moss,
    warning: palette.amber,
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

  hairline: 0.5,
  touchTarget: 44,

  fonts: {
    sans,
    serif,
    mono,
  },

  text: {
    display: {
      fontFamily: serif,
      fontSize: 36,
      lineHeight: 42,
      fontWeight: "700",
      letterSpacing: -0.5,
    } satisfies TextStyle,
    h1: {
      fontFamily: serif,
      fontSize: 28,
      lineHeight: 34,
      fontWeight: "700",
      letterSpacing: -0.3,
    } satisfies TextStyle,
    h2: {
      fontFamily: serif,
      fontSize: 22,
      lineHeight: 28,
      fontWeight: "700",
      letterSpacing: -0.2,
    } satisfies TextStyle,
    h3: {
      fontFamily: serif,
      fontSize: 18,
      lineHeight: 24,
      fontWeight: "700",
    } satisfies TextStyle,
    eyebrow: {
      fontFamily: sans,
      fontSize: 11,
      lineHeight: 14,
      fontWeight: "600",
      letterSpacing: 1.2,
      textTransform: "uppercase",
    } satisfies TextStyle,
    bodyLg: {
      fontFamily: sans,
      fontSize: 16,
      lineHeight: 22,
      fontWeight: "400",
    } satisfies TextStyle,
    body: {
      fontFamily: sans,
      fontSize: 14,
      lineHeight: 20,
      fontWeight: "400",
    } satisfies TextStyle,
    bodyEmphasis: {
      fontFamily: sans,
      fontSize: 14,
      lineHeight: 20,
      fontWeight: "600",
    } satisfies TextStyle,
    caption: {
      fontFamily: sans,
      fontSize: 12,
      lineHeight: 16,
      fontWeight: "500",
    } satisfies TextStyle,
    mono: {
      fontFamily: mono,
      fontSize: 13,
      lineHeight: 18,
    } satisfies TextStyle,
  },
} as const;

export type Theme = typeof theme;
export type TextPreset = keyof typeof theme.text;
