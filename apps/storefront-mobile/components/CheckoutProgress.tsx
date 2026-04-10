import { useMemo } from "react";
import { View, Text, StyleSheet } from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";

interface CheckoutProgressProps {
  currentStep: 1 | 2 | 3 | 4;
}

const STEPS = ["Details", "Shipping", "Payment", "Review"] as const;

export function CheckoutProgress({ currentStep }: CheckoutProgressProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  return (
    <View style={styles.container} accessibilityRole="progressbar">
      {STEPS.map((label, index) => {
        const stepNum = (index + 1) as 1 | 2 | 3 | 4;
        const isActive = stepNum === currentStep;
        const isCompleted = stepNum < currentStep;

        return (
          <View key={label} style={styles.stepWrapper}>
            {index > 0 && (
              <View
                style={[
                  styles.line,
                  (isActive || isCompleted) && styles.lineActive,
                ]}
              />
            )}
            <View
              style={[
                styles.circle,
                isActive && styles.circleActive,
                isCompleted && styles.circleCompleted,
              ]}
            >
              <Text
                style={[
                  styles.circleText,
                  (isActive || isCompleted) && styles.circleTextActive,
                ]}
              >
                {isCompleted ? "✓" : String(stepNum)}
              </Text>
            </View>
            <Text
              style={[styles.label, isActive && styles.labelActive]}
              numberOfLines={1}
            >
              {label}
            </Text>
          </View>
        );
      })}
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "flex-start",
    justifyContent: "center",
    paddingHorizontal: 24,
    paddingVertical: 12,
    backgroundColor: theme.background,
  },
  stepWrapper: {
    flex: 1,
    alignItems: "center",
    gap: 4,
  },
  line: {
    position: "absolute",
    top: 12,
    right: "50%",
    width: "100%",
    height: 2,
    backgroundColor: theme.border,
    zIndex: -1,
  },
  lineActive: {
    backgroundColor: theme.accent,
  },
  circle: {
    width: 26,
    height: 26,
    borderRadius: 13,
    backgroundColor: theme.elevated,
    borderWidth: 2,
    borderColor: theme.border,
    alignItems: "center",
    justifyContent: "center",
  },
  circleActive: {
    borderColor: theme.primary,
    backgroundColor: theme.primary,
  },
  circleCompleted: {
    borderColor: theme.accent,
    backgroundColor: theme.accent,
  },
  circleText: {
    fontSize: 12,
    fontWeight: "700",
    color: theme.textSecondary,
  },
  circleTextActive: {
    color: theme.elevated,
  },
  label: {
    fontSize: 11,
    color: theme.textSecondary,
    fontWeight: "500",
  },
  labelActive: {
    color: theme.text,
    fontWeight: "600",
  },
});
}
