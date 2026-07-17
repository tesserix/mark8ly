import type { ReactNode } from "react";
import { TouchableOpacity, View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";

interface MarketingRowProps {
  icon: ReactNode;
  label: string;
  description: string;
  onPress: () => void;
}

/** A single entry in the Marketing hub: icon, label, one-line description. */
export function MarketingRow({ icon, label, description, onPress }: MarketingRowProps) {
  return (
    <TouchableOpacity
      style={styles.container}
      onPress={onPress}
      activeOpacity={0.6}
      accessibilityRole="button"
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
  icon: { width: 24, alignItems: "center" },
  info: { flex: 1, gap: 2 },
});
