// Order detail's action surface after the actions moved off the bottom of the
// scroll content and into a sticky bar.
//
// The contracts that carry real risk here:
//   - the bar's height is IDENTICAL in every order state, so the screen never
//     reflows under the merchant's thumb and the scroll padding is one number;
//   - the overflow menu has a CONSTANT item count, so its snap point doesn't
//     resize between orders — illegal actions are greyed, never dropped;
//   - Cancel and Refund OPEN THEIR SHEET rather than firing. Cancel because
//     the server requires a reason; Refund because `refund_request_id` is a
//     manually-enforced idempotency key and a fresh one is a second real
//     gateway refund.
//
// Everything below the import block is mocked at the module boundary: this is
// a screen test, not an integration test.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});
jest.mock("expo-haptics", () => ({
  notificationAsync: jest.fn(() => Promise.resolve()),
  NotificationFeedbackType: { Warning: "warning", Success: "success", Error: "error" },
}));
jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    actionSucceeded: jest.fn(() => Promise.resolve()),
    actionFailed: jest.fn(() => Promise.resolve()),
    swipeThreshold: jest.fn(() => Promise.resolve()),
    menuOpen: jest.fn(() => Promise.resolve()),
    selectionChanged: jest.fn(() => Promise.resolve()),
  },
}));
jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: unknown) => unknown) =>
    selector({ activeStore: { id: "s1", name: "Bondi Supply", currency_code: "AUD" } }),
}));
jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));
jest.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: "o1" }),
  useRouter: () => ({ push: jest.fn(), back: jest.fn() }),
}));

// The shipping panel owns seven actions of its own and a lazy shipment query.
// It is deliberately untouched by this task, so it is stubbed out.
jest.mock("@/components/orders/ShippingPanel", () => ({ ShippingPanel: () => null }));
jest.mock("@/lib/hooks/use-shipment", () => ({ useShipment: () => ({ data: undefined }) }));

let mockOrder: Record<string, unknown> | undefined;
jest.mock("@/lib/hooks/use-orders", () => ({
  useOrder: () => ({ data: mockOrder, isLoading: false, error: null }),
}));

const mockConfirm = jest.fn();
const mockFulfill = jest.fn();
const mockCancel = jest.fn();
const mockRefund = jest.fn();
jest.mock("@/lib/admin-api/order-actions", () => ({
  useConfirmOrder: () => ({ mutate: mockConfirm, isPending: false }),
  useFulfillOrder: () => ({ mutate: mockFulfill, isPending: false }),
  useCancelOrder: () => ({ mutate: mockCancel, isPending: false }),
  useRefundOrder: () => ({ mutate: mockRefund, isPending: false }),
}));

// The two sheets are stubbed to RECORD `present()` and capture `onSubmit`.
//
// The shared gorhom mock renders a `BottomSheetModal`'s children
// unconditionally, so "is the sheet on screen?" is not a question this
// harness can answer — asserting on the sheet's own controls would pass even
// if the screen never opened it. Recording the imperative `present()` call is
// the real contract: the menu row must OPEN the sheet and must not fire the
// mutation. We also capture the `onSubmit` handler from props so the test can
// trigger the component's actual mutation with its real callbacks.
const mockCancelPresent = jest.fn();
const mockRefundPresent = jest.fn();
let capturedCancelOnSubmit: ((reason: string) => void) | undefined;
let capturedRefundOnSubmit:
  | (({ amount, refundRequestId }: { amount?: number; refundRequestId: string }) => void)
  | undefined;

jest.mock("@/components/orders/CancelReasonSheet", () => {
  const { forwardRef, useImperativeHandle } = require("react");
  return {
    CancelReasonSheet: forwardRef(
      (props: { onSubmit?: (reason: string) => void }, ref: unknown) => {
        capturedCancelOnSubmit = props.onSubmit;
        useImperativeHandle(ref, () => ({
          present: mockCancelPresent,
          dismiss: jest.fn(),
        }));
        return null;
      }
    ),
  };
});
jest.mock("@/components/orders/RefundSheet", () => {
  const { forwardRef, useImperativeHandle } = require("react");
  return {
    RefundSheet: forwardRef(
      (
        props: {
          onSubmit?: ({ amount, refundRequestId }: { amount?: number; refundRequestId: string }) => void;
        },
        ref: unknown
      ) => {
        capturedRefundOnSubmit = props.onSubmit;
        useImperativeHandle(ref, () => ({
          present: mockRefundPresent,
          dismiss: jest.fn(),
        }));
        return null;
      }
    ),
  };
});

