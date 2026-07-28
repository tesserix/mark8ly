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
// Keyed BY ORDER, and honouring `enabled`, because both are load-bearing:
// the screen drives one lazy probe whose key is derived from whichever
// sheet/menu is open, so "which order is being probed" and "is the probe on
// at all" are exactly the properties under test. A mock that returned the
// same shipment for every id, enabled or not, could not tell a correct probe
// from a probe pointed at the wrong order — or at nothing.
let mockShipments: Record<string, unknown> = {};
const mockUseShipment = jest.fn();
jest.mock("@/lib/hooks/use-shipment", () => ({
  useShipment: (orderId: string, enabled?: boolean) => {
    mockUseShipment(orderId, enabled);
    return { data: enabled && orderId ? mockShipments[orderId] : undefined };
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
import { FlatList, StyleSheet } from "react-native";
import Animated from "react-native-reanimated";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import OrdersScreen from "../app/(tabs)/orders/index";
import { MAX_FONT_SCALE } from "@/components/ui";
import { theme } from "@/lib/theme";
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
  mockShipments = {};
  mockPending.confirmOrder = false;
  mockPending.fulfillOrder = false;
  mockPending.cancelOrder = false;
  mockPending.refundOrder = false;
});

/** Opens the long-press menu on a given row. */
function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`order-row-${id}`) as never, "longPress");
}

type Root = ReturnType<typeof render>["UNSAFE_root"];

/** Every mounted `SwipeRow` element, so its props can be read directly. */
function swipeRows(root: Root) {
  return root.findAll(
    (n) => typeof n.type !== "string" && (n.type as { name?: string }).name === "SwipeRow",
  );
}

function swipeRow(root: Root, testID: string) {
  return swipeRows(root).find((r) => r.props.testID === testID);
}

interface RowActions {
  leadingActions?: { key: string; tone: string }[];
  trailingActions?: { key: string; tone: string }[];
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

