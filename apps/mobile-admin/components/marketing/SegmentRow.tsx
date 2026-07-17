import { View, TouchableOpacity, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Segment } from "@repo/mobile-shared/api/types";

interface SegmentRowProps {
  segment: Segment;
  onPress: (segment: Segment) => void;
}

export function SegmentRow({ segment, onPress }: SegmentRowProps) {
  const count = segment.member_count;
  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(segment)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`Segment ${segment.name}, ${count} members`}
    >
      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {segment.name}
        </Text>
        <Text preset="caption" color="textSecondary" numberOfLines={1}>
          {count} {count === 1 ? "member" : "members"}
          {segment.description ? ` · ${segment.description}` : ""}
        </Text>
      </View>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    gap: theme.spacing.md,
  },
  info: { flex: 1, gap: 4 },
});
