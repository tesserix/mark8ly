import { useAnimatedScrollHandler, useSharedValue } from "react-native-reanimated";
import type { SharedValue } from "react-native-reanimated";

/**
 * The scroll offset a `CollapsingHeader` reads, plus the handler that feeds
 * it.
 *
 * `CollapsingHeader` deliberately never creates its own scroll handler — the
 * caller owns the shared value so the same one can drive other scroll-linked
 * UI on the screen. That correctly-placed ownership meant every screen
 * hand-rolled the identical four lines, which is how the pair drifts (one
 * screen forgetting `scrollEventThrottle`, another clamping the offset, a
 * third dropping the `"worklet"` directive). One hook, one pair.
 */
export interface CollapsingScroll {
  /** Pass to `CollapsingHeader`'s `scrollY`. */
  scrollY: SharedValue<number>;
  /** Spread onto the Animated scroll view: `onScroll={onScroll}` + `scrollEventThrottle={16}`. */
  onScroll: ReturnType<typeof useAnimatedScrollHandler>;
}

export function useCollapsingScroll(): CollapsingScroll {
  const scrollY = useSharedValue(0);
  // No `Math.max(0, …)` clamp, and that is not a regression for the caller
  // that had one. iOS bounce and pull-to-refresh push `contentOffset.y`
  // negative, but `CollapsingHeader` clamps on BOTH of its branches — the
  // normal path through `interpolate(…, Extrapolation.CLAMP)` and the
  // reduced-motion path through `scrollY.value > 0` — so a negative offset
  // and a clamped 0 produce the identical header state at every text size.
  // The handler is auto-workletised by reanimated's babel plugin; the
  // explicit `"worklet"` directive the two migrated screens carried was
  // belt-and-braces, not load-bearing.
  const onScroll = useAnimatedScrollHandler((event) => {
    scrollY.value = event.contentOffset.y;
  });
  return { scrollY, onScroll };
}
