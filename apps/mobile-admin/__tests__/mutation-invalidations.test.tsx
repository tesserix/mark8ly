// Which query keys a successful mutation invalidates.
//
// This exists because of a bug the Dashboard could not have fixed on its own:
// the order mutations invalidated ["orders"], the Dashboard's action queue
// reads ["dashboard"], and NOTHING anywhere invalidated ["dashboard"]. A
// merchant approved an order, the row was hidden optimistically, and the
// queue then went on serving the pre-confirm payload for the life of the
// screen — same order, still labelled Pending, still swipeable, with a second
// confirm one swipe away in an app that has no undo.
//
// The dashboard payload carries its OWN copy of facts these mutations move
// (recent_orders, stats.orders_pending/_fulfilled/_cancelled, revenue,
// stats.pending_reviews), so any mutation that changes one of them has to
// invalidate it. Tickets are the one exception: nothing in the dashboard
// response describes them (see api/schemas/dashboard.ts), so a ticket
// mutation invalidating ["dashboard"] would be a refetch for nothing.
import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useConfirmOrder, useCancelOrder } from "@/lib/admin-api/order-actions";
import { useApproveReview } from "@/lib/admin-api/review-actions";
import { useUpdateTicketStatus } from "@/lib/admin-api/ticket-actions";

jest.mock("@repo/mobile-shared/api/orders", () => ({
  createOrdersApi: () => ({
    confirm: jest.fn(() => Promise.resolve({})),
    cancel: jest.fn(() => Promise.resolve({})),
    fulfill: jest.fn(() => Promise.resolve({})),
    refund: jest.fn(() => Promise.resolve({})),
  }),
}));
jest.mock("@repo/mobile-shared/api/reviews", () => ({
  createReviewsApi: () => ({
    approve: jest.fn(() => Promise.resolve({})),
    reject: jest.fn(() => Promise.resolve({})),
    setFeatured: jest.fn(() => Promise.resolve({})),
    reply: jest.fn(() => Promise.resolve({})),
  }),
}));
jest.mock("@repo/mobile-shared/api/tickets", () => ({
  createTicketsApi: () => ({
    create: jest.fn(() => Promise.resolve({})),
    reply: jest.fn(() => Promise.resolve({})),
    updateStatus: jest.fn(() => Promise.resolve({})),
  }),
}));
jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));

let queryClient: QueryClient;
let invalidate: jest.SpyInstance;

function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

/** Every `queryKey` passed to `invalidateQueries`, as JSON for easy matching. */
function invalidatedKeys(): string[] {
  return invalidate.mock.calls.map((call) => JSON.stringify(call[0]?.queryKey));
}

beforeEach(() => {
  queryClient = new QueryClient({
    // gcTime 0 so no cache-eviction timer outlives the test and holds the
    // jest worker open.
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false, gcTime: 0 },
    },
  });
  invalidate = jest.spyOn(queryClient, "invalidateQueries");
});

afterEach(() => {
  queryClient.clear();
});

describe("order mutations", () => {
  it("invalidates the dashboard as well as the orders list on confirm", async () => {
    const { result } = renderHook(() => useConfirmOrder(), { wrapper });
    result.current.mutate({ id: "o1" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).toEqual(
      expect.arrayContaining(['["orders"]', '["dashboard"]']),
    );
  });

  it("invalidates the dashboard on cancel", async () => {
    const { result } = renderHook(() => useCancelOrder(), { wrapper });
    result.current.mutate({ id: "o1", reason: "Out of stock" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).toContain('["dashboard"]');
  });
});

describe("review mutations", () => {
  // stats.pending_reviews lives on the dashboard payload and drives the
  // queue's "See all N pending reviews" row.
  it("invalidates the dashboard alongside the review list and detail", async () => {
    const { result } = renderHook(() => useApproveReview(), { wrapper });
    result.current.mutate("r1");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).toEqual(
      expect.arrayContaining(['["reviews"]', '["review","r1"]', '["dashboard"]']),
    );
  });
});

describe("ticket mutations", () => {
  it("does NOT invalidate the dashboard — it carries nothing about tickets", async () => {
    const { result } = renderHook(() => useUpdateTicketStatus(), { wrapper });
    result.current.mutate({ id: "t1", status: "closed" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).toContain('["tickets"]');
    expect(invalidatedKeys()).not.toContain('["dashboard"]');
  });
});
