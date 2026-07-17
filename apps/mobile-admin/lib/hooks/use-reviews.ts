import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createReviewsApi } from "@repo/mobile-shared/api/reviews";
import type { Review } from "@repo/mobile-shared/api/types";
import type { ReviewListResponse } from "@repo/mobile-shared/api/schemas/reviews";
import { useApiClient } from "@/lib/api-client";

interface ReviewListParams {
  /** "pending" | "approved" | "rejected"; omitted means all. */
  status?: string;
}

const PAGE_SIZE = 50;

/**
 * The next page param, or undefined once the last page is reached (stops
 * infinite scroll). Exported so the off-by-one can be pinned without a
 * QueryClient — same shape as nextCustomerPage.
 */
export function nextReviewPage(lastPage: ReviewListResponse): number | undefined {
  const { page, total_pages } = lastPage.meta;
  return page < total_pages ? page + 1 : undefined;
}

/** Paginated review list, filterable by status. Infinite scroll like useCustomers. */
export function useReviews(params?: ReviewListParams) {
  const client = useApiClient();
  const reviewsApi = createReviewsApi(client);

  return useInfiniteQuery({
    queryKey: ["reviews", params?.status],
    queryFn: ({ pageParam }) =>
      reviewsApi.list({
        ...(params?.status ? { status: params.status } : {}),
        page: String(pageParam),
        page_size: String(PAGE_SIZE),
      }),
    initialPageParam: 1,
    getNextPageParam: nextReviewPage,
    refetchOnWindowFocus: true,
  });
}

export function useReview(id: string) {
  const client = useApiClient();
  const reviewsApi = createReviewsApi(client);

  return useQuery<Review>({
    queryKey: ["review", id],
    queryFn: () => reviewsApi.get(id),
    enabled: !!id,
  });
}
