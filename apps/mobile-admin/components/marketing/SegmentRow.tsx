import { View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { PressableRow, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Segment } from "@repo/mobile-shared/api/types";

interface SegmentRowProps {
  segment: Segment;
  onPress: (segment: Segment) => void;
}

export function SegmentRow({ segment, onPress }: SegmentRowProps) {
  const count = segment.member_count;
  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(segment)}
      style={styles.row}
      testID={`segment-row-${segment.id}`}
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
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  row: {
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  info: { flex: 1, gap: 4 },
});
