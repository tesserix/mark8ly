// Orders = the gesture-triage screen (inc2 Task 9). Pins the three things
// the brief calls out — a filter change refetches, the swipe actions reach
// the right mutations, and a long press opens the menu — plus the two
// judgement calls that carry real risk: which actions are LEGAL for a given
// order (this app has no undo, and a 409 is not a recoverable outcome for a
// merchant mid-triage), and the deliberate absence of an optimistic hide.
//
// Everything below the import block is mocked at the module boundary: this
// is a screen test, not an integration test.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));


jest.mock("expo-image", () => {
  const { View } = require("react-native");
  return { Image: View };
});

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

const mockPush = jest.fn();
jest.mock("expo-router", () => ({ useRouter: () => ({ push: mockPush }) }));

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

// --- controllable order list -------------------------------------------
// `useOrders` is called TWICE by the screen: once for the visible list and
// once, pinned to status "pending", for the header count. The mock keys off
// the params so the two can be asserted on independently — and so the count
// query can be proven not to disturb the list.
interface ListCall {
  status?: string;
  payment_status?: string;
  search?: string;
}
const mockListCalls: ListCall[] = [];
let mockOrders: unknown[] = [];
let mockPendingTotal = 0;
let mockIsError = false;
let mockIsLoading = false;
const mockRefetch = jest.fn();
const mockFetchNextPage = jest.fn();

jest.mock("@/lib/hooks/use-orders", () => ({
  useOrders: (params: ListCall = {}) => {
    mockListCalls.push(params);
    const isCountQuery = params.status === "pending" && params.search === undefined;
    const data = isCountQuery
      ? { pages: [{ data: [], meta: { page: 1, total: mockPendingTotal, total_pages: 1 } }] }
      : {
          pages: [
            {
              data: mockOrders.filter(
                (o) => !params.status || (o as { status: string }).status === params.status,
              ),
              meta: { page: 1, total: mockOrders.length, total_pages: 1 },
            },
          ],
        };
    return {
      data: mockIsError ? undefined : data,
      isError: mockIsError,
      isLoading: mockIsLoading,
      isRefetching: false,
      isFetchingNextPage: false,
      hasNextPage: false,
      refetch: mockRefetch,
      fetchNextPage: mockFetchNextPage,
    };
  },
}));

// --- controllable shipment ---------------------------------------------
let mockShipment: unknown = null;
const mockUseShipment = jest.fn();
jest.mock("@/lib/hooks/use-shipment", () => ({
  useShipment: (orderId: string, enabled?: boolean) => {
    mockUseShipment(orderId, enabled);
    return { data: mockShipment };
  },
}));

// --- controllable mutations --------------------------------------------
const mockConfirmOrder = jest.fn();
const mockFulfillOrder = jest.fn();
const mockCancelOrder = jest.fn();
const mockRefundOrder = jest.fn();
const mockEmailLabel = jest.fn();

const mockPending = {
  confirmOrder: false,
  fulfillOrder: false,
  cancelOrder: false,
  refundOrder: false,
};

// Sticky, never-reset mutation errors — react-query does not clear them. If
// a sheet is ever bound straight back to `mutation.error`, the "does not
// leak between orders" suite goes red on the first open. Same trap the
// Dashboard shipped once.
const mockStickyError = new Error("a mutation that failed some time ago");

jest.mock("@/lib/admin-api/order-actions", () => ({
  useConfirmOrder: () => ({ mutate: mockConfirmOrder, isPending: mockPending.confirmOrder }),
  useFulfillOrder: () => ({ mutate: mockFulfillOrder, isPending: mockPending.fulfillOrder }),
  useCancelOrder: () => ({
    mutate: mockCancelOrder,
    isPending: mockPending.cancelOrder,
    error: mockStickyError,
  }),
  useRefundOrder: () => ({
    mutate: mockRefundOrder,
    isPending: mockPending.refundOrder,
    error: mockStickyError,
  }),
}));

jest.mock("@/lib/admin-api/shipment-actions", () => ({
  useEmailLabel: () => ({ mutate: mockEmailLabel, isPending: false }),
}));

import { act, fireEvent, render } from "@testing-library/react-native";
import { FlatList } from "react-native";
import Animated from "react-native-reanimated";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import OrdersScreen from "../app/(tabs)/orders/index";
import type { Order } from "@repo/mobile-shared/api/types";

