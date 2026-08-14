import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useStores, shouldRetryStores } from "@/lib/hooks/use-store";

const REAL_RESPONSE = {
  data: [
    {
      id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
      name: "The Bondi Store",
      slug: "the-bondi-store",
      country_code: "AU",
      currency_code: "AUD",
      status: "active",
    },
  ],
};

jest.mock("@/lib/api-client", () => {
  const getTenant = jest.fn();
  return {
    useApiClient: () => ({ getTenant }),
    __getTenant: getTenant,
  };
});

// eslint-disable-next-line @typescript-eslint/no-var-requires
const { __getTenant } = require("@/lib/api-client") as { __getTenant: jest.Mock };

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe("useStores", () => {
  beforeEach(() => {
    __getTenant.mockReset();
  });

  it("returns the array from the {data:[...]} envelope", async () => {
    // The regression that made the dashboard unreachable: the hook read
    // `.items`, the endpoint sends `.data`, so it always returned [] and every
    // merchant saw "No store yet".
    __getTenant.mockResolvedValue(REAL_RESPONSE);
    const { result } = renderHook(() => useStores(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].name).toBe("The Bondi Store");
  });

  it("passes the stores schema to the client", async () => {
    __getTenant.mockResolvedValue(REAL_RESPONSE);
    const { result } = renderHook(() => useStores(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(__getTenant).toHaveBeenCalledWith("/stores", undefined, expect.anything());
  });
});

describe("shouldRetryStores", () => {
  it("retries a transient failure past the app-wide default of 2", () => {
    expect(shouldRetryStores(2, new Error("Network request failed"))).toBe(true);
    expect(shouldRetryStores(4, new Error("Network request failed"))).toBe(true);
  });

  it("gives up eventually", () => {
    expect(shouldRetryStores(5, new Error("Network request failed"))).toBe(false);
  });

  it("does not retry a 403 — retrying a denial just fails again", () => {
    expect(shouldRetryStores(0, new ApiError(403, "forbidden", "Forbidden"))).toBe(false);
  });

  it("does not retry a contract mismatch — the payload will not change", () => {
    expect(
      shouldRetryStores(0, new ApiError(200, "contract_mismatch", "bad shape")),
    ).toBe(false);
  });

  it("retries a 503 from a restarting backend", () => {
    expect(shouldRetryStores(0, new ApiError(503, "unavailable", "no upstream"))).toBe(true);
  });
});
