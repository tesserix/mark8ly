// Dashboard = the action-queue screen (inc2 Task 8). These tests pin the
// three behaviours the brief calls out — the queue renders, the empty state
// renders, and a single failing source degrades to ONE muted row rather
// than taking the whole screen down — plus the swipe-action wiring, which
// is where the screen's only business logic lives.
//
// Everything below the import block is mocked at the module boundary: this
// is a screen test, not an integration test. Each data hook is independently
// controllable so a single source can be failed in isolation.
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

jest.mock("@/lib/hooks/use-notifications", () => ({
  useNotifications: () => ({ data: { notifications: [] } }),
}));

// --- controllable query state -------------------------------------------
interface QueryState {
  data: unknown;
  isError: boolean;
  /** react-query's fetch timestamp — the screen clears its optimistic
   *  `dismissed` overlay whenever the newest of the three moves. */
  dataUpdatedAt?: number;
}
const mockQueryState: Record<"dashboard" | "reviews" | "tickets", QueryState> = {
  dashboard: { data: undefined, isError: false },
  reviews: { data: undefined, isError: false },
  tickets: { data: undefined, isError: false },
};
const mockRefetchDashboard = jest.fn();
const mockRefetchReviews = jest.fn();
const mockRefetchTickets = jest.fn();

jest.mock("@/lib/hooks/use-dashboard", () => ({
  useDashboard: () => ({
    data: mockQueryState.dashboard.data,
    isError: mockQueryState.dashboard.isError,
    dataUpdatedAt: mockQueryState.dashboard.dataUpdatedAt ?? 0,
    isLoading: false,
    isRefetching: false,
    refetch: mockRefetchDashboard,
  }),
}));
jest.mock("@/lib/hooks/use-reviews", () => ({
  useReviews: () => ({
    data: mockQueryState.reviews.data,
    isError: mockQueryState.reviews.isError,
    dataUpdatedAt: mockQueryState.reviews.dataUpdatedAt ?? 0,
    isLoading: false,
    refetch: mockRefetchReviews,
  }),
}));
jest.mock("@/lib/hooks/use-tickets", () => ({
  useTickets: () => ({
    data: mockQueryState.tickets.data,
    isError: mockQueryState.tickets.isError,
    dataUpdatedAt: mockQueryState.tickets.dataUpdatedAt ?? 0,
    isLoading: false,
    refetch: mockRefetchTickets,
  }),
}));

// --- controllable mutations ---------------------------------------------
const mockConfirmOrder = jest.fn();
const mockCancelOrder = jest.fn();
const mockApproveReview = jest.fn();
const mockRejectReview = jest.fn();
const mockUpdateTicketStatus = jest.fn();

// `error` is deliberately a NON-null sticky value: react-query never resets
// a mutation error, and the screen must not read it. If the screen ever binds
// the cancel sheet back to `cancelOrder.error`, the "stale cancel error"
// suite below goes red on the very first open.
const mockStickyCancelError = new Error("a cancel that failed some time ago");

/**
 * In-flight state per mutation. Everything defaults to settled; a test that
 * needs "the request is still running" sets its flag and rerenders — that is
 * the window in which the optimistic hide must survive a refetch.
 */
const mockPending = {
  confirmOrder: false,
  cancelOrder: false,
  approveReview: false,
  rejectReview: false,
  updateTicketStatus: false,
};

jest.mock("@/lib/admin-api/order-actions", () => ({
  useConfirmOrder: () => ({ mutate: mockConfirmOrder, isPending: mockPending.confirmOrder }),
  useCancelOrder: () => ({
    mutate: mockCancelOrder,
    isPending: mockPending.cancelOrder,
    error: mockStickyCancelError,
  }),
}));
jest.mock("@/lib/admin-api/review-actions", () => ({
  useApproveReview: () => ({ mutate: mockApproveReview, isPending: mockPending.approveReview }),
  useRejectReview: () => ({ mutate: mockRejectReview, isPending: mockPending.rejectReview }),
}));
jest.mock("@/lib/admin-api/ticket-actions", () => ({
  useUpdateTicketStatus: () => ({
    mutate: mockUpdateTicketStatus,
    isPending: mockPending.updateTicketStatus,
  }),
}));

import { RefreshControl, StyleSheet } from "react-native";
import { act, fireEvent, render, within } from "@testing-library/react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import DashboardScreen from "../app/(tabs)/index";
import { theme } from "@/lib/theme";
import type { DashboardResponse } from "@repo/mobile-shared/api/types";

