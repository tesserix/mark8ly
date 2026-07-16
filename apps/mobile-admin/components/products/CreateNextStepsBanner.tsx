import { View, TouchableOpacity, StyleSheet } from "react-native";
import Animated, { FadeIn, FadeOut, useReducedMotion } from "react-native-reanimated";
import { X } from "lucide-react-native";
import { Card, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { DISCLOSURE_DURATION, DISCLOSURE_EASING, DISCLOSURE_EXIT_DURATION } from "./disclosure-motion";
import type { CreatedBannerSection } from "@/lib/hooks/use-created-banner";

interface CreateNextStepsBannerProps {
  title: string;
  onJump: (section: CreatedBannerSection) => void;
  onDismiss: () => void;
}

const CHIPS: { key: CreatedBannerSection; label: string }[] = [
  { key: "photos", label: "Add photos" },
  { key: "options", label: "Add options" },
  { key: "variants", label: "Review variants" },
];

/**
 * Post-create nudge shown once on hand-off from the streamlined create
 * screen (`?created=1`). Ghost card on surfaceAlt, one accent (moss chips on
 * press), dismissible; a normal edit visit never shows it.
 */
export function CreateNextStepsBanner({ title, onJump, onDismiss }: CreateNextStepsBannerProps) {
  const reduceMotion = useReducedMotion();

  return (
    <Animated.View
      testID="create-next-steps-banner"
      entering={
        reduceMotion ? undefined : FadeIn.duration(DISCLOSURE_DURATION).easing(DISCLOSURE_EASING)
      }
      exiting={
        reduceMotion
          ? undefined
          : FadeOut.duration(DISCLOSURE_EXIT_DURATION).easing(DISCLOSURE_EASING)
      }
    >
      <Card variant="ghost" padding="md" style={styles.card}>
        <View style={styles.header}>
          <View style={styles.copy}>
            <Text preset="h3" color="text">
              {`Nice — '${title}' is live.`}
            </Text>
            <Text preset="caption" color="textSecondary">
              Add photos, options, and extra variants whenever you&apos;re ready.
            </Text>
          </View>
          <TouchableOpacity
            onPress={onDismiss}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel="Dismiss"
          >
            <X size={18} color={theme.colors.textTertiary} strokeWidth={1.75} />
          </TouchableOpacity>
        </View>
        <View style={styles.chips}>
          {CHIPS.map((chip) => (
            <TouchableOpacity
              key={chip.key}
              style={styles.chip}
              onPress={() => onJump(chip.key)}
              activeOpacity={0.7}
              accessibilityRole="button"
              accessibilityLabel={chip.label}
            >
              <Text preset="caption" color="text">
                {chip.label}
              </Text>
            </TouchableOpacity>
          ))}
        </View>
      </Card>
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  card: {
    marginHorizontal: theme.spacing.lg,
    marginTop: theme.spacing.md,
    marginBottom: theme.spacing.sm,
    backgroundColor: theme.colors.surfaceAlt,
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-start",
    gap: theme.spacing.sm,
  },
  copy: { flex: 1, gap: theme.spacing.xs },
  chips: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: theme.spacing.sm,
    marginTop: theme.spacing.md,
  },
  chip: {
    minHeight: theme.touchTarget,
    justifyContent: "center",
    paddingHorizontal: theme.spacing.md,
    borderRadius: theme.radii.pill,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
  },
});
