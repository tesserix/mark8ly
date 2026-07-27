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
  // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so a
  // bare `<Eyebrow />` (no style override) aligns with the rows/cards below
  // it. Not theme.spacing.lg — callers that pass their own `style` (most
  // section eyebrows) are unaffected by this default.
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.lg,
    paddingBottom: theme.spacing.sm,
  },
});
