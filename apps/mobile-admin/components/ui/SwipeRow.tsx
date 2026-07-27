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
  /**
   * Fire on a full swipe past `FULL_SWIPE_FRACTION` without requiring a tap.
   * Leave false for anything destructive — this app has no undo.
   *
   * Hard-blocked when `tone` is "danger", regardless of this flag: the
   * app-wide convention puts destructive actions (cancel order, block
   * customer, reject review) on the trailing/danger edge, and a full swipe
   * can be released by accident. Opting a non-destructive action in is a
   * deliberate caller decision; opting a destructive one in is not honoured.
   */
  autoFireOnFullSwipe?: boolean;
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
 * settles the row OPEN (resting at `ACTION_WIDTH * action count` on that
 * side) instead of springing shut. Crossing this alone never fires
 * anything — the revealed action buttons still require their own tap. See
 * `FULL_SWIPE_FRACTION` for the opt-in auto-fire gesture.
 */
const THRESHOLD_FRACTION = 0.4;
/**
 * Fraction of the row's own measured width a drag must cross before release
 * auto-fires the primary (first) action on that side, iOS-Mail-style —
 * deliberately much larger than `THRESHOLD_FRACTION` so the same drag that
 * merely opens the row can never also trigger it. Only honoured when that
 * action sets `autoFireOnFullSwipe` (and isn't `tone: "danger"` — see
 * `SwipeAction`).
 */
