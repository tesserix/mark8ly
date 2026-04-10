export interface StoreBranding {
  primary?: string;
  accent?: string;
  background?: string;
  elevated?: string;
  text?: string;
  textSecondary?: string;
  border?: string;
  fontFamily?: string;
}

export interface MerchantTheme {
  primary: string;
  accent: string;
  background: string;
  elevated: string;
  text: string;
  textSecondary: string;
  border: string;
  fontFamily: string;
}

// Default Mark8ly brand tokens (fallback when B1 not configured)
const DEFAULT_THEME: MerchantTheme = {
  primary: "#0E0E0C",    // Ink
  accent: "#2D4A2B",     // Moss
  background: "#F7F6F2", // Paper
  elevated: "#FFFFFF",
  text: "#0E0E0C",
  textSecondary: "#666666",
  border: "#E5E4DF",
  fontFamily: "SourceSans3",
} as const;

export function buildMerchantTheme(branding: StoreBranding | null): MerchantTheme {
  if (!branding) return { ...DEFAULT_THEME };
  return {
    primary: branding.primary ?? DEFAULT_THEME.primary,
    accent: branding.accent ?? DEFAULT_THEME.accent,
    background: branding.background ?? DEFAULT_THEME.background,
    elevated: branding.elevated ?? DEFAULT_THEME.elevated,
    text: branding.text ?? DEFAULT_THEME.text,
    textSecondary: branding.textSecondary ?? DEFAULT_THEME.textSecondary,
    border: branding.border ?? DEFAULT_THEME.border,
    fontFamily: branding.fontFamily ?? DEFAULT_THEME.fontFamily,
  };
}

export function getMerchantTheme(): MerchantTheme {
  // Try to load theme-overrides.json (baked at build time), fallback to defaults
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const overrides = require("../../theme-overrides.json") as StoreBranding;
    return buildMerchantTheme(overrides);
  } catch {
    return { ...DEFAULT_THEME };
  }
}