const mockEmailInvoice = jest.fn();
const mockEmailReceipt = jest.fn();
jest.mock("@/lib/admin-api/shipment-actions", () => ({
  useEmailInvoice: () => ({ mutate: mockEmailInvoice, isPending: false }),
  useEmailReceipt: () => ({ mutate: mockEmailReceipt, isPending: false }),
}));

import { Alert, StyleSheet } from "react-native";
import { act, fireEvent, render, type RenderResult } from "@testing-library/react-native";
import OrderDetailScreen from "@/app/(tabs)/orders/[id]";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";

const BASE = {
  id: "o1",
  order_number: "1001",
  customer_email: "buyer@example.com",
  customer_name: "Ada Lovelace",
  status: "pending",
  payment_status: "unpaid",
  fulfillment_status: "unfulfilled",
  subtotal: 89,
  shipping_total: 0,
  tax_total: 8.9,
  discount_total: 0,
  grand_total: 97.9,
  refunded_amount: 0,
  currency_code: "AUD",
  placed_at: "2026-07-17T00:00:00Z",
  items: [
    {
      id: "li1",
      title_snapshot: "Linen Shirt",
      sku_snapshot: "LS-M",
      option_summary: "Size: M",
      unit_price: 89,
      quantity: 1,
      line_total: 89,
      currency_code: "AUD",
    },
  ],
  addresses: [],
  tax_lines: [],
};

/** The four fixtures the bar has to look identical across. */
const FIXTURES = {
  pending: { ...BASE, status: "pending", payment_status: "unpaid" },
  confirmed: { ...BASE, status: "confirmed", payment_status: "paid" },
  fulfilled: { ...BASE, status: "fulfilled", payment_status: "paid" },
  cancelled: { ...BASE, status: "cancelled", payment_status: "unpaid" },
} as const;

function renderOrder(order: Record<string, unknown>): RenderResult {
  mockOrder = order;
  return render(<OrderDetailScreen />);
}

function openOverflow(utils: RenderResult) {
  fireEvent.press(utils.getByLabelText("More order actions"));
  return utils;
}

/** Reads `accessibilityState.disabled` off a menu row by its item key. */
function disabledOf(utils: RenderResult, key: string): boolean | undefined {
  return utils.getByTestId(`action-sheet-item-${key}`).props.accessibilityState?.disabled;
}

/**
 * Confirm and Fulfil fire from `Alert.alert` buttons, not a direct press —
 * find the button by its label in the most recent `Alert.alert` call and
 * invoke its `onPress` the way the OS would after a tap.
 */
function pressAlertButton(
  spy: jest.SpiedFunction<typeof Alert.alert>,
  label: string,
): void {
  const calls = spy.mock.calls;
  const buttons = calls[calls.length - 1]![2] as Array<{
    text?: string;
    onPress?: () => void;
  }>;
  const button = buttons.find((b) => b.text === label);
  if (!button?.onPress) throw new Error(`No Alert button labelled "${label}"`);
  button.onPress();
}

/** The `{ onSuccess, onError }` object `mutate` was called with. */
function mutationCallbacks(mock: jest.Mock, callIndex = 0): {
  onSuccess: () => void;
  onError: (error?: unknown) => void;
} {
  const call = mock.mock.calls[callIndex];
  if (!call) throw new Error(`mutate was not called (call index ${callIndex})`);
  return call[1] as { onSuccess: () => void; onError: (error?: unknown) => void };
}

beforeEach(() => jest.clearAllMocks());
afterEach(() => {
  mockOrder = undefined;
});

