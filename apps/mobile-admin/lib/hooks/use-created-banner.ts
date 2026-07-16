import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import type { ScrollView } from "react-native";
import * as Haptics from "expo-haptics";
import { useReducedMotion } from "react-native-reanimated";

export type CreatedBannerSection = "photos" | "options" | "variants";

/**
 * Orchestrates the post-create hand-off banner on the product detail screen:
 * shown once when the create screen hands off via `?created=1` and not yet
 * locally dismissed; fires a landing haptic exactly once; scroll-jumps to a
 * section using its captured y-offset (set via each section's `onLayout`).
 *
 * Pulled into its own hook — rather than inlined in [id].tsx — to keep that
 * screen under its pinned line-count regression test
 * (__tests__/product-detail-sections.test.tsx).
 */
export function useCreatedBanner(created: string | undefined, scrollRef: RefObject<ScrollView | null>) {
  const [dismissed, setDismissed] = useState(false);
  const hasFiredLandingHaptic = useRef(false);
  const sectionOffsets = useRef<Partial<Record<CreatedBannerSection, number>>>({});
  const reduceMotion = useReducedMotion();

  const show = created === "1" && !dismissed;

  useEffect(() => {
    if (created === "1" && !hasFiredLandingHaptic.current) {
      hasFiredLandingHaptic.current = true;
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    }
  }, [created]);

  const registerSectionOffset = useCallback((section: CreatedBannerSection, y: number) => {
    sectionOffsets.current[section] = y;
  }, []);

  const jumpTo = useCallback(
    (section: CreatedBannerSection) => {
      const y = sectionOffsets.current[section];
      if (y !== undefined) {
        scrollRef.current?.scrollTo({ y: Math.max(y - 16, 0), animated: !reduceMotion });
      }
    },
    [scrollRef, reduceMotion],
  );

  const dismiss = useCallback(() => setDismissed(true), []);

  return { show, dismiss, registerSectionOffset, jumpTo };
}
