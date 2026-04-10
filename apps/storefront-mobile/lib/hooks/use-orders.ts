import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useApiClient, useStoreSlug } from "./use-api-client";
import { listOrders, getOrder } from "@/lib/storefront-api/orders";

const PAGE_SIZE = 20;

export function useOrders() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useInfiniteQuery({
    queryKey: ["store", slug, "orders"],
    queryFn: ({ pageParam }) => listOrders(client, pageParam, PAGE_SIZE),
    initialPageParam: 1,
    getNextPageParam: (lastPage) => {
      const { page, page_size, total } = lastPage.meta;
      return page * page_size < total ? page + 1 : undefined;
    },
    select: (pages) => pages.pages.flatMap((p) => p.data),
  });
}

export function useOrder(id: string) {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "order", id],
    queryFn: () => getOrder(client, id),
    enabled: id.length > 0,
    select: (res) => res.data,
  });
}
