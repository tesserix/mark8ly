import type { ReactNode } from "react";
import { View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { PressableRow, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

interface MarketingRowProps {
  icon: ReactNode;
  label: string;
  description: string;
  onPress: () => void;
}

function slugify(label: string): string {
  return label.toLowerCase().replace(/\s+/g, "-");
}

/** A single entry in the Marketing hub: icon, label, one-line description. */
export function MarketingRow({ icon, label, description, onPress }: MarketingRowProps) {
  return (
    <PressableRow
      lines={1}
      onPress={onPress}
      style={styles.row}
      testID={`marketing-row-${slugify(label)}`}
      accessibilityLabel={`${label}. ${description}`}
    >
      <View style={styles.icon}>{icon}</View>
      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text">
          {label}
        </Text>
        <Text preset="caption" color="textTertiary" numberOfLines={1}>
          {description}
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
  icon: { width: 24, alignItems: "center" },
  info: { flex: 1, gap: 2 },
});
