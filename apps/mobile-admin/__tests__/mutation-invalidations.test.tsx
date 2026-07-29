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
import { act, renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import React from "react";
import { useConfirmOrder, useCancelOrder } from "@/lib/admin-api/order-actions";
import { useApproveReview } from "@/lib/admin-api/review-actions";
import { useUpdateTicketStatus } from "@/lib/admin-api/ticket-actions";
import { useSetProductStatus } from "@/lib/admin-api/product-status";
import { useUpdateVariant } from "@/lib/admin-api/product-crud";
import { useProducts } from "@/lib/hooks/use-products";

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
jest.mock("@repo/mobile-shared/api/products", () => {
  const mockUpdate = jest.fn(() => Promise.resolve({}));
  const mockUpdateVariant = jest.fn(() => Promise.resolve({}));
  const mockList = jest.fn();
  return {
    createProductsApi: () => ({
      update: mockUpdate,
      updateVariant: mockUpdateVariant,
      list: mockList,
    }),
    __mockList: mockList,
  };
});
jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const { __mockList } = require("@repo/mobile-shared/api/products") as { __mockList: jest.Mock };

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

  // GUARD, not a feature test. `onSuccess` must NOT return the
  // `invalidateQueries` promises. Returning them is the single most natural
  // react-query reflex there is — it's what you write when you want the
  // refetch to have landed before the mutation reports done, and both doc
  // comments in lib/admin-api/order-actions.ts warn against it precisely
  // because it has now shipped broken three times.
  //
  // Returning them makes react-query AWAIT the refetches before flipping the
  // mutation out of pending, so `isPending` stays true until fresh data
  // arrives. `useExpireHidesOnFreshAnswer` in app/(tabs)/index.tsx keys its
  // watermark off exactly that ordering: it requires the refetch to land
  // AFTER `isPending` goes false, or `isMutating` is still true when the new
  // `dataUpdatedAt` ticks, the expiry is skipped, and the optimistically
  // hidden row stays hidden against fresh data that says it is still there.
  //
  // Every OTHER assertion in this file passes with the `return` added — they
  // only inspect which keys were invalidated, never when the mutation
  // settled relative to the refetch.
  it("settles the mutation WITHOUT awaiting the refetches its invalidation kicks off", async () => {
    let releaseRefetch: (() => void) | undefined;
    let fetches = 0;
    const queryFn = () => {
      fetches += 1;
      // First load resolves; the invalidation-driven refetch is held open so
      // "did the mutation settle before it landed?" is observable at all.
      if (fetches === 1) return Promise.resolve("initial");
      return new Promise<string>((resolve) => {
        releaseRefetch = () => resolve("refetched");
      });
    };

    const { result } = renderHook(
      () => ({
        dashboard: useQuery({ queryKey: ["dashboard"], queryFn }),
        confirm: useConfirmOrder(),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.dashboard.isSuccess).toBe(true));

    act(() => {
      result.current.confirm.mutate({ id: "o1" });
    });
    // Waits on `isSuccess`, NOT on `!isPending` — an un-started mutation is
    // already `isPending: false`, so that wait would pass at the idle state
    // before `mutate` had done anything.
    await waitFor(() => expect(result.current.confirm.isSuccess).toBe(true));

    // The mutation is done, and the refetch it triggered is demonstrably
    // STILL in flight — which is the whole ordering guarantee. With `return`
    // added, the mutation stays pending while this promise is unresolved and
    // the wait above times out instead.
    expect(fetches).toBe(2);
    expect(result.current.dashboard.isFetching).toBe(true);

    await act(async () => {
      releaseRefetch?.();
    });
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

describe("product status mutations", () => {
  // The Products list is keyed ["products", status, search] and the prefix
  // ["products"] reaches it — which is the ENTIRE reason that screen needs no
  // optimistic hide. If this invalidation is ever narrowed to an exact key,
  // the list stops refetching itself and a merchant's Activate silently does
  // nothing visible.
  it("invalidates the products list prefix and the product detail", async () => {
    const { result } = renderHook(() => useSetProductStatus(), { wrapper });
    result.current.mutate({ id: "p1", status: "active" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).toEqual(
      expect.arrayContaining(['["products"]', '["product","p1"]']),
    );
  });

  // Checked, not assumed. The dashboard payload's only product-shaped blocks
  // are `top_products` (sales-derived) and `low_stock`, whose query filters on
  // deleted_at and inventory quantity and never reads `p.status`
  // (dashboard.go:145-163). A status change moves nothing on it.
  it("does NOT invalidate the dashboard — no block on it reads product status", async () => {
    const { result } = renderHook(() => useSetProductStatus(), { wrapper });
    result.current.mutate({ id: "p1", status: "archived" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).not.toContain('["dashboard"]');
  });

  // Same guard as the orders one above: returning the invalidation promises
  // from `onSuccess` keeps `isPending` true until the refetch lands, and the
  // Products screen's per-row busy guard would then hold the row disabled for
  // the length of a network round trip it has no business waiting on.
  it("settles the mutation WITHOUT awaiting the refetches its invalidation kicks off", async () => {
    let releaseRefetch: (() => void) | undefined;
    let fetches = 0;
    const queryFn = () => {
      fetches += 1;
      if (fetches === 1) return Promise.resolve("initial");
      return new Promise<string>((resolve) => {
        releaseRefetch = () => resolve("refetched");
      });
    };

    const { result } = renderHook(
      () => ({
        list: useQuery({ queryKey: ["products", undefined, undefined], queryFn }),
        setStatus: useSetProductStatus(),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.list.isSuccess).toBe(true));

    act(() => {
      result.current.setStatus.mutate({ id: "p1", status: "active" });
    });
    await waitFor(() => expect(result.current.setStatus.isSuccess).toBe(true));

    expect(fetches).toBe(2);
    expect(result.current.list.isFetching).toBe(true);

    await act(async () => {
      releaseRefetch?.();
    });
  });

  /**
   * Task 15's specific regression — react-query invalidation is PREFIX
   * matching (`exact: false` by default in v5): `invalidateQueries({queryKey:
   * ["products"]})` reaches `["products", status, search]` whether that key
   * belongs to a plain query or an infinite one, but an infinite query's
   * cache shape (`{pages: [...], pageParams: [...]}`) differs from a plain
   * one's. The risk this guards: does the invalidated refetch keep every
   * page the merchant had already scrolled through, or does it collapse back
   * to just page 1 and silently yank a merchant on page 3 back to the top?
   *
   * v5's documented default for an invalidated infinite query is to refetch
   * every currently-loaded page, in order, and keep them all — this is the
   * test that would catch it if that ever stopped being true (a
   * `refetchType`/`maxPages` misconfiguration, a hook rewritten as a plain
   * `useQuery` that silently drops pages 2+, etc).
   */
  it("keeps every already-loaded page on the infinite products cache after invalidation — does not reset to page 1", async () => {
    __mockList.mockImplementation((params: { page?: string }) => {
      const pageNum = Number(params.page ?? "1");
      return Promise.resolve({
        data: [{ id: `p${pageNum}`, title: `Product ${pageNum}`, status: "active" }],
        meta: { page: pageNum, page_size: 20, total: 2, total_pages: 2 },
      });
    });

    const { result } = renderHook(
      () => ({
        list: useProducts(),
        setStatus: useSetProductStatus(),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.list.isSuccess).toBe(true));
    expect(result.current.list.data?.pages).toHaveLength(1);

    // The merchant scrolls to page 2 — the exact position a mutation fired
    // from THAT row must not disturb.
    act(() => {
      result.current.list.fetchNextPage();
    });
    await waitFor(() => expect(result.current.list.data?.pages).toHaveLength(2));

    // A status change on the row the merchant is looking at, on page 2.
    act(() => {
      result.current.setStatus.mutate({ id: "p2", status: "active" });
    });
    await waitFor(() => expect(result.current.setStatus.isSuccess).toBe(true));

    // The invalidation-driven refetch lands asynchronously.
    await waitFor(() => expect(result.current.list.isFetching).toBe(false));

    // BOTH pages are still there — the merchant is still looking at page 2's
    // worth of products, not silently dropped back to page 1's single row.
    expect(result.current.list.data?.pages).toHaveLength(2);
    expect(result.current.list.data?.pages.map((p) => p.data[0]?.id)).toEqual(["p1", "p2"]);
  });
});

describe("product variant mutations (editor)", () => {
  // Sweep C, Task 11: `useUpdateVariant` — the product EDITOR's save path for
  // a variant's price/stock (`app/(tabs)/products/[id].tsx`) — invalidated
  // only `["product", id]`. Editing a variant's price in the editor left the
  // Products LIST showing the pre-edit price until something else happened to
  // invalidate it. `useQuickEditVariant` in variant-quick-edit.ts (the LIST
  // screen's own quick-edit hook — deliberately a different hook, see that
  // file's docstring) already invalidated both; this brings the editor's hook
  // to the same shape.
  it("invalidates the products list prefix as well as the product detail", async () => {
    const { result } = renderHook(() => useUpdateVariant(), { wrapper });
    result.current.mutate({ productId: "p1", variantId: "v1", body: { price: 999 } });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).toEqual(
      expect.arrayContaining(['["products"]', '["product","p1"]']),
    );
  });

  // The same regression Task 15 introduced a risk for on `useSetProductStatus`
  // above: react-query invalidation is PREFIX matching, and an invalidated
  // infinite query must refetch and KEEP every already-loaded page, not
  // collapse back to page 1. A merchant who edited a variant's price from the
  // editor, having reached that product by scrolling to page 2 of the list,
  // must land back on page 2 — not be silently returned to page 1.
  it("keeps every already-loaded page on the infinite products cache after a variant edit — does not reset to page 1", async () => {
    __mockList.mockImplementation((params: { page?: string }) => {
      const pageNum = Number(params.page ?? "1");
      return Promise.resolve({
        data: [{ id: `p${pageNum}`, title: `Product ${pageNum}`, status: "active" }],
        meta: { page: pageNum, page_size: 20, total: 2, total_pages: 2 },
      });
    });

    const { result } = renderHook(
      () => ({
        list: useProducts(),
        updateVariant: useUpdateVariant(),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.list.isSuccess).toBe(true));
    expect(result.current.list.data?.pages).toHaveLength(1);

    act(() => {
      result.current.list.fetchNextPage();
    });
    await waitFor(() => expect(result.current.list.data?.pages).toHaveLength(2));

    // A variant price edit, from the editor, on a product the merchant
    // reached via page 2.
    act(() => {
      result.current.updateVariant.mutate({ productId: "p2", variantId: "v1", body: { price: 500 } });
    });
    await waitFor(() => expect(result.current.updateVariant.isSuccess).toBe(true));

    await waitFor(() => expect(result.current.list.isFetching).toBe(false));

    expect(result.current.list.data?.pages).toHaveLength(2);
    expect(result.current.list.data?.pages.map((p) => p.data[0]?.id)).toEqual(["p1", "p2"]);
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
