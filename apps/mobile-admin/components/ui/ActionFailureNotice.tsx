import { useEffect, useRef, useState } from "react";
import {
  AccessibilityInfo,
  Animated,
  Platform,
  Pressable,
  StyleSheet,
  View,
} from "react-native";
import { useReducedMotion } from "react-native-reanimated";
import { AlertTriangle, X } from "lucide-react-native";
import { Text } from "./Text";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import { theme } from "@/lib/theme";
import type { ActionFailure } from "@/lib/use-action-failure";

/**
 * The one place a merchant is told a mutation failed.
 *
 * Before this, a failed activate / archive / block / close produced a failure
 * HAPTIC and nothing else. On Customers that made success and failure
 * literally indistinguishable: `CustomerRow` carries no status badge, so
 * under the default "All" filter a blocked customer's row looks the same
 * either way — and a merchant who did not feel the haptic, or who cannot,
 * got no confirmation of any kind.
 *
 * Deliberately NOT an `Alert.alert`. Alert is modal and interruptive; during
 * triage a merchant fires several rows before any of them settle, and three
 * failures would become three dialogs to tap through before the list could be
 * touched again. This is a non-blocking strip that REPLACES itself, so any
 * rate of failure stays one readable message (see `useActionFailure`).
 *
 * Anchored at the bottom, above the floating Ink dock, because that is where
 * the thumb already is — the dismiss control has to be reachable one-handed,
 * and a top-anchored banner puts it at the far end of the screen.
 *
 * `--danger` here is FUNCTIONAL, not decorative, so it does not spend the
 * screen's one accent. Oxblood #8B2E20 on the #F6E4E1 danger tint is 6.82:1.
 * The warning glyph carries the same meaning without relying on colour
 * (WCAG 2.1 SC 1.4.1).
 */
export interface ActionFailureNoticeProps {
  /** The failure to show, or `null` to show nothing at all. */
  failure: ActionFailure | null;
  onDismiss: () => void;
  /**
   * Extra clearance ABOVE the dock, in points. ADDITIVE — defaults to 0, so
   * a screen with nothing else floating there is unaffected.
   *
   * Products needs it: its add-product FAB is anchored at the same
   * `useDockClearance()` bottom and would otherwise sit directly on top of
   * this strip's dismiss button.
   */
  bottomOffset?: number;
}

/** How far the strip travels on entry. Small: it is a report, not an event. */
const ENTER_TRAVEL = 10;
const ENTER_DURATION = 180;
const ICON_SIZE = 18;