const STATS: DashboardResponse["stats"] = {
  revenue_today: 312,
  revenue_week: 1104,
  revenue_month: 4280,
  revenue_change_pct: 12.4,
  revenue_trend: [10, 14, 12, 22, 19, 28, 31],
  orders_today: 12,
  orders_pending: 1,
  orders_fulfilled: 38,
  orders_cancelled: 2,
  customers_total: 140,
  customers_new_this_week: 6,
  pending_reviews: 1,
};

function dashboardPayload(over: Partial<DashboardResponse> = {}): DashboardResponse {
  return {
    stats: STATS,
    recent_orders: [
      {
        id: "o1",
        order_number: "1042",
        customer_email: "ana@example.com",
        customer_name: "Ana Ruiz",
        grand_total: 189,
        status: "pending",
        created_at: "2026-07-27T09:00:00Z",
      },
    ],
    top_products: [],
    low_stock: [
      {
        id: "v1",
        product_id: "p1",
        title: "Sandstone Mug",
        variant_title: "Large",
        quantity: 2,
        low_stock_threshold: 10,
      },
    ],
    setup_checklist: {
      has_store: true,
      has_brand_assets: true,
      has_product: true,
      has_storefront_theme: true,
      has_payment_provider: true,
      has_shipping_carrier: true,
      has_return_policy: true,
      has_custom_domain: true,
    },
    ...over,
  };
}

/** Two pending orders — needed to prove an error raised on one order does
 *  not follow the merchant to a DIFFERENT order's cancel sheet. */
function twoPendingOrders(): DashboardResponse {
  return dashboardPayload({
    stats: { ...STATS, orders_pending: 2 },
    recent_orders: [
      {
        id: "o1",
        order_number: "1042",
        customer_email: "ana@example.com",
        customer_name: "Ana Ruiz",
        grand_total: 189,
        status: "pending",
        created_at: "2026-07-27T09:00:00Z",
      },
      {
        id: "o2",
        order_number: "1043",
        customer_email: "bo@example.com",
        customer_name: "Bo Chen",
        grand_total: 64.5,
        status: "pending",
        created_at: "2026-07-27T08:30:00Z",
      },
    ],
  });
}

const REVIEW_PAGES = {
  pages: [
    {
      data: [
        {
          id: "r1",
          product_id: "p1",
          customer_name: "Tom Baird",
          rating: 3,
          title: "Runs small but lovely",
          content: "Runs small but lovely",
          status: "pending",
          is_featured: false,
          created_at: "2026-07-27T08:00:00Z",
        },
      ],
    },
  ],
};

const TICKET_PAGES = {
  pages: [
    {
      data: [
        {
          id: "t1",
          subject: "Where is my order?",
          submitted_by_name: "Priya N.",
          status: "open",
          priority: "normal",
          created_at: "2026-07-27T07:00:00Z",
        },
      ],
    },
  ],
};

function loadAll() {
  mockQueryState.dashboard = { data: dashboardPayload(), isError: false };
  mockQueryState.reviews = { data: REVIEW_PAGES, isError: false };
  mockQueryState.tickets = { data: TICKET_PAGES, isError: false };
}

beforeEach(() => {
  jest.clearAllMocks();
  loadAll();
  mockPending.confirmOrder = false;
  mockPending.cancelOrder = false;
  mockPending.approveReview = false;
  mockPending.rejectReview = false;
  mockPending.updateTicketStatus = false;
});

describe("Dashboard — header", () => {
  /** `todayLabel()`'s shape: "Monday, 27 July". */
  const DATELINE = /^\w+day, \d{1,2} \w+$/;

  it("titles the screen with the store name and the date as the eyebrow", () => {
    const { getAllByText, getByText } = render(<DashboardScreen />);
    // The CollapsingHeader mounts both the expanded and collapsed layers.
    expect(getAllByText("Bondi Supply").length).toBeGreaterThan(0);
    // The eyebrow lives only in the expanded layer, so this is unambiguous.
    expect(getByText(DATELINE)).toBeTruthy();
  });

  // `CollapsingHeader`'s eyebrow DEFAULT is the uppercase small-caps label,
  // and this screen deliberately opts out: the dateline is running prose, the
  // first thing the eye lands on, and the mockup does not shout it. The
  // primitive's SUPPORT for `eyebrowPreset` is tested both ways in
  // collapsing-header.test.tsx; this pins its USE here, which deleting the
  // prop otherwise left every dashboard and header test green.
  //
  // Asserted on the resolved utility classes, not a flattened style:
  // NativeWind compiles classNames natively and does not resolve them to RN
  // style objects under jest, so `textTransform` is undefined for BOTH
  // presets here and would pass vacuously.
  it("renders the dateline in sentence case, not the uppercase eyebrow default", () => {
    const { getByText } = render(<DashboardScreen />);
    const className = getByText(DATELINE).props.className as string;
    expect(className).toContain("text-caption");
    expect(className).not.toContain("uppercase");
  });
});

