import type { ReactNode } from "react";
import { View, TouchableOpacity, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { ChevronLeft } from "lucide-react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

interface BackHeaderProps {
  title?: string;
  eyebrow?: string;
  rightSlot?: ReactNode;
}

// The screen's <Screen> wrapper already applies the top safe-area inset, so
// BackHeader (its first child) sits below the status bar without adding its
// own inset — adding one here double-pads the top.
export function BackHeader({ title, eyebrow, rightSlot }: BackHeaderProps) {
  const router = useRouter();

  return (
    <View style={styles.wrap}>
      <TouchableOpacity
        onPress={() => router.back()}
        style={styles.back}
        hitSlop={12}
        accessibilityRole="button"
        accessibilityLabel="Go back"
      >
        <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
      </TouchableOpacity>
      <View style={styles.center}>
        {eyebrow ? (
          <Text preset="eyebrow" color="textTertiary" align="center">
            {eyebrow}
          </Text>
        ) : null}
        {title ? (
          <Text preset="bodyEmphasis" color="text" align="center" numberOfLines={1}>
            {title}
          </Text>
        ) : null}
      </View>
      <View style={styles.right}>{rightSlot}</View>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.sm,
    height: 48,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    backgroundColor: theme.colors.background,
  },
  back: {
    width: 44,
    height: 44,
    alignItems: "center",
    justifyContent: "center",
  },
  center: { flex: 1, alignItems: "center", paddingHorizontal: theme.spacing.sm },
  // minWidth, NOT width: a fixed 44 is wide enough for the back chevron but not
  // for a text rightSlot — "Saving…"/"Saved" wrapped to two lines ("Save"/"d"),
  // and "Mark all" on the notifications screen has the same problem. `center`
  // is flex:1 so it yields the space; this can only ever grow the slot.
  right: { minWidth: 44, alignItems: "flex-end", paddingRight: theme.spacing.sm },
});