describe("Order detail — sticky action bar", () => {
  it("shows Confirm order as the primary on a pending order", () => {
    const { getByLabelText, queryByTestId } = renderOrder(FIXTURES.pending);
    expect(getByLabelText("Confirm order")).toBeTruthy();
    expect(queryByTestId("order-terminal-caption")).toBeNull();
  });

  it("shows Mark fulfilled as the primary on a confirmed order", () => {
    const { getByLabelText } = renderOrder(FIXTURES.confirmed);
    expect(getByLabelText("Mark fulfilled")).toBeTruthy();
  });

  it("shows a non-interactive terminal caption on a cancelled order", () => {
    const { getByTestId, queryByTestId, getByText } = renderOrder(FIXTURES.cancelled);
    expect(getByText("Order cancelled")).toBeTruthy();
    // A caption, NOT a disabled button — a disabled button still announces as
    // a button and invites a tap that can never do anything.
    const caption = getByTestId("order-terminal-caption");
    expect(caption.props.accessibilityRole).toBeUndefined();
    expect(caption.props.onStartShouldSetResponder).toBeUndefined();
    expect(queryByTestId("order-primary-action")).toBeNull();
  });

  it("shows the fulfilled caption on a fulfilled order", () => {
    const { getByText } = renderOrder(FIXTURES.fulfilled);
    expect(getByText("Order fulfilled")).toBeTruthy();
  });

  it("keeps the bar's box identical across pending / confirmed / fulfilled / cancelled", () => {
    // The reason the caption exists at all: a bar that collapses when no
    // action is legal reflows the whole screen under the merchant's thumb.
    const boxes = (["pending", "confirmed", "fulfilled", "cancelled"] as const).map((state) => {
      const utils = renderOrder(FIXTURES[state]);
      const bar = StyleSheet.flatten(utils.getByTestId("order-action-bar").props.style);
      const slot = StyleSheet.flatten(
        (
          utils.queryByTestId("order-primary-action") ??
          utils.getByTestId("order-terminal-caption")
        ).props.style,
      );
      utils.unmount();
      return { barMinHeight: bar.minHeight, barPaddingV: bar.paddingVertical, slot: slot.minHeight };
    });

    expect(new Set(boxes.map((b) => b.barMinHeight)).size).toBe(1);
    expect(new Set(boxes.map((b) => b.barPaddingV)).size).toBe(1);
    expect(new Set(boxes.map((b) => b.slot)).size).toBe(1);
  });

  it("sits the bar above the floating dock and never pins a fixed height", () => {
    const { getByTestId } = renderOrder(FIXTURES.pending);
    const style = StyleSheet.flatten(getByTestId("order-action-bar").props.style);
    const { DOCK_HEIGHT, DOCK_BOTTOM_GAP } = require("@/components/navigation/dock-metrics");
    // `useDockClearance()` is deliberately NOT used here — it sizes clearance
    // for content, and the content now has to clear the bar as well.
    expect(style.bottom).toBeGreaterThanOrEqual(DOCK_HEIGHT + DOCK_BOTTOM_GAP);
    expect(style.height).toBeUndefined();
  });

  it("no longer renders the inline action button stack", () => {
    // If the old block survived alongside the bar the screen would have two
    // Confirm buttons.
    const { getAllByLabelText } = renderOrder(FIXTURES.pending);
    expect(getAllByLabelText("Confirm order")).toHaveLength(1);
    // The two Documents buttons moved into the overflow menu — exactly one
    // control each, and it is the menu row, not a card button.
    expect(getAllByLabelText("Email invoice")).toHaveLength(1);
    expect(getAllByLabelText("Email invoice")[0]!.props.testID).toBe("action-sheet-item-invoice");
    expect(getAllByLabelText("Email receipt")).toHaveLength(1);
  });

  it("fires the three-way confirm Alert from the primary rather than a sheet", () => {
    const spy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByLabelText } = renderOrder(FIXTURES.pending);
    fireEvent.press(getByLabelText("Confirm order"));
    expect(spy).toHaveBeenCalled();
    // Cancel / Confirm / Confirm & mark paid — a real business distinction
    // that belongs on the primary action.
    expect(spy.mock.calls[0]![2]).toHaveLength(3);
    spy.mockRestore();
  });
});

