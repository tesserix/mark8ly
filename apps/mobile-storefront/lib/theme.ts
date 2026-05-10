import { Platform, type TextStyle } from "react-native";
import { getMerchant } from "./merchant";

/**
 * Storefront theme — base typography + spacing tokens shared across
 * every white-label build, with palette colors stamped per merchant.
 *
 * Two layers of branding:
 *
 *   1. Build-time (this file): merchant config injects primary, accent,
 *      background, text via `app.config.js` extras. Picked up the moment
 *      the app boots, before any network call.
 *   2. Runtime (lib/hooks/use-branding.ts): `/storefront/branding`
 *      returns the latest merchant-edited palette + logo + banner so
 *      the merchant can tweak colors without re-submitting the app.
 *
 * The 4px spacing scale, hairlines, radii, and serif/sans pairing are
 * shared across every build to keep the UX consistent.
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

function buildPalette() {
  const merchant = getMerchant();
  const c = merchant.colors;
  return {
    background: c.background,
    elevated: "#FFFFFF",
    surfaceAlt: lighten(c.background, 0.04),
    border: "rgba(14, 14, 12, 0.12)",
    hairline: "rgba(14, 14, 12, 0.08)",
    overlay: "rgba(14, 14, 12, 0.45)",

    text: c.text,
    textSecondary: withAlpha(c.text, 0.75),
    textTertiary: withAlpha(c.text, 0.5),
    textMuted: withAlpha(c.text, 0.35),
    inverse: c.background,

    primary: c.primary,
    accent: c.accent,
    accentSoft: withAlpha(c.accent, 0.12),
    success: c.accent,
    warning: "#B08A30",
    danger: "#8B2020",
  } as const;
}

function lighten(hex: string, amount: number): string {
  // Naive lighten — ok for the warm-paper variant. Returns the input
  // unchanged when we can't parse it; the visual fallback degrades to
  // the base background, which is acceptable.
  if (!hex.startsWith("#") || hex.length !== 7) return hex;
  const r = Math.min(255, parseInt(hex.slice(1, 3), 16) + Math.floor(255 * amount));
  const g = Math.min(255, parseInt(hex.slice(3, 5), 16) + Math.floor(255 * amount));
  const b = Math.min(255, parseInt(hex.slice(5, 7), 16) + Math.floor(255 * amount));
  return `#${[r, g, b].map((x) => x.toString(16).padStart(2, "0")).join("")}`;
}

function withAlpha(hex: string, alpha: number): string {
  if (!hex.startsWith("#") || hex.length !== 7) return hex;
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

export const theme = {
  colors: buildPalette(),

  spacing: {
    xs: 4,
    sm: 8,
    md: 12,
    lg: 16,
    xl: 20,
    xxl: 24,
    xxxl: 32,
    huge: 48,
  },

  radii: {
    none: 0,
    sm: 4,
    md: 8,
    lg: 12,
    xl: 18,
    pill: 999,
  },

  hairline: 0.5,
  touchTarget: 44,

  fonts: { sans, serif, mono },

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
    price: {
      fontFamily: sans,
      fontSize: 16,
      lineHeight: 22,
      fontWeight: "700",
    } satisfies TextStyle,
  },
} as const;

export type Theme = typeof theme;
export type TextPreset = keyof typeof theme.text;
