import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createCouponsApi } from "@repo/mobile-shared/api/coupons";
import type { CouponListResponse } from "@repo/mobile-shared/api/schemas/coupons";
import type { CouponDetailEnvelope } from "@repo/mobile-shared/api/schemas/coupons";
import { useApiClient } from "@/lib/api-client";

const PER_PAGE = 20;

/** Coupons list uses `{data,total,page}` (no total_pages), so derive the next
 *  page from whether page*PER_PAGE has covered `total`. */
export function nextCouponPage(lastPage: CouponListResponse): number | undefined {
  return lastPage.page * PER_PAGE < lastPage.total ? lastPage.page + 1 : undefined;
}

export function useCoupons(params?: { status?: string; search?: string }) {
  const client = useApiClient();
  const couponsApi = createCouponsApi(client);

  return useInfiniteQuery({
    queryKey: ["coupons", "list", params?.status, params?.search],
    queryFn: ({ pageParam }) =>
      couponsApi.list({
        ...(params?.status ? { status: params.status } : {}),
        ...(params?.search ? { search: params.search } : {}),
        page: String(pageParam),
        per_page: String(PER_PAGE),
      }),
    initialPageParam: 1,
    getNextPageParam: nextCouponPage,
    refetchOnWindowFocus: true,
  });
}

export function useCoupon(id: string) {
  const client = useApiClient();
  const couponsApi = createCouponsApi(client);

  return useQuery<CouponDetailEnvelope>({
    queryKey: ["coupons", "detail", id],
    queryFn: () => couponsApi.get(id),
    enabled: !!id,
  });
}
