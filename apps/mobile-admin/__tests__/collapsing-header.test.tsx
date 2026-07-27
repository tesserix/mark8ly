// react-native-reanimated 4.x's real module (and even its shipped mock.js)
// requires the native Worklets module at import time, which throws under
// jest ("Native part of Worklets doesn't seem to be initialized"). Hand-roll
// a minimal virtual mock covering only what CollapsingHeader uses — enough
// to observe the interpolated opacity/height the tests below assert on.
// `interpolate` is a real (simplified, 2-point, clamped) linear
// implementation rather than a stub, because the behaviour under test IS
// the interpolation math (collapsed vs expanded crossover at offset 64).
jest.mock("react-native-reanimated", () => {
  const { View } = require("react-native");

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
    default: { View },
    Extrapolation: { CLAMP: "clamp" },
    interpolate,
    useAnimatedStyle: (factory: () => unknown) => factory(),
    useDerivedValue: (factory: () => number) => ({ value: factory() }),
    useReducedMotion: jest.fn(() => false),
  };
});

import { StyleSheet } from "react-native";
import { render } from "@testing-library/react-native";
import type { ReactTestInstance } from "react-test-renderer";
import type { SharedValue } from "react-native-reanimated";
import { CollapsingHeader } from "@/components/ui/CollapsingHeader";

function sharedValue(value: number): SharedValue<number> {
  return { value } as unknown as SharedValue<number>;
}

function opacityOf(node: ReactTestInstance): number | undefined {
  return (StyleSheet.flatten(node.props.style) as { opacity?: number }).opacity;
}

describe("CollapsingHeader", () => {
  it("renders fully expanded at scroll offset 0", () => {
    const { getByTestId } = render(
      <CollapsingHeader title="Orders" scrollY={sharedValue(0)} />,
    );
    expect(opacityOf(getByTestId("collapsing-header-expanded"))).toBe(1);
    expect(opacityOf(getByTestId("collapsing-header-collapsed"))).toBe(0);
  });

  it("renders fully collapsed at scroll offset >= 64", () => {
    const { getByTestId } = render(
      <CollapsingHeader title="Orders" scrollY={sharedValue(64)} />,
    );
    expect(opacityOf(getByTestId("collapsing-header-collapsed"))).toBe(1);
    expect(opacityOf(getByTestId("collapsing-header-expanded"))).toBe(0);
  });

  it("renders the title in both the expanded and collapsed layers", () => {
    const { getAllByText } = render(
      <CollapsingHeader title="Orders" scrollY={sharedValue(32)} />,
    );
    // Both layers are always mounted (cross-faded via opacity, not
    // conditionally rendered) so the title text node exists twice.
    expect(getAllByText("Orders")).toHaveLength(2);
  });

  describe("reduced motion", () => {
    const reanimated = jest.requireMock("react-native-reanimated") as {
      useReducedMotion: jest.Mock;
    };

    afterEach(() => {
      reanimated.useReducedMotion.mockReturnValue(false);
    });

    it("snaps straight to the collapsed state at any non-zero offset, with no partial interpolation", () => {
      reanimated.useReducedMotion.mockReturnValue(true);
      // 10px is well short of the 64px collapse distance — under normal
      // interpolation this would be ~16% collapsed, not fully collapsed.
      const { getByTestId } = render(
        <CollapsingHeader title="Orders" scrollY={sharedValue(10)} />,
      );
      expect(opacityOf(getByTestId("collapsing-header-collapsed"))).toBe(1);
      expect(opacityOf(getByTestId("collapsing-header-expanded"))).toBe(0);
    });

    it("stays expanded at offset 0 even with reduced motion on", () => {
      reanimated.useReducedMotion.mockReturnValue(true);
      const { getByTestId } = render(
        <CollapsingHeader title="Orders" scrollY={sharedValue(0)} />,
      );
      expect(opacityOf(getByTestId("collapsing-header-expanded"))).toBe(1);
      expect(opacityOf(getByTestId("collapsing-header-collapsed"))).toBe(0);
    });
  });
});
