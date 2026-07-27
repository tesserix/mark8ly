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
  // Deliberately theme.spacing.lg (16), NOT the row gutter (xl/20). Eyebrow
  // is a shared primitive with ~15 call sites across screens with different
  // gutters — it must not carry a screen-level layout decision. A screen
  // that needs its bare `<Eyebrow />` to align with a 20pt gutter passes its
  // own `style={{ paddingHorizontal: theme.spacing.xl }}` explicitly (see
  // more/account.tsx, more/security.tsx); every screen that doesn't sweeps
  // stays internally consistent at this default.
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.lg,
    paddingBottom: theme.spacing.sm,
  },
});
