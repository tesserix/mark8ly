// The submit/dismiss pair every reason sheet ends with.
//
// This suite exists for a LIVE defect: at accessibility text sizes the submit
// labels clipped inside their own buttons — "Issue Refun|d", "Cancel Orde",
// "Block Custo|". `bee7ee8e` made those buttons reachable inside the sheet;
// their labels were still truncated one level further in, because each button
// was a `height: 48`, `flex: 1` box holding text that scales to 2×.
//
// The fixture matters more than the assertion here. Every one of these cases
// PASSES against the broken code at fontScale 1 — which is exactly why the
// bug shipped and why `bee7ee8e` didn't catch it. Each test below either runs
// at an accessibility scale or asserts on the absence of the fixed box.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});
jest.mock("expo-haptics", () => ({
  notificationAsync: jest.fn(() => Promise.resolve()),
  NotificationFeedbackType: { Warning: "warning", Success: "success", Error: "error" },
}));

import { createRef } from "react";
import { act, render } from "@testing-library/react-native";
import { Dimensions, StyleSheet } from "react-native";
import {
  SheetActionButton,
  SheetActions,
  SHEET_ACTIONS_STACK_SCALE,
  SHEET_BUTTON_MIN_HEIGHT,
} from "@/components/ui";
import { MAX_FONT_SCALE } from "@/components/ui";
import {
  CancelReasonSheet,
  type CancelReasonSheetHandle,
} from "@/components/orders/CancelReasonSheet";
import { RefundSheet, type RefundSheetHandle } from "@/components/orders/RefundSheet";
import { EmailLabelSheet, type EmailLabelSheetHandle } from "@/components/orders/EmailLabelSheet";
import {
  BlockReasonSheet,
  type BlockReasonSheetHandle,
} from "@/components/customers/BlockReasonSheet";

/** The accessibility size the defect was reported at. */
const AX_SCALE = MAX_FONT_SCALE;

function setFontScale(fontScale: number) {
  jest
    .spyOn(Dimensions, "get")
    .mockReturnValue({ width: 390, height: 844, scale: 3, fontScale });
}

afterEach(() => jest.restoreAllMocks());