const FULL_SWIPE_FRACTION = 0.85;
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
  // Snapshot of `translateX` at the start of the CURRENT drag. Needed now
  // that a release can settle the row open (non-zero rest position): the
  // next drag must continue from wherever the row actually is, not jump to
  // 0, since `event.translationX` is always relative to that drag's own
  // touch-down point.
  const dragStartX = useSharedValue(0);
  // Tracks whether the CURRENT drag has already crossed the reveal
  // threshold, so the haptic fires once per crossing rather than once per
  // `onUpdate` frame. Reset on release and whenever the drag recedes back
  // under the threshold, so dragging past it, back, and past it again
  // counts as two separate crossings — matching how the haptic reads to a
  // user's thumb.
  const hasCrossed = useSharedValue(false);
  // Drives the tap-to-close overlay (see below). Only needs to exist on the
  // JS thread — it's set from `runOnJS` when a drag settles open/closed and
  // directly from `closeRow` when a revealed action or the overlay is
  // tapped.
  const [isOpen, setIsOpen] = useState(false);

  const hasLeading = leadingActions.length > 0;
  const hasTrailing = trailingActions.length > 0;
  const primaryLeading = leadingActions[0];
  const primaryTrailing = trailingActions[0];
  const openLeadingWidth = ACTION_WIDTH * leadingActions.length;
  const openTrailingWidth = ACTION_WIDTH * trailingActions.length;

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

  // Springs (or snaps, under reduced motion) back to rest without firing
  // anything. Shared by: tapping the row content while open, tapping the
  // tap-to-close overlay, and firing a revealed action button (which closes
  // immediately after).
  const closeRow = useCallback(() => {
    translateX.value = reduceMotion ? 0 : withSpring(0);
    setIsOpen(false);
  }, [reduceMotion, translateX]);

  const pan = Gesture.Pan()
    .enabled(enabled)
    .onStart(() => {
      "worklet";
      dragStartX.value = translateX.value;
    })
    .onUpdate((event) => {
      "worklet";
      // Belt-and-suspenders with `.enabled(enabled)` above: guard explicitly
      // rather than relying solely on the native gesture recognizer, the
      // same pattern PressableRow/IconButton use for their `disabled` prop.
      if (!enabled) return;

      let next = dragStartX.value + event.translationX;
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
      hasCrossed.value = false;

      const current = translateX.value;
      const threshold = rowWidth.value * THRESHOLD_FRACTION;
      const fullSwipeThreshold = rowWidth.value * FULL_SWIPE_FRACTION;
      const passedThreshold = threshold > 0 && Math.abs(current) >= threshold;
      const passedFullSwipe = fullSwipeThreshold > 0 && Math.abs(current) >= fullSwipeThreshold;

      if (current > 0 && hasLeading) {
        const canAutoFire =
          passedFullSwipe &&
          !!primaryLeading?.autoFireOnFullSwipe &&
          primaryLeading.tone !== "danger";
        if (canAutoFire) {
          translateX.value = reduceMotion ? 0 : withSpring(0);
          runOnJS(setIsOpen)(false);
          runOnJS(primaryLeading.onPress)();
          return;
        }
        if (passedThreshold) {
          translateX.value = reduceMotion ? openLeadingWidth : withSpring(openLeadingWidth);
          runOnJS(setIsOpen)(true);
          return;
        }
      } else if (current < 0 && hasTrailing) {
        const canAutoFire =
          passedFullSwipe &&
          !!primaryTrailing?.autoFireOnFullSwipe &&
          primaryTrailing.tone !== "danger";
        if (canAutoFire) {
          translateX.value = reduceMotion ? 0 : withSpring(0);
          runOnJS(setIsOpen)(false);
          runOnJS(primaryTrailing.onPress)();
          return;
        }
        if (passedThreshold) {
          translateX.value = reduceMotion ? -openTrailingWidth : withSpring(-openTrailingWidth);
          runOnJS(setIsOpen)(true);
          return;
        }
      }

      translateX.value = reduceMotion ? 0 : withSpring(0);
      runOnJS(setIsOpen)(false);
    });

  const contentStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: translateX.value }],
  }));

  return (
    <View style={styles.container} onLayout={handleLayout} testID={testID}>
      {hasLeading ? (
        <View style={[styles.actionsPanel, styles.leadingPanel]} pointerEvents="box-none">
          {leadingActions.map((action) => (
            <SwipeActionButton
              key={action.key}
              action={action}
              onActivate={() => {
                action.onPress();
                closeRow();
              }}
              testID={`${testID}-action-${action.key}`}
            />
          ))}
        </View>
      ) : null}
      {hasTrailing ? (
        <View style={[styles.actionsPanel, styles.trailingPanel]} pointerEvents="box-none">
          {trailingActions.map((action) => (
            <SwipeActionButton
              key={action.key}
              action={action}
              onActivate={() => {
                action.onPress();
                closeRow();
              }}
              testID={`${testID}-action-${action.key}`}
            />
          ))}
        </View>
      ) : null}
      <GestureDetector gesture={pan}>
        <Animated.View style={[styles.content, contentStyle]} testID={`${testID}-content`}>
          {children}
          {isOpen ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="Close swipe actions"
              onPress={closeRow}
              style={styles.closeOverlay}
              testID={`${testID}-close-overlay`}
            />
          ) : null}
        </Animated.View>
      </GestureDetector>
    </View>
  );
}

interface SwipeActionButtonProps {
  action: SwipeAction;
  /** Fires `action.onPress` and then closes the row — see `closeRow` in the
   * parent. Kept distinct from `action.onPress` so this component never
   * needs to know about the row's open/close state. */
  onActivate: () => void;
  testID: string;
}

function SwipeActionButton({ action, onActivate, testID }: SwipeActionButtonProps) {
  // Same explicit press-state tracking as PressableRow/IconButton — a
  // function `style` prop on `Pressable` is silently dropped under
  // NativeWind's JSX interop, so pressed state is plain useState driven by
  // onPressIn/onPressOut, merged into a plain style ARRAY.
  const [pressed, setPressed] = useState(false);
  const handlePressIn = useCallback(() => setPressed(true), []);
  const handlePressOut = useCallback(() => setPressed(false), []);

  return (
    <Pressable
      onPress={onActivate}
      onPressIn={handlePressIn}
      onPressOut={handlePressOut}
      accessibilityRole="button"
      accessibilityLabel={action.label}
      android_ripple={{ ...theme.press.rippleOnDark, borderless: false }}
      testID={testID}
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
  // Transparent tap target painted over `children` only while the row is
  // resting open — swallows the tap (closing the row) instead of letting it
  // fall through to the row content's own `onPress`.
  closeOverlay: StyleSheet.absoluteFill,
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
