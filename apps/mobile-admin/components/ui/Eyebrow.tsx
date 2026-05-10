import { View, StyleSheet, type ViewStyle } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

interface EyebrowProps {
  label: string;
  rightSlot?: React.ReactNode;
  style?: ViewStyle;
}

/** Section header — uppercased eyebrow + optional right-aligned action. */
export function Eyebrow({ label, rightSlot, style }: EyebrowProps) {
  return (
    <View style={[styles.row, style]}>
      <Text preset="eyebrow" color="textTertiary">
        {label}
      </Text>
      {rightSlot ? <View>{rightSlot}</View> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.lg,
    paddingBottom: theme.spacing.sm,
  },
});
