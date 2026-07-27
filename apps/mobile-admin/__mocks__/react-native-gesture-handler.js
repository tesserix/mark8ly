// Global manual mock for react-native-gesture-handler, applied automatically
// by Jest to every test that transitively imports it (no `jest.mock(...)`
// call needed — see https://jestjs.io/docs/manual-mocks#mocking-node-modules
// and the sibling `react-native-reanimated.js` mock in this directory, which
// this follows the same convention as).
//
// Why this exists: `components/ui/SwipeRow.tsx` (Task 4) is the first
// `components/ui` barrel file to import react-native-gesture-handler, so
// every test that imports ANYTHING from `@/components/ui` (even unrelated
// components, via the shared barrel) now transitively requires this module.
// The real native module throws under jest (no native Gesture Handler runtime
// registered), so without this mock those suites fail at import time, not
// because their own code is wrong — the exact class of bug the reanimated
// mock's header describes.
//
// `Gesture.Pan()` returns a chainable builder that just records each handler
// function by name instead of wiring up real native gesture recognition.
// `GestureDetector` is a pure passthrough — it renders `children` unchanged,
// stashing the `gesture` prop it received so a test can retrieve it via
// `__getLastGesture()` and invoke `.handlers.onUpdate(...)` /
// `.handlers.onEnd(...)` directly, driving the same worklet bodies the real
// gesture would call, without needing a real drag.
let lastGesture = null;

const CHAIN_METHODS = [
  "enabled",
  "onBegin",
  "onStart",
  "onUpdate",
  "onChange",
  "onEnd",
  "onFinalize",
  "activeOffsetX",
  "activeOffsetY",
  "failOffsetX",
  "failOffsetY",
  "minDistance",
  "shouldCancelWhenOutside",
  "simultaneousWithExternalGesture",
  "requireExternalGestureToFail",
  "withRef",
];

function createGestureBuilder() {
  const handlers = {};
  const builder = { handlers };
  for (const method of CHAIN_METHODS) {
    builder[method] = (value) => {
      handlers[method] = value;
      return builder;
    };
  }
  return builder;
}

function GestureDetector({ gesture, children }) {
  lastGesture = gesture;
  return children;
}

module.exports = {
  __esModule: true,
  Gesture: {
    Pan: createGestureBuilder,
    Tap: createGestureBuilder,
    LongPress: createGestureBuilder,
  },
  GestureDetector,
  GestureHandlerRootView: require("react-native").View,
  // Test-only escape hatch — not part of the real module's API. Returns the
  // `gesture` most recently passed to `GestureDetector`, so a test can drive
  // its `.handlers.onUpdate`/`.handlers.onEnd` directly.
  __getLastGesture: () => lastGesture,
};
