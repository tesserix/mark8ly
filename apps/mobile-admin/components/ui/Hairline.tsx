import { View, type ViewStyle } from "react-native";
import { theme } from "@/lib/theme";

interface HairlineProps {
  inset?: number;
  style?: ViewStyle;
}

/** 0.5px low-contrast horizontal divider. */
export function Hairline({ inset = 0, style }: HairlineProps) {
  return (
    <View
      style={[
        {
          height: theme.hairline,
          backgroundColor: theme.colors.hairline,
          marginLeft: inset,
        },
        style,
      ]}
    />
  );
}
