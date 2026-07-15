import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useStores } from "@/lib/hooks/use-store";

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
