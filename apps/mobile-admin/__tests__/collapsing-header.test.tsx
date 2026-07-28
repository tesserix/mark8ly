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

import { Dimensions, StyleSheet } from "react-native";
import { render } from "@testing-library/react-native";
import type { ReactTestInstance } from "react-test-renderer";
import type { SharedValue } from "react-native-reanimated";
import {
  CollapsingHeader,
  MAX_FONT_SCALE,
  headerHeightsFor,
} from "@/components/ui/CollapsingHeader";
import { theme } from "@/lib/theme";

function sharedValue(value: number): SharedValue<number> {
  return { value } as unknown as SharedValue<number>;
}

function opacityOf(node: ReactTestInstance): number | undefined {
  return (StyleSheet.flatten(node.props.style) as { opacity?: number }).opacity;
}

function heightOf(node: ReactTestInstance): number | undefined {
  return (StyleSheet.flatten(node.props.style) as { height?: number }).height;
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

  /**
   * Gate-A residual (inc2 Task 8 review): `git grep allowFontScaling|
   * maxFontSizeMultiplier` across the app returned ZERO hits. RN scales both
   * `fontSize` AND `lineHeight` by the Dynamic Type multiplier, and
   * `styles.block` is `position:absolute; top:0; bottom:0` inside a container
   * with `overflow:"hidden"` — so at 1.9× a merchant lost the ascenders of
   * their own shop name. WCAG 2.1 AA is a project baseline and this primitive
   * goes onto Orders next, so the fix lives HERE, not on the Dashboard.
   */
  describe("Dynamic Type", () => {
    // Line boxes RN will scale. One box per element — enforced by the
    // numberOfLines={1} assertions below.
    const EXPANDED_CONTENT =
      theme.text.caption.lineHeight + // eyebrow (the taller of the two presets)
      4 + // styles.eyebrow marginBottom
      theme.text.h1.lineHeight +
      4 + // styles.expandedSubtitle marginTop
      theme.text.body.lineHeight;
    const COLLAPSED_CONTENT =
      theme.text.h3.lineHeight + 2 + theme.text.caption.lineHeight;

    it.each([1, 1.35, 1.6, 1.9, 3.1])(
      "keeps both states taller than their own scaled content at fontScale %s",
      (fontScale) => {
        const { expanded, collapsed } = headerHeightsFor(fontScale);
        const scale = Math.min(Math.max(fontScale, 1), MAX_FONT_SCALE);
        expect(expanded).toBeGreaterThan(EXPANDED_CONTENT * scale);
        expect(collapsed).toBeGreaterThan(COLLAPSED_CONTENT * scale);
      },
    );

    it("caps the multiplier so the accessibility sizes above 2x stop growing the header", () => {
      expect(headerHeightsFor(3.1)).toEqual(headerHeightsFor(MAX_FONT_SCALE));
      // …but honours WCAG 1.4.4's 200% resize rather than clamping lower.
      expect(MAX_FONT_SCALE).toBeGreaterThanOrEqual(2);
    });

    it("never shrinks below the design heights when fontScale is under 1", () => {
      expect(headerHeightsFor(0.85)).toEqual(headerHeightsFor(1));
    });

    it("sizes the rendered container from the device font scale, not a fixed 96", () => {
      // jest-expo reports fontScale 2, so this render also proves the hook is
      // actually read rather than defaulted to 1.
      const { getByTestId } = render(
        <CollapsingHeader title="Bondi Supply" eyebrow="Monday, 27 July" scrollY={sharedValue(0)} />,
      );
      const { fontScale } = Dimensions.get("window");
      expect(fontScale).toBeGreaterThan(1);
      expect(heightOf(getByTestId("collapsing-header"))).toBe(
        headerHeightsFor(fontScale).expanded,
      );
      expect(heightOf(getByTestId("collapsing-header"))).not.toBe(96);
    });

    it("caps every line at MAX_FONT_SCALE and holds it to a single line", () => {
      const { getByTestId } = render(
        <CollapsingHeader
          title="A Very Long Shop Name Indeed"
          eyebrow="Monday, 27 July"
          subtitle="12 orders today"
          scrollY={sharedValue(0)}
        />,
      );
      for (const layer of ["collapsing-header-expanded", "collapsing-header-collapsed"]) {
        // Host "Text" nodes, matched by tag rather than by the RN component
        // reference — the repo has two @types/react copies and the class
        // identity doesn't typecheck across them.
        const texts = getByTestId(layer).findAll((n) => String(n.type) === "Text");
        expect(texts.length).toBeGreaterThan(0);
        for (const t of texts) {
          expect(t.props.maxFontSizeMultiplier).toBe(MAX_FONT_SCALE);
          expect(t.props.numberOfLines).toBe(1);
        }
      }
    });
  });

  describe("eyebrowPreset", () => {
    // Additive prop, NOT a changed default: this primitive has ~15 call sites
    // and flipping the shared default rippled through all of them earlier in
    // this increment.
    // Asserted on the resolved utility classes rather than a flattened
    // style: NativeWind compiles classNames natively and does not resolve
    // them to RN style objects under jest, so `textTransform` is undefined in
    // this environment for BOTH presets and would pass vacuously.
    it("defaults to the uppercase small-caps eyebrow", () => {
      const { getByText } = render(
        <CollapsingHeader title="Orders" eyebrow="Today" scrollY={sharedValue(0)} />,
      );
      expect(getByText("Today").props.className).toContain("uppercase");
      expect(getByText("Today").props.className).toContain("text-eyebrow");
    });

    it("renders the eyebrow in sentence case when the caption preset is asked for", () => {
      const { getByText } = render(
        <CollapsingHeader
          title="Bondi Supply"
          eyebrow="Monday, 27 July"
          eyebrowPreset="caption"
          scrollY={sharedValue(0)}
        />,
      );
      const className = getByText("Monday, 27 July").props.className as string;
      expect(className).not.toContain("uppercase");
      expect(className).toContain("text-caption");
    });
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
