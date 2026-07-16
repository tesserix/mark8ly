import { View, TouchableOpacity, StyleSheet, type ViewStyle } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

interface Segment<T extends string> {
  key: T;
  label: string;
}

interface SegmentedControlProps<T extends string> {
  segments: Segment<T>[];
  value: T;
  onChange: (key: T) => void;
  style?: ViewStyle;
}

export function SegmentedControl<T extends string>({
  segments,
  value,
  onChange,
  style,
}: SegmentedControlProps<T>) {
  return (
    <View style={[styles.row, style]}>
      {segments.map((seg) => {
        const active = seg.key === value;
        return (
          <TouchableOpacity
            key={seg.key}
            onPress={() => onChange(seg.key)}
            activeOpacity={0.7}
            accessibilityRole="tab"
            accessibilityState={{ selected: active }}
            accessibilityLabel={`Filter: ${seg.label}`}
            style={[styles.tab, active && styles.tabActive]}
          >
            <Text
              preset="caption"
              color={active ? "text" : "textTertiary"}
              style={active ? styles.tabLabelActive : styles.tabLabel}
            >
              {seg.label}
            </Text>
          </TouchableOpacity>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.md,
    paddingBottom: theme.spacing.sm,
    gap: theme.spacing.xs,
  },
  tab: {
    paddingHorizontal: theme.spacing.md,
    paddingVertical: 8,
    minHeight: theme.touchTarget,
    justifyContent: "center",
    borderBottomWidth: 1.5,
    borderBottomColor: "transparent",
  },
  tabActive: {
    borderBottomColor: theme.colors.text,
  },
  tabLabel: {
    fontSize: 13,
    fontWeight: "500",
  },
  tabLabelActive: {
    fontSize: 13,
    fontWeight: "700",
  },
});
