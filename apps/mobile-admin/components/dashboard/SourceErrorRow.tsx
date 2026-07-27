import { useCallback, useState } from "react";
import { Platform, Pressable, View, StyleSheet } from "react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";

interface SourceErrorRowProps {
  /** Human names of the sources that failed, e.g. ["reviews", "tickets"]. */
  sources: string[];
  onRetry: () => void;
}

/** "tickets" · "reviews and tickets" · "orders, reviews and tickets" */
function listSources(sources: string[]): string {
  if (sources.length <= 1) return sources[0] ?? "";
  return `${sources.slice(0, -1).join(", ")} and ${sources[sources.length - 1]}`;
}

/**
 * ONE muted row naming every source that failed, plus a retry — never a
 * per-source stack, and never a full-screen error. The Dashboard composes
 * three independent queries; if tickets time out, the metrics band, the
 * orders and the reviews must still be on screen. This row is the entire
 * cost of that failure.
 *
 * Deliberately NOT moss: this screen's one accent is already spent on the
 * revenue chart and the Approve swipe, so "Retry" is ink on a hairline
 * outline rather than an accent text link.
 */
export function SourceErrorRow({ sources, onRetry }: SourceErrorRowProps) {
  // Explicit press state, plain style ARRAY — a function `style` prop on
  // `Pressable` is silently dropped under NativeWind's JSX interop (see
  // components/ui/PressableRow.tsx).
  const [pressed, setPressed] = useState(false);
  const handlePressIn = useCallback(() => setPressed(true), []);
  const handlePressOut = useCallback(() => setPressed(false), []);

  return (
    <View style={styles.root} testID="dashboard-source-error">
      <Text preset="caption" color="textTertiary" style={styles.message}>
        Couldn&apos;t load {listSources(sources)}. The rest of this screen is up to
        date.
      </Text>
      <Pressable
        onPress={onRetry}
        onPressIn={handlePressIn}
        onPressOut={handlePressOut}
        accessibilityRole="button"
        accessibilityLabel={`Retry loading ${listSources(sources)}`}
        android_ripple={theme.press.rippleInk}
        testID="dashboard-source-error-retry"
        style={[
          styles.retry,
          pressed && Platform.OS === "ios" ? styles.retryPressed : null,
        ]}
      >
        <Text preset="caption" color="text">
          Retry
        </Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.row.gap,
    paddingHorizontal: theme.spacing.xl,
    paddingVertical: theme.spacing.md,
    minHeight: theme.row.minHeightSingle,
    backgroundColor: theme.colors.sink,
  },
  message: { flex: 1 },
  retry: {
    minHeight: theme.touchTarget,
    minWidth: 72,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: theme.spacing.md,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.textTertiary,
  },
  retryPressed: { opacity: theme.press.opacityStandard },
});
