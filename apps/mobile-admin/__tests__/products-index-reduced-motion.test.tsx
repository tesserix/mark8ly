// The products list screen crossfades its loaded FlatList in (rather than a
// hard pop from the spinner) via an Animated.View wrapper — this pins that
// the crossfade is gated on useReducedMotion() the same way every other
// custom motion site in the app is: instant (undefined `entering`) when
// reduced motion is on, a real animation otherwise.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-reanimated", () => {
  const { View } = require("react-native");
  class ChainableAnimation {
    duration() {
      return this;
    }
    easing() {
      return this;
    }
  }
  return {
    __esModule: true,
    default: { View },
    FadeIn: new ChainableAnimation(),
    Easing: { bezier: () => (t: number) => t },
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
