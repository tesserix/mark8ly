import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useProducts } from "@/lib/hooks/use-products";

const REAL_RESPONSE = {
  data: [
    {
      id: "product-1",
      title: "Sample Product",
      status: "active",
    },
  ],
};

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

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe("useProducts", () => {
  beforeEach(() => {
    __mockList.mockReset();
  });

  it("sends page_size=100 to the API client", async () => {
    __mockList.mockResolvedValue(REAL_RESPONSE);
    const { result } = renderHook(() => useProducts(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__mockList).toHaveBeenCalledWith({
      page_size: "100",
    });
  });

  it("merges page_size with status parameter", async () => {
    __mockList.mockResolvedValue(REAL_RESPONSE);
    const { result } = renderHook(() => useProducts({ status: "active" }), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__mockList).toHaveBeenCalledWith({
      page_size: "100",
      status: "active",
    });
  });

  it("merges page_size with search parameter", async () => {
    __mockList.mockResolvedValue(REAL_RESPONSE);
    const { result } = renderHook(() => useProducts({ search: "test" }), {
      wrapper,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__mockList).toHaveBeenCalledWith({
      page_size: "100",
      search: "test",
    });
  });

  it("merges page_size with both status and search parameters", async () => {
    __mockList.mockResolvedValue(REAL_RESPONSE);
    const { result } = renderHook(
      () => useProducts({ status: "active", search: "test" }),
      { wrapper }
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__mockList).toHaveBeenCalledWith({
      page_size: "100",
      status: "active",
      search: "test",
    });
  });
});
