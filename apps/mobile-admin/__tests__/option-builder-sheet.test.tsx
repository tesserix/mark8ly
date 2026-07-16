// Importing OptionBuilderSheet.tsx pulls in lucide-react-native icons and
// @gorhom/bottom-sheet's BottomSheetModal — the latter requires
// react-native-reanimated, which throws under jest without a full
// worklets/logger setup this project doesn't have. Most of this file only
// exercises the pure `buildOptionSubmission` logic (see that function's doc
// comment for why it was extracted — the sheet renders through a portal,
// impractical to mount for most of its behaviour); the chip
// reduced-motion describe block below is the one exception, and renders
// children directly (rather than returning null) to reach the chips.
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});
jest.mock("@gorhom/bottom-sheet", () => {
  const React = require("react");
  return {
    __esModule: true,
    BottomSheetModal: React.forwardRef(
      ({ children }: { children?: React.ReactNode }, ref: React.Ref<unknown>) => {
        React.useImperativeHandle(ref, () => ({ present: () => {}, dismiss: () => {} }));
        return children ?? null;
      },
    ),
    BottomSheetView: ({ children }: { children?: React.ReactNode }) => children ?? null,
    BottomSheetScrollView: ({ children }: { children?: React.ReactNode }) => children ?? null,
  };
});

// OptionBuilderSheet now animates its value chips (Animated.View +
// FadeIn/FadeOut/LinearTransition), so importing it pulls in
// react-native-reanimated at module load time. The real module (and its
// shipped mock.js) needs the native Worklets module, which throws under
// jest. Hand-roll the same minimal virtual mock the disclosure tests use.
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
    LinearTransition: new ChainableAnimation(),
    Easing: { bezier: () => (t: number) => t },
    useReducedMotion: jest.fn(() => false),
  };
});

import { render, fireEvent } from "@testing-library/react-native";
import { createRef } from "react";
import {
  buildOptionSubmission,
  OptionBuilderSheet,
  type OptionBuilderSheetHandle,
} from "@/components/products/OptionBuilderSheet";

describe("buildOptionSubmission", () => {
  it("builds an option from a trimmed name and values", () => {
    expect(buildOptionSubmission("Size", ["S", "M", "L"])).toEqual({
      name: "Size",
      values: ["S", "M", "L"],
    });
  });

  it("trims the name", () => {
    expect(buildOptionSubmission("  Size  ", ["S"])).toEqual({ name: "Size", values: ["S"] });
  });

  it("trims each value and drops blanks", () => {
    expect(buildOptionSubmission("Size", [" S ", "  ", "M"])).toEqual({
      name: "Size",
      values: ["S", "M"],
    });
  });

  it("dedupes values, keeping the first occurrence's order", () => {
    expect(buildOptionSubmission("Size", ["S", "M", "S", "M", "L"])).toEqual({
      name: "Size",
      values: ["S", "M", "L"],
    });
  });

  it("returns null for a blank name", () => {
    expect(buildOptionSubmission("   ", ["S", "M"])).toBeNull();
  });

  it("returns null when there are no values", () => {
    expect(buildOptionSubmission("Size", [])).toBeNull();
  });

  it("returns null when every value is blank", () => {
    expect(buildOptionSubmission("Size", ["  ", " "])).toBeNull();
  });
});

describe("OptionBuilderSheet — chip reduced motion", () => {
  const reanimated = jest.requireMock("react-native-reanimated") as {
    useReducedMotion: jest.Mock;
  };

  afterEach(() => {
    reanimated.useReducedMotion.mockReturnValue(false);
  });

  function renderWithOneChip() {
    const ref = createRef<OptionBuilderSheetHandle>();
    const utils = render(<OptionBuilderSheet ref={ref} onSubmit={jest.fn()} />);
    fireEvent.changeText(utils.getByLabelText("Add a value"), "Small");
    fireEvent(utils.getByLabelText("Add a value"), "submitEditing");
    return utils;
  }

  it("passes undefined (instant) entering/exiting/layout on a chip when reduced motion is enabled", () => {
    reanimated.useReducedMotion.mockReturnValue(true);
    const { getByTestId } = renderWithOneChip();
    const chip = getByTestId("option-chip-Small");
    expect(chip.props.entering).toBeUndefined();
    expect(chip.props.exiting).toBeUndefined();
    expect(chip.props.layout).toBeUndefined();
  });

  it("passes real entering/exiting/layout animations on a chip when reduced motion is off", () => {
    reanimated.useReducedMotion.mockReturnValue(false);
    const { getByTestId } = renderWithOneChip();
    const chip = getByTestId("option-chip-Small");
    expect(chip.props.entering).toBeDefined();
    expect(chip.props.exiting).toBeDefined();
    expect(chip.props.layout).toBeDefined();
  });
});