  // `CollapsingHeader` computes its container height from MAX_FONT_SCALE and
  // gives every line it draws itself a known allowance. A `rightSlot` is
  // outside that loop, so an uncapped, unbounded-line-count text slot is
  // measured against a box that was never sized for it — the primitive's
  // Dynamic Type contract broken from the caller's side. Asserted here
  // because the Gate-A run that would have caught it only ever exercised the
  // Dashboard's fixed 40pt monogram slot.
  it("honours the header's Dynamic Type contract in the slot it fills", () => {
    const { getByTestId } = render(<OrdersScreen />);
    const count = getByTestId("orders-pending-count");
    expect(count.props.maxFontSizeMultiplier).toBe(MAX_FONT_SCALE);
    expect(count.props.numberOfLines).toBe(1);
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

// Search is PINNED — visible at rest, outside the list. It briefly lived
// inside the scroll content with the list parked past it; on device that
// left it with no affordance of any kind and it was undiscoverable. These
// pin the revert so nobody re-hides it by reintroducing either half of the
// mechanism (a list header, or a non-zero contentOffset).
describe("Orders — the search field is pinned", () => {
  it("renders the search field", () => {
    const { getByTestId, getByLabelText } = render(<OrdersScreen />);
    expect(getByTestId("orders-search-block")).toBeTruthy();
    expect(getByLabelText("Search orders")).toBeTruthy();
  });

  it("does not park the list past a hidden search field", () => {
    const { getByTestId } = render(<OrdersScreen />);
    const list = getByTestId("orders-list");
    // Either absent, or explicitly at the top — never a positive y, which is
    // what scrolled the field out of view.
    expect(list.props.contentOffset?.y ?? 0).toBe(0);
  });

  it("keeps the field out of the list, so scrolling can never take it away", () => {
    const { getByTestId } = render(<OrdersScreen />);
    expect(getByTestId("orders-list").props.ListHeaderComponent).toBeFalsy();
  });

  // The field is visible on a list WITH rows and on an empty one alike. The
  // old behaviour differed between the two: `flexGrow: 1` made an empty
  // list's content exactly viewport height, so the contentOffset could not
  // be honoured and the field showed at rest only when there was nothing to
  // search — the screen was inconsistent with itself.
  it("shows the field on an empty store too", () => {
    mockOrders = [];
    const { getByTestId, getByText } = render(<OrdersScreen />);
    expect(getByTestId("orders-search-block")).toBeTruthy();
    expect(getByText("No orders found")).toBeTruthy();
  });

  it("reads the scroll offset straight through, with no rebase to cancel", () => {
    const { getByTestId } = render(<OrdersScreen />);
    // A rebase (`contentOffset.y - SEARCH_BLOCK_HEIGHT`) only ever existed
    // to undo the park. Both go together; neither should come back alone.
    expect(getByTestId("orders-list").props.onScroll).toBeTruthy();
    expect(getByTestId("orders-list").props.contentOffset?.y ?? 0).toBe(0);
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
    const rows = swipeRows(UNSAFE_root);
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

// WHICH SIDE and WHICH COLOUR, not just "the action exists".
//
// `SwipeRow` gives leading and trailing buttons the SAME testID pattern
// (`${testID}-action-${key}`), so every test that reaches an action by its
// id passes identically whether Cancel sits on the leading edge or the
// trailing one: swapping the two props left the whole suite green while
// putting the destructive action under the constructive gesture. Tone had
// no assertion anywhere either, so painting Cancel moss was equally free.
//
// In an app with no undo, on the screen with the destructive actions, the
// side/colour pairing IS the safety property — a merchant's thumb learns
// "right is safe, left is not" across every list, and one screen that
// inverts it is worse than one that has no gesture at all.
describe("Orders — the swipe convention", () => {
  const CONSTRUCTIVE_TONE = "accent";
  const DESTRUCTIVE_TONE = "danger";

  it("puts Approve on the LEADING edge (drag right) in the accent tone", () => {
    const { UNSAFE_root } = render(<OrdersScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-o1")?.props as RowActions;
    expect(row.leadingActions).toHaveLength(1);
    expect(row.leadingActions?.[0]).toMatchObject({
      key: "approve",
      tone: CONSTRUCTIVE_TONE,
    });
  });

  it("puts Cancel on the TRAILING edge (drag left) in the danger tone", () => {
    const { UNSAFE_root } = render(<OrdersScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-o1")?.props as RowActions;
    expect(row.trailingActions).toHaveLength(1);
    expect(row.trailingActions?.[0]).toMatchObject({
      key: "cancel",
      tone: DESTRUCTIVE_TONE,
    });
  });

  // The invariant stated as an invariant, over every row rather than one:
  // nothing destructive may ever be reachable by the constructive gesture.
  it("never puts a destructive action on the leading edge, on any row", () => {
    const { UNSAFE_root } = render(<OrdersScreen />);
    const rows = swipeRows(UNSAFE_root);
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      const { leadingActions = [], trailingActions = [] } = row.props as RowActions;
      for (const action of leadingActions) expect(action.tone).not.toBe(DESTRUCTIVE_TONE);
      for (const action of trailingActions) expect(action.tone).not.toBe(CONSTRUCTIVE_TONE);
    }
  });

  // Tone → paint, so a tone swap is caught at the pixel and not only at the
  // prop. Moss is the screen's ONE accent and it is spent here; the active
  // filter chip is deliberately ink (see filter-chips.test.tsx).
  it("paints Approve moss and Cancel danger", () => {
    const { getByTestId } = render(<OrdersScreen />);
    const approve = StyleSheet.flatten(getByTestId("swipe-o1-action-approve").props.style);
    const cancel = StyleSheet.flatten(getByTestId("swipe-o1-action-cancel").props.style);
    expect(approve.backgroundColor).toBe(theme.colors.accent);
    expect(cancel.backgroundColor).toBe(theme.colors.danger);
    expect(approve.backgroundColor).not.toBe(cancel.backgroundColor);
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

    expect(swipeRow(UNSAFE_root, "swipe-o1")?.props.enabled).toBe(false);
    // Only the row being acted on — the rest of the list stays live.
    expect(swipeRow(UNSAFE_root, "swipe-o2")?.props.enabled).not.toBe(false);
  });

  it("re-enables the row once the mutation settles", () => {
    const { getByTestId, rerender, UNSAFE_root } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    act(() => (mockConfirmOrder.mock.calls[0][1].onSuccess as () => void)());
    rerender(<OrdersScreen />);

    expect(swipeRow(UNSAFE_root, "swipe-o1")?.props.enabled).not.toBe(false);
  });

  // The guard is per ROW, and triage is a queue: a merchant fires the next
  // row long before the previous one's request comes back. Held in a single
  // slot, the second action overwrote the first's guard, and the FIRST
  // order's `onSuccess` then cleared the slot outright — re-arming the
  // second row while its own request was still open, which is exactly the
  // double-fire the guard exists to prevent.
  it("guards two in-flight rows at once", () => {
    const { getByTestId, UNSAFE_root } = render(<OrdersScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    longPress(getByTestId, "o2");
    fireEvent.press(getByTestId("action-sheet-item-fulfil"));

    expect(mockConfirmOrder).toHaveBeenCalledTimes(1);
    expect(mockFulfillOrder).toHaveBeenCalledTimes(1);
    expect(swipeRow(UNSAFE_root, "swipe-o1")?.props.enabled).toBe(false);
    expect(swipeRow(UNSAFE_root, "swipe-o2")?.props.enabled).toBe(false);
  });

  it("releases only the row that settled, leaving the other still guarded", () => {
    const { getByTestId, UNSAFE_root } = render(<OrdersScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    longPress(getByTestId, "o2");
    fireEvent.press(getByTestId("action-sheet-item-fulfil"));

    act(() => (mockConfirmOrder.mock.calls[0][1].onSuccess as () => void)());

    expect(swipeRow(UNSAFE_root, "swipe-o1")?.props.enabled).not.toBe(false);
    expect(swipeRow(UNSAFE_root, "swipe-o2")?.props.enabled).toBe(false);
  });

  // The swipe guard alone left a SECOND, ungated route onto an in-flight
  // row: the long-press menu, whose Fulfil fires a mutation directly. Today's
  // blast radius is small (the backend's CanTransitionTo rejects the second
  // call and `fulfilled` is terminal) — but this row is the template
  // increment 3's long-press menus copy, and Archive / Delete / Disable are
  // not idempotent the same way. Both routes onto a row must read the same
  // guard.
  it("suppresses the long-press menu on a row whose own mutation is in flight", () => {
    const { getByTestId, queryByTestId } = render(<OrdersScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(mockConfirmOrder).toHaveBeenCalledTimes(1);

    // The gesture no longer resolves to a handler, so no sheet opens and
    // there is no second Fulfil to press.
    longPress(getByTestId, "o1");
    expect(queryByTestId("action-sheet-item-fulfil")).toBeNull();
    expect(mockFulfillOrder).not.toHaveBeenCalled();

    // A DIFFERENT row is untouched — the guard is per row, like the swipe's.
    longPress(getByTestId, "o2");
    expect(getByTestId("action-sheet-item-fulfil")).toBeTruthy();
  });

  it("restores the long-press menu once that row's mutation settles", () => {
    const { getByTestId } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    act(() => (mockConfirmOrder.mock.calls[0][1].onSuccess as () => void)());

    longPress(getByTestId, "o1");
    expect(getByTestId("action-sheet-item-fulfil")).toBeTruthy();
  });

  it("releases a row on failure too, so a failed action is retryable", () => {
    const { getByTestId, UNSAFE_root } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(swipeRow(UNSAFE_root, "swipe-o1")?.props.enabled).toBe(false);

    act(() => (mockConfirmOrder.mock.calls[0][1].onError as (e: Error) => void)(new Error("500")));
    expect(swipeRow(UNSAFE_root, "swipe-o1")?.props.enabled).not.toBe(false);
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
    mockShipments = { o2: { id: "sh1", provider: "delhivery" } };
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

// The lazy shipment probe drives IRREVERSIBLE copy, so "which order is it
// pointed at" is a correctness property, not a performance one.
//
// A full refund also cancels or returns the shipment at the carrier. The
// refund sheet says so — but only if it is told the order has a shipment,
// and the probe key was built from the menu order and the cancel target
// only. Tapping Refund dismisses the menu, which cleared the menu order, so
// by the time the sheet rendered the probe was disabled and the warning
// silently did not render: a merchant refunding a shipped order in full was
// never told, on a screen with no undo.
//
// The mirror-image failure is a target that is never released: neither sheet
// could report its own dismissal, so backing out of a cancel left that order
// pinned for the life of the screen — and it is ALSO what intermittently
// masked the bug above, since a stale cancel target kept the probe alive and
// made the warning appear on orders it had no business describing.
describe("Orders — the shipment probe follows the open sheet", () => {
  const CARRIER_WARNING =
    "A full refund will also cancel or return this shipment with the carrier.";
  const SHIPMENT = { id: "sh1", provider: "delhivery" };

  /** Long-presses an order and taps Refund — the path that clears the menu. */
  function openRefundFromMenu(getByTestId: (id: string) => unknown, id: string) {
    longPress(getByTestId, id);
    fireEvent.press(getByTestId(`action-sheet-item-refund`) as never);
  }

  it("warns that a full refund cancels the carrier shipment", () => {
    mockShipments = { o2: SHIPMENT };
    const { getByTestId, getByText } = render(<OrdersScreen />);
    openRefundFromMenu(getByTestId, "o2");
    expect(getByText(CARRIER_WARNING)).toBeTruthy();
  });

  it("keeps probing the refunded order after the menu that opened it closes", () => {
    mockShipments = { o2: SHIPMENT };
    const { getByTestId } = render(<OrdersScreen />);
    openRefundFromMenu(getByTestId, "o2");
    // Not "was o2 ever probed" — the menu probed it too. The LAST call is
    // the one the sheet renders against.
    expect(mockUseShipment.mock.calls.at(-1)).toEqual(["o2", true]);
  });

  it("says nothing about a carrier when the order never shipped", () => {
    mockShipments = {};
    const { getByTestId, queryByText } = render(<OrdersScreen />);
    openRefundFromMenu(getByTestId, "o2");
    expect(queryByText(CARRIER_WARNING)).toBeNull();
  });

  it("names the carrier on the cancel sheet for a shipped order", () => {
    mockShipments = { o1: SHIPMENT };
    const { getByTestId, getByText } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    expect(getByText(/The Delhivery shipment will also be cancelled\./)).toBeTruthy();
  });

  it("releases the cancel target when the sheet is dismissed without submitting", () => {
    mockShipments = { o1: SHIPMENT };
    const { getByTestId, getByLabelText } = render(<OrdersScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    fireEvent.press(getByLabelText("Keep order"));
    expect(mockCancelOrder).not.toHaveBeenCalled();
    expect(mockUseShipment.mock.calls.at(-1)).toEqual(["", false]);
  });

  // The exact sequence the reviewer reproduced: order A shipped, order B
  // not. Back out of A's cancel sheet, then refund B — B's sheet must not
  // inherit A's carrier warning.
  it("does not carry a dismissed cancel target's shipment into another order's refund", () => {
    mockShipments = { o1: SHIPMENT };
    const { getByTestId, getByLabelText, queryByText } = render(<OrdersScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    fireEvent.press(getByLabelText("Keep order"));

    openRefundFromMenu(getByTestId, "o2");
    expect(queryByText(CARRIER_WARNING)).toBeNull();
  });

  it("releases the refund target when the refund sheet is dismissed without submitting", () => {
    mockShipments = { o2: SHIPMENT };
    const { getByTestId, queryByText } = render(<OrdersScreen />);

    openRefundFromMenu(getByTestId, "o2");
    expect(queryByText(CARRIER_WARNING)).toBeTruthy();

    fireEvent.press(getByTestId("refund-sheet-dismiss"));
    expect(mockRefundOrder).not.toHaveBeenCalled();
    expect(queryByText(CARRIER_WARNING)).toBeNull();
    expect(mockUseShipment.mock.calls.at(-1)).toEqual(["", false]);
  });

  // Still lazy: nothing is probed while a merchant is only scrolling.
  it("probes nothing until a merchant asks for an action", () => {
    mockShipments = { o1: SHIPMENT, o2: SHIPMENT };
    render(<OrdersScreen />);
    for (const call of mockUseShipment.mock.calls) expect(call).toEqual(["", false]);
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