// jest-expo mocks `react-native-reanimated` with a stub whose default
// `Animated` export provides ONLY `View` and `ScrollView`. The real library
// also exports `Animated.FlatList`, which this screen uses so the
// CollapsingHeader can be scroll-driven from a VIRTUALIZED list — the
// Dashboard could use a plain `Animated.ScrollView` because its queue is
// capped at a handful of rows, whereas Orders pages at 50 and scrolls
// infinitely. Rendering the undefined export crashes inside nativewind's
// interop with "Cannot read properties of undefined (reading 'displayName')".
//
// Patched onto the existing stub rather than re-mocking the module
// (`react-native-reanimated/mock` re-enters the real source, which needs the
// native Worklets runtime and throws under jest) and rather than downgrading
// the screen to a plain FlatList with a JS-thread `onScroll` — the gap is in
// the test environment, not the app. `Animated.FlatList` is read per render,
// not at module load, so assigning it here is early enough. Promote to
// lib/test-support/ if a second animated list needs it.
(Animated as unknown as Record<string, unknown>).FlatList = FlatList;

function order(over: Partial<Order> = {}): Order {
  return {
    id: "o1",
    order_number: "1042",
    customer_email: "ana@example.com",
    customer_name: "Ana Ruiz",
    status: "pending",
    payment_status: "pending",
    fulfillment_status: "unfulfilled",
    grand_total: 189,
    refunded_amount: 0,
    currency_code: "AUD",
    placed_at: new Date(Date.now() - 12 * 60_000).toISOString(),
    ...over,
  } as unknown as Order;
}

const PENDING = order();
const CONFIRMED = order({ id: "o2", order_number: "1043", status: "confirmed", payment_status: "paid" });
const FULFILLED = order({ id: "o3", order_number: "1044", status: "fulfilled", payment_status: "paid" });
const CANCELLED = order({ id: "o4", order_number: "1045", status: "cancelled" });

beforeEach(() => {
  jest.clearAllMocks();
  mockListCalls.length = 0;
  mockOrders = [PENDING, CONFIRMED, FULFILLED, CANCELLED];
  mockPendingTotal = 5;
  mockIsError = false;
  mockIsLoading = false;
  mockShipment = null;
  mockPending.confirmOrder = false;
  mockPending.fulfillOrder = false;
  mockPending.cancelOrder = false;
  mockPending.refundOrder = false;
});

/** Opens the long-press menu on a given row. */
function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`order-row-${id}`) as never, "longPress");
}

describe("Orders — header", () => {
  it("titles the screen and carries the pending count in the right slot", () => {
    const { getAllByText, getByTestId } = render(<OrdersScreen />);
    // CollapsingHeader mounts both the expanded and the collapsed layer.
    expect(getAllByText("Inbox").length).toBeGreaterThan(0);
    expect(getByTestId("orders-pending-count")).toHaveTextContent("5 pending");
  });

  it("hides the count entirely when nothing is pending", () => {
    mockPendingTotal = 0;
    const { queryByTestId } = render(<OrdersScreen />);
    expect(queryByTestId("orders-pending-count")).toBeNull();
  });

  it("sources the count from its own status-pinned query, not the visible list", () => {
    render(<OrdersScreen />);
    expect(mockListCalls).toContainEqual({ status: "pending" });
  });
});

describe("Orders — filters", () => {
  const LABELS = ["All", "Pending", "Confirmed", "Completed", "Cancelled"];

  // Scoped to the chip testIDs, not `getByText`: "Pending" and "Cancelled"
  // are also order-status badge copy in the rows below.
  it("renders the filter set as pill chips", () => {
    const { getByTestId, getByText } = render(<OrdersScreen />);
    const keys = ["all", "pending", "confirmed", "completed", "cancelled"];
    for (const key of keys) expect(getByTestId(`filter-chip-${key}`)).toBeTruthy();
    expect(getByText("All")).toBeTruthy();
    expect(getByText("Completed")).toBeTruthy();
    expect(LABELS).toHaveLength(keys.length);
  });

  it("refetches under the new status when a chip is tapped", () => {
    const { getByTestId } = render(<OrdersScreen />);
    mockListCalls.length = 0;
    fireEvent.press(getByTestId("filter-chip-confirmed-target"));
    expect(mockListCalls).toContainEqual({ status: "confirmed" });
  });

  it("maps Completed onto the real 'fulfilled' status", () => {
    const { getByTestId } = render(<OrdersScreen />);
    mockListCalls.length = 0;
    fireEvent.press(getByTestId("filter-chip-completed-target"));
    expect(mockListCalls).toContainEqual({ status: "fulfilled" });
  });

  it("fires the selection haptic on a filter change", () => {
    const { getByTestId } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("filter-chip-confirmed-target"));
    expect(adminHaptics.selectionChanged).toHaveBeenCalled();
  });

  // The hard-won constraint: the backend matches status EXACTLY
  // (orders.go:170 `status = ?`), so a comma-joined "pending,confirmed"
  // silently matches NOTHING. One real status per chip, forever.
  it("never sends a comma-joined status", () => {
    const { getByTestId } = render(<OrdersScreen />);
    for (const key of ["all", "pending", "confirmed", "completed", "cancelled"]) {
      fireEvent.press(getByTestId(`filter-chip-${key}-target`));
    }
    for (const call of mockListCalls) {
      expect(call.status ?? "").not.toContain(",");
    }
  });
});

