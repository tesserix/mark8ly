import { useCallback, useState, type ReactNode } from "react";
import {
  Platform,
  Pressable,
  StyleSheet,
  View,
  type LayoutChangeEvent,
} from "react-native";
import { Gesture, GestureDetector } from "react-native-gesture-handler";
import Animated, {
  runOnJS,
  useAnimatedStyle,
  useReducedMotion,
  useSharedValue,
  withSpring,
} from "react-native-reanimated";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

export interface SwipeAction {
  key: string;
  label: string;
  icon: ReactNode;
  tone: "accent" | "danger" | "neutral";
  onPress: () => void;
}

export interface SwipeRowProps {
  children: ReactNode;
  /** Revealed by dragging RIGHT — constructive (moss), app-wide convention. */
  leadingActions?: SwipeAction[];
  /** Revealed by dragging LEFT — destructive (danger), app-wide convention. */
  trailingActions?: SwipeAction[];
  /** Default true. When false, the pan gesture is fully disabled. */
  enabled?: boolean;
  testID?: string;
}

/**
 * Fraction of the row's own measured width a drag must cross before release
 * auto-fires the primary (first) action on that side — the same "full swipe"
 * convenience gesture as iOS Mail, on top of the individually-tappable
 * action buttons the drag reveals.
 */
const THRESHOLD_FRACTION = 0.4;
const ACTION_WIDTH = 84;

const TONE_BACKGROUND: Record<SwipeAction["tone"], string> = {
  accent: theme.colors.accent,
  danger: theme.colors.danger,
  neutral: theme.colors.sink,
};

/**
 * Horizontal swipe container: wraps a `PressableRow` (or any row content)
 * with leading/trailing action panels revealed by a `Gesture.Pan()` drag.
 *
 * Owns no business logic — every action's `onPress` is supplied by the
 * caller. This is the one row-gesture primitive in the app; Task 8
 * (Dashboard queue), Task 9 (Orders), and increment 3 (reviews, tickets,
 * coupons, products) all wrap their own row content in it rather than
 * hand-rolling `Gesture.Pan()` again.
 *
 * Reduced motion gates the spring-back only — the actions stay reachable
 * either way, they just snap to rest instead of easing.
 */
