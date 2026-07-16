jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

// CreateNextStepsBanner now animates its own mount/dismiss (Animated.View +
// FadeIn/FadeOut), so importing it pulls in react-native-reanimated at
// module load time. The real module (and its shipped mock.js) needs the
// native Worklets module, which throws under jest. Hand-roll the same
// minimal virtual mock the disclosure tests use.
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
  };
});

import { render, fireEvent } from "@testing-library/react-native";
import { CreateNextStepsBanner } from "@/components/products/CreateNextStepsBanner";

describe("CreateNextStepsBanner", () => {
  it("renders the product title and all three jump chips", () => {
    const { getByText } = render(
      <CreateNextStepsBanner title="Ceramic Mug" onJump={jest.fn()} onDismiss={jest.fn()} />,
    );
    expect(getByText(`Nice — 'Ceramic Mug' is live.`)).toBeTruthy();
    expect(getByText("Add photos")).toBeTruthy();
    expect(getByText("Add options")).toBeTruthy();
    expect(getByText("Review variants")).toBeTruthy();
  });

  it("calls onJump with 'photos' when the photos chip is tapped", () => {
    const onJump = jest.fn();
    const { getByText } = render(
      <CreateNextStepsBanner title="Mug" onJump={onJump} onDismiss={jest.fn()} />,
    );
    fireEvent.press(getByText("Add photos"));
    expect(onJump).toHaveBeenCalledWith("photos");
  });

  it("calls onJump with 'options' when the options chip is tapped", () => {
    const onJump = jest.fn();
    const { getByText } = render(
      <CreateNextStepsBanner title="Mug" onJump={onJump} onDismiss={jest.fn()} />,
    );
    fireEvent.press(getByText("Add options"));
    expect(onJump).toHaveBeenCalledWith("options");
  });

  it("calls onJump with 'variants' when the variants chip is tapped", () => {
    const onJump = jest.fn();
    const { getByText } = render(
      <CreateNextStepsBanner title="Mug" onJump={onJump} onDismiss={jest.fn()} />,
    );
    fireEvent.press(getByText("Review variants"));
    expect(onJump).toHaveBeenCalledWith("variants");
  });

  it("calls onDismiss when the dismiss button is tapped", () => {
    const onDismiss = jest.fn();
    const { getByLabelText } = render(
      <CreateNextStepsBanner title="Mug" onJump={jest.fn()} onDismiss={onDismiss} />,
    );
    fireEvent.press(getByLabelText("Dismiss"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});

describe("CreateNextStepsBanner — reduced motion", () => {
  const reanimated = jest.requireMock("react-native-reanimated") as {
    useReducedMotion: jest.Mock;
  };

  afterEach(() => {
    reanimated.useReducedMotion.mockReturnValue(false);
  });

  it("passes undefined (instant) entering/exiting animations when reduced motion is enabled", () => {
    reanimated.useReducedMotion.mockReturnValue(true);
    const { getByTestId } = render(
      <CreateNextStepsBanner title="Mug" onJump={jest.fn()} onDismiss={jest.fn()} />,
    );
    const banner = getByTestId("create-next-steps-banner");
    expect(banner.props.entering).toBeUndefined();
    expect(banner.props.exiting).toBeUndefined();
  });

  it("passes real entering/exiting animations when reduced motion is off", () => {
    reanimated.useReducedMotion.mockReturnValue(false);
    const { getByTestId } = render(
      <CreateNextStepsBanner title="Mug" onJump={jest.fn()} onDismiss={jest.fn()} />,
    );
    const banner = getByTestId("create-next-steps-banner");
    expect(banner.props.entering).toBeDefined();
    expect(banner.props.exiting).toBeDefined();
  });
});
