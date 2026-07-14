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

export interface TextComponentProps extends RNTextProps {
  preset?: TextPreset;
  className?: string;
}

export function Text({
  preset = 'body',
  className,
  ...rest
}: TextComponentProps) {
  const merged = className
    ? `${PRESET_CLASSES[preset]} ${className}`
    : PRESET_CLASSES[preset];
  return <RNText className={merged} {...rest} />;
}
