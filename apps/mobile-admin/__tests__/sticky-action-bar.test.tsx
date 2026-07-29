// The bar primitive Order detail's actions live in, and the one
// products/new.tsx's hand-rolled footer became.
//
// Three contracts, and the third is the one that keeps biting this app: the
// bar is a BOX HOLDING SCALABLE TEXT, so its height is a function of the
// device font scale, and any host that pads its scroll content by a constant
// hides that content behind the bar at accessibility sizes.
// Imported through the `components/ui` BARREL on purpose — that is how every
// screen reaches it, and this barrel has transitively broken unrelated suites
// twice. These two mocks are what the barrel drags in (lucide ships untranspiled
// ESM; safe-area needs a provider).
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

import { render } from "@testing-library/react-native";
import { Dimensions, StyleSheet, Text as RNText, View } from "react-native";
import {
  StickyActionBar,
  STICKY_BAR_HEIGHT,
  STICKY_BAR_CONTENT_HEIGHT,
  stickyBarHeightFor,
  useStickyBarHeight,
} from "@/components/ui";
import { MAX_FONT_SCALE } from "@/components/ui";
import { theme } from "@/lib/theme";

function setFontScale(fontScale: number) {
  jest
    .spyOn(Dimensions, "get")
    .mockReturnValue({ width: 390, height: 844, scale: 3, fontScale });
}

afterEach(() => jest.restoreAllMocks());

describe("StickyActionBar", () => {
  it("renders children in a full-width bar with a hairline top rule", () => {
    const { getByTestId } = render(
      <StickyActionBar bottom={0} testID="bar">
        <RNText testID="child">Confirm order</RNText>
      </StickyActionBar>,
    );

    expect(getByTestId("child")).toBeTruthy();
    const style = StyleSheet.flatten(getByTestId("bar").props.style);
    expect(style.position).toBe("absolute");
    expect(style.left).toBe(0);
    expect(style.right).toBe(0);
    expect(style.flexDirection).toBe("row");
    expect(style.borderTopWidth).toBe(theme.hairline);
    expect(style.borderTopColor).toBe(theme.colors.hairline);
    // Solid Paper, not a floating pill and never a blur.
    expect(style.backgroundColor).toBe(theme.colors.background);
  });

  it("honours the caller's `bottom` rather than computing its own", () => {
    // The two callers sit in different worlds — a (tabs) screen must clear
    // the floating dock, a modal covers it — so this must be pure pass-through.
    const { getByTestId } = render(
      <StickyActionBar bottom={123} testID="bar">
        <View />
      </StickyActionBar>,
    );
    expect(StyleSheet.flatten(getByTestId("bar").props.style).bottom).toBe(123);
  });

  it("passes a plain ARRAY style, never a function", () => {
    // NativeWind's JSX interop resolves an array `style` and silently drops a
    // function one, which is how press states have been lost here before.
    const { getByTestId } = render(
      <StickyActionBar bottom={0} testID="bar">
        <View />
      </StickyActionBar>,
    );
    expect(typeof getByTestId("bar").props.style).not.toBe("function");
    expect(Array.isArray(getByTestId("bar").props.style)).toBe(true);
  });

  it("grows its box with the device font scale instead of pinning a height", () => {
    setFontScale(2);
    const { getByTestId } = render(
      <StickyActionBar bottom={0} testID="bar">
        <View />
      </StickyActionBar>,
    );
    const style = StyleSheet.flatten(getByTestId("bar").props.style);
    // A FIXED `height` is the defect class this whole programme keeps
    // shipping. There must not be one.
    expect(style.height).toBeUndefined();
    expect(style.minHeight).toBe(stickyBarHeightFor(2));
    expect(style.minHeight).toBeGreaterThan(STICKY_BAR_HEIGHT);
  });

  it("reports its measured height so a host can pad by the truth, not the estimate", () => {
    const onHeightChange = jest.fn();
    const { getByTestId } = render(
      <StickyActionBar bottom={0} onHeightChange={onHeightChange} testID="bar">
        <View />
      </StickyActionBar>,
    );
    getByTestId("bar").props.onLayout({ nativeEvent: { layout: { height: 137 } } });
    expect(onHeightChange).toHaveBeenCalledWith(137);
  });
});

describe("stickyBarHeightFor", () => {
  it("is STICKY_BAR_HEIGHT at fontScale 1", () => {
    expect(stickyBarHeightFor(1)).toBe(STICKY_BAR_HEIGHT);
  });

  it.each([1, 1.35, 1.6, 2, 3.1])("clears its own scaled content at fontScale %s", (fs) => {
    const scale = Math.min(Math.max(fs, 1), MAX_FONT_SCALE);
    expect(stickyBarHeightFor(fs)).toBeGreaterThan(STICKY_BAR_CONTENT_HEIGHT * scale);
  });

  it("clamps at MAX_FONT_SCALE so an AX5 device doesn't get an absurd bar", () => {
    expect(stickyBarHeightFor(3.1)).toBe(stickyBarHeightFor(MAX_FONT_SCALE));
  });
});

describe("useStickyBarHeight", () => {
  it("returns the height for the device's current font scale", () => {
    setFontScale(1.6);
    function Probe() {
      return <RNText testID="h">{String(useStickyBarHeight())}</RNText>;
    }
    const { getByTestId } = render(<Probe />);
    expect(getByTestId("h").props.children).toBe(String(stickyBarHeightFor(1.6)));
  });
});
