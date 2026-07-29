/**
 * A direct test of the hook's contract, because nothing else can be one.
 *
 * Both screen suites that use `useCollapsingScroll` mock
 * `useAnimatedScrollHandler` to a passthrough and never invoke the handler —
 * so a hook that wrote `event.contentOffset.x`, or wrote nothing at all,
 * keeps every one of their assertions green while the header on device never
 * collapses (or collapses on horizontal scroll). The axis is asserted here
 * with a DIFFERENT x and y, so swapping them fails.
 */
jest.mock("react-native-reanimated", () => {
  // Minimal virtual mock: reanimated 4.x's real module requires the native
  // Worklets module at import time and throws under jest. `useSharedValue`
  // returns a real mutable box so the write is observable; the scroll-handler
  // factory hands the worklet straight back so the test can invoke it.
  const useSharedValue = (initial: number) => ({ value: initial });
  const useAnimatedScrollHandler = (handler: unknown) => handler;
  return { __esModule: true, useSharedValue, useAnimatedScrollHandler };
});

import { renderHook } from "@testing-library/react-native";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";

type ScrollEvent = { contentOffset: { x: number; y: number } };

function handlerOf(onScroll: unknown): (event: ScrollEvent) => void {
  return onScroll as (event: ScrollEvent) => void;
}

describe("useCollapsingScroll", () => {
  it("starts the shared value at the top of the list", () => {
    const { result } = renderHook(() => useCollapsingScroll());
    expect(result.current.scrollY.value).toBe(0);
  });

  it("writes the VERTICAL offset into scrollY, not the horizontal one", () => {
    const { result } = renderHook(() => useCollapsingScroll());
    // Distinct values on the two axes: an `event.contentOffset.x` hook reads
    // 999 here and fails, rather than passing on a coincidence.
    handlerOf(result.current.onScroll)({ contentOffset: { x: 999, y: 64 } });
    expect(result.current.scrollY.value).toBe(64);
  });

  it("tracks every subsequent offset, including the negative one iOS bounce produces", () => {
    const { result } = renderHook(() => useCollapsingScroll());
    handlerOf(result.current.onScroll)({ contentOffset: { x: 0, y: 120 } });
    expect(result.current.scrollY.value).toBe(120);
    // Deliberately unclamped — `CollapsingHeader` clamps on both of its
    // branches, so a rubber-banded negative offset and a clamped 0 produce
    // the identical header state. A `Math.max(0, …)` added here would fail.
    handlerOf(result.current.onScroll)({ contentOffset: { x: 0, y: -40 } });
    expect(result.current.scrollY.value).toBe(-40);
  });

  it("hands back the same shared value the handler writes to", () => {
    const { result } = renderHook(() => useCollapsingScroll());
    // The pair is the point of the hook: a screen that passed the handler to
    // its scroll view and a DIFFERENT shared value to the header would show a
    // header that never moves.
    handlerOf(result.current.onScroll)({ contentOffset: { x: 0, y: 32 } });
    expect(result.current.scrollY.value).toBe(32);
  });
});
