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
const { View } = require("react-native");

function interpolate(value, inputRange, outputRange) {
  const [inMin, inMax] = inputRange;
  const [outMin, outMax] = outputRange;
  const t = Math.max(0, Math.min(1, (value - inMin) / (inMax - inMin)));
  return outMin + t * (outMax - outMin);
}

module.exports = {
  __esModule: true,
  default: { View },
  Extrapolation: { CLAMP: "clamp", EXTEND: "extend", IDENTITY: "identity" },
  interpolate,
  useAnimatedStyle: (factory) => factory(),
  useDerivedValue: (factory) => ({ value: factory() }),
  useReducedMotion: jest.fn(() => false),
};
