import { Text as RNText, type TextProps as RNTextProps } from 'react-native';

export type TextPreset =
  | 'heroNumeral'
  | 'display'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'eyebrow'
  | 'bodyLg'
  | 'body'
  | 'bodyEmphasis'
  | 'label'
  | 'caption';

// Each preset is a fixed set of nativewind classes: family + size + default
// color. Callers pass `className` to override color/spacing per use.
const PRESET_CLASSES: Record<TextPreset, string> = {
  heroNumeral: 'font-serif-bold text-hero text-ink',
  display: 'font-serif-bold text-display text-ink',
  h1: 'font-serif text-h1 text-ink',
  h2: 'font-serif text-h2 text-ink',
  h3: 'font-serif text-h3 text-ink',
  eyebrow: 'font-sans-semibold text-eyebrow uppercase text-ink-muted',
  bodyLg: 'font-sans text-body-lg text-ink',
  body: 'font-sans text-body text-ink',
  bodyEmphasis: 'font-sans-semibold text-body text-ink',
  label: 'font-sans-medium text-label text-ink',
  caption: 'font-sans-medium text-caption text-ink-soft',
};

/**
 * Back-compat colour + alignment tokens. The pre-existing screens (ported in
 * Phase 2) call `<Text color="inverse" align="center">`; without these maps
 * the new preset colour would win and, e.g., `inverse` (white) text would
 * render as dark ink on a coloured button — unreadable. Mapping them to
 * nativewind classes keeps the legacy screens correct until they're restyled.
 * `color` overrides the preset's default colour (appended last).
 */
export type TextColor =
  | 'text'
  | 'textSecondary'
  | 'textTertiary'
  | 'textMuted'
  | 'inverse'
  | 'accent'
  | 'success'
  | 'danger'
  | 'warning';

export type TextAlign = 'left' | 'center' | 'right';

/**
 * App-wide cap on the iOS Dynamic Type / Android font-scale multiplier.
 *
 * 2.0 honours WCAG 2.1 SC 1.4.4 (text resizable to 200% without loss of
 * content or functionality) while stopping the accessibility categories above
 * it — iOS reaches ~3.1× at AX5 — from growing a line past the box that holds
 * it. Uncapped, the Dashboard's four-up strip labels collide with the tools
 * FAB, `THIS MONTH` runs under the trend badge, and the dock's five labels
 * degrade to `H… O… P… C… M…`.
 *
 * This is a DEFAULT at the chokepoint, not a per-call opt-in, and it is the
 * one deliberate exception to this app's "shared primitives get additive
 * props, never changed defaults" rule: the defect it fixes is precisely that
 * a screen has to remember to opt in, and ~20 text nodes did not.
 * `CollapsingHeader` and `FilterChips` independently arrived at the same 2 —
 * both now derive their `MAX_FONT_SCALE` from this constant so the header's
 * `headerHeightsFor` arithmetic and the text it measures cannot drift apart.
 *
 * A caller may still pass a TIGHTER cap (`TenantMonogram`'s initial pins 1 —
 * an identity mark has nothing to resize), and that explicit prop wins. Do
 * not pass a looser one without a stated reason; `allowFontScaling` is left
 * alone entirely so text still scales, just not without bound.
 */
export const MAX_FONT_SCALE = 2;

const COLOR_CLASSES: Record<TextColor, string> = {
  text: 'text-ink',
  textSecondary: 'text-ink-soft',
  textTertiary: 'text-ink-muted',
  textMuted: 'text-ink-faint',
  inverse: 'text-paper',
  accent: 'text-moss',
  success: 'text-moss',
  danger: 'text-danger',
  warning: 'text-warning',
};

const ALIGN_CLASSES: Record<TextAlign, string> = {
  left: 'text-left',
  center: 'text-center',
  right: 'text-right',
};

export interface TextComponentProps extends RNTextProps {
  preset?: TextPreset;
  className?: string;
  /**
   * Overrides the preset's default colour. Accepts either a semantic token
   * ("inverse", "success", …) OR a raw colour value (e.g. a `theme.colors.*`
   * hex). Some pre-existing screens pass a hex directly (e.g. StatusBadge),
   * so both forms must resolve — a hex is applied as an inline style, a token
   * as a nativewind class.
   */
  color?: TextColor | (string & {});
  align?: TextAlign;
}

// Strip the preset's default colour token so an explicit `color` can't
// collide with it (nativewind doesn't guarantee last-wins for two text-*).
const PRESET_COLOR_TOKENS = /\btext-ink(?:-\w+)?\b/g;

function isColorToken(c: string): c is TextColor {
  return Object.prototype.hasOwnProperty.call(COLOR_CLASSES, c);
}

export function Text({
  preset = 'body',
  className,
  color,
  align,
  style,
  maxFontSizeMultiplier = MAX_FONT_SCALE,
  ...rest
}: TextComponentProps) {
  const token = color && isColorToken(color) ? color : null;
  // Anything that isn't a known token is treated as a raw colour value and
  // applied inline (wins over the className colour).
  const rawColor = color && !token ? color : null;

  const presetClasses = color
    ? PRESET_CLASSES[preset].replace(PRESET_COLOR_TOKENS, '').trim()
    : PRESET_CLASSES[preset];
  const merged = [
    presetClasses,
    align ? ALIGN_CLASSES[align] : null,
    token ? COLOR_CLASSES[token] : null,
    className,
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <RNText
      className={merged}
      // Destructured above (so it is NOT in `rest`) purely so the default
      // applies; an explicit prop still overrides it, in either direction.
      maxFontSizeMultiplier={maxFontSizeMultiplier}
      style={rawColor ? [{ color: rawColor }, style] : style}
      {...rest}
    />
  );
}
