// Which query keys a successful segment delete invalidates (inc3 Task 7).
//
// Two keys, not one, and the second is the whole reason this file exists.
// `campaigns.segment_id` points AT a segment, so a campaign's audience is a
// fact about a row that no longer exists the moment the segment goes: the
// campaign list's cached payload, and the audience picker built from it,
// would both keep offering a deleted segment for the life of the screen. The
// cross-invalidation lives in `segment-actions.ts`; nothing pinned it, so a
// future refactor that trimmed it to the "obvious" `["segments"]` key would
// have been fully green.
//
// It lives in its OWN file rather than in `segments-screen.test.tsx` because
// that suite mocks `@/lib/admin-api/segment-actions` wholesale — the module
// under test here — and a `jest.mock` is file-wide.
jest.mock("@repo/mobile-shared/api/segments", () => ({
  createSegmentsApi: () => ({
    create: jest.fn(() => Promise.resolve({})),
    update: jest.fn(() => Promise.resolve({})),
    remove: jest.fn(() => Promise.resolve({})),
  }),
}));
jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));

import React from "react";
import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useDeleteSegment } from "@/lib/admin-api/segment-actions";

let queryClient: QueryClient;
let invalidate: jest.SpyInstance;

function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  invalidate = jest.spyOn(queryClient, "invalidateQueries");
});

afterEach(() => {
  invalidate.mockRestore();
  queryClient.clear();
});

/** Every `queryKey` handed to `invalidateQueries`, as plain arrays. */
function invalidatedKeys(): unknown[][] {
  return invalidate.mock.calls.map(
    (call) => (call[0] as { queryKey: unknown[] }).queryKey,
  );
}

describe("segment delete", () => {
  it("invalidates the campaigns list as well as the segments list", async () => {
    const { result } = renderHook(() => useDeleteSegment(), { wrapper });
    result.current.mutate("seg-lapsed");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const keys = invalidatedKeys();
    expect(keys).toContainEqual(["segments"]);
    // The half a "tidy-up" would drop: a campaign's audience is a fact about
    // the segment that just stopped existing.
    expect(keys).toContainEqual(["campaigns"]);
  });

  // Prefix keys, not exact ones: the list is ["segments", "list"] and the
  // detail is ["segments", "detail", id], so an exact key would refresh
  // neither.
  it("invalidates by PREFIX so both the list and every detail are covered", async () => {
    const { result } = renderHook(() => useDeleteSegment(), { wrapper });
    result.current.mutate("seg-lapsed");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    for (const key of invalidatedKeys()) expect(key).toHaveLength(1);
  });
});
