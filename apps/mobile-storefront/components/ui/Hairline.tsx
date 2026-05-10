import { View, StyleSheet, type ViewStyle } from "react-native";
import { theme } from "@/lib/theme";

interface HairlineProps {
  inset?: number;
  style?: ViewStyle;
}

export function Hairline({ inset = 0, style }: HairlineProps) {
  return <View style={[styles.line, inset ? { marginLeft: inset } : null, style]} />;
}

const styles = StyleSheet.create({
  line: {
    height: theme.hairline,
    backgroundColor: theme.colors.hairline,
  },
});
