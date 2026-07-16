import { Easing, useAnimatedStyle, withTiming, type LayoutAnimation } from "react-native-reanimated";

// App-standard ease-out-quart — no bounce, no overshoot. Mirrors Dock.tsx's
// ENTRANCE_EASING so every expand/collapse in the app reads as the same motion.
export const DISCLOSURE_EASING = Easing.bezier(0.22, 1, 0.36, 1);
export const DISCLOSURE_DURATION = 220;

// A collapse is a system response to a tap, not a deliberate reveal — the eye
// already knows where the content is going, so it should snap back faster
// than the open took. Kept as its own constant (not derived from
// DISCLOSURE_DURATION) so the asymmetry reads as intentional, not a
// copy-paste drift the next time someone tunes one of the two.
export const DISCLOSURE_EXIT_DURATION = 160;

// Distance (px) the disclosure body settles in from / lifts out to on
// reveal/collapse — small enough to read as physical settling, not a slide.
const REVEAL_OFFSET = 6;
const COLLAPSE_OFFSET = 4;

/**
 * Custom entering animation for a disclosure body: fades in while settling
 * down `REVEAL_OFFSET`px, so a reveal reads as content dropping into place
 * rather than a flat cross-fade.
 *
 * Deliberately NOT `FadeIn.withInitialValues({ transform: [...] })`: reanimated
 * only interpolates a style key when it also appears under the returned
 * `animations` map. Plain `FadeIn`'s `animations` only contains `opacity`, so
 * seeding a `transform` solely via `withInitialValues` would apply it once
 * and leave it there for good — a permanently-offset view, not a settling
 * one. Defining both `initialValues` and `animations` for `transform` here
 * keeps the two in sync.
 */
export function disclosureEntering(): LayoutAnimation {
  "worklet";
  return {
    initialValues: { opacity: 0, transform: [{ translateY: -REVEAL_OFFSET }] },
    animations: {
      opacity: withTiming(1, { duration: DISCLOSURE_DURATION, easing: DISCLOSURE_EASING }),
      transform: [
        { translateY: withTiming(0, { duration: DISCLOSURE_DURATION, easing: DISCLOSURE_EASING }) },
      ],
    },
  };
}

/**
 * Mirrors `disclosureEntering` for exit: fades out while lifting
 * `COLLAPSE_OFFSET`px, on the shorter `DISCLOSURE_EXIT_DURATION` — the
 * asymmetric collapse-is-snappier half of the same reveal/collapse pair.
 */
export function disclosureExiting(): LayoutAnimation {
  "worklet";
  return {
    initialValues: { opacity: 1, transform: [{ translateY: 0 }] },
    animations: {
      opacity: withTiming(0, { duration: DISCLOSURE_EXIT_DURATION, easing: DISCLOSURE_EASING }),
      transform: [
        {
          translateY: withTiming(-COLLAPSE_OFFSET, {
            duration: DISCLOSURE_EXIT_DURATION,
            easing: DISCLOSURE_EASING,
          }),
        },
      ],
    },
  };
}

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