describe("Dashboard — metrics band", () => {
  it("renders one elevated card with the hero numeral and the revenue chart", () => {
    const { getByTestId, getByText } = render(<DashboardScreen />);
    expect(getByTestId("dashboard-metrics-card")).toBeTruthy();
    expect(getByTestId("revenue-chart")).toBeTruthy();
    expect(getByText("$4,280")).toBeTruthy();
  });

  it("renders the four-up order strip", () => {
    const { getByTestId } = render(<DashboardScreen />);
    // Scoped to the card: "Pending" is also the order row's status badge.
    const card = within(getByTestId("dashboard-metrics-card"));
    expect(card.getByText("Today")).toBeTruthy();
    expect(card.getByText("Pending")).toBeTruthy();
    expect(card.getByText("Fulfilled")).toBeTruthy();
    expect(card.getByText("Cancelled")).toBeTruthy();
  });
});

describe("Dashboard — queue", () => {
  it("renders a row per queue item composed from all three sources", () => {
    const { getByTestId } = render(<DashboardScreen />);
    expect(getByTestId("queue-row-o1")).toBeTruthy();
    expect(getByTestId("queue-row-v1")).toBeTruthy();
    expect(getByTestId("queue-row-t1")).toBeTruthy();
    expect(getByTestId("queue-row-r1")).toBeTruthy();
  });

  it("wraps an order row in a SwipeRow with Approve leading and Cancel trailing", () => {
    const { getByTestId } = render(<DashboardScreen />);
    expect(getByTestId("swipe-o1-action-approve")).toBeTruthy();
    expect(getByTestId("swipe-o1-action-cancel")).toBeTruthy();
  });

  it("gives a low-stock row no swipe actions at all", () => {
    const { queryByTestId } = render(<DashboardScreen />);
    expect(queryByTestId("swipe-v1")).toBeNull();
  });

  it("gives a ticket row a trailing Close only", () => {
    const { getByTestId, queryByTestId } = render(<DashboardScreen />);
    expect(getByTestId("swipe-t1-action-close")).toBeTruthy();
    expect(queryByTestId("swipe-t1-action-approve")).toBeNull();
  });
});