describe("Orders — search moves into the scroll content", () => {
  it("renders the search field in the list header, not pinned above it", () => {
    const { getByTestId } = render(<OrdersScreen />);
    expect(getByTestId("orders-search-block")).toBeTruthy();
  });

  // The list opens scrolled PAST the search block, so it costs no permanent
  // screen height — pulling down reveals it.
  it("starts scrolled past the search block", () => {
    const { getByTestId } = render(<OrdersScreen />);
    const list = getByTestId("orders-list");
    expect(list.props.contentOffset.y).toBeGreaterThan(0);
  });
});

describe("Orders — rows", () => {
  it("renders a row per order", () => {
    const { getByTestId } = render(<OrdersScreen />);
    for (const o of [PENDING, CONFIRMED, FULFILLED, CANCELLED]) {
      expect(getByTestId(`order-row-${o.id}`)).toBeTruthy();
    }
  });

  it("opens the order on tap", () => {
    const { getByTestId } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("order-row-o1"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/orders/o1");
  });
});

describe("Orders — swipe actions", () => {
  it("Approve on a pending order confirms it (there is no approve endpoint)", () => {
    const { getByTestId } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(mockConfirmOrder).toHaveBeenCalledTimes(1);
    expect(mockConfirmOrder.mock.calls[0][0]).toMatchObject({ id: "o1" });
  });

  // Confirming an already-confirmed order is a guaranteed 409, and this app
  // has no undo. The leading edge is empty rather than armed with a mistake.
  it("offers no Approve on an order that is already confirmed", () => {
    const { queryByTestId } = render(<OrdersScreen />);
    expect(queryByTestId("swipe-o2-action-approve")).toBeNull();
  });

  it("offers Cancel while an order can still be cancelled", () => {
    const { getByTestId } = render(<OrdersScreen />);
    expect(getByTestId("swipe-o1-action-cancel")).toBeTruthy();
    expect(getByTestId("swipe-o2-action-cancel")).toBeTruthy();
  });

  it("offers no swipe at all on a terminal order", () => {
    const { queryByTestId } = render(<OrdersScreen />);
    expect(queryByTestId("swipe-o3-action-cancel")).toBeNull();
    expect(queryByTestId("swipe-o3-action-approve")).toBeNull();
    expect(queryByTestId("swipe-o4-action-cancel")).toBeNull();
  });

  // `ordersApi.cancel(id, reason)` has a REQUIRED reason — omitting it is an
  // unconditional 400. The revealed Cancel opens the sheet, exactly as the
  // order detail screen does.
  it("Cancel opens the reason sheet instead of firing the mutation", () => {
    const { getByTestId } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    expect(mockCancelOrder).not.toHaveBeenCalled();
  });

  it("submits the reason from the sheet to the cancel mutation", () => {
    const { getByTestId, getByLabelText } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    fireEvent.changeText(getByLabelText("Cancellation reason"), "Out of stock");
    fireEvent.press(getByLabelText("Cancel order"));
    expect(mockCancelOrder).toHaveBeenCalledTimes(1);
    expect(mockCancelOrder.mock.calls[0][0]).toEqual({ id: "o1", reason: "Out of stock" });
  });

  it("never opts any action into full-swipe auto-fire (this app has no undo)", () => {
    const { UNSAFE_root } = render(<OrdersScreen />);
    const rows = UNSAFE_root.findAll(
      (n) => typeof n.type !== "string" && (n.type as { name?: string }).name === "SwipeRow",
    );
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      const actions = [
        ...((row.props.leadingActions ?? []) as { autoFireOnFullSwipe?: boolean }[]),
        ...((row.props.trailingActions ?? []) as { autoFireOnFullSwipe?: boolean }[]),
      ];
      for (const a of actions) expect(a.autoFireOnFullSwipe).toBeFalsy();
    }
  });
});

