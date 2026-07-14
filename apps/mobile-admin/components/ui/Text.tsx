import { Text as RNText, type TextProps as RNTextProps } from 'react-native';

export type TextPreset =
  | 'display'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'eyebrow'
  | 'bodyLg'
  | 'body'
  | 'bodyEmphasis'
  | 'caption';

// Each preset is a fixed set of nativewind classes: family + size + default
// color. Callers pass `className` to override color/spacing per use.
const PRESET_CLASSES: Record<TextPreset, string> = {
  display: 'font-serif-bold text-display text-ink',
  h1: 'font-serif text-h1 text-ink',
  h2: 'font-serif text-h2 text-ink',
  h3: 'font-serif text-h3 text-ink',
  eyebrow: 'font-sans-semibold text-eyebrow uppercase text-ink-muted',
  bodyLg: 'font-sans text-body-lg text-ink',
  body: 'font-sans text-body text-ink',
  bodyEmphasis: 'font-sans-semibold text-body text-ink',
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
  /** Overrides the preset's default colour. */
  color?: TextColor;
  align?: TextAlign;
}

// Strip the preset's default colour token so an explicit `color` can't
// collide with it (nativewind doesn't guarantee last-wins for two text-*).
const PRESET_COLOR_TOKENS = /\btext-ink(?:-\w+)?\b/g;

export function Text({
  preset = 'body',
  className,
  color,
  align,
  ...rest
}: TextComponentProps) {
  const presetClasses = color
    ? PRESET_CLASSES[preset].replace(PRESET_COLOR_TOKENS, '').trim()
    : PRESET_CLASSES[preset];
  const merged = [
    presetClasses,
    align ? ALIGN_CLASSES[align] : null,
    color ? COLOR_CLASSES[color] : null,
    className,
  ]
    .filter(Boolean)
    .join(' ');
  return <RNText className={merged} {...rest} />;
}