// WHICH SIDE and WHICH COLOUR, not just "the action exists".
//
// Ported from orders-screen.test.tsx, where this net was written after the
// same gap was found there. It belongs HERE more than there: Orders has one
// row shape, the Dashboard has FOUR (order, review, ticket, low stock) and
// three of them wire swipe actions. `SwipeRow` gives leading and trailing
// buttons the SAME testID pattern (`${testID}-action-${key}`), so the
// positive assertions above pass identically whether Cancel sits on the
// leading edge or the trailing one — swapping `leadingActions` and
// `trailingActions` on every row above left the whole suite green while
// putting the destructive action under the constructive gesture.
//
// In an app with no undo, the side/colour pairing IS the safety property: a
// merchant's thumb learns "right is safe, left is not" across every list, and
// one screen that inverts it is worse than one with no gesture at all.
describe("Dashboard — the swipe convention", () => {
  const CONSTRUCTIVE_TONE = "accent";
  const DESTRUCTIVE_TONE = "danger";

  type Root = ReturnType<typeof render>["UNSAFE_root"];
  interface RowActions {
    leadingActions?: { key: string; tone: string }[];
    trailingActions?: { key: string; tone: string }[];
  }

  /** Every mounted `SwipeRow` element, so its props can be read directly. */
  function swipeRows(root: Root) {
    return root.findAll(
      (n) => typeof n.type !== "string" && (n.type as { name?: string }).name === "SwipeRow",
    );
  }

  function swipeRow(root: Root, testID: string) {
    return swipeRows(root).find((r) => r.props.testID === testID);
  }

  it("puts an order's Approve on the LEADING edge in the accent tone", () => {
    const { UNSAFE_root } = render(<DashboardScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-o1")?.props as RowActions;
    expect(row.leadingActions).toHaveLength(1);
    expect(row.leadingActions?.[0]).toMatchObject({
      key: "approve",
      tone: CONSTRUCTIVE_TONE,
    });
  });

  it("puts an order's Cancel on the TRAILING edge in the danger tone", () => {
    const { UNSAFE_root } = render(<DashboardScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-o1")?.props as RowActions;
    expect(row.trailingActions).toHaveLength(1);
    expect(row.trailingActions?.[0]).toMatchObject({
      key: "cancel",
      tone: DESTRUCTIVE_TONE,
    });
  });

  it("puts a review's Approve leading and its Reject trailing, in the same two tones", () => {
    const { UNSAFE_root } = render(<DashboardScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-r1")?.props as RowActions;
    expect(row.leadingActions?.[0]).toMatchObject({
      key: "approve",
      tone: CONSTRUCTIVE_TONE,
    });
    expect(row.trailingActions?.[0]).toMatchObject({
      key: "reject",
      tone: DESTRUCTIVE_TONE,
    });
  });

  // Closing a resolved ticket is a normal outcome, not a destruction — the
  // trailing edge is a POSITION, not a tone, and this is the row that proves
  // the invariant below isn't just "trailing means danger".
  it("puts a ticket's Close on the trailing edge in the NEUTRAL tone, with nothing leading", () => {
    const { UNSAFE_root } = render(<DashboardScreen />);
    const row = swipeRow(UNSAFE_root, "swipe-t1")?.props as RowActions;
    expect(row.leadingActions ?? []).toHaveLength(0);
    expect(row.trailingActions?.[0]).toMatchObject({ key: "close", tone: "neutral" });
  });

  // The invariant stated as an invariant, over every row rather than one.
  it("never puts a destructive action on the leading edge, on any row type", () => {
    const { UNSAFE_root } = render(<DashboardScreen />);
    const rows = swipeRows(UNSAFE_root);
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      const { leadingActions = [], trailingActions = [] } = row.props as RowActions;
      for (const action of leadingActions) expect(action.tone).not.toBe(DESTRUCTIVE_TONE);
      for (const action of trailingActions) expect(action.tone).not.toBe(CONSTRUCTIVE_TONE);
    }
  });

  // Tone → paint, so a tone swap is caught at the pixel and not only at the
  // prop. Moss is this screen's ONE accent and the spec spends it on the
  // chart and the Approve swipe — nowhere else.
  it("paints Approve moss and Cancel danger", () => {
    const { getByTestId } = render(<DashboardScreen />);
    const approve = StyleSheet.flatten(getByTestId("swipe-o1-action-approve").props.style);
    const cancel = StyleSheet.flatten(getByTestId("swipe-o1-action-cancel").props.style);
    expect(approve.backgroundColor).toBe(theme.colors.accent);
    expect(cancel.backgroundColor).toBe(theme.colors.danger);
    expect(approve.backgroundColor).not.toBe(cancel.backgroundColor);
  });

  // This app has no undo, so nothing may fire from the drag itself.
  it("never opts any action into full-swipe auto-fire", () => {
    const { UNSAFE_root } = render(<DashboardScreen />);
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

describe("Dashboard — empty state", () => {
  it("shows the All clear editorial moment when nothing needs the merchant", () => {
    mockQueryState.dashboard = {
      data: dashboardPayload({
        stats: { ...STATS, orders_pending: 0, pending_reviews: 0 },
        recent_orders: [],
        low_stock: [],
      }),
      isError: false,
    };
    mockQueryState.reviews = { data: { pages: [{ data: [] }] }, isError: false };
    mockQueryState.tickets = { data: { pages: [{ data: [] }] }, isError: false };

    const { getByText, queryByTestId } = render(<DashboardScreen />);
    expect(getByText("All clear")).toBeTruthy();
    expect(queryByTestId("dashboard-source-error")).toBeNull();
  });
});

describe("Dashboard — independent source failure", () => {
  it("keeps the rest of the screen when tickets fail, and names the failure once", () => {
    mockQueryState.tickets = { data: undefined, isError: true };

    const { getByTestId, queryByTestId, getAllByTestId } = render(<DashboardScreen />);

    // The screen did not fail wholesale.
    expect(getByTestId("dashboard-metrics-card")).toBeTruthy();
    expect(getByTestId("queue-row-o1")).toBeTruthy();
    expect(getByTestId("queue-row-r1")).toBeTruthy();
    expect(queryByTestId("queue-row-t1")).toBeNull();

    // Exactly ONE error row, naming what failed.
    expect(getAllByTestId("dashboard-source-error")).toHaveLength(1);
    expect(getByTestId("dashboard-source-error")).toHaveTextContent(/tickets/i);
  });

  it("retries only the failed source", () => {
    mockQueryState.tickets = { data: undefined, isError: true };
    const { getByTestId } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("dashboard-source-error-retry"));
    expect(mockRefetchTickets).toHaveBeenCalledTimes(1);
    expect(mockRefetchDashboard).not.toHaveBeenCalled();
    expect(mockRefetchReviews).not.toHaveBeenCalled();
  });

  it("still renders reviews and tickets when the dashboard itself fails", () => {
    mockQueryState.dashboard = { data: undefined, isError: true };
    const { getByTestId, queryByTestId } = render(<DashboardScreen />);

    expect(queryByTestId("dashboard-metrics-card")).toBeNull();
    expect(getByTestId("queue-row-r1")).toBeTruthy();
    expect(getByTestId("queue-row-t1")).toBeTruthy();
    // The row names the failure in merchant terms ("orders and stock"), not
    // in query-key terms ("dashboard") — it is copy a merchant reads.
    expect(getByTestId("dashboard-source-error")).toHaveTextContent(/orders and stock/i);
  });

  it("names every failed source in the one row", () => {
    mockQueryState.reviews = { data: undefined, isError: true };
    mockQueryState.tickets = { data: undefined, isError: true };
    const { getAllByTestId, getByTestId } = render(<DashboardScreen />);

    expect(getAllByTestId("dashboard-source-error")).toHaveLength(1);
    const row = getByTestId("dashboard-source-error");
    expect(row).toHaveTextContent(/reviews/i);
    expect(row).toHaveTextContent(/tickets/i);
  });
});

describe("Dashboard — swipe actions", () => {
  it("Approve on an order confirms it (there is no approve endpoint)", () => {
    const { getByTestId } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(mockConfirmOrder).toHaveBeenCalledTimes(1);
    expect(mockConfirmOrder.mock.calls[0][0]).toMatchObject({ id: "o1" });
  });

  it("optimistically removes the approved row", () => {
    const { getByTestId, queryByTestId } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(queryByTestId("queue-row-o1")).toBeNull();
  });

  it("rolls the row back and fires the failure haptic when the mutation errors", () => {
    const { getByTestId, queryByTestId } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(queryByTestId("queue-row-o1")).toBeNull();

    const onError = mockConfirmOrder.mock.calls[0][1].onError as (e: Error) => void;
    act(() => onError(new Error("boom")));

    expect(getByTestId("queue-row-o1")).toBeTruthy();
    expect(adminHaptics.actionFailed).toHaveBeenCalled();
  });

  it("fires the success haptic when the mutation resolves", () => {
    const { getByTestId } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    const onSuccess = mockConfirmOrder.mock.calls[0][1].onSuccess as () => void;
    act(() => onSuccess());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalled();
  });

  it("Cancel opens the reason sheet instead of firing the mutation", () => {
    const { getByTestId } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    expect(mockCancelOrder).not.toHaveBeenCalled();
  });

  it("submits the reason from the sheet to the cancel mutation", () => {
    const { getByTestId, getByLabelText } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    fireEvent.changeText(getByLabelText("Cancellation reason"), "Customer changed mind");
    fireEvent.press(getByLabelText("Cancel order"));
    expect(mockCancelOrder).toHaveBeenCalledTimes(1);
    expect(mockCancelOrder.mock.calls[0][0]).toEqual({
      id: "o1",
      reason: "Customer changed mind",
    });
  });

  // Separate renders: approving optimistically unmounts the row, so the
  // reject action no longer exists to press in the same tree.
  it("Approve on a review calls the approve mutation", () => {
    const { getByTestId } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-r1-action-approve"));
    expect(mockApproveReview).toHaveBeenCalledWith("r1", expect.anything());
  });

  it("Reject on a review calls the reject mutation", () => {
    const { getByTestId } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-r1-action-reject"));
    expect(mockRejectReview).toHaveBeenCalledWith("r1", expect.anything());
  });

  it("Close on a ticket sets its status to closed", () => {
    const { getByTestId } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-t1-action-close"));
    expect(mockUpdateTicketStatus).toHaveBeenCalledTimes(1);
    expect(mockUpdateTicketStatus.mock.calls[0][0]).toEqual({ id: "t1", status: "closed" });
  });

  it("never opts any action into full-swipe auto-fire (this app has no undo)", () => {
    const { UNSAFE_root } = render(<DashboardScreen />);
    const swipeRows = UNSAFE_root.findAll(
      (n) => typeof n.type !== "string" && (n.type as { name?: string }).name === "SwipeRow",
    );
    expect(swipeRows.length).toBeGreaterThan(0);
    for (const row of swipeRows) {
      const actions = [
        ...((row.props.leadingActions ?? []) as { autoFireOnFullSwipe?: boolean }[]),
        ...((row.props.trailingActions ?? []) as { autoFireOnFullSwipe?: boolean }[]),
      ];
      for (const a of actions) {
        expect(a.autoFireOnFullSwipe).toBeFalsy();
      }
    }
  });
});

describe("Dashboard — 'Needs you' counts work, not rows", () => {
  // Reproduced on device: the header read 7 over a list of 5 actionable rows
  // plus two "See all …" links. `buildQueue` emits those links as
  // `kind: "seeAll"` — navigational affordances, not things that need the
  // merchant — and the count must exclude them.
  it("excludes 'See all' rows from the count", () => {
    mockQueryState.dashboard = {
      data: dashboardPayload({
        // Authoritative counts far above TYPE_CAP, so buildQueue appends a
        // "See all" row for BOTH orders and reviews.
        stats: { ...STATS, orders_pending: 5, pending_reviews: 4 },
      }),
      isError: false,
    };

    const { getByTestId, getByText } = render(<DashboardScreen />);

    // Two navigational rows are on screen…
    expect(getByText(/See all 5 pending orders/)).toBeTruthy();
    expect(getByText(/See all 4 pending reviews/)).toBeTruthy();
    // …and neither is counted: 1 order + 1 stock + 1 ticket + 1 review = 4.
    expect(getByTestId("dashboard-needs-you-count")).toHaveTextContent("4");
  });

  it("drops the count as rows are actioned away", () => {
    const { getByTestId } = render(<DashboardScreen />);
    expect(getByTestId("dashboard-needs-you-count")).toHaveTextContent("4");

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(getByTestId("dashboard-needs-you-count")).toHaveTextContent("3");
  });
});

describe("Dashboard — the cancel error does not leak between orders", () => {
  // `error` was bound straight to `cancelOrder.error`, which react-query never
  // resets and nothing here called `.reset()` on. One failed cancel therefore
  // greeted the merchant on EVERY subsequent order's cancel sheet, before
  // they had typed anything. The order detail screen (orders/[id].tsx) had
  // the right pattern already: local state, cleared on present.
  const ERROR_COPY = "Couldn't cancel this order. Try again.";

  beforeEach(() => {
    mockQueryState.dashboard = { data: twoPendingOrders(), isError: false };
  });

  it("shows no error when the sheet is first opened, despite a sticky mutation error", () => {
    const { getByTestId, queryByText } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    expect(queryByText(ERROR_COPY)).toBeNull();
  });

  it("surfaces the error inline when THAT order's cancel fails", () => {
    const { getByTestId, getByLabelText, getByText } = render(<DashboardScreen />);
    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    fireEvent.changeText(getByLabelText("Cancellation reason"), "Out of stock");
    fireEvent.press(getByLabelText("Cancel order"));

    const onError = mockCancelOrder.mock.calls[0][1].onError as (e: Error) => void;
    act(() => onError(new Error("500")));

    expect(getByText(ERROR_COPY)).toBeTruthy();
  });

  it("clears the error when the sheet is re-opened for a DIFFERENT order", () => {
    const { getByTestId, getByLabelText, getByText, queryByText } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    fireEvent.changeText(getByLabelText("Cancellation reason"), "Out of stock");
    fireEvent.press(getByLabelText("Cancel order"));
    act(() => (mockCancelOrder.mock.calls[0][1].onError as (e: Error) => void)(new Error("500")));
    expect(getByText(ERROR_COPY)).toBeTruthy();

    fireEvent.press(getByTestId("swipe-o2-action-cancel"));
    expect(queryByText(ERROR_COPY)).toBeNull();
  });

  it("clears the error when the SAME order's sheet is re-opened", () => {
    const { getByTestId, getByLabelText, getByText, queryByText } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    fireEvent.changeText(getByLabelText("Cancellation reason"), "Out of stock");
    fireEvent.press(getByLabelText("Cancel order"));
    act(() => (mockCancelOrder.mock.calls[0][1].onError as (e: Error) => void)(new Error("500")));
    expect(getByText(ERROR_COPY)).toBeTruthy();

    fireEvent.press(getByTestId("swipe-o1-action-cancel"));
    expect(queryByText(ERROR_COPY)).toBeNull();
  });
});

describe("Dashboard — optimistic dismissals expire when fresh data lands", () => {
  // `dismissed` is a local overlay on the server's answer. It was never
  // cleared, so an id that legitimately RETURNED to the queue (a ticket
  // reopened on another device, an approval the server refused out of band)
  // stayed invisible for the life of the screen.
  it("un-hides a row that is still in the queue after a successful refetch", () => {
    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 1 };
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(queryByTestId("queue-row-o1")).toBeNull();

    // A refetch lands and the row is STILL pending server-side.
    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 2 };
    rerender(<DashboardScreen />);

    expect(getByTestId("queue-row-o1")).toBeTruthy();
  });

  it("keeps the row hidden while no new data has arrived", () => {
    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 1 };
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    rerender(<DashboardScreen />);

    expect(queryByTestId("queue-row-o1")).toBeNull();
  });

  it("un-hides a review row when the REVIEWS query answers again", () => {
    mockQueryState.reviews = { data: REVIEW_PAGES, isError: false, dataUpdatedAt: 1 };
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-r1-action-approve"));
    expect(queryByTestId("queue-row-r1")).toBeNull();

    mockQueryState.reviews = { data: REVIEW_PAGES, isError: false, dataUpdatedAt: 2 };
    rerender(<DashboardScreen />);

    expect(getByTestId("queue-row-r1")).toBeTruthy();
  });
});

describe("Dashboard — a refetch only expires its OWN source's dismissals", () => {
  // The clear used to key on the MAX of the three `dataUpdatedAt` values, so
  // a refetch of one query un-hid rows belonging to the other two. Reachable
  // on any staggered initial load (dashboard resolves first, tickets last)
  // and on every `refetchOnWindowFocus`: the row the merchant just approved
  // popped back mid-flight, swipeable again — a second `useConfirmOrder` on
  // the same order one tap away — then vanished when the mutation resolved.
  it("keeps an approved ORDER hidden when only the TICKETS query refetches", () => {
    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 100 };
    mockQueryState.tickets = { data: TICKET_PAGES, isError: false, dataUpdatedAt: 100 };
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(queryByTestId("queue-row-o1")).toBeNull();

    // Neither onSuccess nor onError has fired — the request is still running.
    expect(mockConfirmOrder).toHaveBeenCalledTimes(1);

    // Only tickets answers again. The dashboard has said nothing new about o1.
    mockQueryState.tickets = { data: TICKET_PAGES, isError: false, dataUpdatedAt: 200 };
    rerender(<DashboardScreen />);

    expect(queryByTestId("queue-row-o1")).toBeNull();
  });

  it("keeps a closed TICKET hidden when only the DASHBOARD query refetches", () => {
    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 100 };
    mockQueryState.tickets = { data: TICKET_PAGES, isError: false, dataUpdatedAt: 100 };
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-t1-action-close"));
    expect(queryByTestId("queue-row-t1")).toBeNull();

    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 200 };
    rerender(<DashboardScreen />);

    expect(queryByTestId("queue-row-t1")).toBeNull();
  });

  it("keeps the row hidden while its OWN mutation is still in flight", () => {
    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 100 };
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(queryByTestId("queue-row-o1")).toBeNull();

    // The confirm is still running AND the dashboard answers again (a window
    // focus refetch mid-request). Its answer predates the mutation, so it is
    // not authoritative about o1 yet.
    mockPending.confirmOrder = true;
    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 200 };
    rerender(<DashboardScreen />);

    expect(queryByTestId("queue-row-o1")).toBeNull();

    // Settling is NOT itself an answer. That mid-flight response at 200 was
    // computed before the confirm committed, so it stays non-authoritative
    // after the mutation ends too — the row must not come back on it.
    mockPending.confirmOrder = false;
    rerender(<DashboardScreen />);

    expect(queryByTestId("queue-row-o1")).toBeNull();

    // Only an answer that postdates the mutation is authoritative. Here it
    // still lists o1 as pending, so the row legitimately returns.
    mockQueryState.dashboard = { data: dashboardPayload(), isError: false, dataUpdatedAt: 300 };
    rerender(<DashboardScreen />);

    expect(getByTestId("queue-row-o1")).toBeTruthy();
  });
});

