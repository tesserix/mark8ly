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
    isLoading: false,
    isRefetching: false,
    refetch: mockRefetchDashboard,
  }),
}));
jest.mock("@/lib/hooks/use-reviews", () => ({
  useReviews: () => ({
    data: mockQueryState.reviews.data,
    isError: mockQueryState.reviews.isError,
    isLoading: false,
    refetch: mockRefetchReviews,
  }),
}));
jest.mock("@/lib/hooks/use-tickets", () => ({
  useTickets: () => ({
    data: mockQueryState.tickets.data,
    isError: mockQueryState.tickets.isError,
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

jest.mock("@/lib/admin-api/order-actions", () => ({
  useConfirmOrder: () => ({ mutate: mockConfirmOrder, isPending: false }),
  useCancelOrder: () => ({ mutate: mockCancelOrder, isPending: false, error: null }),
}));
jest.mock("@/lib/admin-api/review-actions", () => ({
  useApproveReview: () => ({ mutate: mockApproveReview, isPending: false }),
  useRejectReview: () => ({ mutate: mockRejectReview, isPending: false }),
}));
jest.mock("@/lib/admin-api/ticket-actions", () => ({
  useUpdateTicketStatus: () => ({ mutate: mockUpdateTicketStatus, isPending: false }),
}));

import { act, fireEvent, render, within } from "@testing-library/react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import DashboardScreen from "../app/(tabs)/index";
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
});

describe("Dashboard — header", () => {
  it("titles the screen with the store name and the date as the eyebrow", () => {
    const { getAllByText } = render(<DashboardScreen />);
    // The CollapsingHeader mounts both the expanded and collapsed layers.
    expect(getAllByText("Bondi Supply").length).toBeGreaterThan(0);
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
