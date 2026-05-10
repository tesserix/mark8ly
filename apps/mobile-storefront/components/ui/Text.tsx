import { Text as RNText, type TextProps as RNTextProps } from "react-native";
import { theme, type TextPreset } from "@/lib/theme";

type ColorKey =
  | "text"
  | "textSecondary"
  | "textTertiary"
  | "textMuted"
  | "inverse"
  | "primary"
  | "accent"
  | "danger"
  | "success"
  | "warning";

interface TextProps extends RNTextProps {
  preset?: TextPreset;
  color?: ColorKey | string;
  align?: "left" | "center" | "right";
}

export function Text({
  preset = "body",
  color = "text",
  align,
  style,
  ...rest
}: TextProps) {
  const colorValue =
    color in theme.colors ? theme.colors[color as ColorKey] : (color as string);
  return (
    <RNText
      {...rest}
      style={[
        theme.text[preset],
        { color: colorValue, textAlign: align },
        style,
      ]}
    />
  );
}
