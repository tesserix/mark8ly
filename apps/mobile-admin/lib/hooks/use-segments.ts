import { useQuery } from "@tanstack/react-query";
import { createSegmentsApi } from "@repo/mobile-shared/api/segments";
import type { Segment, SegmentListResponse } from "@repo/mobile-shared/api/schemas/segments";
import { useApiClient } from "@/lib/api-client";

/** Segments list is a bare `{data}` (no pagination) — a plain query. */
export function useSegments() {
  const client = useApiClient();
  const api = createSegmentsApi(client);

  return useQuery<SegmentListResponse>({
    queryKey: ["segments", "list"],
    queryFn: () => api.list(),
    refetchOnWindowFocus: true,
  });
}

export function useSegment(id: string) {
  const client = useApiClient();
  const api = createSegmentsApi(client);

  return useQuery<Segment>({
    queryKey: ["segments", "detail", id],
    queryFn: () => api.get(id),
    enabled: !!id,
  });
}