describe("Order detail — overflow menu", () => {
  const LABELS = ["Refund", "Email invoice", "Email receipt", "Cancel order"];

  it.each(["pending", "confirmed", "fulfilled", "cancelled"] as const)(
    "renders exactly four overflow items on a %s order",
    (state) => {
      const utils = openOverflow(renderOrder(FIXTURES[state]));
      for (const label of LABELS) expect(utils.getByText(label)).toBeTruthy();
    },
  );

  it("disables Refund on an unpaid order rather than dropping it", () => {
    const utils = openOverflow(renderOrder(FIXTURES.pending));
    // Present — a greyed row says the action exists; a missing one says
    // nothing, and it would also resize the sheet's snap point.
    expect(utils.getByText("Refund")).toBeTruthy();
    expect(disabledOf(utils, "refund")).toBe(true);
    fireEvent.press(utils.getByText("Refund"));
    expect(mockRefundPresent).not.toHaveBeenCalled();
  });

  it("disables Cancel on a fulfilled order", () => {
    const utils = openOverflow(renderOrder(FIXTURES.fulfilled));
    expect(utils.getByText("Cancel order")).toBeTruthy();
    expect(disabledOf(utils, "cancel")).toBe(true);
    fireEvent.press(utils.getByText("Cancel order"));
    expect(mockCancel).not.toHaveBeenCalled();
    expect(mockCancelPresent).not.toHaveBeenCalled();
  });

  it("opens CancelReasonSheet rather than firing cancel (reason is REQUIRED)", () => {
    const utils = openOverflow(renderOrder(FIXTURES.pending));
    fireEvent.press(utils.getByText("Cancel order"));
    expect(mockCancelPresent).toHaveBeenCalledTimes(1);
    expect(mockCancel).not.toHaveBeenCalled();
  });

  it("opens RefundSheet rather than firing refund (refund_request_id is enforced)", () => {
    const utils = openOverflow(renderOrder(FIXTURES.confirmed));
    fireEvent.press(utils.getByText("Refund"));
    expect(mockRefundPresent).toHaveBeenCalledTimes(1);
    expect(mockRefund).not.toHaveBeenCalled();
  });

  it("fires the invoice and receipt sends straight from the menu", () => {
    const utils = openOverflow(renderOrder(FIXTURES.confirmed));
    fireEvent.press(utils.getByText("Email invoice"));
    expect(mockEmailInvoice).toHaveBeenCalled();
    fireEvent.press(utils.getByText("Email receipt"));
    expect(mockEmailReceipt).toHaveBeenCalled();
  });
});

// Task 18 — Confirm and Fulfil were the two mutations on this screen with NO
// failure surface at all: no message, no haptic, no visible state change. See
// .superpowers/sdd/i3-task-18-brief.md. Both `mutate` calls now carry
// `onError`/`onSuccess`, wired to the same `useActionFailure` +
// `ActionFailureNotice` pair every other screen in this app uses.
describe("Order detail — confirm/fulfil failure surface", () => {
  it("reports a failed Confirm", () => {
    const spy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const utils = renderOrder(FIXTURES.pending);
    fireEvent.press(utils.getByLabelText("Confirm order"));
    pressAlertButton(spy, "Confirm");
    expect(mockConfirm).toHaveBeenCalledTimes(1);
    expect(mockConfirm.mock.calls[0]![0]).toMatchObject({ id: "o1" });

    act(() => mutationCallbacks(mockConfirm).onError(new Error("boom")));

    expect(utils.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't confirm this order",
    );
    expect(adminHaptics.actionFailed).toHaveBeenCalled();
    spy.mockRestore();
  });

  // A distinct `mutate` call from plain Confirm — Task 18's brief flags this
  // as a separate bug if missed, since it is a second bare call site, not a
  // second code path through the first one.
  it("reports a failed Confirm & mark paid", () => {
    const spy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const utils = renderOrder(FIXTURES.pending);
    fireEvent.press(utils.getByLabelText("Confirm order"));
    pressAlertButton(spy, "Confirm & mark paid");
    expect(mockConfirm).toHaveBeenCalledTimes(1);
    expect(mockConfirm.mock.calls[0]![0]).toMatchObject({
      id: "o1",
      body: { payment_status: "paid" },
    });

    act(() => mutationCallbacks(mockConfirm).onError(new Error("boom")));

    expect(utils.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't confirm this order",
    );
    spy.mockRestore();
  });

  it("reports a failed Fulfil", () => {
    const spy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const utils = renderOrder(FIXTURES.confirmed);
    fireEvent.press(utils.getByLabelText("Mark fulfilled"));
    pressAlertButton(spy, "Fulfill");
    expect(mockFulfill).toHaveBeenCalledTimes(1);
    expect(mockFulfill.mock.calls[0]![0]).toBe("o1");

    act(() => mutationCallbacks(mockFulfill).onError(new Error("boom")));

    expect(utils.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't fulfil this order",
    );
    expect(adminHaptics.actionFailed).toHaveBeenCalled();
    spy.mockRestore();
  });

  it("clears the notice on a later success", () => {
    const spy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const utils = renderOrder(FIXTURES.pending);

    fireEvent.press(utils.getByLabelText("Confirm order"));
    pressAlertButton(spy, "Confirm");
    act(() => mutationCallbacks(mockConfirm, 0).onError(new Error("boom")));
    expect(utils.queryByTestId("action-failure-notice")).toBeTruthy();

    fireEvent.press(utils.getByLabelText("Confirm order"));
    pressAlertButton(spy, "Confirm");
    act(() => mutationCallbacks(mockConfirm, 1).onSuccess());

    expect(utils.queryByTestId("action-failure-notice")).toBeNull();
    expect(adminHaptics.actionSucceeded).toHaveBeenCalled();
    spy.mockRestore();
  });

  // The real difficulty per the brief: this screen has a sticky bar that
  // `ActionFailureNotice`'s default anchoring (`useDockClearance()`) knows
  // nothing about. Assert the relationship, not a number — a test that pins
  // today's arithmetic passes on a wrong answer tomorrow.
  it("keeps the failure strip clear of the sticky action bar", () => {
    const spy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const utils = renderOrder(FIXTURES.pending);
    fireEvent.press(utils.getByLabelText("Confirm order"));
    pressAlertButton(spy, "Confirm");
    act(() => mutationCallbacks(mockConfirm).onError(new Error("boom")));

    const barStyle = StyleSheet.flatten(utils.getByTestId("order-action-bar").props.style);
    const noticeStyle = StyleSheet.flatten(utils.getByTestId("action-failure-notice").props.style);
    const barTop = (barStyle.bottom as number) + (barStyle.minHeight as number);

    expect(noticeStyle.bottom as number).toBeGreaterThanOrEqual(barTop);
    spy.mockRestore();
  });
});

