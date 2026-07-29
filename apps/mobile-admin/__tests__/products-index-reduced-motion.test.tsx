// The products list screen crossfades its loaded FlatList in (rather than a
// hard pop from the spinner) via an Animated.View wrapper — this pins that
// the crossfade is gated on useReducedMotion() the same way every other
// custom motion site in the app is: instant (undefined `entering`) when
// reduced motion is on, a real animation otherwise.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

// Grown in inc3 Task 2: the screen now renders a scroll-driven
// `CollapsingHeader` over an `Animated.FlatList`, and per-row `SwipeRow`s, so
// this local mock needs the shared-value/scroll-handler/interpolation surface
// as well as the `FadeIn` this suite is actually about. The ASSERTIONS below
// are untouched — this is a test-environment gap, not a semantics change.
jest.mock("react-native-reanimated", () => {
  const { View, FlatList } = require("react-native");
  const { useRef } = require("react");
  class ChainableAnimation {
    duration() {
      return this;
    }
    easing() {
      return this;
    }
  }
  function interpolate(
    value: number,
    inputRange: [number, number],
    outputRange: [number, number],
  ) {
    const [inMin, inMax] = inputRange;
    const [outMin, outMax] = outputRange;
    const t = Math.max(0, Math.min(1, (value - inMin) / (inMax - inMin)));
    return outMin + t * (outMax - outMin);
  }
  return {
    __esModule: true,
    default: { View, FlatList },
    FadeIn: new ChainableAnimation(),
    Easing: { bezier: () => (t: number) => t },
    Extrapolation: { CLAMP: "clamp", EXTEND: "extend", IDENTITY: "identity" },
    interpolate,
    useAnimatedStyle: (factory: () => unknown) => factory(),
    useDerivedValue: (factory: () => number) => ({ value: factory() }),
    useAnimatedScrollHandler: (handler: unknown) => handler,
    useSharedValue: (initial: number) => {
      const ref = useRef(undefined) as { current: { value: number } | undefined };
      if (ref.current === undefined) ref.current = { value: initial };
      return ref.current;
    },
    runOnJS: (fn: (...args: unknown[]) => unknown) => fn,
    withSpring: (toValue: number) => toValue,
    withTiming: (toValue: number) => toValue,
    useReducedMotion: jest.fn(() => false),
  };
});

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

jest.mock("expo-router", () => ({
  useRouter: () => ({ push: jest.fn() }),
}));

jest.mock("@/lib/hooks/use-products", () => ({
  useProducts: () => ({
    data: { data: [] },
    isLoading: false,
    isRefetching: false,
    refetch: jest.fn(),
  }),
}));

// The screen's status swipes/menu (inc3 Task 2) reach a real react-query
// mutation, which pulls in the api client and, through it, the auth provider
// — a module chain this suite has no reason to boot.
jest.mock("@/lib/admin-api/product-status", () => ({
  useSetProductStatus: () => ({ mutate: jest.fn(), isPending: false }),
}));
// Task 12's quick-edit mutation reaches the same api-client/auth-provider
// module chain the status mutation above does, and for the same reason is
// stubbed rather than booted.
jest.mock("@/lib/admin-api/variant-quick-edit", () => ({
  useQuickEditVariant: () => ({ mutate: jest.fn(), isPending: false }),
}));

import { render } from "@testing-library/react-native";
import ProductsScreen from "../app/(tabs)/products/index";

describe("ProductsScreen — list crossfade reduced motion", () => {
  const reanimated = jest.requireMock("react-native-reanimated") as {
    useReducedMotion: jest.Mock;
  };

  afterEach(() => {
    reanimated.useReducedMotion.mockReturnValue(false);
  });

  it("passes an undefined (instant) entering animation on the list wrapper when reduced motion is enabled", () => {
    reanimated.useReducedMotion.mockReturnValue(true);
    const { getByTestId } = render(<ProductsScreen />);
    expect(getByTestId("products-list-wrap").props.entering).toBeUndefined();
  });

  it("passes a real entering animation on the list wrapper when reduced motion is off", () => {
    reanimated.useReducedMotion.mockReturnValue(false);
    const { getByTestId } = render(<ProductsScreen />);
    expect(getByTestId("products-list-wrap").props.entering).toBeDefined();
  });
});