describe("Dashboard — a mutation SETTLING is not fresh data", () => {
  // Regression, round 3. The clear was `if (isMutating) return; clear(source)`
  // with `isMutating` in the dependency array, so the true→false transition
  // was ITSELF a clear trigger: nothing checked that fresh data had actually
  // arrived, only that the request had stopped.
  //
  // Worst for orders, and PERMANENT there: the order mutations invalidated
  // ["orders"], the dashboard's key is ["dashboard"], so a confirm never
  // refetched the dashboard at all. The just-approved order came back still
  // labelled Pending, still swipeable, with a second confirm one swipe away —
  // in an app with no undo.
  it("keeps an approved ORDER hidden when its confirm settles with no refetch", () => {
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-o1-action-approve"));
    expect(queryByTestId("queue-row-o1")).toBeNull();

    mockPending.confirmOrder = true;
    rerender(<DashboardScreen />);
    expect(queryByTestId("queue-row-o1")).toBeNull();

    // The request stops. `dataUpdatedAt` has NOT moved — no new answer.
    mockPending.confirmOrder = false;
    rerender(<DashboardScreen />);

    expect(queryByTestId("queue-row-o1")).toBeNull();
  });

  it("keeps a closed TICKET hidden when its update settles with no refetch", () => {
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-t1-action-close"));
    expect(queryByTestId("queue-row-t1")).toBeNull();

    mockPending.updateTicketStatus = true;
    rerender(<DashboardScreen />);
    expect(queryByTestId("queue-row-t1")).toBeNull();

    mockPending.updateTicketStatus = false;
    rerender(<DashboardScreen />);

    expect(queryByTestId("queue-row-t1")).toBeNull();
  });

  // The clear must still happen — it is what lets an id that legitimately
  // RETURNS to the queue reappear. It just needs a real answer to hang on.
  it("still un-hides once the source answers again after the mutation settles", () => {
    mockQueryState.tickets = { data: TICKET_PAGES, isError: false, dataUpdatedAt: 100 };
    const { getByTestId, queryByTestId, rerender } = render(<DashboardScreen />);

    fireEvent.press(getByTestId("swipe-t1-action-close"));
    mockPending.updateTicketStatus = true;
    rerender(<DashboardScreen />);
    mockPending.updateTicketStatus = false;
    rerender(<DashboardScreen />);
    expect(queryByTestId("queue-row-t1")).toBeNull();

    mockQueryState.tickets = { data: TICKET_PAGES, isError: false, dataUpdatedAt: 200 };
    rerender(<DashboardScreen />);

    expect(getByTestId("queue-row-t1")).toBeTruthy();
  });
});

