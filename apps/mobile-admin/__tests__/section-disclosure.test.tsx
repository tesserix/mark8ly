// `@/components/ui`'s barrel eagerly re-exports BackHeader/SearchField, which
// pull lucide-react-native's ESM build — unmocked it throws "Unexpected
// token 'export'" under jest-expo (see variant-editor.test.tsx).
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

// react-native-reanimated 4.x's real module (and even its shipped mock.js)
// requires the native Worklets module at import time, which throws under
// jest ("Native part of Worklets doesn't seem to be initialized"). Hand-roll
// a minimal virtual mock covering only what SectionDisclosure/VariantRow use
// — enough to observe the animation props/values the reduced-motion tests
// below assert on.
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
    FadeOut: new ChainableAnimation(),
    Easing: { bezier: () => (t: number) => t },
    useReducedMotion: jest.fn(() => false),
    useAnimatedStyle: (factory: () => unknown) => factory(),
    withTiming: (
      toValue: unknown,
      _config?: unknown,
      callback?: (finished: boolean) => void,
    ) => {
      callback?.(true);
      return toValue;
    },
  };
});

import { render, fireEvent } from "@testing-library/react-native";
import { Text } from "react-native";
import { SectionDisclosure } from "@/components/products/SectionDisclosure";

describe("SectionDisclosure", () => {
  it("does not render the body when collapsed", () => {
    const { queryByText } = render(
      <SectionDisclosure title="Shipping & dimensions">
        <Text>Weight</Text>
      </SectionDisclosure>,
    );
    expect(queryByText("Weight")).toBeNull();
  });

  it("renders the body immediately when defaultOpen is set", () => {
    const { getByText } = render(
      <SectionDisclosure title="Shipping & dimensions" defaultOpen>
        <Text>Weight</Text>
      </SectionDisclosure>,
    );
    expect(getByText("Weight")).toBeTruthy();
  });

  it("expands on tap and flips accessibilityState.expanded", () => {
    const { getByRole, queryByText, getByText } = render(
      <SectionDisclosure title="Shipping & dimensions">
        <Text>Weight</Text>
      </SectionDisclosure>,
    );
    const header = getByRole("button");
    expect(header.props.accessibilityState.expanded).toBe(false);
    expect(queryByText("Weight")).toBeNull();

    fireEvent.press(header);

    expect(header.props.accessibilityState.expanded).toBe(true);
    expect(getByText("Weight")).toBeTruthy();
  });

  it("collapses again on a second tap — body unmounts", () => {
    const { getByRole, queryByText } = render(
      <SectionDisclosure title="Shipping & dimensions" defaultOpen>
        <Text>Weight</Text>
      </SectionDisclosure>,
    );
    fireEvent.press(getByRole("button"));
    expect(queryByText("Weight")).toBeNull();
  });
});

describe("SectionDisclosure — reduced motion", () => {
  const reanimated = jest.requireMock("react-native-reanimated") as {
    useReducedMotion: jest.Mock;
  };

  afterEach(() => {
    reanimated.useReducedMotion.mockReturnValue(false);
  });

  it("passes an undefined (instant) entering animation when reduced motion is enabled", () => {
    reanimated.useReducedMotion.mockReturnValue(true);
    const { getByTestId } = render(
      <SectionDisclosure title="Shipping & dimensions" defaultOpen>
        <Text>Weight</Text>
      </SectionDisclosure>,
    );
    expect(getByTestId("section-disclosure-body").props.entering).toBeUndefined();
  });

  it("passes a real entering animation when reduced motion is off", () => {
    reanimated.useReducedMotion.mockReturnValue(false);
    const { getByTestId } = render(
      <SectionDisclosure title="Shipping & dimensions" defaultOpen>
        <Text>Weight</Text>
      </SectionDisclosure>,
    );
    expect(getByTestId("section-disclosure-body").props.entering).toBeDefined();
  });
});