export function SwipeRow({
  children,
  leadingActions = [],
  trailingActions = [],
  enabled = true,
  testID = "swipe-row",
}: SwipeRowProps) {
  const reduceMotion = useReducedMotion();
  const translateX = useSharedValue(0);
  const rowWidth = useSharedValue(0);
  // Tracks whether the CURRENT drag has already crossed the threshold, so
  // the haptic fires once per crossing rather than once per `onUpdate`
  // frame. Reset on release and whenever the drag recedes back under the
  // threshold, so dragging past it, back, and past it again counts as two
  // separate crossings — matching how the haptic reads to a user's thumb.
  const hasCrossed = useSharedValue(false);

  const hasLeading = leadingActions.length > 0;
  const hasTrailing = trailingActions.length > 0;
  const primaryLeading = leadingActions[0];
  const primaryTrailing = trailingActions[0];

  // `adminHaptics.swipeThreshold` is async and does its own error
  // swallowing (see feedback.ts) — this wrapper just gives `runOnJS` a
  // fire-and-forget void function to call from the worklet.
  const fireThresholdHaptic = useCallback(() => {
    void adminHaptics.swipeThreshold();
  }, []);

  const handleLayout = useCallback(
    (event: LayoutChangeEvent) => {
      rowWidth.value = event.nativeEvent.layout.width;
    },
    [rowWidth],
  );

  const pan = Gesture.Pan()
    .enabled(enabled)
    .onUpdate((event) => {
      "worklet";
      // Belt-and-suspenders with `.enabled(enabled)` above: guard explicitly
      // rather than relying solely on the native gesture recognizer, the
      // same pattern PressableRow/IconButton use for their `disabled` prop.
      if (!enabled) return;

      let next = event.translationX;
      if (next > 0 && !hasLeading) next = 0;
      if (next < 0 && !hasTrailing) next = 0;
      translateX.value = next;

      const threshold = rowWidth.value * THRESHOLD_FRACTION;
      const isPastThreshold = threshold > 0 && Math.abs(next) >= threshold;
      if (isPastThreshold && !hasCrossed.value) {
        hasCrossed.value = true;
        runOnJS(fireThresholdHaptic)();
      } else if (!isPastThreshold && hasCrossed.value) {
        hasCrossed.value = false;
      }
    })
    .onEnd(() => {
      "worklet";
      if (!enabled) return;

      const threshold = rowWidth.value * THRESHOLD_FRACTION;
      const passedThreshold = threshold > 0 && Math.abs(translateX.value) >= threshold;
      if (passedThreshold) {
        if (translateX.value > 0 && primaryLeading) {
          runOnJS(primaryLeading.onPress)();
        } else if (translateX.value < 0 && primaryTrailing) {
          runOnJS(primaryTrailing.onPress)();
        }
      }
      hasCrossed.value = false;
      translateX.value = reduceMotion ? 0 : withSpring(0);
    });

  const contentStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: translateX.value }],
  }));

  return (
    <View style={styles.container} onLayout={handleLayout} testID={testID}>
      {hasLeading ? (
        <View style={[styles.actionsPanel, styles.leadingPanel]} pointerEvents="box-none">
          {leadingActions.map((action) => (
            <SwipeActionButton key={action.key} action={action} />
          ))}
        </View>
      ) : null}
      {hasTrailing ? (
        <View style={[styles.actionsPanel, styles.trailingPanel]} pointerEvents="box-none">
          {trailingActions.map((action) => (
            <SwipeActionButton key={action.key} action={action} />
          ))}
        </View>
      ) : null}
      <GestureDetector gesture={pan}>
        <Animated.View style={[styles.content, contentStyle]} testID={`${testID}-content`}>
          {children}
        </Animated.View>
      </GestureDetector>
    </View>
  );
}

interface SwipeActionButtonProps {
  action: SwipeAction;
}

function SwipeActionButton({ action }: SwipeActionButtonProps) {
  // Same explicit press-state tracking as PressableRow/IconButton — a
  // function `style` prop on `Pressable` is silently dropped under
  // NativeWind's JSX interop, so pressed state is plain useState driven by
  // onPressIn/onPressOut, merged into a plain style ARRAY.
  const [pressed, setPressed] = useState(false);
  const handlePressIn = useCallback(() => setPressed(true), []);
  const handlePressOut = useCallback(() => setPressed(false), []);

  return (
    <Pressable
      onPress={action.onPress}
      onPressIn={handlePressIn}
      onPressOut={handlePressOut}
      accessibilityRole="button"
      accessibilityLabel={action.label}
      android_ripple={{ ...theme.press.rippleOnDark, borderless: false }}
      style={[
        styles.actionButton,
        { backgroundColor: TONE_BACKGROUND[action.tone] },
        pressed && Platform.OS === "ios" ? { opacity: theme.press.opacitySolidFill } : null,
      ]}
    >
      {action.icon}
      <Text
        preset="caption"
        color={action.tone === "neutral" ? "text" : "inverse"}
        style={styles.actionLabel}
      >
        {action.label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  container: {
    position: "relative",
  },
  content: {
    backgroundColor: theme.colors.background,
  },
  actionsPanel: {
    position: "absolute",
    top: 0,
    bottom: 0,
    flexDirection: "row",
  },
  leadingPanel: {
    left: 0,
  },
  trailingPanel: {
    right: 0,
  },
  actionButton: {
    width: ACTION_WIDTH,
    minHeight: theme.touchTarget,
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing.xs,
  },
  actionLabel: {
    marginTop: 2,
  },
});