describe("Dashboard — pull to refresh", () => {
  it("keeps the spinner up until every source settles, not just the dashboard", async () => {
    const settle: (() => void)[] = [];
    const pending = () => new Promise<void>((resolve) => settle.push(resolve));
    mockRefetchDashboard.mockImplementation(pending);
    mockRefetchReviews.mockImplementation(pending);
    mockRefetchTickets.mockImplementation(pending);

    const { UNSAFE_getByType } = render(<DashboardScreen />);
    const control = UNSAFE_getByType(RefreshControl);
    expect(control.props.refreshing).toBe(false);

    await act(async () => {
      void control.props.onRefresh();
    });
    expect(UNSAFE_getByType(RefreshControl).props.refreshing).toBe(true);

    // Only the dashboard resolves — the old wiring (refreshing =
    // dashboard.isRefetching) stopped spinning right here.
    await act(async () => {
      settle[0]();
    });
    expect(UNSAFE_getByType(RefreshControl).props.refreshing).toBe(true);

    await act(async () => {
      settle[1]();
      settle[2]();
    });
    expect(UNSAFE_getByType(RefreshControl).props.refreshing).toBe(false);
  });

  it("keeps the spinner honest when a source rejects", async () => {
    mockRefetchDashboard.mockRejectedValue(new Error("offline"));
    mockRefetchReviews.mockResolvedValue(undefined);
    mockRefetchTickets.mockResolvedValue(undefined);

    const { UNSAFE_getByType } = render(<DashboardScreen />);
    await act(async () => {
      await UNSAFE_getByType(RefreshControl).props.onRefresh();
    });

    expect(UNSAFE_getByType(RefreshControl).props.refreshing).toBe(false);
  });
});
