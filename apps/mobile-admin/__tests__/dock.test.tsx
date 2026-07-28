jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: { selectionChanged: jest.fn(() => Promise.resolve()) },
}));

// `Dock` uses `Easing.bezier` at MODULE level and `FadeIn` on the active
// tab, neither of which the global `__mocks__/react-native-reanimated.js`
// provides — so it throws at import time. A LOCAL factory wins over the
// global mock (the global mock's own doc says so), which is the right lever
// here: growing the global one to satisfy this file would change what 84
// other suites resolve. Same hand-rolled shape the disclosure and
// CreateNextStepsBanner tests already use.
// `Dock` positions itself off `useSafeAreaInsets()`, which throws without a
// `<SafeAreaProvider>` ancestor. Same official jest mock the other suites
// that render safe-area consumers already use.
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

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

import { render } from "@testing-library/react-native";

import { Dock } from "@/components/navigation/Dock";
import { MAX_FONT_SCALE } from "@/components/ui/Text";

const ROUTES = [
  { key: "index-1", name: "index" },
  { key: "orders-1", name: "orders" },
  { key: "products-1", name: "products" },
  { key: "customers-1", name: "customers" },
  { key: "more-1", name: "more" },
];

const DESCRIPTORS = {
  "index-1": { options: { title: "Dashboard" } },
  "orders-1": { options: { title: "Orders" } },
  "products-1": { options: { title: "Products" } },
  "customers-1": { options: { title: "Customers" } },
  "more-1": { options: { title: "More" } },
};

function renderDock(activeIndex = 0) {
  return render(
    <Dock
      state={{ index: activeIndex, routes: ROUTES }}
      descriptors={DESCRIPTORS}
      navigation={{
        emit: () => ({ defaultPrevented: false }),
        navigate: jest.fn(),
      }}
    />,
  );
}

/**
 * The dock's labels carry their OWN font-scale cap, tighter than the
 * app-wide 2 in `Text`.
 *
 * Measured on device at the app-wide cap: a slot is a fifth of a ~390pt bar
 * (~74pt), and at 2x the 11pt label becomes 22pt, so `Products` and
 * `Customers` truncated to `Prod…` and `Cust…`. A truncated NAVIGATION label
 * is worse than a small one — it destroys the one thing the label exists to
 * say.
 *
 * These assertions are deliberately relational rather than just literal: the
 * point is not that the number is 1.4, it is that the dock caps TIGHTER than
 * the app and still lets the labels grow. Deleting the prop, or "tidying" it
 * up to match `MAX_FONT_SCALE`, puts the truncation straight back.
 */
describe("Dock — labels cap their own font scale rather than truncating", () => {
  it("caps every label tighter than the app-wide cap", () => {
    const { getByText } = renderDock();

    for (const label of ["Home", "Orders", "Products", "Customers", "More"]) {
      const node = getByText(label);
      expect(node.props.maxFontSizeMultiplier).toBeLessThan(MAX_FONT_SCALE);
      expect(node.props.maxFontSizeMultiplier).toBe(1.4);
    }
  });

  it("caps the ACTIVE label too — it renders from a separate branch", () => {
    // Home active in the first render, Customers active here: the component
    // has two distinct JSX branches for active vs. inactive tabs, and an
    // earlier version of this fix could easily have been applied to only one.
    const { getByText } = renderDock(3);
    expect(getByText("Customers").props.maxFontSizeMultiplier).toBe(1.4);
    expect(getByText("Home").props.maxFontSizeMultiplier).toBe(1.4);
  });

  it("still lets labels grow — the cap is a bound, not a freeze", () => {
    const { getByText } = renderDock();
    expect(getByText("Customers").props.maxFontSizeMultiplier).toBeGreaterThan(1);
    // `allowFontScaling` is left alone entirely; disabling scaling outright
    // would fail WCAG rather than bound it.
    expect(getByText("Customers").props.allowFontScaling).not.toBe(false);
  });

  it("keeps every label to one line so a capped label still cannot wrap", () => {
    const { getByText } = renderDock();
    for (const label of ["Home", "Orders", "Products", "Customers", "More"]) {
      expect(getByText(label).props.numberOfLines).toBe(1);
    }
  });
});
