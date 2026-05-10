import { forwardRef } from "react";
import { Text as RNText, type TextProps as RNTextProps } from "react-native";
import { theme, type TextPreset } from "@/lib/theme";

interface TextProps extends RNTextProps {
  preset?: TextPreset;
  color?: keyof typeof theme.colors | string;
  align?: "left" | "center" | "right";
}

function resolveColor(color: TextProps["color"]): string {
  if (!color) return theme.colors.text;
  if (color in theme.colors) {
    const v = (theme.colors as Record<string, unknown>)[color as string];
    if (typeof v === "string") return v;
  }
  return color;
}

export const Text = forwardRef<RNText, TextProps>(function Text(
  { preset = "body", color, align, style, ...rest },
  ref,
) {
  return (
    <RNText
      ref={ref}
      style={[
        theme.text[preset],
        { color: resolveColor(color) },
        align ? { textAlign: align } : null,
        style,
      ]}
      {...rest}
    />
  );
});
