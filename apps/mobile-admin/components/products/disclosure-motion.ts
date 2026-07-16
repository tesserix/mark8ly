import { Easing, useAnimatedStyle, withTiming } from "react-native-reanimated";

// App-standard ease-out-quart — no bounce, no overshoot. Mirrors Dock.tsx's
// ENTRANCE_EASING so every expand/collapse in the app reads as the same motion.
export const DISCLOSURE_EASING = Easing.bezier(0.22, 1, 0.36, 1);
export const DISCLOSURE_DURATION = 220;

/**
 * Chevron rotation shared by `SectionDisclosure` and `VariantRow` — both flip
 * a chevron 180° on open/close. Reduced motion collapses the timing to 0ms
 * (instant, not merely fast): the same reduced-motion contract Dock.tsx uses
 * for its `entering` prop (`reduceMotion ? undefined : FadeIn...`), applied
 * here to a continuous transform instead of a mount animation — there's no
 * "undefined" for a running rotation, so instant is the closest equivalent.
 *
 * GPU-cheap: only `transform` is driven, never a layout-affecting property.
 */
export function useChevronRotationStyle(open: boolean, reduceMotion: boolean) {
  return useAnimatedStyle(
    () => ({
      transform: [
        {
          rotate: withTiming(open ? "180deg" : "0deg", {
            duration: reduceMotion ? 0 : DISCLOSURE_DURATION,
            easing: DISCLOSURE_EASING,
          }),
        },
      ],
    }),
    [open, reduceMotion],
  );
}
