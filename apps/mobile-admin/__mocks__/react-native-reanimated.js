// Global manual mock for react-native-reanimated, applied automatically by
// Jest to every test that transitively imports it (no `jest.mock(...)` call
// needed — see https://jestjs.io/docs/manual-mocks#mocking-node-modules).
//
// Why this exists: react-native-reanimated 4.x's real module (and even its
// shipped mock.js) requires the native Worklets module at import time, which
// throws under jest ("Native part of Worklets doesn't seem to be
// initialized"). `components/ui/CollapsingHeader.tsx` is the first
// `components/ui` barrel file to import reanimated, so every test that
// imports ANYTHING from `@/components/ui` (even unrelated components, via
// the shared barrel) now transitively requires this module. Without this
// mock those suites fail at import time, not because their own code is
// wrong.
//
// A test file that needs to assert on specific animation behaviour (e.g.
// reduced-motion branching) should still add its own `jest.mock(
// "react-native-reanimated", () => {...})` — an inline factory in the test
// file always wins over this one. Keep this file's surface to the minimum
// needed to load without throwing; grow it only as more `components/ui`
// files start depending on reanimated (increment 2 adds several).
const { useRef } = require("react");
const { View, ScrollView } = require("react-native");

function interpolate(value, inputRange, outputRange) {
  const [inMin, inMax] = inputRange;
  const [outMin, outMax] = outputRange;
  const t = Math.max(0, Math.min(1, (value - inMin) / (inMax - inMin)));
  return outMin + t * (outMax - outMin);
}

// Added for Task 4 (SwipeRow): a real reanimated `useSharedValue` returns a
// stable mutable ref-like object across re-renders (mutating `.value` never
// itself triggers a React re-render). Backing this with `useRef` reproduces
// that persistence under jest — a naive `{ value: initial }` literal would
// be recreated (and reset) on every render, which would silently drop
// in-progress gesture state (translateX, "has this drag crossed the
// threshold yet") the moment anything else re-rendered the component.
function useSharedValue(initial) {
  const ref = useRef(undefined);
  if (ref.current === undefined) {
    ref.current = { value: initial };
  }
  return ref.current;
}

// Real `runOnJS` hops a worklet's call back to the JS thread; under jest
// there is no separate thread, so it's a same-tick passthrough.
function runOnJS(fn) {
  return (...args) => fn(...args);
}

// Real `withSpring`/`withTiming` return animation descriptors that reanimated
// resolves over several frames on the UI thread. Under jest there is no
// frame loop, so both resolve straight to the target value — sufficient to
// assert the FINAL rest state (e.g. "springs back to translateX 0") without
// simulating the animation curve itself.
function withSpring(toValue) {
  return toValue;
}

function withTiming(toValue) {
  return toValue;
}

// Added for Task 8 (Dashboard): the screen owns the `scrollY` shared value
// that drives `CollapsingHeader`, so it renders an `Animated.ScrollView`
// wired to a `useAnimatedScrollHandler`. Under jest there is no UI thread
// and no scroll events, so the handler is returned as a plain function
// (never invoked by RNTL) and `Animated.ScrollView` is the real RN
// `ScrollView`, which renders its children synchronously — exactly what a
// screen test needs.
function useAnimatedScrollHandler(handlerOrHandlers) {
  return typeof handlerOrHandlers === "function"
    ? handlerOrHandlers
    : (handlerOrHandlers && handlerOrHandlers.onScroll) || (() => {});
}

module.exports = {
  __esModule: true,
  default: { View, ScrollView },
  useAnimatedScrollHandler,
  Extrapolation: { CLAMP: "clamp", EXTEND: "extend", IDENTITY: "identity" },
  interpolate,
  useAnimatedStyle: (factory) => factory(),
  useDerivedValue: (factory) => ({ value: factory() }),
  useReducedMotion: jest.fn(() => false),
  useSharedValue,
  runOnJS,
  withSpring,
  withTiming,
};
