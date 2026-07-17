import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { createLoyaltyApi } from "@repo/mobile-shared/api/loyalty";
import type {
  LoyaltyProgram,
  LoyaltyMemberDetail,
  LoyaltyMemberListResponse,
  ReferralListResponse,
} from "@repo/mobile-shared/api/schemas/loyalty";
import { useApiClient } from "@/lib/api-client";

const LIMIT = 20;

/** Loyalty lists use `{data, meta:{total,page,limit}}` — derive next from
 *  whether page*limit has covered total. */
function nextLoyaltyPage(meta: { total: number; page: number; limit: number }): number | undefined {
  return meta.page * meta.limit < meta.total ? meta.page + 1 : undefined;
}

export function useLoyaltyProgram() {
  const client = useApiClient();
  const api = createLoyaltyApi(client);

  return useQuery<LoyaltyProgram | null>({
    queryKey: ["loyalty", "program"],
    queryFn: () => api.getProgram(),
    refetchOnWindowFocus: true,
  });
}

export function useLoyaltyMembers() {
  const client = useApiClient();
  const api = createLoyaltyApi(client);

  return useInfiniteQuery({
    queryKey: ["loyalty", "members"],
    queryFn: ({ pageParam }) => api.listMembers({ page: String(pageParam), limit: String(LIMIT) }),
    initialPageParam: 1,
    getNextPageParam: (lastPage: LoyaltyMemberListResponse) => nextLoyaltyPage(lastPage.meta),
    refetchOnWindowFocus: true,
  });
}

export function useLoyaltyMember(id: string) {
  const client = useApiClient();
  const api = createLoyaltyApi(client);

  return useQuery<LoyaltyMemberDetail>({
    queryKey: ["loyalty", "member", id],
    queryFn: () => api.getMember(id),
    enabled: !!id,
  });
}

export function useReferrals() {
  const client = useApiClient();
  const api = createLoyaltyApi(client);

  return useInfiniteQuery({
    queryKey: ["loyalty", "referrals"],
    queryFn: ({ pageParam }) => api.listReferrals({ page: String(pageParam), limit: String(LIMIT) }),
    initialPageParam: 1,
    getNextPageParam: (lastPage: ReferralListResponse) => nextLoyaltyPage(lastPage.meta),
    refetchOnWindowFocus: true,
  });
}
