import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createCampaignsApi } from "@repo/mobile-shared/api/campaigns";
import type { Campaign, CampaignListResponse } from "@repo/mobile-shared/api/schemas/campaigns";
import { useApiClient } from "@/lib/api-client";

const PAGE_SIZE = 20;

export function nextCampaignPage(lastPage: CampaignListResponse): number | undefined {
  const { page, total_pages } = lastPage.meta;
  return page < total_pages ? page + 1 : undefined;
}

export function useCampaigns(params?: { status?: string }) {
  const client = useApiClient();
  const api = createCampaignsApi(client);

  return useInfiniteQuery({
    queryKey: ["campaigns", "list", params?.status],
    queryFn: ({ pageParam }) =>
      api.list({
        ...(params?.status ? { status: params.status } : {}),
        page: String(pageParam),
        page_size: String(PAGE_SIZE),
      }),
    initialPageParam: 1,
    getNextPageParam: nextCampaignPage,
    refetchOnWindowFocus: true,
  });
}

export function useCampaign(id: string) {
  const client = useApiClient();
  const api = createCampaignsApi(client);

  return useQuery<Campaign>({
    queryKey: ["campaigns", "detail", id],
    queryFn: () => api.get(id),
    enabled: !!id,
  });
}
