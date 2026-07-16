import { useState, type ReactNode } from "react";
import { Pressable, View, StyleSheet } from "react-native";
import Animated, { FadeIn, FadeOut, useReducedMotion } from "react-native-reanimated";
import { ChevronDown } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { DISCLOSURE_DURATION, DISCLOSURE_EASING, useChevronRotationStyle } from "./disclosure-motion";

interface SectionDisclosureProps {
  title: string;
  defaultOpen?: boolean;
  children: ReactNode;
}

/**
 * Generic reduced-motion-aware expand/collapse. Used standalone (the nested
 * "Shipping & dimensions" block inside VariantEditor) — VariantRow needs a
 * richer summary (label/caption/badge) than a plain title, so it re-implements
 * the same reduced-motion contract rather than wrapping this component.
 *
 * Body is only mounted while open — collapsed content isn't just visually
 * hidden, it isn't in the tree at all (cheaper, and what the tests assert on).
 */
export function SectionDisclosure({ title, defaultOpen = false, children }: SectionDisclosureProps) {
  const [open, setOpen] = useState(defaultOpen);
  const reduceMotion = useReducedMotion();
  const chevronStyle = useChevronRotationStyle(open, reduceMotion);

  return (
    <View>
      <Pressable
        onPress={() => setOpen((current) => !current)}
        accessibilityRole="button"
        accessibilityState={{ expanded: open }}
        accessibilityLabel={`${title}, ${open ? "expanded" : "collapsed"}`}
        style={styles.header}
        hitSlop={4}
      >
        <Text preset="bodyEmphasis" color="text">
          {title}
        </Text>
        <Animated.View style={chevronStyle}>
          <ChevronDown size={18} color={theme.colors.textSecondary} strokeWidth={2} />
        </Animated.View>
      </Pressable>
      {open ? (
        <Animated.View
          testID="section-disclosure-body"
          entering={
            reduceMotion ? undefined : FadeIn.duration(DISCLOSURE_DURATION).easing(DISCLOSURE_EASING)
          }
          exiting={
            reduceMotion ? undefined : FadeOut.duration(DISCLOSURE_DURATION).easing(DISCLOSURE_EASING)
          }
        >
          {children}
        </Animated.View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  header: {
    minHeight: theme.touchTarget,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: theme.spacing.sm,
  },
});
