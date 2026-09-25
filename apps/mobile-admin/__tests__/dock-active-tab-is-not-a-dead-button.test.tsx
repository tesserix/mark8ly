// Pressing the tab you are already on must still navigate.
//
// The dock used to return early when `isActive`, which made the focused tab's
// button a no-op. That is fine only while `state.index` and what is actually
// rendered agree. Reported from a device after opening the notifications inbox
// from More: the screen came back as the Dashboard while the dock still had
// More selected, so the More button did nothing and there was no way back to
// it — "More screen becomes inaccessible".
//
// Navigating to the focused tab is also what every other tab bar does: it pops
// that tab's stack back to its anchor route.

jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: { selectionChanged: jest.fn(() => Promise.resolve()) },
}));
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});
// Same hand-rolled reanimated factory dock.test.tsx uses: Dock calls
// `Easing.bezier` at module level and `FadeIn` on the active tab, neither of
// which the global mock provides.
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

import type { ComponentProps } from "react";
import { render, fireEvent } from "@testing-library/react-native";
import { Dock } from "@/components/navigation/Dock";

type DockProps = ComponentProps<typeof Dock>;

function makeProps(activeIndex: number) {
  const routes = [
    { key: "index-1", name: "index" },
    { key: "orders-1", name: "orders" },
    { key: "products-1", name: "products" },
    { key: "customers-1", name: "customers" },
    { key: "more-1", name: "more" },
  ];
  const navigate = jest.fn();
  return {
    navigate,
    props: {
      state: { index: activeIndex, routes },
      descriptors: Object.fromEntries(
        routes.map((r) => [r.key, { options: { title: r.name } }]),
      ),
      navigation: {
        emit: () => ({ defaultPrevented: false }),
        navigate,
      },
    } as unknown as DockProps,
  };
}

describe("Dock", () => {
  it("navigates when pressing a tab that is NOT active", () => {
    const { navigate, props } = makeProps(0);
    const { getByLabelText } = render(<Dock {...props} />);

    fireEvent.press(getByLabelText("more"));

    expect(navigate).toHaveBeenCalledWith("more");
  });

  it("still navigates when pressing the tab that IS active", () => {
    // index 4 is `more`. Before the fix this assertion failed with zero calls,
    // which is exactly the state that stranded the device: the dock believed
    // More was selected, so pressing More did nothing at all.
    const { navigate, props } = makeProps(4);
    const { getByLabelText } = render(<Dock {...props} />);

    fireEvent.press(getByLabelText("more"));

    expect(navigate).toHaveBeenCalledWith("more");
  });

  it("respects a listener that prevents the default tab press", () => {
    const navigate = jest.fn();
    const routes = [{ key: "more-1", name: "more" }];
    const props = {
      state: { index: 0, routes },
      descriptors: { "more-1": { options: { title: "more" } } },
      navigation: { emit: () => ({ defaultPrevented: true }), navigate },
    } as unknown as DockProps;

    const { getByLabelText } = render(<Dock {...props} />);
    fireEvent.press(getByLabelText("more"));

    expect(navigate).not.toHaveBeenCalled();
  });
});
