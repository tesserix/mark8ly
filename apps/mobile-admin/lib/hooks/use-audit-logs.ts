import { useInfiniteQuery } from "@tanstack/react-query";
import { createAuditLogApi } from "@repo/mobile-shared/api/audit-log";
import type { AuditLogListResponse } from "@repo/mobile-shared/api/schemas/audit-log";
import { useApiClient } from "@/lib/api-client";

const PAGE_SIZE = 20;

export function nextAuditPage(lastPage: AuditLogListResponse): number | undefined {
  const { page, total_pages } = lastPage.meta;
  return page < total_pages ? page + 1 : undefined;
}

export function useAuditLogs() {
  const client = useApiClient();
  const api = createAuditLogApi(client);

  return useInfiniteQuery({
    queryKey: ["audit-logs", "list"],
    queryFn: ({ pageParam }) => api.list({ page: String(pageParam), page_size: String(PAGE_SIZE) }),
    initialPageParam: 1,
    getNextPageParam: nextAuditPage,
    refetchOnWindowFocus: true,
  });
}