describe("SheetActionButton", () => {
  it("never pins a fixed height — the box has to grow with the label", () => {
    setFontScale(AX_SCALE);
    const { getByLabelText } = render(
      <SheetActionButton label="Issue Refund" tone="ink" onPress={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByLabelText("Issue Refund").props.style);
    expect(style.height).toBeUndefined();
    expect(style.minHeight).toBe(SHEET_BUTTON_MIN_HEIGHT * AX_SCALE);
  });

  it("scales its floor with the font scale rather than holding 48", () => {
    setFontScale(1);
    const small = render(<SheetActionButton label="Issue Refund" onPress={jest.fn()} />);
    expect(StyleSheet.flatten(small.getByLabelText("Issue Refund").props.style).minHeight).toBe(
      SHEET_BUTTON_MIN_HEIGHT,
    );
    small.unmount();

    setFontScale(1.6);
    const large = render(<SheetActionButton label="Issue Refund" onPress={jest.fn()} />);
    expect(
      StyleSheet.flatten(large.getByLabelText("Issue Refund").props.style).minHeight,
    ).toBeGreaterThan(SHEET_BUTTON_MIN_HEIGHT);
  });

  it("leaves the label unclamped — a truncated label is the bug, not the fix", () => {
    setFontScale(AX_SCALE);
    const { getByText } = render(
      <SheetActionButton label="Block Customer" tone="danger" onPress={jest.fn()} />,
    );
    // `numberOfLines` would reintroduce the exact truncation being fixed.
    expect(getByText("Block Customer").props.numberOfLines).toBeUndefined();
  });

  it("drops the half-width `flex: 1` once the pair stacks", () => {
    // `flex: 1` in a COLUMN resolves against an indefinite main size — a
    // separate Yoga foot-gun, so it belongs only to the row.
    setFontScale(1);
    const row = render(<SheetActionButton label="Cancel Order" onPress={jest.fn()} />);
    expect(StyleSheet.flatten(row.getByLabelText("Cancel Order").props.style).flex).toBe(1);
    row.unmount();

    setFontScale(AX_SCALE);
    const stacked = render(<SheetActionButton label="Cancel Order" onPress={jest.fn()} />);
    expect(StyleSheet.flatten(stacked.getByLabelText("Cancel Order").props.style).flex).toBeUndefined();
  });
});

describe("SheetActions", () => {
  it("sits the pair side by side at normal text sizes", () => {
    setFontScale(1);
    const { getByTestId } = render(
      <SheetActions testID="pair">
        <SheetActionButton label="Cancel" onPress={jest.fn()} />
      </SheetActions>,
    );
    expect(StyleSheet.flatten(getByTestId("pair").props.style).flexDirection).toBe("row");
  });

  it("stacks the pair once half the sheet can no longer hold a submit label", () => {
    setFontScale(SHEET_ACTIONS_STACK_SCALE + 0.05);
    const { getByTestId } = render(
      <SheetActions testID="pair">
        <SheetActionButton label="Cancel" onPress={jest.fn()} />
      </SheetActions>,
    );
    expect(StyleSheet.flatten(getByTestId("pair").props.style).flexDirection).toBe("column");
  });
});

// --- the four real sheets ------------------------------------------------
// Named individually rather than parameterised over the primitive, because
// the defect was reported per-sheet and the point is that all four are fixed
// by ONE change. `EmailLabelSheet` is included deliberately: it was never
// opened on a device during `bee7ee8e` (its menu item needs a shipment).
const SHEETS: Array<{
  name: string;
  submitLabel: string;
  render: () => ReturnType<typeof render>;
}> = [
  {
    name: "RefundSheet",
    submitLabel: "Issue refund",
    render: () => {
      const ref = createRef<RefundSheetHandle>();
      const utils = render(
        <RefundSheet
          ref={ref}
          onSubmit={jest.fn()}
          refundableAmount={97.9}
          currencyCode="AUD"
          // The carrier-warning variant — the LONGER of the two layouts, and
          // the one `bee7ee8e` never rendered.
          hasShipment
        />,
      );
      act(() => ref.current?.present());
      return utils;
    },
  },
  {
    name: "CancelReasonSheet",
    submitLabel: "Cancel order",
    render: () => {
      const ref = createRef<CancelReasonSheetHandle>();
      const utils = render(
        <CancelReasonSheet ref={ref} onSubmit={jest.fn()} hasShipment carrier="delhivery" />,
      );
      act(() => ref.current?.present());
      return utils;
    },
  },
  {
    name: "EmailLabelSheet",
    submitLabel: "Send label",
    render: () => {
      const ref = createRef<EmailLabelSheetHandle>();
      const utils = render(<EmailLabelSheet ref={ref} onSubmit={jest.fn()} />);
      act(() => ref.current?.present());
      return utils;
    },
  },
  {
    name: "BlockReasonSheet",
    submitLabel: "Block customer",
    render: () => {
      const ref = createRef<BlockReasonSheetHandle>();
      const utils = render(
        <BlockReasonSheet
          ref={ref}
          customerLabel="Ada Lovelace"
          onSubmit={jest.fn()}
          isSubmitting={false}
          error={null}
          onDismiss={jest.fn()}
        />,
      );
      act(() => ref.current?.present());
      return utils;
    },
  },
];

describe.each(SHEETS)("$name at accessibility text size", ({ submitLabel, render: mount }) => {
  it(`gives "${submitLabel}" a growable box instead of a fixed 48pt one`, () => {
    setFontScale(AX_SCALE);
    const { getByLabelText } = mount();
    const style = StyleSheet.flatten(getByLabelText(submitLabel).props.style) as {
      height?: number;
      minHeight?: number;
    };
    // The whole defect in one assertion: `height: 48` cannot hold a 32pt
    // label, let alone a wrapped one.
    expect(style.height).toBeUndefined();
    expect(style.minHeight).toBe(SHEET_BUTTON_MIN_HEIGHT * AX_SCALE);
  });
});
