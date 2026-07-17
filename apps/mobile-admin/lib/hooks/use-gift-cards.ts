import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createGiftCardsApi } from "@repo/mobile-shared/api/gift-cards";
import type { GiftCardDetail, GiftCardListResponse } from "@repo/mobile-shared/api/schemas/gift-cards";
import { useApiClient } from "@/lib/api-client";

const PAGE_SIZE = 20;

export function nextGiftCardPage(lastPage: GiftCardListResponse): number | undefined {
  const { page, total_pages } = lastPage.meta;
  return page < total_pages ? page + 1 : undefined;
}

export function useGiftCards(params?: { status?: string }) {
  const client = useApiClient();
  const api = createGiftCardsApi(client);

  return useInfiniteQuery({
    queryKey: ["gift-cards", "list", params?.status],
    queryFn: ({ pageParam }) =>
      api.list({
        ...(params?.status ? { status: params.status } : {}),
        page: String(pageParam),
        page_size: String(PAGE_SIZE),
      }),
    initialPageParam: 1,
    getNextPageParam: nextGiftCardPage,
    refetchOnWindowFocus: true,
  });
}

export function useGiftCard(id: string) {
  const client = useApiClient();
  const api = createGiftCardsApi(client);

  return useQuery<GiftCardDetail>({
    queryKey: ["gift-cards", "detail", id],
    queryFn: () => api.get(id),
    enabled: !!id,
  });
}
