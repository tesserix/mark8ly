import { act, renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useProducts } from "@/lib/hooks/use-products";

// The real api module is mocked below, so this never goes through
// productListSchema (it would fail — no options/variants/media on these
// fixtures). It's a stub for the mocked list() call, not a stand-in for a
// real wire payload.
function page(pageNum: number, totalPages: number) {
  return {
    data: [{ id: `product-${pageNum}`, title: `Sample Product ${pageNum}`, status: "active" }],
    meta: { page: pageNum, page_size: 20, total: totalPages, total_pages: totalPages },
  };
}

jest.mock("@repo/mobile-shared/api/products", () => {
  const mockList = jest.fn();
  return {
    createProductsApi: () => ({
      list: mockList,
    }),
    __mockList: mockList,
  };
});

jest.mock("@/lib/api-client", () => {
  return {
    useApiClient: () => ({}),
  };
});

// eslint-disable-next-line @typescript-eslint/no-var-requires
const { __mockList } = require("@repo/mobile-shared/api/products") as { __mockList: jest.Mock };

// ONE client per test, created in `beforeEach` rather than inside `wrapper`
// itself: `wrapper` is a component and React re-invokes it on every re-render
// the hook causes, so a `new QueryClient()` in its body would hand
// `fetchNextPage` a BRAND NEW, empty cache on the very next render — page 1
// would vanish before page 2 could be appended to it. `wrapper` here only
// ever reads the one instance the test already has.
let queryClient: QueryClient;

function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("useProducts", () => {
  beforeEach(() => {
    __mockList.mockReset();
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  });

  it("requests page 1 at the default page size", async () => {
    __mockList.mockResolvedValue(page(1, 1));
    const { result } = renderHook(() => useProducts(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__mockList).toHaveBeenCalledWith({
      page: "1",
      page_size: "20",
    });
  });

  it("merges the page params with status", async () => {
    __mockList.mockResolvedValue(page(1, 1));
    const { result } = renderHook(() => useProducts({ status: "active" }), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__mockList).toHaveBeenCalledWith({
      status: "active",
      page: "1",
      page_size: "20",
    });
  });

  it("merges the page params with search", async () => {
    __mockList.mockResolvedValue(page(1, 1));
    const { result } = renderHook(() => useProducts({ search: "test" }), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__mockList).toHaveBeenCalledWith({
      search: "test",
      page: "1",
      page_size: "20",
    });
  });

  it("merges the page params with both status and search", async () => {
    __mockList.mockResolvedValue(page(1, 1));
    const { result } = renderHook(
      () => useProducts({ status: "active", search: "test" }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__mockList).toHaveBeenCalledWith({
      status: "active",
      search: "test",
      page: "1",
      page_size: "20",
    });
  });

  // The fixture this whole task exists for: MORE rows than one page.
  // `meta.total = rows.length` (every other products test's shape, before
  // Task 2's mockTotalOverride) makes "past the first page" unfalsifiable —
  // a fixture built that way cannot express a second page existing at all.
  it("appends the second page to the first rather than replacing it", async () => {
    __mockList
      .mockResolvedValueOnce(page(1, 2))
      .mockResolvedValueOnce(page(2, 2));
    const { result } = renderHook(() => useProducts(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.pages).toHaveLength(1);
    expect(result.current.hasNextPage).toBe(true);

    act(() => {
      result.current.fetchNextPage();
    });
    await waitFor(() => expect(result.current.data?.pages).toHaveLength(2));

    expect(__mockList).toHaveBeenLastCalledWith({ page: "2", page_size: "20" });
    // APPENDED — page 1 is still there, not dropped.
    expect(result.current.data?.pages.map((p) => p.data[0]?.id)).toEqual([
      "product-1",
      "product-2",
    ]);
  });

  // The list must stop cleanly at the end, not poll forever past the last
  // page — this is what makes that possible: no next page param once the
  // server's own total_pages says there is nothing further.
  it("reports no next page once the last page has been fetched", async () => {
    __mockList.mockResolvedValue(page(1, 1));
    const { result } = renderHook(() => useProducts(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasNextPage).toBe(false);
  });

  // A filter or search change is a NEW query key (["products", status,
  // search]), so react-query starts that key fresh at page 1 rather than
  // carrying over whatever page a previous filter had scrolled to.
  it("starts a new filter at page 1, independent of another filter's pages", async () => {
    __mockList.mockImplementation((params) =>
      Promise.resolve(page(Number(params.page), 2)),
    );
    const draft = renderHook(() => useProducts({ status: "draft" }), { wrapper });
    await waitFor(() => expect(draft.result.current.isSuccess).toBe(true));
    act(() => {
      draft.result.current.fetchNextPage();
    });
    await waitFor(() => expect(draft.result.current.data?.pages).toHaveLength(2));

    const archived = renderHook(() => useProducts({ status: "archived" }), { wrapper });
    await waitFor(() => expect(archived.result.current.isSuccess).toBe(true));
    // A fresh filter's own first fetch is page 1, never page 2 — proven by
    // asserting on the LAST call the mock received for this render, not on
    // draft's calls.
    expect(__mockList).toHaveBeenLastCalledWith({
      status: "archived",
      page: "1",
      page_size: "20",
    });
    expect(archived.result.current.data?.pages).toHaveLength(1);
  });
});
