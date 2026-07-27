import { useState } from "react";
import { Platform, View, Pressable, StyleSheet, type ViewStyle } from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
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

// Extracted so press state can live in `useState`: this tab renders once per
// segment inside `segments.map()`, and hooks can't be called from inside a
// `.map()` callback — each tab needs its own component instance instead.
function SegmentTab<T extends string>({
  seg,
  active,
  onPress,
}: {
  seg: Segment<T>;
  active: boolean;
  onPress: () => void;
}) {
  const [pressed, setPressed] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      accessibilityRole="tab"
      accessibilityState={{ selected: active }}
      accessibilityLabel={`Filter: ${seg.label}`}
      android_ripple={theme.press.rippleInk}
      style={[
        styles.tab,
        active && styles.tabActive,
        pressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
      ]}
    >
      <Text
        preset="caption"
        color={active ? "text" : "textTertiary"}
        style={active ? styles.tabLabelActive : styles.tabLabel}
      >
        {seg.label}
      </Text>
    </Pressable>
  );
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
          <SegmentTab
            key={seg.key}
            seg={seg}
            active={active}
            onPress={() => {
              void adminHaptics.selectionChanged();
              onChange(seg.key);
            }}
          />
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