describe("Orders — no optimistic hide", () => {
  // Unlike the Dashboard, this list is the authority on its own contents:
  // `useOrders` is keyed ["orders","list",…] and every order mutation
  // invalidates the ["orders"] prefix, so the refetch removes (or re-badges)
  // the row by itself. Hiding it locally would only re-introduce the
  // watermark problem the Dashboard needed four rounds to get right.
  it("leaves the row on screen after Approve — the refetch is the authority", () => {
    const { getByTestId } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(getByTestId("order-row-o1")).toBeTruthy();
  });

  // Without a hide, the row stays swipeable — so the in-flight window is
  // closed by DISABLING the gesture, not by faking the result. This keys on
  // the mutation lifecycle only ("stop blocking the control"), never on
  // "fresh data arrived".
  it("disables that row's swipe while its own mutation is in flight", () => {
    const { getByTestId, rerender, UNSAFE_root } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    mockPending.confirmOrder = true;
    rerender(<OrdersScreen />);

    const rows = UNSAFE_root.findAll(
      (n) => typeof n.type !== "string" && (n.type as { name?: string }).name === "SwipeRow",
    );
    const busy = rows.find((r) => r.props.testID === "swipe-o1");
    const other = rows.find((r) => r.props.testID === "swipe-o2");
    expect(busy?.props.enabled).toBe(false);
    // Only the row being acted on — the rest of the list stays live.
    expect(other?.props.enabled).not.toBe(false);
  });

  it("re-enables the row once the mutation settles", () => {
    const { getByTestId, rerender, UNSAFE_root } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    act(() => (mockConfirmOrder.mock.calls[0][1].onSuccess as () => void)());
    rerender(<OrdersScreen />);

    const rows = UNSAFE_root.findAll(
      (n) => typeof n.type !== "string" && (n.type as { name?: string }).name === "SwipeRow",
    );
    expect(rows.find((r) => r.props.testID === "swipe-o1")?.props.enabled).not.toBe(false);
  });

  it("fires the success and failure haptics", () => {
    const { getByTestId } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    act(() => (mockConfirmOrder.mock.calls[0][1].onSuccess as () => void)());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalled();

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    act(() => (mockConfirmOrder.mock.calls[1][1].onError as (e: Error) => void)(new Error("x")));
    expect(adminHaptics.actionFailed).toHaveBeenCalled();
  });
});