export function ActionFailureNotice({
  failure,
  onDismiss,
  bottomOffset = 0,
}: ActionFailureNoticeProps) {
  const dockClearance = useDockClearance();
  const reduceMotion = useReducedMotion();
  // Press state tracked explicitly rather than via `Pressable`'s
  // `style={({pressed}) => …}` callback form: under NativeWind's JSX interop
  // a FUNCTION style prop is silently dropped at runtime while every unit
  // test stays green (RNTL renders without the NativeWind runtime). This
  // shipped once across 41 call sites. Keep it a plain array.
  const [pressed, setPressed] = useState(false);

  const progress = useRef(new Animated.Value(0)).current;
  const key = failure?.key ?? null;

  useEffect(() => {
    if (key === null) return;
    if (reduceMotion) return;
    progress.setValue(0);
    const animation = Animated.timing(progress, {
      toValue: 1,
      duration: ENTER_DURATION,
      useNativeDriver: true,
    });
    animation.start();
    return () => animation.stop();
  }, [key, reduceMotion, progress]);

  /**
   * Announce it, don't just show it.
   *
   * Keyed on `failure.key` rather than on the copy: the same action failing
   * twice produces identical text, and a merchant on VoiceOver must hear the
   * second failure too. Android's assertive live region already speaks the
   * strip when it mounts, so announcing there as well would say it twice.
   */
  const announcedKey = useRef<number | null>(null);
  useEffect(() => {
    if (failure === null) {
      announcedKey.current = null;
      return;
    }
    if (announcedKey.current === failure.key) return;
    announcedKey.current = failure.key;
    if (Platform.OS !== "android") {
      AccessibilityInfo.announceForAccessibility(`${failure.title}. ${failure.detail}`);
    }
  }, [failure]);

  // After the hooks, never before them.
  if (failure === null) return null;

  const motionStyle = reduceMotion
    ? styles.settled
    : {
        opacity: progress,
        transform: [
          {
            translateY: progress.interpolate({
              inputRange: [0, 1],
              outputRange: [ENTER_TRAVEL, 0],
            }),
          },
        ],
      };

  return (
    <Animated.View
      testID="action-failure-notice"
      // The strip covers part of the list; only the card itself may take
      // touches, so a tap beside it still reaches the row underneath.
      pointerEvents="box-none"
      style={[styles.wrap, { bottom: dockClearance + bottomOffset }, motionStyle]}
    >
      <View style={styles.card} testID="action-failure-card">
        <AlertTriangle
          size={ICON_SIZE}
          color={theme.colors.danger}
          strokeWidth={2}
          // Optically aligned to the title's cap height rather than its line
          // box, which is what makes an icon look "half a pixel low".
          style={styles.icon}
        />
        {/* One VoiceOver stop for the whole message: the title alone
            ("Couldn't archive this product") is not the information — the
            reason underneath it is. */}
        <View
          testID="action-failure-copy"
          style={styles.copy}
          accessible
          accessibilityRole="alert"
          accessibilityLiveRegion="assertive"
          accessibilityLabel={`${failure.title}. ${failure.detail}`}
        >
          <Text testID="action-failure-title" preset="bodyEmphasis" color="danger">
            {failure.title}
          </Text>
          {/* Ink, not oxblood: the reason is the part that gets READ, and the
              danger tone is already carried by the title, the glyph and the
              field. No `numberOfLines` — this text scales and the box holding
              it does not pin a height. */}
          <Text testID="action-failure-detail" preset="body" color="text">
            {failure.detail}
          </Text>
        </View>
        <Pressable
          testID="action-failure-dismiss"
          accessibilityRole="button"
          accessibilityLabel="Dismiss"
          onPress={onDismiss}
          onPressIn={() => setPressed(true)}
          onPressOut={() => setPressed(false)}
          android_ripple={theme.press.rippleDanger}
          style={[styles.dismiss, pressed ? styles.dismissPressed : null]}
        >
          <X size={ICON_SIZE} color={theme.colors.danger} strokeWidth={2} />
        </Pressable>
      </View>
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    position: "absolute",
    // `bottom` is applied at the call site from `useDockClearance()`. The
    // floating Ink dock is absolutely positioned, 64pt tall, at
    // `insets.bottom + DOCK_BOTTOM_GAP`; a static bottom would park this
    // strip underneath it exactly as it once parked the Products FAB.
    left: theme.spacing.lg,
    right: theme.spacing.lg,
  },
  card: {
    flexDirection: "row",
    // Top-aligned, not centred: the message wraps to several lines at an
    // accessibility text size and a centred glyph would drift to the middle
    // of the paragraph.
    alignItems: "flex-start",
    gap: theme.spacing.md,
    paddingLeft: theme.spacing.lg,
    // Tighter on the right because the 44pt dismiss box carries its own
    // optical padding around a 18pt glyph.
    paddingRight: theme.spacing.xs,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.dangerTint,
    borderRadius: theme.radius,
    // A hairline rule, the app's own surface vocabulary — not a 1pt box.
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: theme.colors.danger,
    shadowColor: theme.colors.text,
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.12,
    shadowRadius: 12,
    elevation: 4,
  },
  icon: { marginTop: 3 },
  // NO height and NO maxHeight, here or on any ancestor. Seven silent
  // clipping bugs in this app have had the same shape — scalable text inside
  // a box that does not scale — the most recent being sheets whose fixed snap
  // point hid their own buttons at accessibility sizes. The strip is as tall
  // as its message.
  copy: { flex: 1, gap: 2 },
  dismiss: {
    width: theme.touchTarget,
    height: theme.touchTarget,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radius,
  },
  dismissPressed: { opacity: theme.press.opacityStandard },
  settled: { opacity: 1, transform: [{ translateY: 0 }] },
});
