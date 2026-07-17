import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createTicketsApi } from "@repo/mobile-shared/api/tickets";
import type { Ticket, TicketListResponse } from "@repo/mobile-shared/api/schemas/tickets";
import { useApiClient } from "@/lib/api-client";

const PAGE_SIZE = 20;

export function nextTicketPage(lastPage: TicketListResponse): number | undefined {
  const { page, total_pages } = lastPage.meta;
  return page < total_pages ? page + 1 : undefined;
}

export function useTickets(params?: { status?: string }) {
  const client = useApiClient();
  const api = createTicketsApi(client);

  return useInfiniteQuery({
    queryKey: ["tickets", "list", params?.status],
    queryFn: ({ pageParam }) =>
      api.list({
        ...(params?.status ? { status: params.status } : {}),
        page: String(pageParam),
        page_size: String(PAGE_SIZE),
      }),
    initialPageParam: 1,
    getNextPageParam: nextTicketPage,
    refetchOnWindowFocus: true,
  });
}

export function useTicket(id: string) {
  const client = useApiClient();
  const api = createTicketsApi(client);

  return useQuery<Ticket>({
    queryKey: ["tickets", "detail", id],
    queryFn: () => api.get(id),
    enabled: !!id,
  });
}