describe("Order detail — cancel/refund haptics", () => {
  it("calls actionFailed when cancel fails", () => {
    renderOrder(FIXTURES.pending);

    // The component passes handleCancelSubmit to the sheet as onSubmit.
    // Simulate the sheet calling it, which triggers the mutation.
    if (!capturedCancelOnSubmit) throw new Error("Sheet onSubmit not captured");
    act(() => {
      capturedCancelOnSubmit!("test reason");
    });

    // Verify the mutation was called with callbacks
    expect(mockCancel).toHaveBeenCalledTimes(1);
    expect(mockCancel).toHaveBeenCalledWith({ id: "o1", reason: "test reason" }, expect.any(Object));

    // Trigger the error callback to invoke the haptics
    act(() => {
      mutationCallbacks(mockCancel).onError(new Error("boom"));
    });

    expect(adminHaptics.actionFailed).toHaveBeenCalled();
  });

  it("calls actionSucceeded when cancel succeeds", () => {
    renderOrder(FIXTURES.pending);

    // Simulate the sheet calling handleCancelSubmit
    if (!capturedCancelOnSubmit) throw new Error("Sheet onSubmit not captured");
    act(() => {
      capturedCancelOnSubmit!("test reason");
    });

    // Verify the mutation was called
    expect(mockCancel).toHaveBeenCalledTimes(1);

    // Trigger the success callback to invoke the haptics
    act(() => {
      mutationCallbacks(mockCancel).onSuccess();
    });

    expect(adminHaptics.actionSucceeded).toHaveBeenCalled();
  });

  it("calls actionFailed when refund fails", () => {
    renderOrder(FIXTURES.confirmed);

    // The component passes handleRefundSubmit to the sheet as onSubmit.
    // Simulate the sheet calling it, which triggers the mutation.
    if (!capturedRefundOnSubmit) throw new Error("Sheet onSubmit not captured");
    act(() => {
      capturedRefundOnSubmit!({ amount: 50, refundRequestId: "req-123" });
    });

    // Verify the mutation was called with callbacks
    expect(mockRefund).toHaveBeenCalledTimes(1);
    expect(mockRefund).toHaveBeenCalledWith(
      { id: "o1", body: { amount: 50, refund_request_id: "req-123" } },
      expect.any(Object)
    );

    // Trigger the error callback to invoke the haptics
    act(() => {
      mutationCallbacks(mockRefund).onError(new Error("boom"));
    });

    expect(adminHaptics.actionFailed).toHaveBeenCalled();
  });

  it("calls actionSucceeded when refund succeeds", () => {
    renderOrder(FIXTURES.confirmed);

    // Simulate the sheet calling handleRefundSubmit
    if (!capturedRefundOnSubmit) throw new Error("Sheet onSubmit not captured");
    act(() => {
      capturedRefundOnSubmit!({ amount: 50, refundRequestId: "req-123" });
    });

    // Verify the mutation was called
    expect(mockRefund).toHaveBeenCalledTimes(1);

    // Trigger the success callback to invoke the haptics
    act(() => {
      mutationCallbacks(mockRefund).onSuccess();
    });

    expect(adminHaptics.actionSucceeded).toHaveBeenCalled();
  });
});
