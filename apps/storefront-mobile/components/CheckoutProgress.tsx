import { View, Text, StyleSheet } from "react-native";

interface CheckoutProgressProps {
  currentStep: 1 | 2 | 3 | 4;
}

const STEPS = ["Details", "Shipping", "Payment", "Review"] as const;

export function CheckoutProgress({ currentStep }: CheckoutProgressProps) {
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

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "flex-start",
    justifyContent: "center",
    paddingHorizontal: 24,
    paddingVertical: 12,
    backgroundColor: "#F7F6F2",
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
    backgroundColor: "#E5E4DF",
    zIndex: -1,
  },
  lineActive: {
    backgroundColor: "#2D4A2B",
  },
  circle: {
    width: 26,
    height: 26,
    borderRadius: 13,
    backgroundColor: "#FFFFFF",
    borderWidth: 2,
    borderColor: "#E5E4DF",
    alignItems: "center",
    justifyContent: "center",
  },
  circleActive: {
    borderColor: "#0E0E0C",
    backgroundColor: "#0E0E0C",
  },
  circleCompleted: {
    borderColor: "#2D4A2B",
    backgroundColor: "#2D4A2B",
  },
  circleText: {
    fontSize: 12,
    fontWeight: "700",
    color: "#999999",
  },
  circleTextActive: {
    color: "#FFFFFF",
  },
  label: {
    fontSize: 11,
    color: "#999999",
    fontWeight: "500",
  },
  labelActive: {
    color: "#0E0E0C",
    fontWeight: "600",
  },
});
