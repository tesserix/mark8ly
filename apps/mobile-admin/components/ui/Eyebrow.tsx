import { View, StyleSheet, type ViewStyle } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

interface EyebrowProps {
  label: string;
  rightSlot?: React.ReactNode;
  style?: ViewStyle;
  /**
   * Clamps the label to a fixed number of line boxes.
   *
   * ADDITIVE and undefined by default — an unclamped label still wraps
   * freely, which is what every section-header call site wants (its copy is
   * a hand-written constant that the author has already sized).
   *
   * It exists for the call sites whose label is ARBITRARY text: `ActionSheet`
   * renders its `title` through this component and budgets the block's height
   * in its `snapPoints` arithmetic, so a merchant-supplied product title that
   * wrapped to three lines silently pushed the sheet's last row below the
   * fold. A caller that MEASURES this block must also be able to bound it.
   */
  numberOfLines?: number;
}

/** Section header — uppercased eyebrow + optional right-aligned action. */
export function Eyebrow({ label, rightSlot, style, numberOfLines }: EyebrowProps) {
  return (
    <View style={[styles.row, style]}>
      <Text
        preset="eyebrow"
        color="textTertiary"
        numberOfLines={numberOfLines}
        // Paired with the clamp, never applied without it. A clamped label
        // whose natural single-line width exceeds the row would otherwise be
        // measured at that full width and run off the right edge instead of
        // ellipsising (RN's flexShrink defaults to 0). Every unclamped call
        // site keeps its existing layout exactly.
        style={numberOfLines === undefined ? undefined : styles.clamped}
      >
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
  clamped: { flexShrink: 1 },
});