describe("Orders — long-press menu", () => {
  it("opens on long press with all four actions", () => {
    const { getByTestId } = render(<OrdersScreen />);
    longPress(getByTestId, "o1");
    for (const key of ["fulfil", "email-label", "refund", "cancel"]) {
      expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
    }
  });

  // Fulfil is the ONE action in the menu that fires a mutation directly —
  // it needs no extra input. The other three open their existing sheets.
  it("Fulfil fires the fulfil mutation directly on a confirmed order", () => {
    const { getByTestId } = render(<OrdersScreen />);
    longPress(getByTestId, "o2");
    fireEvent.press(getByTestId("action-sheet-item-fulfil"));
    expect(mockFulfillOrder).toHaveBeenCalledTimes(1);
    expect(mockFulfillOrder.mock.calls[0][0]).toBe("o2");
  });

  it("disables Fulfil on an order that is not yet confirmed", () => {
    const { getByTestId } = render(<OrdersScreen />);
    longPress(getByTestId, "o1");
    fireEvent.press(getByTestId("action-sheet-item-fulfil"));
    expect(mockFulfillOrder).not.toHaveBeenCalled();
  });

  // NB: the shared gorhom mock renders a sheet's children unconditionally,
  // so "the amount field is on screen" is true whether or not the sheet was
  // ever presented and proves nothing. These assert on what the sheet's
  // submit actually reaches instead.
  it("Refund opens the refund sheet rather than firing a bare mutation", () => {
    const { getByTestId, getByLabelText } = render(<OrdersScreen />);
    longPress(getByTestId, "o2");
    fireEvent.press(getByTestId("action-sheet-item-refund"));
    expect(mockRefundOrder).not.toHaveBeenCalled();

    fireEvent.press(getByLabelText("Issue refund"));
    expect(mockRefundOrder).toHaveBeenCalledTimes(1);
    expect(mockRefundOrder.mock.calls[0][0]).toMatchObject({ id: "o2" });
  });

  it("disables Refund on an unpaid order", () => {
    const { getByTestId, getByLabelText } = render(<OrdersScreen />);
    longPress(getByTestId, "o1");
    expect(getByTestId("action-sheet-item-refund").props.accessibilityState.disabled).toBe(true);

    fireEvent.press(getByTestId("action-sheet-item-refund"));
    // No target was armed, so even driving the always-mounted sheet's submit
    // reaches nothing.
    fireEvent.press(getByLabelText("Issue refund"));
    expect(mockRefundOrder).not.toHaveBeenCalled();
  });

  it("Cancel opens the reason sheet rather than firing a bare mutation", () => {
    const { getByTestId, getByLabelText, getAllByLabelText } = render(<OrdersScreen />);
    longPress(getByTestId, "o1");
    fireEvent.press(getByTestId("action-sheet-item-cancel"));
    expect(mockCancelOrder).not.toHaveBeenCalled();

    // Two controls answer to "Cancel order" once the menu is open: the menu
    // ITEM and the reason sheet's SUBMIT. Only one is reachable at a time in
    // the real app (the sheet is modal), but the shared gorhom mock mounts
    // sheet children unconditionally, so the submit is picked out by
    // excluding the menu item's testID rather than by position.
    fireEvent.changeText(getByLabelText("Cancellation reason"), "Duplicate");
    const submit = getAllByLabelText("Cancel order").find(
      (node) => node.props.testID !== "action-sheet-item-cancel",
    );
    fireEvent.press(submit as never);
    expect(mockCancelOrder.mock.calls[0][0]).toEqual({ id: "o1", reason: "Duplicate" });
  });

  // "Email label" emails a label that belongs to a SHIPMENT — an id the
  // orders LIST payload does not carry. It is fetched lazily for the
  // long-pressed order only, and the item stays disabled until one exists.
  it("fetches the shipment only for the long-pressed order", () => {
    const { getByTestId } = render(<OrdersScreen />);
    expect(mockUseShipment).toHaveBeenCalledWith("", false);
    mockUseShipment.mockClear();
    longPress(getByTestId, "o2");
    expect(mockUseShipment).toHaveBeenCalledWith("o2", true);
  });

  it("disables Email label when the order has no shipment", () => {
    const { getByTestId, getByLabelText } = render(<OrdersScreen />);
    longPress(getByTestId, "o2");
    expect(getByTestId("action-sheet-item-email-label").props.accessibilityState.disabled).toBe(
      true,
    );

    fireEvent.press(getByTestId("action-sheet-item-email-label"));
    fireEvent.changeText(getByLabelText("Recipient email"), "warehouse@example.com");
    fireEvent.press(getByLabelText("Send label"));
    expect(mockEmailLabel).not.toHaveBeenCalled();
  });

  it("emails the label for the shipment once one exists", () => {
    mockShipment = { id: "sh1", provider: "delhivery" };
    const { getByTestId, getByLabelText } = render(<OrdersScreen />);
    longPress(getByTestId, "o2");
    fireEvent.press(getByTestId("action-sheet-item-email-label"));
    fireEvent.changeText(getByLabelText("Recipient email"), "warehouse@example.com");
    fireEvent.press(getByLabelText("Send label"));
    expect(mockEmailLabel).toHaveBeenCalledTimes(1);
    expect(mockEmailLabel.mock.calls[0][0]).toMatchObject({
      orderId: "o2",
      shipmentId: "sh1",
      recipient: "warehouse@example.com",
    });
  });
});

describe("Orders — the cancel error does not leak between orders", () => {
  const ERROR_COPY = "Couldn't cancel this order. Try again.";

  it("shows no error on first open, despite a sticky mutation error", () => {
    const { getByTestId, queryByText } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    expect(queryByText(ERROR_COPY)).toBeNull();
  });

  it("clears the error when the sheet is re-opened for a DIFFERENT order", () => {
    const { getByTestId, getByLabelText, getByText, queryByText } = render(<OrdersScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    fireEvent.changeText(getByLabelText("Cancellation reason"), "Out of stock");
    fireEvent.press(getByLabelText("Cancel order"));
    act(() => (mockCancelOrder.mock.calls[0][1].onError as (e: Error) => void)(new Error("500")));
    expect(getByText(ERROR_COPY)).toBeTruthy();

    fireEvent.press(getByTestId("swipe-o2-action-cancel"));
    expect(queryByText(ERROR_COPY)).toBeNull();
  });
});

describe("Orders — load and failure states", () => {
  it("offers a retry when the list fails outright", () => {
    mockIsError = true;
    mockOrders = [];
    const { getByText } = render(<OrdersScreen />);
    fireEvent.press(getByText("Try again"));
    expect(mockRefetch).toHaveBeenCalled();
  });

  it("shows the empty state when there are no orders", () => {
    mockOrders = [];
    const { getByText } = render(<OrdersScreen />);
    expect(getByText("No orders found")).toBeTruthy();
  });
});
